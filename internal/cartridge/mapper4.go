// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	mmc3PRGBankSize     = 0x2000 // 8 KB PRG bank size
	mmc3CHRBankSize     = 0x0400 // 1 KB CHR bank size
	mmc3PRGWindowMask   = 0x1FFF // 8 KB window offset mask ($8000-$9FFF, etc.)
	mmc3CHRWindowMask   = 0x03FF // 1 KB window offset mask
	mmc3PRGBaseAddress  = 0x8000 // Starting address of PRG window in CPU space
	mmc3CHRBaseAddress  = 0x0000 // Starting address of CHR window in PPU space
	mmc3CHRWindowCount  = 8
	mmc3PRGWindowCount  = 4
	mmc3RegisterCount   = 8

	mmc3TargetRegisterMask = 0x07 // Bits 0-2 select register R0-R7
	mmc3PRGModeMask        = 0x40 // Bit 6 selects PRG bank mode (0: $8000 swappable, 1: $C000 swappable)
	mmc3CHRInversionMask   = 0x80 // Bit 7 selects CHR A12 inversion (0: 2KB at $0000, 1: 2KB at $1000)

	mmc3MirroringVertical   = 0
	mmc3MirroringHorizontal = 1
	mmc3MirroringMask       = 0x01

	mmc3RAMEnableMask       = 0x80 // Bit 7 enables PRG-RAM
	mmc3RAMWriteProtectMask = 0x40 // Bit 6 write-protects PRG-RAM

	mmc3CHRBankR0 = 0
	mmc3CHRBankR1 = 1
	mmc3CHRBankR2 = 2
	mmc3CHRBankR3 = 3
	mmc3CHRBankR4 = 4
	mmc3CHRBankR5 = 5
	mmc3PRGBankR6 = 6
	mmc3PRGBankR7 = 7

	mmc3CHR2KBMask = 0xFE // R0 and R1 ignore the lowest bit (2 KB alignment)

	minMMC3PRGBanks = 2
	minMMC3CHRBanks = 1
)

// Mapper4 implements the Nintendo MMC3 / TxROM mapper (iNES Mapper 4).
// Supports games like Super Mario Bros. 3, Mega Man 3-6, Kirby's Adventure, Ninja Gaiden 2-3.
type Mapper4 struct {
	cartridge       *Cartridge
	prgBanks8K      int
	chrBanks1K      int
	targetRegister  int
	prgMode         int
	chrInversion    int
	registers       [mmc3RegisterCount]int
	prgOffsets      [mmc3PRGWindowCount]int
	chrOffsets      [mmc3CHRWindowCount]int
	irqCounter      byte
	irqLatch        byte
	irqReload       bool
	irqEnabled      bool
	irqPending      bool
	ramEnabled      bool
	ramWriteProtect bool
}

// NewMapper4 creates and initializes a new Mapper4 (MMC3) instance.
func NewMapper4(cart *Cartridge) *Mapper4 {
	prgBanks := max(len(cart.PRG)/mmc3PRGBankSize, minMMC3PRGBanks)
	chrBanks := max(len(cart.CHR)/mmc3CHRBankSize, minMMC3CHRBanks)

	m := &Mapper4{
		cartridge:  cart,
		prgBanks8K: prgBanks,
		chrBanks1K: chrBanks,
		ramEnabled: true,
	}

	m.registers[mmc3PRGBankR6] = 0
	m.registers[mmc3PRGBankR7] = 1
	m.updateOffsets()
	return m
}

// ReadPRG maps CPU reads to PRG-RAM ($6000-$7FFF) or banked PRG-ROM ($8000-$FFFF).
func (m *Mapper4) ReadPRG(addr uint16) byte {
	if addr >= SRAMBaseAddress && addr <= SRAMEndAddress {
		if m.ramEnabled && len(m.cartridge.SRAM) > 0 {
			return m.cartridge.SRAM[addr-SRAMBaseAddress]
		}
		return 0
	}

	if addr < mmc3PRGBaseAddress {
		return 0
	}

	window := int((addr - mmc3PRGBaseAddress) / mmc3PRGBankSize)
	offset := m.prgOffsets[window] + int(addr&mmc3PRGWindowMask)
	if offset >= len(m.cartridge.PRG) {
		return 0
	}

	return m.cartridge.PRG[offset]
}

// WritePRG handles CPU writes to PRG-RAM ($6000-$7FFF) or MMC3 control registers ($8000-$FFFF).
func (m *Mapper4) WritePRG(addr uint16, data byte) {
	if addr >= SRAMBaseAddress && addr <= SRAMEndAddress {
		if m.ramEnabled && !m.ramWriteProtect && len(m.cartridge.SRAM) > 0 {
			m.cartridge.SRAM[addr-SRAMBaseAddress] = data
			m.cartridge.MarkSRAMDirty()
		}
		return
	}

	if addr < mmc3PRGBaseAddress {
		return
	}

	isOdd := (addr & 1) != 0
	switch {
	case addr <= 0x9FFF:
		m.handleBankSelectWrite(isOdd, data)
	case addr <= 0xBFFF:
		m.handleMirroringAndRAMWrite(isOdd, data)
	case addr <= 0xDFFF:
		m.handleIRQCounterWrite(isOdd, data)
	default:
		m.handleIRQControlWrite(isOdd)
	}
}

func (m *Mapper4) handleBankSelectWrite(isOdd bool, data byte) {
	if !isOdd {
		m.targetRegister = int(data & mmc3TargetRegisterMask)
		m.prgMode = int(data & mmc3PRGModeMask)
		m.chrInversion = int(data & mmc3CHRInversionMask)
		m.updateOffsets()
		return
	}

	val := int(data)
	if m.targetRegister == mmc3CHRBankR0 || m.targetRegister == mmc3CHRBankR1 {
		val &= mmc3CHR2KBMask
	}
	m.registers[m.targetRegister] = val
	m.updateOffsets()
}

func (m *Mapper4) handleMirroringAndRAMWrite(isOdd bool, data byte) {
	if isOdd {
		m.ramEnabled = (data & mmc3RAMEnableMask) != 0
		m.ramWriteProtect = (data & mmc3RAMWriteProtectMask) != 0
		return
	}
	if m.cartridge.Mirroring == MirrorFourScreen {
		return
	}
	if (data & mmc3MirroringMask) == mmc3MirroringVertical {
		m.cartridge.Mirroring = MirrorVertical
		return
	}
	m.cartridge.Mirroring = MirrorHorizontal
}

func (m *Mapper4) handleIRQCounterWrite(isOdd bool, data byte) {
	if !isOdd {
		m.irqLatch = data
		return
	}
	m.irqReload = true
}

func (m *Mapper4) handleIRQControlWrite(isOdd bool) {
	if !isOdd {
		m.irqEnabled = false
		m.irqPending = false
		return
	}
	m.irqEnabled = true
}

// ReadCHR maps PPU reads in $0000-$1FFF using the 1 KB CHR bank offset table.
func (m *Mapper4) ReadCHR(addr uint16) byte {
	if int(addr) >= mmc3CHRBankSize*mmc3CHRWindowCount {
		return 0
	}

	window := int(addr / mmc3CHRBankSize)
	offset := m.chrOffsets[window] + int(addr&mmc3CHRWindowMask)
	if offset >= len(m.cartridge.CHR) {
		return 0
	}

	return m.cartridge.CHR[offset]
}

// WriteCHR maps PPU writes to CHR-RAM in $0000-$1FFF if the cartridge uses RAM.
func (m *Mapper4) WriteCHR(addr uint16, data byte) {
	if int(addr) >= mmc3CHRBankSize*mmc3CHRWindowCount {
		return
	}

	window := int(addr / mmc3CHRBankSize)
	offset := m.chrOffsets[window] + int(addr&mmc3CHRWindowMask)
	if offset >= len(m.cartridge.CHR) {
		return
	}

	m.cartridge.CHR[offset] = data
}

// ClockScanline decrements the internal MMC3 scanline IRQ counter.
func (m *Mapper4) ClockScanline(_ int) {
	if m.irqCounter == 0 || m.irqReload {
		m.irqCounter = m.irqLatch
		m.irqReload = false
		m.checkIRQTrigger()
		return
	}

	m.irqCounter--
	m.checkIRQTrigger()
}

func (m *Mapper4) checkIRQTrigger() {
	if m.irqCounter == 0 && m.irqEnabled {
		m.irqPending = true
	}
}

// IRQPending reports whether the mapper is asserting a hardware IRQ line.
func (m *Mapper4) IRQPending() bool {
	return m.irqPending
}

func (m *Mapper4) updateOffsets() {
	m.updatePRGOffsets()
	m.updateCHROffsets()
}

func (m *Mapper4) updatePRGOffsets() {
	bank0 := m.registers[mmc3PRGBankR6] % m.prgBanks8K
	bank1 := m.registers[mmc3PRGBankR7] % m.prgBanks8K
	bankPenultimate := (m.prgBanks8K - 2) % m.prgBanks8K
	bankLast := (m.prgBanks8K - 1) % m.prgBanks8K

	if m.prgMode == 0 {
		m.prgOffsets[0] = bank0 * mmc3PRGBankSize
		m.prgOffsets[1] = bank1 * mmc3PRGBankSize
		m.prgOffsets[2] = bankPenultimate * mmc3PRGBankSize
		m.prgOffsets[3] = bankLast * mmc3PRGBankSize
		return
	}

	m.prgOffsets[0] = bankPenultimate * mmc3PRGBankSize
	m.prgOffsets[1] = bank1 * mmc3PRGBankSize
	m.prgOffsets[2] = bank0 * mmc3PRGBankSize
	m.prgOffsets[3] = bankLast * mmc3PRGBankSize
}

func (m *Mapper4) updateCHROffsets() {
	r0 := m.registers[mmc3CHRBankR0] % m.chrBanks1K
	r1 := m.registers[mmc3CHRBankR1] % m.chrBanks1K
	r2 := m.registers[mmc3CHRBankR2] % m.chrBanks1K
	r3 := m.registers[mmc3CHRBankR3] % m.chrBanks1K
	r4 := m.registers[mmc3CHRBankR4] % m.chrBanks1K
	r5 := m.registers[mmc3CHRBankR5] % m.chrBanks1K

	if m.chrInversion == 0 {
		m.chrOffsets[0] = r0 * mmc3CHRBankSize
		m.chrOffsets[1] = (r0 + 1) * mmc3CHRBankSize
		m.chrOffsets[2] = r1 * mmc3CHRBankSize
		m.chrOffsets[3] = (r1 + 1) * mmc3CHRBankSize
		m.chrOffsets[4] = r2 * mmc3CHRBankSize
		m.chrOffsets[5] = r3 * mmc3CHRBankSize
		m.chrOffsets[6] = r4 * mmc3CHRBankSize
		m.chrOffsets[7] = r5 * mmc3CHRBankSize
		return
	}

	m.chrOffsets[0] = r2 * mmc3CHRBankSize
	m.chrOffsets[1] = r3 * mmc3CHRBankSize
	m.chrOffsets[2] = r4 * mmc3CHRBankSize
	m.chrOffsets[3] = r5 * mmc3CHRBankSize
	m.chrOffsets[4] = r0 * mmc3CHRBankSize
	m.chrOffsets[5] = (r0 + 1) * mmc3CHRBankSize
	m.chrOffsets[6] = r1 * mmc3CHRBankSize
	m.chrOffsets[7] = (r1 + 1) * mmc3CHRBankSize
}

// SaveState serializes the MMC3 internal register state.
func (m *Mapper4) SaveState(w io.Writer) error {
	header := [3]int32{
		int32(m.targetRegister & 0x7FFFFFFF),
		int32(m.prgMode & 0x7FFFFFFF),
		int32(m.chrInversion & 0x7FFFFFFF),
	}
	if err := binary.Write(w, binary.LittleEndian, header); err != nil {
		return fmt.Errorf("write mapper4 header: %w", err)
	}

	var regs [mmc3RegisterCount]int32
	for i, reg := range m.registers {
		regs[i] = int32(reg & 0x7FFFFFFF)
	}
	if err := binary.Write(w, binary.LittleEndian, regs); err != nil {
		return fmt.Errorf("write mapper4 registers: %w", err)
	}

	flags := byte(0)
	if m.irqReload {
		flags |= 0x01
	}
	if m.irqEnabled {
		flags |= 0x02
	}
	if m.irqPending {
		flags |= 0x04
	}
	if m.ramEnabled {
		flags |= 0x08
	}
	if m.ramWriteProtect {
		flags |= 0x10
	}

	mirror := byte(MirrorHorizontal)
	if m.cartridge != nil {
		mirror = byte(m.cartridge.Mirroring)
	}

	stateBytes := [4]byte{
		m.irqCounter,
		m.irqLatch,
		flags,
		mirror,
	}
	if err := binary.Write(w, binary.LittleEndian, stateBytes); err != nil {
		return fmt.Errorf("write mapper4 state bytes: %w", err)
	}

	return nil
}

// LoadState restores the MMC3 internal register state.
func (m *Mapper4) LoadState(r io.Reader) error {
	var header [3]int32
	if err := binary.Read(r, binary.LittleEndian, &header); err != nil {
		return fmt.Errorf("read mapper4 header: %w", err)
	}
	m.targetRegister = int(header[0])
	m.prgMode = int(header[1])
	m.chrInversion = int(header[2])

	var regs [mmc3RegisterCount]int32
	if err := binary.Read(r, binary.LittleEndian, &regs); err != nil {
		return fmt.Errorf("read mapper4 registers: %w", err)
	}
	for i, reg := range regs {
		m.registers[i] = int(reg)
	}

	var stateBytes [4]byte
	if err := binary.Read(r, binary.LittleEndian, &stateBytes); err != nil {
		return fmt.Errorf("read mapper4 state bytes: %w", err)
	}
	m.irqCounter = stateBytes[0]
	m.irqLatch = stateBytes[1]

	flags := stateBytes[2]
	m.irqReload = (flags & 0x01) != 0
	m.irqEnabled = (flags & 0x02) != 0
	m.irqPending = (flags & 0x04) != 0
	m.ramEnabled = (flags & 0x08) != 0
	m.ramWriteProtect = (flags & 0x10) != 0

	if m.cartridge != nil {
		m.cartridge.Mirroring = Mirroring(stateBytes[3])
	}
	m.updateOffsets()
	return nil
}

// Reset clears pending interrupt state on console reset.
func (m *Mapper4) Reset() {
	m.irqPending = false
}
