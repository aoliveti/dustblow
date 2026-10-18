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
	mmc1ShiftResetMask     byte   = 0x80
	mmc1DataBitMask        byte   = 0x01
	mmc1ShiftCompleteCount byte   = 5
	mmc1DefaultControl     byte   = 0x0C
	mmc1RegisterSelectMask uint16 = 0x03
	mmc1RegisterShift      uint16 = 13

	// Register 0: Control ($8000-$9FFF) bit masks
	mmc1MirroringMask byte = 0x03
	mmc1PRGModeMask   byte = 0x0C
	mmc1PRGModeShift       = 2
	mmc1CHRModeMask  byte = 0x10
	mmc1CHRModeShift      = 4

	// PRG ROM banking modes
	mmc1PRGMode32KB       byte = 0
	mmc1PRGMode32KBAlt    byte = 1
	mmc1PRGModeFixedFirst byte = 2
	mmc1PRGModeFixedLast  byte = 3

	// CHR banking modes
	mmc1CHRMode8KB byte = 0
	mmc1CHRMode4KB byte = 1

	// Mirroring modes from control register
	mmc1MirrorOneScreenLower byte = 0
	mmc1MirrorOneScreenUpper byte = 1
	mmc1MirrorVertical       byte = 2
	mmc1MirrorHorizontal     byte = 3

	// Memory geometry
	mmc1PRGBank16KB    = 0x4000
	mmc1CHRBank4KB     = 0x1000
	mmc1BankWindowMask = 0x3FFF
	mmc1CHRWindowMask  = 0x0FFF
	mmc1PRGWindowBase  = 0x8000
	mmc1PRGWindowMid   = 0xC000
	mmc1CHRWindowMid   = 0x1000
)

// Mapper1 implements the Nintendo MMC1 mapper (iNES Mapper 1, SxROM).
// Supports games like The Legend of Zelda, Metroid, Mega Man 2, Kid Icarus.
type Mapper1 struct {
	cartridge     *Cartridge
	prgBanks16K   int
	chrBanks4K    int
	shiftRegister byte
	shiftCount    byte
	control       byte
	chrBank0      byte
	chrBank1      byte
	prgBank       byte
	prgOffsets    [2]int
	chrOffsets    [2]int
}

// NewMapper1 constructs and initializes an MMC1 mapper instance.
func NewMapper1(cart *Cartridge) *Mapper1 {
	prgBanks := len(cart.PRG) / mmc1PRGBank16KB
	if prgBanks == 0 {
		prgBanks = 1
	}

	chrBanks := len(cart.CHR) / mmc1CHRBank4KB
	if chrBanks == 0 {
		chrBanks = 1
	}

	m := &Mapper1{
		cartridge:   cart,
		prgBanks16K: prgBanks,
		chrBanks4K:  chrBanks,
		control:     mmc1DefaultControl,
	}

	m.updateOffsets()
	return m
}

// ReadPRG maps CPU reads to PRG-RAM ($6000-$7FFF) or banked PRG-ROM ($8000-$FFFF).
func (m *Mapper1) ReadPRG(addr uint16) byte {
	if addr >= SRAMBaseAddress && addr <= SRAMEndAddress {
		return m.cartridge.ReadSRAM(addr)
	}

	if addr < mmc1PRGWindowBase {
		return 0
	}

	bankIndex := 0
	if addr >= mmc1PRGWindowMid {
		bankIndex = 1
	}

	offset := m.prgOffsets[bankIndex] + int(addr&mmc1BankWindowMask)
	if offset >= len(m.cartridge.PRG) {
		return 0
	}

	return m.cartridge.PRG[offset]
}

// WritePRG handles CPU writes to PRG-RAM ($6000-$7FFF) or the MMC1 serial shift register ($8000-$FFFF).
func (m *Mapper1) WritePRG(addr uint16, data byte) {
	if m.cartridge.WriteSRAM(addr, data) {
		return
	}

	if addr < mmc1PRGWindowBase {
		return
	}

	if (data & mmc1ShiftResetMask) != 0 {
		m.shiftRegister = 0
		m.shiftCount = 0
		m.control |= mmc1DefaultControl
		m.updateOffsets()
		return
	}

	m.shiftRegister |= (data & mmc1DataBitMask) << m.shiftCount
	m.shiftCount++

	if m.shiftCount < mmc1ShiftCompleteCount {
		return
	}

	reg := byte((addr >> mmc1RegisterShift) & mmc1RegisterSelectMask)
	m.applyRegisterWrite(reg, m.shiftRegister)
	m.shiftRegister = 0
	m.shiftCount = 0
	m.updateOffsets()
}

func (m *Mapper1) applyRegisterWrite(reg, value byte) {
	switch reg {
	case 0:
		m.control = value
		m.updateMirroring()
	case 1:
		m.chrBank0 = value
	case 2:
		m.chrBank1 = value
	case 3:
		m.prgBank = value & 0x0F
	}
}

func (m *Mapper1) updateMirroring() {
	switch m.control & mmc1MirroringMask {
	case mmc1MirrorOneScreenLower:
		m.cartridge.Mirroring = MirrorSingle0
	case mmc1MirrorOneScreenUpper:
		m.cartridge.Mirroring = MirrorSingle1
	case mmc1MirrorVertical:
		m.cartridge.Mirroring = MirrorVertical
	case mmc1MirrorHorizontal:
		m.cartridge.Mirroring = MirrorHorizontal
	}
}

func (m *Mapper1) updateOffsets() {
	m.updatePRGOffsets()
	m.updateCHROffsets()
}

func (m *Mapper1) updatePRGOffsets() {
	mode := (m.control & mmc1PRGModeMask) >> mmc1PRGModeShift
	switch mode {
	case mmc1PRGMode32KB, mmc1PRGMode32KBAlt:
		bank := int(m.prgBank & 0x0E) % m.prgBanks16K
		m.prgOffsets[0] = bank * mmc1PRGBank16KB
		m.prgOffsets[1] = (bank + 1) * mmc1PRGBank16KB
	case mmc1PRGModeFixedFirst:
		m.prgOffsets[0] = 0
		m.prgOffsets[1] = (int(m.prgBank) % m.prgBanks16K) * mmc1PRGBank16KB
	case mmc1PRGModeFixedLast:
		m.prgOffsets[0] = (int(m.prgBank) % m.prgBanks16K) * mmc1PRGBank16KB
		m.prgOffsets[1] = (m.prgBanks16K - 1) * mmc1PRGBank16KB
	}
}

func (m *Mapper1) updateCHROffsets() {
	mode := (m.control & mmc1CHRModeMask) >> mmc1CHRModeShift
	switch mode {
	case mmc1CHRMode8KB:
		bank := int(m.chrBank0&0x1E) % m.chrBanks4K
		m.chrOffsets[0] = bank * mmc1CHRBank4KB
		m.chrOffsets[1] = (bank + 1) * mmc1CHRBank4KB
	case mmc1CHRMode4KB:
		m.chrOffsets[0] = (int(m.chrBank0) % m.chrBanks4K) * mmc1CHRBank4KB
		m.chrOffsets[1] = (int(m.chrBank1) % m.chrBanks4K) * mmc1CHRBank4KB
	}
}

// ReadCHR maps PPU reads to the active CHR-ROM/RAM banks ($0000-$1FFF).
func (m *Mapper1) ReadCHR(addr uint16) byte {
	if addr >= 0x2000 {
		return 0
	}

	bankIndex := 0
	if addr >= mmc1CHRWindowMid {
		bankIndex = 1
	}

	offset := m.chrOffsets[bankIndex] + int(addr&mmc1CHRWindowMask)
	if offset >= len(m.cartridge.CHR) {
		return 0
	}

	return m.cartridge.CHR[offset]
}

// WriteCHR maps PPU writes to CHR-RAM ($0000-$1FFF).
func (m *Mapper1) WriteCHR(addr uint16, data byte) {
	if addr >= 0x2000 {
		return
	}

	bankIndex := 0
	if addr >= mmc1CHRWindowMid {
		bankIndex = 1
	}

	offset := m.chrOffsets[bankIndex] + int(addr&mmc1CHRWindowMask)
	if offset >= len(m.cartridge.CHR) {
		return
	}

	m.cartridge.CHR[offset] = data
}

// SaveState serializes the MMC1 internal register state.
func (m *Mapper1) SaveState(w io.Writer) error {
	mirror := byte(MirrorHorizontal)
	if m.cartridge != nil {
		mirror = byte(m.cartridge.Mirroring)
	}
	state := [7]byte{
		m.shiftRegister,
		m.shiftCount,
		m.control,
		m.chrBank0,
		m.chrBank1,
		m.prgBank,
		mirror,
	}
	if err := binary.Write(w, binary.LittleEndian, state); err != nil {
		return fmt.Errorf("write mapper1 state: %w", err)
	}
	return nil
}

// LoadState restores the MMC1 internal register state.
func (m *Mapper1) LoadState(r io.Reader) error {
	var state [7]byte
	if err := binary.Read(r, binary.LittleEndian, &state); err != nil {
		return fmt.Errorf("read mapper1 state: %w", err)
	}
	m.shiftRegister = state[0]
	m.shiftCount = state[1]
	m.control = state[2]
	m.chrBank0 = state[3]
	m.chrBank1 = state[4]
	m.prgBank = state[5]
	if m.cartridge != nil {
		m.cartridge.Mirroring = Mirroring(state[6])
	}
	m.updateOffsets()
	return nil
}
