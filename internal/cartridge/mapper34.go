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
	bnromPRGBankSize32KB = 0x8000 // 32 KB PRG bank size
	bnromCHRBankSize4KB  = 0x1000 // 4 KB CHR bank size
	bnromPRGWindowMask   = 0x7FFF // 32 KB offset mask ($8000-$FFFF)
	bnromCHRWindowMask   = 0x0FFF // 4 KB offset mask ($0000-$0FFF / $1000-$1FFF)
	bnromCHRBankSizeTotal = 0x2000 // Total 8 KB pattern table space

	bnromPRGBaseAddress = 0x8000
	bnromCHRWindow1Base = 0x1000

	addressNINAPRGBankSelect  = 0x7FFD // NINA-001 32 KB PRG bank select
	addressNINACHR0BankSelect = 0x7FFE // NINA-001 4 KB CHR bank 0 select ($0000-$0FFF)
	addressNINACHR1BankSelect = 0x7FFF // NINA-001 4 KB CHR bank 1 select ($1000-$1FFF)

	bnromPRGBankMask = 0x03 // BNROM uses 2 bits for 32 KB PRG bank
	ninaPRGBankMask  = 0x01 // NINA-001 uses 1 bit for 32 KB PRG bank
	ninaCHRBankMask  = 0x0F // NINA-001 uses 4 bits for 4 KB CHR banks

	minBNROMPRGBanks32K = 1
	minBNROMCHRBanks4K  = 2
)

// Mapper34 implements iNES Mapper 34 (BNROM and Nina-001).
// Supports Deadly Towers, Mashou, Impossible Mission II, and Chiller.
type Mapper34 struct {
	cartridge   *Cartridge
	prgBanks32K int
	chrBanks4K  int
	prgBank     int
	chrBank0    int
	chrBank1    int
}

// NewMapper34 constructs and initializes a new Mapper 34 instance.
func NewMapper34(cart *Cartridge) *Mapper34 {
	prgBanks := max(len(cart.PRG)/bnromPRGBankSize32KB, minBNROMPRGBanks32K)
	chrBanks := max(len(cart.CHR)/bnromCHRBankSize4KB, minBNROMCHRBanks4K)

	return &Mapper34{
		cartridge:   cart,
		prgBanks32K: prgBanks,
		chrBanks4K:  chrBanks,
		prgBank:     0,
		chrBank0:    0,
		chrBank1:    1,
	}
}

// ReadPRG maps CPU reads in the range $6000-$7FFF (SRAM) and $8000-$FFFF (PRG-ROM).
func (m *Mapper34) ReadPRG(addr uint16) byte {
	if addr >= SRAMBaseAddress && addr <= SRAMEndAddress {
		return m.cartridge.ReadSRAM(addr)
	}

	if addr < bnromPRGBaseAddress {
		return 0
	}

	offset := (m.prgBank * bnromPRGBankSize32KB) + int(addr&bnromPRGWindowMask)
	if offset >= len(m.cartridge.PRG) {
		return 0
	}

	return m.cartridge.PRG[offset]
}

// WritePRG handles BNROM bank switching in $8000-$FFFF, NINA-001 registers at $7FFD-$7FFF,
// and standard PRG-RAM writes in $6000-$7FFF.
func (m *Mapper34) WritePRG(addr uint16, data byte) {
	if m.cartridge.WriteSRAM(addr, data) {
		switch addr {
		case addressNINAPRGBankSelect:
			m.prgBank = int(data&ninaPRGBankMask) % m.prgBanks32K
		case addressNINACHR0BankSelect:
			m.chrBank0 = int(data&ninaCHRBankMask) % m.chrBanks4K
		case addressNINACHR1BankSelect:
			m.chrBank1 = int(data&ninaCHRBankMask) % m.chrBanks4K
		}
		return
	}

	if addr < bnromPRGBaseAddress {
		return
	}

	m.prgBank = int(data&bnromPRGBankMask) % m.prgBanks32K
}

// ReadCHR maps PPU reads in $0000-$1FFF to the active 4 KB CHR banks.
func (m *Mapper34) ReadCHR(addr uint16) byte {
	if addr >= bnromCHRBankSizeTotal {
		return 0
	}

	bank := m.chrBank0
	if addr >= bnromCHRWindow1Base {
		bank = m.chrBank1
	}

	offset := (bank * bnromCHRBankSize4KB) + int(addr&bnromCHRWindowMask)
	if offset >= len(m.cartridge.CHR) {
		return 0
	}

	return m.cartridge.CHR[offset]
}

// WriteCHR maps PPU writes to CHR-RAM in $0000-$1FFF.
func (m *Mapper34) WriteCHR(addr uint16, data byte) {
	if addr >= bnromCHRBankSizeTotal {
		return
	}

	bank := m.chrBank0
	if addr >= bnromCHRWindow1Base {
		bank = m.chrBank1
	}

	offset := (bank * bnromCHRBankSize4KB) + int(addr&bnromCHRWindowMask)
	if offset >= len(m.cartridge.CHR) {
		return
	}

	m.cartridge.CHR[offset] = data
}

// SaveState serializes Mapper 34 (BNROM / NINA-001) internal register state.
func (m *Mapper34) SaveState(w io.Writer) error {
	state := struct {
		PRGBank  int32
		CHRBank0 int32
		CHRBank1 int32
	}{
		PRGBank:  int32(m.prgBank & 0x7FFFFFFF),
		CHRBank0: int32(m.chrBank0 & 0x7FFFFFFF),
		CHRBank1: int32(m.chrBank1 & 0x7FFFFFFF),
	}
	if err := binary.Write(w, binary.LittleEndian, state); err != nil {
		return fmt.Errorf("write mapper34 state: %w", err)
	}
	return nil
}

// LoadState restores Mapper 34 (BNROM / NINA-001) internal register state.
func (m *Mapper34) LoadState(r io.Reader) error {
	var state struct {
		PRGBank  int32
		CHRBank0 int32
		CHRBank1 int32
	}
	if err := binary.Read(r, binary.LittleEndian, &state); err != nil {
		return fmt.Errorf("read mapper34 state: %w", err)
	}
	m.prgBank = int(state.PRGBank)
	m.chrBank0 = int(state.CHRBank0)
	m.chrBank1 = int(state.CHRBank1)
	return nil
}
