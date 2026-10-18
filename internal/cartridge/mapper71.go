// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cartridge implements NES cartridge loading, iNES header parsing, and memory mappers.
package cartridge

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	m71PRGBankSize16K   = 0x4000
	m71CHRBankSize8K    = 0x2000
	m71BankWindowMask   = 0x3FFF
	m71PRGWindowBase    = 0x8000
	m71PRGWindowUpper   = 0xC000
	m71RegMirroringBase = 0x9000
	m71RegMirroringEnd  = 0x9FFF
	m71RegPRGBankBase   = 0xC000
	m71RegPRGBankEnd    = 0xFFFF

	m71MirrorBitMask    = 0x10
	m71PRGBankMask4Bit  = 0x0F
	m71DefaultBank0     = 0
	m71SinglePRGBank    = 1
)

// Mapper71 implements the Camerica / Codemasters ASIC mapper (iNES Mapper 71).
// Supports games like Micro Machines, Fire Hawk, Dizzy the Adventurer, The Fantastic Adventures of Dizzy,
// Bee 52, Big Nose the Caveman, Linus Spacehead, and Mig-29 Soviet Fighter.
type Mapper71 struct {
	UnbankedCHR
	cartridge   *Cartridge
	prgBanks16K int
	prgBank0    int // Switchable 16 KB PRG bank mapped at $8000-$BFFF
	prgBank1    int // Fixed 16 KB PRG bank mapped at $C000-$FFFF (last bank)
}

// NewMapper71 constructs and initializes a new Mapper71 (Camerica / Codemasters) instance.
func NewMapper71(cart *Cartridge) *Mapper71 {
	if cart != nil {
		cart.IsCHRRAM = true
	}
	prgBanks := len(cart.PRG) / m71PRGBankSize16K
	if prgBanks == 0 {
		prgBanks = m71SinglePRGBank
	}

	return &Mapper71{
		UnbankedCHR: NewUnbankedCHR(cart),
		cartridge:   cart,
		prgBanks16K: prgBanks,
		prgBank0:    m71DefaultBank0,
		prgBank1:    prgBanks - 1,
	}
}

// ReadPRG maps CPU reads in the range $6000-$7FFF (SRAM) and $8000-$FFFF (PRG-ROM).
func (m *Mapper71) ReadPRG(addr uint16) byte {
	if addr >= SRAMBaseAddress && addr <= SRAMEndAddress {
		return m.cartridge.ReadSRAM(addr)
	}

	if addr < m71PRGWindowBase {
		return m.cartridge.OpenBus
	}

	bank := m.prgBank0
	if addr >= m71PRGWindowUpper {
		bank = m.prgBank1
	}

	offset := (bank * m71PRGBankSize16K) + int(addr&m71BankWindowMask)
	if offset >= len(m.cartridge.PRG) {
		return m.cartridge.OpenBus
	}

	return m.cartridge.PRG[offset]
}

// WritePRG maps CPU writes:
// - $6000-$7FFF: Standard PRG-RAM writes if present.
// - $9000-$9FFF: Single-screen mirroring select (bit 4: 0 = Page 0, 1 = Page 1) for games like Fire Hawk.
// - $C000-$FFFF: Selects 16 KB switchable PRG-ROM bank at $8000-$BFFF.
func (m *Mapper71) WritePRG(addr uint16, data byte) {
	if m.cartridge.WriteSRAM(addr, data) {
		return
	}

	switch {
	case addr >= m71RegMirroringBase && addr <= m71RegMirroringEnd:
		m.cartridge.Mirroring = MirrorSingle0
		if (data & m71MirrorBitMask) != 0 {
			m.cartridge.Mirroring = MirrorSingle1
		}

	case addr >= m71RegPRGBankBase && addr <= m71RegPRGBankEnd:
		bank := int(data & m71PRGBankMask4Bit)
		m.prgBank0 = bank % m.prgBanks16K
	}
}

// SaveState serializes the mapper's internal register state.
func (m *Mapper71) SaveState(w io.Writer) error {
	if err := binary.Write(w, binary.LittleEndian, int32(m.prgBank0&0xFFFF)); err != nil {
		return fmt.Errorf("write mapper71 prgBank0: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, byte(m.cartridge.Mirroring)); err != nil {
		return fmt.Errorf("write mapper71 mirror: %w", err)
	}
	return nil
}

// LoadState restores the mapper's internal register state.
func (m *Mapper71) LoadState(r io.Reader) error {
	var bank0 int32
	if err := binary.Read(r, binary.LittleEndian, &bank0); err != nil {
		return fmt.Errorf("read mapper71 prgBank0: %w", err)
	}
	var mirror byte
	if err := binary.Read(r, binary.LittleEndian, &mirror); err != nil {
		return fmt.Errorf("read mapper71 mirror: %w", err)
	}

	m.prgBank0 = int(bank0) % m.prgBanks16K
	m.cartridge.Mirroring = Mirroring(mirror)
	return nil
}
