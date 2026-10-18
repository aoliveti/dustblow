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
	mmc4PRGBankSize16KB  = 0x4000 // 16 KB PRG bank size
	mmc4CHRBankSize4KB   = 0x1000 // 4 KB CHR bank size
	mmc4PRGWindowMask    = 0x3FFF // 16 KB offset mask
	mmc4CHRWindowMask    = 0x0FFF // 4 KB offset mask
	mmc4CHRBankSizeTotal = 0x2000 // Total 8 KB pattern table space ($0000-$1FFF)

	mmc4PRGWindow0Base = 0x8000 // $8000-$BFFF: switchable PRG bank
	mmc4PRGWindow1Base = 0xC000 // $C000-$FFFF: fixed to last 16 KB bank (prgBanks - 1)

	mmc4RegPRGBankSelect = 0xA000 // Select 16 KB PRG bank ($8000-$BFFF)
	mmc4RegCHR0BankFD    = 0xB000 // Select 4 KB CHR bank 0 for latch $FD
	mmc4RegCHR0BankFE    = 0xC000 // Select 4 KB CHR bank 0 for latch $FE
	mmc4RegCHR1BankFD    = 0xD000 // Select 4 KB CHR bank 1 for latch $FD
	mmc4RegCHR1BankFE    = 0xE000 // Select 4 KB CHR bank 1 for latch $FE
	mmc4RegMirroring     = 0xF000 // Mirroring control register
	mmc4RegMask          = 0xF000 // Register selection mask ($A000, $B000, ..., $F000)

	mmc4PRGBankMask   = 0x0F // 4 bits of PRG bank select
	mmc4CHRBankMask   = 0x1F // 5 bits of CHR bank select
	mmc4MirroringMask = 0x01 // Bit 0 selects mirroring (0: Vertical, 1: Horizontal)

	mmc4LatchFD = 0xFD
	mmc4LatchFE = 0xFE

	mmc4Trigger0FDStart = 0x0FD0
	mmc4Trigger0FDEnd   = 0x0FDF
	mmc4Trigger0FEStart = 0x0FE0
	mmc4Trigger0FEEnd   = 0x0FEF
	mmc4Trigger1FDStart = 0x1FD0
	mmc4Trigger1FDEnd   = 0x1FDF
	mmc4Trigger1FEStart = 0x1FE0
	mmc4Trigger1FEEnd   = 0x1FEF

	minMMC4PRGBanks16K = 2
	minMMC4CHRBanks4K  = 1
)

// Mapper10 implements the Nintendo MMC4 mapper (iNES Mapper 10: FxROM, FJROM, FKROM).
// Supports Fire Emblem: Shadow Dragon and the Blade of Light, Fire Emblem Gaiden, and Famicom Wars.
type Mapper10 struct {
	cartridge   *Cartridge
	prgBanks16K int
	chrBanks4K  int
	prgBank     int
	chrBank0FD  int
	chrBank0FE  int
	chrBank1FD  int
	chrBank1FE  int
	latch0      byte
	latch1      byte
}

// NewMapper10 constructs and initializes a new MMC4 mapper instance.
func NewMapper10(cart *Cartridge) *Mapper10 {
	prgBanks := max(len(cart.PRG)/mmc4PRGBankSize16KB, minMMC4PRGBanks16K)
	chrBanks := max(len(cart.CHR)/mmc4CHRBankSize4KB, minMMC4CHRBanks4K)

	return &Mapper10{
		cartridge:   cart,
		prgBanks16K: prgBanks,
		chrBanks4K:  chrBanks,
		prgBank:     0,
		chrBank0FD:  0,
		chrBank0FE:  0,
		chrBank1FD:  0,
		chrBank1FE:  0,
		latch0:      mmc4LatchFE,
		latch1:      mmc4LatchFE,
	}
}

// ReadPRG maps CPU reads in the range $6000-$7FFF (SRAM) and $8000-$FFFF (PRG-ROM).
func (m *Mapper10) ReadPRG(addr uint16) byte {
	if addr >= SRAMBaseAddress && addr <= SRAMEndAddress {
		return m.cartridge.ReadSRAM(addr)
	}

	if addr < mmc4PRGWindow0Base {
		return 0
	}

	bank := m.prgBank
	if addr >= mmc4PRGWindow1Base {
		bank = m.prgBanks16K - 1
	}

	offset := (bank * mmc4PRGBankSize16KB) + int(addr&mmc4PRGWindowMask)
	if offset >= len(m.cartridge.PRG) {
		return 0
	}

	return m.cartridge.PRG[offset]
}

// WritePRG handles bank switching and mirroring writes in $A000-$FFFF, and PRG-RAM writes in $6000-$7FFF.
func (m *Mapper10) WritePRG(addr uint16, data byte) {
	if m.cartridge.WriteSRAM(addr, data) {
		return
	}

	switch addr & mmc4RegMask {
	case mmc4RegPRGBankSelect:
		m.prgBank = int(data&mmc4PRGBankMask) % m.prgBanks16K
	case mmc4RegCHR0BankFD:
		m.chrBank0FD = int(data&mmc4CHRBankMask) % m.chrBanks4K
	case mmc4RegCHR0BankFE:
		m.chrBank0FE = int(data&mmc4CHRBankMask) % m.chrBanks4K
	case mmc4RegCHR1BankFD:
		m.chrBank1FD = int(data&mmc4CHRBankMask) % m.chrBanks4K
	case mmc4RegCHR1BankFE:
		m.chrBank1FE = int(data&mmc4CHRBankMask) % m.chrBanks4K
	case mmc4RegMirroring:
		if (data & mmc4MirroringMask) == 0 {
			m.cartridge.Mirroring = MirrorVertical
			return
		}
		m.cartridge.Mirroring = MirrorHorizontal
	}
}

// ReadCHR maps PPU reads to CHR-ROM using hardware latches, updating latches after reading.
func (m *Mapper10) ReadCHR(addr uint16) byte {
	if addr >= mmc4CHRBankSizeTotal {
		return 0
	}

	bank := m.selectCHRBank(addr)
	offset := (bank * mmc4CHRBankSize4KB) + int(addr&mmc4CHRWindowMask)
	val := byte(0)
	if offset < len(m.cartridge.CHR) {
		val = m.cartridge.CHR[offset]
	}

	m.updateLatch(addr)
	return val
}

// WriteCHR maps PPU writes to CHR-RAM, updating latches after writing.
func (m *Mapper10) WriteCHR(addr uint16, data byte) {
	if addr >= mmc4CHRBankSizeTotal {
		return
	}

	bank := m.selectCHRBank(addr)
	offset := (bank * mmc4CHRBankSize4KB) + int(addr&mmc4CHRWindowMask)
	if offset < len(m.cartridge.CHR) {
		m.cartridge.CHR[offset] = data
	}

	m.updateLatch(addr)
}

func (m *Mapper10) selectCHRBank(addr uint16) int {
	if addr < mmc4CHRBankSize4KB {
		if m.latch0 == mmc4LatchFD {
			return m.chrBank0FD
		}
		return m.chrBank0FE
	}

	if m.latch1 == mmc4LatchFD {
		return m.chrBank1FD
	}
	return m.chrBank1FE
}

func (m *Mapper10) updateLatch(addr uint16) {
	switch {
	case addr >= mmc4Trigger0FDStart && addr <= mmc4Trigger0FDEnd:
		m.latch0 = mmc4LatchFD
	case addr >= mmc4Trigger0FEStart && addr <= mmc4Trigger0FEEnd:
		m.latch0 = mmc4LatchFE
	case addr >= mmc4Trigger1FDStart && addr <= mmc4Trigger1FDEnd:
		m.latch1 = mmc4LatchFD
	case addr >= mmc4Trigger1FEStart && addr <= mmc4Trigger1FEEnd:
		m.latch1 = mmc4LatchFE
	}
}

// SaveState serializes MMC4 internal register and latch state.
func (m *Mapper10) SaveState(w io.Writer) error {
	mirror := byte(MirrorVertical)
	if m.cartridge != nil {
		mirror = byte(m.cartridge.Mirroring)
	}
	state := struct {
		PRGBank    int32
		CHRBank0FD int32
		CHRBank0FE int32
		CHRBank1FD int32
		CHRBank1FE int32
		Latch0     byte
		Latch1     byte
		Mirror     byte
	}{
		PRGBank:    int32(m.prgBank & 0x7FFFFFFF),
		CHRBank0FD: int32(m.chrBank0FD & 0x7FFFFFFF),
		CHRBank0FE: int32(m.chrBank0FE & 0x7FFFFFFF),
		CHRBank1FD: int32(m.chrBank1FD & 0x7FFFFFFF),
		CHRBank1FE: int32(m.chrBank1FE & 0x7FFFFFFF),
		Latch0:     m.latch0,
		Latch1:     m.latch1,
		Mirror:     mirror,
	}
	if err := binary.Write(w, binary.LittleEndian, state); err != nil {
		return fmt.Errorf("write mapper10 state: %w", err)
	}
	return nil
}

// LoadState restores MMC4 internal register and latch state.
func (m *Mapper10) LoadState(r io.Reader) error {
	var state struct {
		PRGBank    int32
		CHRBank0FD int32
		CHRBank0FE int32
		CHRBank1FD int32
		CHRBank1FE int32
		Latch0     byte
		Latch1     byte
		Mirror     byte
	}
	if err := binary.Read(r, binary.LittleEndian, &state); err != nil {
		return fmt.Errorf("read mapper10 state: %w", err)
	}
	m.prgBank = int(state.PRGBank)
	m.chrBank0FD = int(state.CHRBank0FD)
	m.chrBank0FE = int(state.CHRBank0FE)
	m.chrBank1FD = int(state.CHRBank1FD)
	m.chrBank1FE = int(state.CHRBank1FE)
	m.latch0 = state.Latch0
	m.latch1 = state.Latch1
	if m.cartridge != nil {
		m.cartridge.Mirroring = Mirroring(state.Mirror)
	}
	return nil
}
