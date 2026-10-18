// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bus implements the NES system interconnect, routing memory reads and writes
// between the 6502 CPU, internal RAM, PPU registers, APU/IO registers, and the Cartridge.
//
// NES CPU Memory Map:
//
//	+------------------+ $FFFF
//	| Cartridge PRG    | ($8000-$FFFF: PRG-ROM / PRG-RAM)
//	+------------------+ $8000
//	| Cartridge SRAM   | ($6000-$7FFF: Save RAM, battery-backed if present)
//	+------------------+ $6000
//	| Expansion ROM    | ($4020-$5FFF: Cartridge expansion space)
//	+------------------+ $4020
//	| APU & I/O Regs   | ($4000-$401F: Sound, Joypads, OAM DMA)
//	+------------------+ $2000
//	| PPU Registers    | ($2000-$2007, mirrored every 8 bytes up to $3FFF)
//	+------------------+ $0800
//	| Internal RAM     | ($0000-$07FF: 2KB CPU RAM, mirrored 3 times up to $1FFF)
//	+------------------+ $0000
package bus

import (
	"github.com/dustblow/dustblow/internal/apu"
	"github.com/dustblow/dustblow/internal/cartridge"
	"github.com/dustblow/dustblow/internal/joypad"
	"github.com/dustblow/dustblow/internal/ppu"
)

const (
	// RAMSize is the physical size of the NES internal work RAM (2KB).
	RAMSize = 0x0800
	// RAMEndAddress is the end of CPU internal RAM mirrors ($1FFF).
	RAMEndAddress uint16 = 0x1FFF
	// RAMMirrorMask wraps addresses in $0000-$1FFF to the physical 2KB RAM ($07FF).
	RAMMirrorMask uint16 = 0x07FF

	// PPURegistersEnd is the end of memory-mapped PPU register mirrors ($3FFF).
	PPURegistersEnd uint16 = 0x3FFF
	// PPURegistersMirrorMask wraps addresses to the 8 physical PPU registers ($2000-$2007).
	PPURegistersMirrorMask uint16 = 0x0007
	// RegisterPPUStatusOffset is the modulo-8 register offset for PPUSTATUS ($2002).
	RegisterPPUStatusOffset uint16 = 2
	// RegisterPPUDATAOffset is the modulo-8 register offset for PPUDATA ($2007).
	RegisterPPUDATAOffset uint16 = 7

	// APURegistersEnd is the end of memory-mapped APU and I/O registers ($4017).
	APURegistersEnd uint16 = 0x4017
	// RegisterAPUChannelEnd is the end of the APU synthesis channel registers ($4000-$4013).
	RegisterAPUChannelEnd uint16 = 0x4013

	// CartridgeSpaceBase is the start of cartridge address space ($4020 for expansion ROM/RAM/registers, $6000 for PRG-RAM, $8000 for PRG-ROM).
	CartridgeSpaceBase uint16 = 0x4020
	// PRGBaseAddress is the start of cartridge PRG-ROM space ($8000).
	PRGBaseAddress uint16 = 0x8000
	// OpenBusValue is the floating bus value returned when reading unmapped or write-only I/O registers.
	OpenBusValue byte = 0xFF

	// RegisterOAMDMA is the CPU memory-mapped address for OAM DMA transfers ($4014).
	RegisterOAMDMA uint16 = 0x4014
	// RegisterAPUStatus is the CPU memory-mapped address for APU status and channel control ($4015).
	RegisterAPUStatus uint16 = 0x4015
	// RegisterJoypad1 is the CPU memory-mapped address for Joypad 1 and strobe ($4016).
	RegisterJoypad1 uint16 = 0x4016
	// RegisterJoypad2 is the CPU memory-mapped address for Joypad 2 ($4017).
	RegisterJoypad2 uint16 = 0x4017
	// OAMDMASize is the number of bytes transferred during an OAM DMA operation (256 bytes).
	OAMDMASize = 256
	// OAMDMACycles is the number of CPU stall cycles consumed by an OAM DMA transfer (1 wait state + 512 DMA cycles).
	OAMDMACycles = 513
	// ShiftDMAAddressPage shifts the page byte into the upper byte of a 16-bit address.
	ShiftDMAAddressPage = 8

	// apuStatusOpenBusBit5Mask masks bit 5 when reading $4015 from the CPU internal data bus.
	apuStatusOpenBusBit5Mask byte = 0x20
	// apuStatusDrivenBitsMask masks the status bits driven by the APU ($4015).
	apuStatusDrivenBitsMask byte = 0xDF

	// joypadDataBitsMask masks the low 5 bits returned by controller ports ($4016/$4017).
	joypadDataBitsMask byte = 0x1F
	// joypadOpenBusBitsMask masks the upper 3 bits driven by open bus on controller reads.
	joypadOpenBusBitsMask byte = 0xE0
)

// Controller represents any peripheral connected to a NES controller port (e.g. standard Joypad, Zapper).
type Controller interface {
	Read() byte
	Write(data byte)
	Peek() byte
}

// Bus interconnects the CPU with internal RAM, PPU, APU, controller ports, and the inserted Cartridge.
// Struct field layout is ordered for optimal memory alignment.
type Bus struct {
	cartridge       *cartridge.Cartridge
	ppu             *ppu.PPU
	apu             *apu.APU
	controllers     [joypad.PortCount]Controller
	ioAccessHook    func(addr uint16, isWrite bool)
	dmaStallCycles  int
	ram             [RAMSize]byte
	dataBus         byte
	internalDataBus byte
}

// DataBus returns the current floating external data bus byte.
func (b *Bus) DataBus() byte {
	return b.dataBus
}

// InternalDataBus returns the current value on the CPU internal data bus.
func (b *Bus) InternalDataBus() byte {
	return b.internalDataBus
}

// SetIOAccessHook configures an optional callback invoked immediately before accessing PPU or APU registers ($2000-$401F).
func (b *Bus) SetIOAccessHook(hook func(addr uint16, isWrite bool)) {
	b.ioAccessHook = hook
}

// SetPPUAccessHook configures an optional callback invoked immediately before accessing PPU registers ($2000-$3FFF).
// Preserved for backward compatibility.
func (b *Bus) SetPPUAccessHook(hook func()) {
	b.SetIOAccessHook(func(uint16, bool) { hook() })
}

// New creates and initializes a new system Bus connected to the provided Cartridge.
func New(cart *cartridge.Cartridge) *Bus {
	return &Bus{
		cartridge:       cart,
		ppu:             ppu.New(cart),
		dataBus:         OpenBusValue,
		internalDataBus: OpenBusValue,
	}
}

// RAM returns a copy of the internal 2KB work RAM.
func (b *Bus) RAM() [RAMSize]byte {
	return b.ram
}

// SetRAM restores the internal 2KB work RAM.
func (b *Bus) SetRAM(ram [RAMSize]byte) {
	b.ram = ram
}

// SetDataBus updates the external data bus value and propagates to cartridge open bus.
func (b *Bus) SetDataBus(val byte) {
	b.dataBus = val
	if b.cartridge != nil {
		b.cartridge.OpenBus = val
	}
}

// AttachController connects a peripheral device to the specified NES controller port.
func (b *Bus) AttachController(port joypad.Port, c Controller) {
	if int(port) < len(b.controllers) {
		b.controllers[port] = c
	}
}

// DetachController disconnects any peripheral from the specified NES controller port.
func (b *Bus) DetachController(port joypad.Port) {
	if int(port) < len(b.controllers) {
		b.controllers[port] = nil
	}
}

// PPU returns the attached PPU instance.
func (b *Bus) PPU() *ppu.PPU {
	return b.ppu
}

// AttachPPU connects an explicit PPU instance to the system bus.
func (b *Bus) AttachPPU(p *ppu.PPU) {
	b.ppu = p
}

// APU returns the attached APU instance.
func (b *Bus) APU() *apu.APU {
	return b.apu
}

// AttachAPU connects an explicit APU instance to the system bus.
func (b *Bus) AttachAPU(a *apu.APU) {
	b.apu = a
}

// Read performs a normal memory read that may trigger hardware side-effects.
func (b *Bus) Read(addr uint16) byte {
	if addr <= RAMEndAddress {
		b.dataBus = b.ram[addr&RAMMirrorMask]
		b.internalDataBus = b.dataBus
		if b.cartridge != nil {
			b.cartridge.OpenBus = b.dataBus
		}
		return b.dataBus
	}

	if addr <= PPURegistersEnd {
		if b.ioAccessHook != nil {
			b.ioAccessHook(addr, false)
		}
		if b.ppu != nil {
			b.dataBus = b.ppu.ReadRegister(addr & PPURegistersMirrorMask)
			b.internalDataBus = b.dataBus
			if b.cartridge != nil {
				b.cartridge.OpenBus = b.dataBus
			}
			return b.dataBus
		}
		return 0
	}

	if addr <= APURegistersEnd {
		if b.ioAccessHook != nil {
			b.ioAccessHook(addr, false)
		}
		return b.readAPUOrControllers(addr)
	}

	if addr >= CartridgeSpaceBase {
		if b.ioAccessHook != nil && addr < PRGBaseAddress {
			b.ioAccessHook(addr, false)
		}
		if b.cartridge != nil {
			val := b.cartridge.ReadPRG(addr)
			b.dataBus = val
			b.internalDataBus = val
			b.cartridge.OpenBus = val
			return val
		}
		return 0
	}

	b.internalDataBus = b.dataBus
	return b.dataBus
}

func (b *Bus) readAPUOrControllers(addr uint16) byte {
	if addr == RegisterAPUStatus {
		if b.apu != nil {
			apuVal := b.apu.ReadRegister(addr)
			// Bit 5 is not driven by the APU; it reflects the CPU's internal data bus.
			// Reading $4015 is internal to the 2A03 and does NOT update the external data bus.
			val := (apuVal & apuStatusDrivenBitsMask) | (b.internalDataBus & apuStatusOpenBusBit5Mask)
			b.internalDataBus = val
			return val
		}
		b.internalDataBus = OpenBusValue
		return OpenBusValue
	}
	if addr < RegisterJoypad1 {
		b.internalDataBus = b.dataBus
		return b.dataBus
	}
	port := addr - RegisterJoypad1
	if int(port) >= len(b.controllers) || b.controllers[port] == nil {
		b.internalDataBus = OpenBusValue
		return OpenBusValue
	}
	ctrlVal := b.controllers[port].Read()
	// On NES, bits 5-7 are driven by the floating data bus.
	b.dataBus = (ctrlVal & joypadDataBitsMask) | (b.dataBus & joypadOpenBusBitsMask)
	b.internalDataBus = b.dataBus
	if b.cartridge != nil {
		b.cartridge.OpenBus = b.dataBus
	}
	return b.dataBus
}

// ReadDMC performs a memory fetch initiated by the DMC DMA unit.
// DMC DMA reads from memory onto the external data bus and into the DMC sample buffer,
// but does NOT update the CPU's internal data bus.
func (b *Bus) ReadDMC(addr uint16) byte {
	if addr <= RAMEndAddress {
		b.dataBus = b.ram[addr&RAMMirrorMask]
		if b.cartridge != nil {
			b.cartridge.OpenBus = b.dataBus
		}
		return b.dataBus
	}

	if addr <= PPURegistersEnd {
		if b.ppu != nil {
			b.dataBus = b.ppu.ReadRegister(addr & PPURegistersMirrorMask)
			if b.cartridge != nil {
				b.cartridge.OpenBus = b.dataBus
			}
			return b.dataBus
		}
		return 0
	}

	if addr <= APURegistersEnd {
		return b.readDMCAPURegisters(addr)
	}

	if addr >= CartridgeSpaceBase {
		if b.cartridge != nil {
			val := b.cartridge.ReadPRG(addr)
			b.dataBus = val
			b.cartridge.OpenBus = val
			return val
		}
		return 0
	}

	return b.dataBus
}

func (b *Bus) readDMCAPURegisters(addr uint16) byte {
	if addr == RegisterAPUStatus {
		if b.apu != nil {
			apuVal := b.apu.ReadRegister(addr)
			return (apuVal & apuStatusDrivenBitsMask) | (b.internalDataBus & apuStatusOpenBusBit5Mask)
		}
		return OpenBusValue
	}
	if addr < RegisterJoypad1 {
		return b.dataBus
	}
	port := addr - RegisterJoypad1
	if int(port) >= len(b.controllers) || b.controllers[port] == nil {
		return OpenBusValue
	}
	ctrlVal := b.controllers[port].Read()
	b.dataBus = (ctrlVal & joypadDataBitsMask) | (b.dataBus & joypadOpenBusBitsMask)
	if b.cartridge != nil {
		b.cartridge.OpenBus = b.dataBus
	}
	return b.dataBus
}

// Write performs a memory write to the target hardware device.
func (b *Bus) Write(addr uint16, data byte) {
	b.dataBus = data
	b.internalDataBus = data
	if b.cartridge != nil {
		b.cartridge.OpenBus = data
	}

	if addr <= RAMEndAddress {
		b.ram[addr&RAMMirrorMask] = data
		return
	}

	if addr <= PPURegistersEnd {
		if b.ioAccessHook != nil {
			b.ioAccessHook(addr, true)
		}
		if b.ppu != nil {
			b.ppu.WriteRegister(addr&PPURegistersMirrorMask, data)
		}
		return
	}

	if addr <= APURegistersEnd {
		b.writeAPURegister(addr, data)
		return
	}

	if addr >= CartridgeSpaceBase && b.cartridge != nil {
		b.cartridge.WritePRG(addr, data)
	}
}

func (b *Bus) writeAPURegister(addr uint16, data byte) {
	if b.ioAccessHook != nil {
		b.ioAccessHook(addr, true)
	}
	switch addr {
	case RegisterOAMDMA:
		b.transferOAMDMA(data)
	case RegisterJoypad1:
		for _, c := range b.controllers {
			if c != nil {
				c.Write(data)
			}
		}
	}

	if (addr <= RegisterAPUChannelEnd || addr == RegisterAPUStatus || addr == RegisterJoypad2) && b.apu != nil {
		b.apu.WriteRegister(addr, data)
	}
}

func (b *Bus) transferOAMDMA(page byte) {
	if b.ppu == nil {
		return
	}
	base := uint16(page) << ShiftDMAAddressPage
	oamAddr := b.ppu.OAMAddr()
	for i := range uint16(OAMDMASize) {
		val := b.Read(base + i)
		b.ppu.WriteOAM(oamAddr+byte(i), val)
		b.dataBus = val
		b.internalDataBus = val
		if b.cartridge != nil {
			b.cartridge.OpenBus = val
		}
	}
	stall := OAMDMACycles
	if b.apu != nil && !b.apu.CurrentCycleIsGet() {
		stall++
	}
	// DMA cannot start on a CPU write cycle. For RMW instructions (e.g. INC $4014),
	// the second write overrides the transfer page and only a single DMA occurs.
	b.dmaStallCycles = stall
	if b.apu != nil {
		b.apu.SyncDMA()
	}
}

// TakeDMAStallCycles returns and resets accumulated CPU stall cycles caused by OAM DMA transfers.
func (b *Bus) TakeDMAStallCycles() int {
	stalls := b.dmaStallCycles
	b.dmaStallCycles = 0
	return stalls
}

// Peek performs a passive, non-intrusive memory read without triggering hardware side-effects.
// Used by tracers and test inspection.
func (b *Bus) Peek(addr uint16) byte {
	if addr <= RAMEndAddress {
		return b.ram[addr&RAMMirrorMask]
	}

	if addr <= PPURegistersEnd {
		if b.ppu != nil {
			return b.ppu.PeekRegister(addr & PPURegistersMirrorMask)
		}
		return 0
	}

	if addr <= APURegistersEnd {
		return b.peekAPUOrControllers(addr)
	}

	if addr >= CartridgeSpaceBase {
		if b.cartridge != nil {
			return b.cartridge.ReadPRG(addr)
		}
		return 0
	}

	return b.dataBus
}

func (b *Bus) peekAPUOrControllers(addr uint16) byte {
	if addr < RegisterJoypad1 {
		return OpenBusValue
	}
	port := addr - RegisterJoypad1
	if int(port) >= len(b.controllers) || b.controllers[port] == nil {
		return OpenBusValue
	}
	ctrlVal := b.controllers[port].Peek()
	return (ctrlVal & joypadDataBitsMask) | (b.dataBus & joypadOpenBusBitsMask)
}

