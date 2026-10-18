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
	gxromPRGBankSize32KB = 0x8000 // 32 KB PRG bank size
	gxromCHRBankSize8KB  = 0x2000 // 8 KB CHR bank size
	gxromPRGWindowMask   = 0x7FFF // 32 KB offset mask ($8000-$FFFF)
	gxromCHRWindowMask   = 0x1FFF // 8 KB offset mask ($0000-$1FFF)

	gxromPRGBaseAddress = 0x8000 // Base address for switchable PRG window
	gxromCHRBankSizeTotal = 0x2000 // Total 8 KB pattern table space

	gxromPRGBankShift = 4
	gxromPRGBankMask  = 0x30 // Bits 4-5 select 32 KB PRG bank
	gxromCHRBankMask  = 0x03 // Bits 0-1 select 8 KB CHR bank

	minGxROMPRGBanks32K = 1
	minGxROMCHRBanks8K  = 1
)

// Mapper66 implements iNES Mapper 66 (GxROM / MHROM / GNROM).
// Used by Super Mario Bros. / Duck Hunt, Doraemon, Dragon Power, and Gumshoe.
type Mapper66 struct {
	cartridge   *Cartridge
	prgBanks32K int
	chrBanks8K  int
	prgBank     int
	chrBank     int
}

// NewMapper66 constructs and initializes a new GxROM mapper instance.
func NewMapper66(cart *Cartridge) *Mapper66 {
	prgBanks := max(len(cart.PRG)/gxromPRGBankSize32KB, minGxROMPRGBanks32K)
	chrBanks := max(len(cart.CHR)/gxromCHRBankSize8KB, minGxROMCHRBanks8K)

	return &Mapper66{
		cartridge:   cart,
		prgBanks32K: prgBanks,
		chrBanks8K:  chrBanks,
		prgBank:     0,
		chrBank:     0,
	}
}

// ReadPRG maps CPU reads in the range $6000-$7FFF (SRAM) and $8000-$FFFF (PRG-ROM).
func (m *Mapper66) ReadPRG(addr uint16) byte {
	if addr >= SRAMBaseAddress && addr <= SRAMEndAddress {
		return m.cartridge.ReadSRAM(addr)
	}

	if addr < gxromPRGBaseAddress {
		return m.cartridge.OpenBus
	}

	offset := (m.prgBank * gxromPRGBankSize32KB) + int(addr&gxromPRGWindowMask)
	if offset >= len(m.cartridge.PRG) {
		return m.cartridge.OpenBus
	}

	return m.cartridge.PRG[offset]
}

// WritePRG handles bank switching writes to $8000-$FFFF and PRG-RAM writes to $6000-$7FFF.
func (m *Mapper66) WritePRG(addr uint16, data byte) {
	if m.cartridge.WriteSRAM(addr, data) {
		return
	}

	if addr < gxromPRGBaseAddress {
		return
	}

	m.prgBank = int((data&gxromPRGBankMask)>>gxromPRGBankShift) % m.prgBanks32K
	m.chrBank = int(data&gxromCHRBankMask) % m.chrBanks8K
}

// ReadCHR maps PPU reads to the currently selected 8 KB CHR-ROM bank.
func (m *Mapper66) ReadCHR(addr uint16) byte {
	if addr >= gxromCHRBankSizeTotal {
		return 0
	}

	offset := (m.chrBank * gxromCHRBankSize8KB) + int(addr&gxromCHRWindowMask)
	if offset >= len(m.cartridge.CHR) {
		return 0
	}

	return m.cartridge.CHR[offset]
}

// WriteCHR maps PPU writes to CHR-RAM if the cartridge has writable CHR memory.
func (m *Mapper66) WriteCHR(addr uint16, data byte) {
	if addr >= gxromCHRBankSizeTotal {
		return
	}

	offset := (m.chrBank * gxromCHRBankSize8KB) + int(addr&gxromCHRWindowMask)
	if offset >= len(m.cartridge.CHR) {
		return
	}

	m.cartridge.CHR[offset] = data
}

// SaveState serializes GxROM internal register state.
func (m *Mapper66) SaveState(w io.Writer) error {
	state := struct {
		PRGBank int32
		CHRBank int32
	}{
		PRGBank: int32(m.prgBank & 0x7FFFFFFF),
		CHRBank: int32(m.chrBank & 0x7FFFFFFF),
	}
	if err := binary.Write(w, binary.LittleEndian, state); err != nil {
		return fmt.Errorf("write mapper66 state: %w", err)
	}
	return nil
}

// LoadState restores GxROM internal register state.
func (m *Mapper66) LoadState(r io.Reader) error {
	var state struct {
		PRGBank int32
		CHRBank int32
	}
	if err := binary.Read(r, binary.LittleEndian, &state); err != nil {
		return fmt.Errorf("read mapper66 state: %w", err)
	}
	m.prgBank = int(state.PRGBank)
	m.chrBank = int(state.CHRBank)
	return nil
}
