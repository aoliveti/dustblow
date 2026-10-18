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
	mmc2PRGBankSize8KB   = 0x2000 // 8 KB PRG bank size
	mmc2CHRBankSize4KB   = 0x1000 // 4 KB CHR bank size
	mmc2PRGWindowMask    = 0x1FFF // 8 KB offset mask
	mmc2CHRWindowMask    = 0x0FFF // 4 KB offset mask
	mmc2CHRBankSizeTotal = 0x2000 // Total 8 KB pattern table space ($0000-$1FFF)

	mmc2PRGWindow0Base = 0x8000 // $8000-$9FFF: switchable PRG bank
	mmc2PRGWindow1Base = 0xA000 // $A000-$BFFF: fixed to penultimate - 1 (prgBanks - 3)
	mmc2PRGWindow2Base = 0xC000 // $C000-$DFFF: fixed to penultimate (prgBanks - 2)
	mmc2PRGWindow3Base = 0xE000 // $E000-$FFFF: fixed to last bank (prgBanks - 1)

	mmc2RegPRGBankSelect = 0xA000 // Select 8 KB PRG bank ($8000-$9FFF)
	mmc2RegCHR0BankFD    = 0xB000 // Select 4 KB CHR bank 0 for latch $FD
	mmc2RegCHR0BankFE    = 0xC000 // Select 4 KB CHR bank 0 for latch $FE
	mmc2RegCHR1BankFD    = 0xD000 // Select 4 KB CHR bank 1 for latch $FD
	mmc2RegCHR1BankFE    = 0xE000 // Select 4 KB CHR bank 1 for latch $FE
	mmc2RegMirroring     = 0xF000 // Mirroring control register
	mmc2RegMask          = 0xF000 // Register selection mask ($A000, $B000, ..., $F000)

	mmc2PRGBankMask   = 0x0F // 4 bits of PRG bank select
	mmc2CHRBankMask   = 0x1F // 5 bits of CHR bank select
	mmc2MirroringMask = 0x01 // Bit 0 selects mirroring (0: Vertical, 1: Horizontal)

	mmc2LatchFD = 0xFD
	mmc2LatchFE = 0xFE

	mmc2Trigger0FDStart = 0x0FD0
	mmc2Trigger0FDEnd   = 0x0FDF
	mmc2Trigger0FEStart = 0x0FE0
	mmc2Trigger0FEEnd   = 0x0FEF
	mmc2Trigger1FDStart = 0x1FD0
	mmc2Trigger1FDEnd   = 0x1FDF
	mmc2Trigger1FEStart = 0x1FE0
	mmc2Trigger1FEEnd   = 0x1FEF

	minMMC2PRGBanks8K = 4
	minMMC2CHRBanks4K = 1
)

// Mapper9 implements the Nintendo MMC2 mapper (iNES Mapper 9: PxROM, PEENROM, PNROM).
// Supports Mike Tyson's Punch-Out!! and Punch-Out!!.
type Mapper9 struct {
	cartridge   *Cartridge
	prgBanks8K  int
	chrBanks4K  int
	prgBank     int
	chrBank0FD  int
	chrBank0FE  int
	chrBank1FD  int
	chrBank1FE  int
	latch0      byte
	latch1      byte
}

// NewMapper9 constructs and initializes a new MMC2 mapper instance.
func NewMapper9(cart *Cartridge) *Mapper9 {
	prgBanks := max(len(cart.PRG)/mmc2PRGBankSize8KB, minMMC2PRGBanks8K)
	chrBanks := max(len(cart.CHR)/mmc2CHRBankSize4KB, minMMC2CHRBanks4K)

	return &Mapper9{
		cartridge:  cart,
		prgBanks8K: prgBanks,
		chrBanks4K: chrBanks,
		prgBank:    0,
		chrBank0FD: 0,
		chrBank0FE: 0,
		chrBank1FD: 0,
		chrBank1FE: 0,
		latch0:     mmc2LatchFE,
		latch1:     mmc2LatchFE,
	}
}

// ReadPRG maps CPU reads in the range $6000-$7FFF (SRAM) and $8000-$FFFF (PRG-ROM).
func (m *Mapper9) ReadPRG(addr uint16) byte {
	if addr >= SRAMBaseAddress && addr <= SRAMEndAddress {
		return m.cartridge.ReadSRAM(addr)
	}

	if addr < mmc2PRGWindow0Base {
		return 0
	}

	bank := m.prgBank
	switch {
	case addr >= mmc2PRGWindow3Base:
		bank = m.prgBanks8K - 1
	case addr >= mmc2PRGWindow2Base:
		bank = m.prgBanks8K - 2
	case addr >= mmc2PRGWindow1Base:
		bank = m.prgBanks8K - 3
	}

	offset := (bank * mmc2PRGBankSize8KB) + int(addr&mmc2PRGWindowMask)
	if offset >= len(m.cartridge.PRG) {
		return 0
	}

	return m.cartridge.PRG[offset]
}

// WritePRG handles bank switching and mirroring writes in $A000-$FFFF, and PRG-RAM writes in $6000-$7FFF.
func (m *Mapper9) WritePRG(addr uint16, data byte) {
	if m.cartridge.WriteSRAM(addr, data) {
		return
	}

	switch addr & mmc2RegMask {
	case mmc2RegPRGBankSelect:
		m.prgBank = int(data&mmc2PRGBankMask) % m.prgBanks8K
	case mmc2RegCHR0BankFD:
		m.chrBank0FD = int(data&mmc2CHRBankMask) % m.chrBanks4K
	case mmc2RegCHR0BankFE:
		m.chrBank0FE = int(data&mmc2CHRBankMask) % m.chrBanks4K
	case mmc2RegCHR1BankFD:
		m.chrBank1FD = int(data&mmc2CHRBankMask) % m.chrBanks4K
	case mmc2RegCHR1BankFE:
		m.chrBank1FE = int(data&mmc2CHRBankMask) % m.chrBanks4K
	case mmc2RegMirroring:
		if (data & mmc2MirroringMask) == 0 {
			m.cartridge.Mirroring = MirrorVertical
			return
		}
		m.cartridge.Mirroring = MirrorHorizontal
	}
}

// ReadCHR maps PPU reads to CHR-ROM using hardware latches, updating latches after reading.
func (m *Mapper9) ReadCHR(addr uint16) byte {
	if addr >= mmc2CHRBankSizeTotal {
		return 0
	}

	bank := m.selectCHRBank(addr)
	offset := (bank * mmc2CHRBankSize4KB) + int(addr&mmc2CHRWindowMask)
	val := byte(0)
	if offset < len(m.cartridge.CHR) {
		val = m.cartridge.CHR[offset]
	}

	m.updateLatch(addr)
	return val
}

// WriteCHR maps PPU writes to CHR-RAM, updating latches after writing.
func (m *Mapper9) WriteCHR(addr uint16, data byte) {
	if addr >= mmc2CHRBankSizeTotal {
		return
	}

	bank := m.selectCHRBank(addr)
	offset := (bank * mmc2CHRBankSize4KB) + int(addr&mmc2CHRWindowMask)
	if offset < len(m.cartridge.CHR) {
		m.cartridge.CHR[offset] = data
	}

	m.updateLatch(addr)
}

func (m *Mapper9) selectCHRBank(addr uint16) int {
	if addr < mmc2CHRBankSize4KB {
		if m.latch0 == mmc2LatchFD {
			return m.chrBank0FD
		}
		return m.chrBank0FE
	}

	if m.latch1 == mmc2LatchFD {
		return m.chrBank1FD
	}
	return m.chrBank1FE
}

func (m *Mapper9) updateLatch(addr uint16) {
	switch {
	case addr >= mmc2Trigger0FDStart && addr <= mmc2Trigger0FDEnd:
		m.latch0 = mmc2LatchFD
	case addr >= mmc2Trigger0FEStart && addr <= mmc2Trigger0FEEnd:
		m.latch0 = mmc2LatchFE
	case addr >= mmc2Trigger1FDStart && addr <= mmc2Trigger1FDEnd:
		m.latch1 = mmc2LatchFD
	case addr >= mmc2Trigger1FEStart && addr <= mmc2Trigger1FEEnd:
		m.latch1 = mmc2LatchFE
	}
}

// SaveState serializes MMC2 internal register and latch state.
func (m *Mapper9) SaveState(w io.Writer) error {
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
		return fmt.Errorf("write mapper9 state: %w", err)
	}
	return nil
}

// LoadState restores MMC2 internal register and latch state.
func (m *Mapper9) LoadState(r io.Reader) error {
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
		return fmt.Errorf("read mapper9 state: %w", err)
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
