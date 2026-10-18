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
	uxromPRGBank16KB    = 0x4000
	uxromCHRBank8KB     = 0x2000
	uxromBankWindowMask = 0x3FFF
	uxromPRGWindowBase  = 0x8000
	uxromPRGWindowUpper = 0xC000
	uxromDefaultBank0   = 0
	uxromSinglePRGBank  = 1
)

// Mapper2 implements the Nintendo UxROM mapper (iNES Mapper 2, UNROM/UOROM).
// Supports games like Castlevania, Contra, Mega Man 1, DuckTales, Metal Gear.
type Mapper2 struct {
	UnbankedCHR
	cartridge   *Cartridge
	prgBanks16K int
	prgBank0    int // Switchable 16 KB PRG bank mapped at $8000-$BFFF
	prgBank1    int // Fixed 16 KB PRG bank mapped at $C000-$FFFF (last bank)
}

// NewMapper2 constructs and initializes a new UxROM mapper instance.
func NewMapper2(cart *Cartridge) *Mapper2 {
	if cart != nil {
		cart.IsCHRRAM = true
	}
	prgBanks := len(cart.PRG) / uxromPRGBank16KB
	if prgBanks == 0 {
		prgBanks = uxromSinglePRGBank
	}

	return &Mapper2{
		UnbankedCHR: NewUnbankedCHR(cart),
		cartridge:   cart,
		prgBanks16K: prgBanks,
		prgBank0:    uxromDefaultBank0,
		prgBank1:    prgBanks - 1,
	}
}

// ReadPRG maps CPU reads in the range $6000-$7FFF (SRAM) and $8000-$FFFF (PRG-ROM).
func (m *Mapper2) ReadPRG(addr uint16) byte {
	if addr >= SRAMBaseAddress && addr <= SRAMEndAddress {
		return m.cartridge.ReadSRAM(addr)
	}

	if addr < uxromPRGWindowBase {
		return m.cartridge.OpenBus
	}

	bank := m.prgBank0
	if addr >= uxromPRGWindowUpper {
		bank = m.prgBank1
	}

	offset := (bank * uxromPRGBank16KB) + int(addr&uxromBankWindowMask)
	if offset >= len(m.cartridge.PRG) {
		return m.cartridge.OpenBus
	}

	return m.cartridge.PRG[offset]
}

// WritePRG handles bank switching writes to $8000-$FFFF and optional PRG-RAM writes.
func (m *Mapper2) WritePRG(addr uint16, data byte) {
	if m.cartridge.WriteSRAM(addr, data) {
		return
	}

	if addr >= uxromPRGWindowBase {
		m.prgBank0 = int(data) % m.prgBanks16K
	}
}

// SaveState serializes UxROM internal register state.
func (m *Mapper2) SaveState(w io.Writer) error {
	if err := binary.Write(w, binary.LittleEndian, int32(m.prgBank0&0x7FFFFFFF)); err != nil {
		return fmt.Errorf("write mapper2 state: %w", err)
	}
	return nil
}

// LoadState restores UxROM internal register state.
func (m *Mapper2) LoadState(r io.Reader) error {
	var prgBank0 int32
	if err := binary.Read(r, binary.LittleEndian, &prgBank0); err != nil {
		return fmt.Errorf("read mapper2 state: %w", err)
	}
	m.prgBank0 = int(prgBank0)
	return nil
}
