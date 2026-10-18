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
	cnromPRGBankSize       = 0x4000 // 16 KB PRG bank size
	cnromPRGBankMirrorMask = 0x3FFF // Mask for 16 KB PRG-ROM mirroring ($C000-$FFFF mirrors $8000-$BFFF on CNROM-128)
	cnromCHRBankSize       = 0x2000 // 8 KB CHR bank size
	cnromCHRBankWindowMask = 0x1FFF // Mask for 8 KB window ($0000-$1FFF)
	cnromPRGWindowBase     = 0x8000 // Starting address of PRG space
	cnromSinglePRGBank     = 1
	cnromSingleCHRBank     = 1
	cnromDefaultCHRBank    = 0
)

// Mapper3 implements the Nintendo CNROM mapper (iNES Mapper 3).
// Supports games like Solomon's Key, Arkanoid, Adventure Island, Gradius (FC), Spy Hunter, Paperboy.
// CNROM features 16 KB or 32 KB PRG-ROM (identical to NROM) and selectable 8 KB CHR-ROM banks.
type Mapper3 struct {
	cartridge *Cartridge
	prgBanks  int
	chrBanks  int
	chrBank   int // Currently selected 8 KB CHR bank
}

// NewMapper3 creates and initializes a new Mapper3 instance.
func NewMapper3(cart *Cartridge) *Mapper3 {
	prgBanks := len(cart.PRG) / cnromPRGBankSize
	if prgBanks == 0 {
		prgBanks = cnromSinglePRGBank
	}

	chrBanks := len(cart.CHR) / cnromCHRBankSize
	if chrBanks == 0 {
		chrBanks = cnromSingleCHRBank
	}

	return &Mapper3{
		cartridge: cart,
		prgBanks:  prgBanks,
		chrBanks:  chrBanks,
		chrBank:   cnromDefaultCHRBank,
	}
}

// ReadPRG maps CPU reads in the range $6000-$7FFF (PRG-RAM/SRAM) and $8000-$FFFF (PRG-ROM).
func (m *Mapper3) ReadPRG(addr uint16) byte {
	if addr >= SRAMBaseAddress && addr <= SRAMEndAddress {
		return m.cartridge.ReadSRAM(addr)
	}

	if addr < cnromPRGWindowBase {
		return m.cartridge.OpenBus
	}

	offset := uint32(addr - cnromPRGWindowBase)
	if m.prgBanks == cnromSinglePRGBank {
		// Mirror 16 KB PRG-ROM for CNROM-128 ($C000-$FFFF mirrors $8000-$BFFF).
		offset &= cnromPRGBankMirrorMask
	}

	if int(offset) >= len(m.cartridge.PRG) {
		return m.cartridge.OpenBus
	}

	return m.cartridge.PRG[offset]
}

// WritePRG handles CPU writes to PRG memory and the bank select latch.
// $6000-$7FFF: writes to cartridge SRAM.
// $8000-$FFFF: selects the active 8 KB CHR bank via the 74HC161 latch.
func (m *Mapper3) WritePRG(addr uint16, data byte) {
	if m.cartridge.WriteSRAM(addr, data) {
		return
	}

	if addr >= cnromPRGWindowBase {
		m.chrBank = int(data) % m.chrBanks
	}
}

// ReadCHR maps PPU reads in the range $0000-$1FFF to the active 8 KB CHR bank.
func (m *Mapper3) ReadCHR(addr uint16) byte {
	if addr >= cnromCHRBankSize {
		return 0
	}

	offset := (m.chrBank * cnromCHRBankSize) + int(addr&cnromCHRBankWindowMask)
	if offset >= len(m.cartridge.CHR) {
		return 0
	}

	return m.cartridge.CHR[offset]
}

// WriteCHR maps PPU writes to CHR-RAM in the range $0000-$1FFF if the cartridge uses RAM.
func (m *Mapper3) WriteCHR(addr uint16, data byte) {
	if addr >= cnromCHRBankSize {
		return
	}

	offset := (m.chrBank * cnromCHRBankSize) + int(addr&cnromCHRBankWindowMask)
	if offset >= len(m.cartridge.CHR) {
		return
	}

	m.cartridge.CHR[offset] = data
}

// SaveState serializes CNROM internal register state.
func (m *Mapper3) SaveState(w io.Writer) error {
	if err := binary.Write(w, binary.LittleEndian, int32(m.chrBank&0x7FFFFFFF)); err != nil {
		return fmt.Errorf("write mapper3 state: %w", err)
	}
	return nil
}

// LoadState restores CNROM internal register state.
func (m *Mapper3) LoadState(r io.Reader) error {
	var chrBank int32
	if err := binary.Read(r, binary.LittleEndian, &chrBank); err != nil {
		return fmt.Errorf("read mapper3 state: %w", err)
	}
	m.chrBank = int(chrBank)
	return nil
}
