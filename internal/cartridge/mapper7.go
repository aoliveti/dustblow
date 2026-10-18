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
	axromPRGBank32KB     = 0x8000 // 32 KB PRG bank size
	axromCHRBank8KB      = 0x2000 // 8 KB CHR bank size
	axromBankWindowMask  = 0x7FFF // Offset mask within 32 KB bank
	axromPRGWindowBase   = 0x8000 // Starting address of PRG-ROM window
	axromDefaultBank     = 0      // Initial bank on reset
	axromSinglePRGBank   = 1      // Minimal bank count fallback
	axromBankSelectMask  = 0x0F   // Bits 0-3 select 32 KB PRG bank (supports up to 512 KB)
	axromMirrorSelectBit = 0x10   // Bit 4 selects single-screen nametable page (0: Page 0, 1: Page 1)
)

// Mapper7 implements the Nintendo AxROM mapper (iNES Mapper 7: AOROM, AMROM, ANROM).
// Supports games such as Battletoads, Marble Madness, Cabal, and Time Lord.
type Mapper7 struct {
	UnbankedCHR
	cartridge   *Cartridge
	prgBanks32K int
	prgBank     int
}

// NewMapper7 constructs and initializes a new AxROM mapper instance.
func NewMapper7(cart *Cartridge) *Mapper7 {
	if cart != nil {
		cart.IsCHRRAM = true
		cart.Mirroring = MirrorSingle0
	}
	prgBanks := len(cart.PRG) / axromPRGBank32KB
	if prgBanks == 0 {
		prgBanks = axromSinglePRGBank
	}

	return &Mapper7{
		UnbankedCHR: NewUnbankedCHR(cart),
		cartridge:   cart,
		prgBanks32K: prgBanks,
		prgBank:     axromDefaultBank,
	}
}

// ReadPRG maps CPU reads in the range $6000-$7FFF (optional SRAM) and $8000-$FFFF (32 KB switchable PRG-ROM).
func (m *Mapper7) ReadPRG(addr uint16) byte {
	if addr >= SRAMBaseAddress && addr <= SRAMEndAddress {
		return m.cartridge.ReadSRAM(addr)
	}

	if addr < axromPRGWindowBase {
		return m.cartridge.OpenBus
	}

	offset := (m.prgBank * axromPRGBank32KB) + int(addr&axromBankWindowMask)
	if offset >= len(m.cartridge.PRG) {
		return m.cartridge.OpenBus
	}

	return m.cartridge.PRG[offset]
}

// WritePRG handles bank switching and single-screen mirroring selection writes to $8000-$FFFF.
func (m *Mapper7) WritePRG(addr uint16, data byte) {
	if m.cartridge.WriteSRAM(addr, data) {
		return
	}

	if addr < axromPRGWindowBase {
		return
	}

	m.prgBank = int(data&axromBankSelectMask) % m.prgBanks32K

	if (data & axromMirrorSelectBit) == 0 {
		m.cartridge.Mirroring = MirrorSingle0
		return
	}
	m.cartridge.Mirroring = MirrorSingle1
}

// SaveState serializes AxROM internal register state.
func (m *Mapper7) SaveState(w io.Writer) error {
	mirror := byte(MirrorSingle0)
	if m.cartridge != nil {
		mirror = byte(m.cartridge.Mirroring)
	}
	state := struct {
		PRGBank int32
		Mirror  byte
	}{
		PRGBank: int32(m.prgBank & 0x7FFFFFFF),
		Mirror:  mirror,
	}
	if err := binary.Write(w, binary.LittleEndian, state); err != nil {
		return fmt.Errorf("write mapper7 state: %w", err)
	}
	return nil
}

// LoadState restores AxROM internal register state.
func (m *Mapper7) LoadState(r io.Reader) error {
	var state struct {
		PRGBank int32
		Mirror  byte
	}
	if err := binary.Read(r, binary.LittleEndian, &state); err != nil {
		return fmt.Errorf("read mapper7 state: %w", err)
	}
	m.prgBank = int(state.PRGBank)
	if m.cartridge != nil {
		m.cartridge.Mirroring = Mirroring(state.Mirror)
	}
	return nil
}
