// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge

const (
	// prgRomBaseAddress is the starting address of PRG-ROM in the 6502 CPU memory map.
	prgRomBaseAddress = 0x8000
	// prgBankSize is the size of a single 16 KB PRG bank.
	prgBankSize = 0x4000
	// prgBankMirrorMask is used to mirror the lower 16 KB PRG bank across $C000-$FFFF on NROM-128.
	prgBankMirrorMask = 0x3FFF
	// singlePRGBank represents a single 16 KB PRG bank configuration (NROM-128).
	singlePRGBank = 1
)

// Mapper0 implements the NROM mapper (iNES Mapper 0).
// NROM supports 16 KB (NROM-128) or 32 KB (NROM-256) PRG-ROM and 8 KB CHR-ROM/RAM.
type Mapper0 struct {
	UnbankedCHR
	cartridge *Cartridge
	prgBanks  int
}

// NewMapper0 creates and initializes a new Mapper0 instance.
func NewMapper0(cart *Cartridge) *Mapper0 {
	banks := len(cart.PRG) / prgBankSize
	if banks == 0 {
		banks = singlePRGBank
	}
	return &Mapper0{
		UnbankedCHR: NewUnbankedCHR(cart),
		cartridge:   cart,
		prgBanks:    banks,
	}
}

// ReadPRG maps CPU read addresses in the range $6000-$7FFF (PRG-RAM) and $8000-$FFFF (PRG-ROM).
func (m *Mapper0) ReadPRG(addr uint16) byte {
	if addr >= SRAMBaseAddress && addr <= SRAMEndAddress {
		return m.cartridge.ReadSRAM(addr)
	}

	if addr < prgRomBaseAddress {
		return m.cartridge.OpenBus
	}

	offset := uint32(addr - prgRomBaseAddress)
	if m.prgBanks == singlePRGBank {
		// Mirror 16 KB PRG-ROM for NROM-128 ($C000-$FFFF mirrors $8000-$BFFF).
		offset &= prgBankMirrorMask
	}

	if int(offset) >= len(m.cartridge.PRG) {
		return m.cartridge.OpenBus
	}

	return m.cartridge.PRG[offset]
}

// WritePRG handles CPU writes to PRG memory. $6000-$7FFF writes to PRG-RAM (SRAM).
func (m *Mapper0) WritePRG(addr uint16, data byte) {
	m.cartridge.WriteSRAM(addr, data)
}
