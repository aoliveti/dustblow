// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cartridge implements NES cartridge loading, iNES header parsing, and memory mappers.
//
// NES CPU Memory Map Overview (Cartridge PRG Window):
//
//	+------------------+ $FFFF
//	| Interrupt Vectors| ($FFFA: NMI, $FFFC: RESET, $FFFE: IRQ/BRK)
//	+------------------+ $C000
//	| PRG-ROM Upper    | (Mirrored from $8000 on 16KB NROM-128; distinct bank on 32KB NROM-256)
//	+------------------+ $8000
//	| PRG-ROM Lower    | (Mapped to first 16KB bank of PRG-ROM)
//	+------------------+ $6000
//	| PRG-RAM (SRAM)   | (Optional save RAM on battery-backed cartridges)
//	+------------------+ $0000
//
// NES PPU Memory Map Overview (Cartridge CHR Window):
//
//	+------------------+ $1FFF
//	| Pattern Table 1  | (Sprites / Background tiles, 4KB)
//	+------------------+ $1000
//	| Pattern Table 0  | (Sprites / Background tiles, 4KB)
//	+------------------+ $0000
package cartridge

import "io"

// Mirroring represents the nametable mirroring mode used by the PPU.
type Mirroring byte

const (
	// MirrorHorizontal arranges nametables horizontally.
	MirrorHorizontal Mirroring = 0
	// MirrorVertical arranges nametables vertically.
	MirrorVertical Mirroring = 1
	// MirrorSingle0 maps all nametables to the lower 1KB VRAM page.
	MirrorSingle0 Mirroring = 2
	// MirrorSingle1 maps all nametables to the upper 1KB VRAM page.
	MirrorSingle1 Mirroring = 3
	// MirrorFourScreen provides 4 distinct nametable screens.
	MirrorFourScreen Mirroring = 4
)

// Mapper defines the interface for hardware mapper chips on a cartridge.
type Mapper interface {
	ReadPRG(addr uint16) byte
	WritePRG(addr uint16, data byte)
	ReadCHR(addr uint16) byte
	WriteCHR(addr uint16, data byte)
}

// ScanlineClockable defines an optional interface for mappers driven by PPU scanline clocks (e.g. MMC3, MMC5).
type ScanlineClockable interface {
	ClockScanline(scanline int)
}

// NametableMapper defines an optional interface for mappers providing custom nametable
// routing, such as MMC5 (CIRAM 0/1, ExRAM, or Fill-mode).
type NametableMapper interface {
	ReadNametable(addr uint16, vram []byte) (byte, bool)
	WriteNametable(addr uint16, val byte, vram []byte) bool
}

// IRQMapper defines an optional interface for mappers capable of asserting maskable CPU IRQ interrupts.
type IRQMapper interface {
	IRQPending() bool
}

// CPUSteppable defines an optional interface for mappers clocked by CPU execution cycles (e.g. Mapper 40).
type CPUSteppable interface {
	StepCPU(cycles int)
}

// SpriteFetchNotifier defines an optional interface for mappers needing to distinguish sprite vs background CHR fetches.
type SpriteFetchNotifier interface {
	SetSpriteFetch(active bool)
}

// ExpansionAudio defines an optional interface for cartridge mappers providing expansion sound (e.g. MMC5, VRC6, Sunsoft 5B).
type ExpansionAudio interface {
	StepAudio(cycles int)
	AudioOutput() float32
}

// StatefulMapper defines an optional interface for mappers capable of serializing their internal state.
type StatefulMapper interface {
	SaveState(w io.Writer) error
	LoadState(r io.Reader) error
}

// Resettable defines an optional interface for cartridge mappers that need to silence expansion
// audio or clear pending interrupt state on console reset.
type Resettable interface {
	Reset()
}

const (
	// SRAMSize is the standard battery-backed PRG-RAM capacity (8KB).
	SRAMSize = 8 * 1024
	// SRAMBaseAddress is the starting address of PRG-RAM in CPU address space ($6000).
	SRAMBaseAddress uint16 = 0x6000
	// SRAMEndAddress is the ending address of PRG-RAM in CPU address space ($7FFF).
	SRAMEndAddress uint16 = 0x7FFF
)

// Cartridge represents an NES cartridge containing PRG-ROM, CHR-ROM/RAM, and a mapper.
type Cartridge struct {
	PRG            []byte
	CHR            []byte
	SRAM           []byte
	MapperID       byte
	Submapper      byte
	Mirroring      Mirroring
	HasBattery     bool
	IsCHRRAM       bool
	OpenBus        byte
	Mapper         Mapper
	Hash           string
	expansionAudio ExpansionAudio
	sramDirty      bool

	boundMapper    Mapper
	irqMapper      IRQMapper
	cpuSteppable   CPUSteppable
	scanlineClock  ScanlineClockable
	nametableMap   NametableMapper
	spriteNotifier SpriteFetchNotifier
	statefulMapper StatefulMapper
	resettable     Resettable
}

// BatterySRAM returns the battery-backed SRAM bytes if the cartridge has a battery, or nil.
func (c *Cartridge) BatterySRAM() []byte {
	if c == nil || !c.HasBattery {
		return nil
	}
	return c.SRAM
}

// LoadBatterySRAM populates the cartridge battery-backed SRAM from persisted data.
func (c *Cartridge) LoadBatterySRAM(sram []byte) {
	if c == nil || !c.HasBattery || len(sram) == 0 {
		return
	}
	if len(c.SRAM) == 0 {
		c.SRAM = make([]byte, SRAMSize)
	}
	copy(c.SRAM, sram)
	c.sramDirty = false
}

// IsSRAMDirty reports whether the battery-backed SRAM has been modified since last save.
func (c *Cartridge) IsSRAMDirty() bool {
	if c == nil {
		return false
	}
	return c.sramDirty
}

// MarkSRAMDirty marks the SRAM as modified if the cartridge has battery-backed PRG-RAM.
func (c *Cartridge) MarkSRAMDirty() {
	if c != nil && c.HasBattery {
		c.sramDirty = true
	}
}

// ClearSRAMDirty clears the modified flag after persisting.
func (c *Cartridge) ClearSRAMDirty() {
	if c != nil {
		c.sramDirty = false
	}
}

// ReadSRAM reads a byte from standard battery-backed or work PRG-RAM ($6000-$7FFF) if present.
func (c *Cartridge) ReadSRAM(addr uint16) byte {
	if c == nil || addr < SRAMBaseAddress || addr > SRAMEndAddress || len(c.SRAM) == 0 {
		if c != nil {
			return c.OpenBus
		}
		return 0
	}
	return c.SRAM[addr-SRAMBaseAddress]
}

// WriteSRAM writes a byte to standard PRG-RAM ($6000-$7FFF) and marks it dirty if battery-backed.
// Returns true if the address fell within PRG-RAM space ($6000-$7FFF).
func (c *Cartridge) WriteSRAM(addr uint16, data byte) bool {
	if c == nil || addr < SRAMBaseAddress || addr > SRAMEndAddress {
		return false
	}
	if len(c.SRAM) > 0 {
		c.SRAM[addr-SRAMBaseAddress] = data
		c.MarkSRAMDirty()
	}
	return true
}

const unbankedCHRBankSize uint16 = 0x2000

// UnbankedCHR provides standard fixed 8KB CHR-ROM or CHR-RAM memory access ($0000-$1FFF).
type UnbankedCHR struct {
	cartridge *Cartridge
}

// NewUnbankedCHR creates an UnbankedCHR handler for fixed 8KB CHR memory.
func NewUnbankedCHR(cart *Cartridge) UnbankedCHR {
	return UnbankedCHR{cartridge: cart}
}

// ReadCHR maps PPU reads in the range $0000-$1FFF to CHR-ROM/RAM.
func (u UnbankedCHR) ReadCHR(addr uint16) byte {
	if u.cartridge == nil || addr >= unbankedCHRBankSize || int(addr) >= len(u.cartridge.CHR) {
		return 0
	}
	return u.cartridge.CHR[addr]
}

// WriteCHR maps PPU writes to CHR-RAM in the range $0000-$1FFF.
func (u UnbankedCHR) WriteCHR(addr uint16, data byte) {
	if u.cartridge == nil || !u.cartridge.IsCHRRAM || addr >= unbankedCHRBankSize || int(addr) >= len(u.cartridge.CHR) {
		return
	}
	u.cartridge.CHR[addr] = data
}

// ReadPRG reads a byte from PRG memory through the mapper.
func (c *Cartridge) ReadPRG(addr uint16) byte {
	if c == nil || c.Mapper == nil {
		return 0
	}
	return c.Mapper.ReadPRG(addr)
}

// WritePRG writes a byte to PRG memory through the mapper.
func (c *Cartridge) WritePRG(addr uint16, data byte) {
	if c == nil || c.Mapper == nil {
		return
	}
	c.Mapper.WritePRG(addr, data)
}

// ReadCHR reads a byte from CHR memory through the mapper.
func (c *Cartridge) ReadCHR(addr uint16) byte {
	if c == nil || c.Mapper == nil {
		return 0
	}
	return c.Mapper.ReadCHR(addr)
}

// WriteCHR writes a byte to CHR memory through the mapper.
func (c *Cartridge) WriteCHR(addr uint16, data byte) {
	if c == nil || c.Mapper == nil {
		return
	}
	c.Mapper.WriteCHR(addr, data)
}

// BindMapper sets the mapper for this cartridge and caches any optional interfaces it implements.
func (c *Cartridge) BindMapper(m Mapper) {
	if c == nil {
		return
	}
	c.Mapper = m
	c.boundMapper = m
	if m == nil {
		c.irqMapper = nil
		c.cpuSteppable = nil
		c.scanlineClock = nil
		c.nametableMap = nil
		c.spriteNotifier = nil
		c.statefulMapper = nil
		c.resettable = nil
		return
	}

	c.irqMapper, _ = m.(IRQMapper)
	c.cpuSteppable, _ = m.(CPUSteppable)
	c.scanlineClock, _ = m.(ScanlineClockable)
	c.nametableMap, _ = m.(NametableMapper)
	c.spriteNotifier, _ = m.(SpriteFetchNotifier)
	c.statefulMapper, _ = m.(StatefulMapper)
	c.resettable, _ = m.(Resettable)
}

func (c *Cartridge) ensureMapperBound() {
	if c.Mapper == c.boundMapper {
		return
	}
	c.BindMapper(c.Mapper)
}

// ReadNametable intercepts nametable reads if supported by the mapper.
func (c *Cartridge) ReadNametable(addr uint16, vram []byte) (byte, bool) {
	if c == nil {
		return 0, false
	}
	c.ensureMapperBound()
	if c.nametableMap != nil {
		return c.nametableMap.ReadNametable(addr, vram)
	}
	return 0, false
}

// WriteNametable intercepts nametable writes if supported by the mapper.
func (c *Cartridge) WriteNametable(addr uint16, val byte, vram []byte) bool {
	if c == nil {
		return false
	}
	c.ensureMapperBound()
	if c.nametableMap != nil {
		return c.nametableMap.WriteNametable(addr, val, vram)
	}
	return false
}

// ClockScanline notifies the mapper of a PPU scanline tick if the mapper supports it.
func (c *Cartridge) ClockScanline(scanline int) {
	if c == nil {
		return
	}
	c.ensureMapperBound()
	if c.scanlineClock != nil {
		c.scanlineClock.ClockScanline(scanline)
	}
}

// IRQPending reports whether the cartridge mapper is currently asserting an IRQ interrupt line.
func (c *Cartridge) IRQPending() bool {
	if c == nil {
		return false
	}
	c.ensureMapperBound()
	if c.irqMapper != nil {
		return c.irqMapper.IRQPending()
	}
	return false
}

// StepCPU advances CPU cycle counters on the cartridge mapper if supported.
func (c *Cartridge) StepCPU(cycles int) {
	if c == nil {
		return
	}
	c.ensureMapperBound()
	if c.cpuSteppable != nil {
		c.cpuSteppable.StepCPU(cycles)
	}
}

// SetSpriteFetch notifies the mapper whether the PPU is actively fetching sprite tiles.
func (c *Cartridge) SetSpriteFetch(active bool) {
	if c == nil {
		return
	}
	c.ensureMapperBound()
	if c.spriteNotifier != nil {
		c.spriteNotifier.SetSpriteFetch(active)
	}
}

// SaveState serializes the mapper's state if supported by the mapper.
func (c *Cartridge) SaveState(w io.Writer) error {
	if c == nil {
		return nil
	}
	c.ensureMapperBound()
	if c.statefulMapper != nil {
		return c.statefulMapper.SaveState(w)
	}
	return nil
}

// LoadState restores the mapper's state if supported by the mapper.
func (c *Cartridge) LoadState(r io.Reader) error {
	if c == nil {
		return nil
	}
	c.ensureMapperBound()
	if c.statefulMapper != nil {
		return c.statefulMapper.LoadState(r)
	}
	return nil
}

// ExpansionAudio returns the expansion audio synthesizer interface if supported by the cartridge mapper.
func (c *Cartridge) ExpansionAudio() ExpansionAudio {
	if c == nil {
		return nil
	}
	return c.expansionAudio
}

// SetExpansionAudio registers the expansion audio synthesizer interface.
func (c *Cartridge) SetExpansionAudio(ea ExpansionAudio) {
	if c != nil {
		c.expansionAudio = ea
	}
}

// Reset silences expansion audio and clears pending IRQ state on console reset.
func (c *Cartridge) Reset() {
	if c == nil {
		return
	}
	c.ensureMapperBound()
	if c.resettable != nil {
		c.resettable.Reset()
	}
}
