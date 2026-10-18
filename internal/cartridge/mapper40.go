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
	mmc40PRGBankSize    = 0x2000 // 8 KB PRG bank size
	mmc40CHRBankSize    = 0x2000 // 8 KB CHR bank size
	mmc40PRGWindowMask  = 0x1FFF // 8 KB offset mask
	mmc40CHRWindowMask  = 0x1FFF // 8 KB offset mask
	mmc40Window6000     = 0x6000 // Starting address of $6000 window
	mmc40PRGWindowCount = 5
	mmc40RegisterMask   = 0xE000

	mmc40RegDisableIRQ = 0x8000
	mmc40RegEnableIRQ  = 0xA000
	mmc40RegSelectBank = 0xE000

	mmc40BankSelectMask = 0x07
	mmc40IRQThreshold   = 4096
	minMMC40PRGBanks    = 8
)

// Mapper40 implements the NTDEC 2722 PCB mapper (iNES Mapper 40).
// Used by cartridge conversions of the FDS title Super Mario Bros. 2 Japan (The Lost Levels).
type Mapper40 struct {
	UnbankedCHR
	cartridge  *Cartridge
	prgBanks8K int
	prgBank    int
	prgOffsets [mmc40PRGWindowCount]int
	irqCounter int
	irqEnabled bool
	irqPending bool
}

// NewMapper40 creates and initializes a new Mapper40 instance.
func NewMapper40(cart *Cartridge) *Mapper40 {
	if cart != nil {
		cart.IsCHRRAM = true
	}
	prgBanks := max(len(cart.PRG)/mmc40PRGBankSize, minMMC40PRGBanks)

	m := &Mapper40{
		UnbankedCHR: NewUnbankedCHR(cart),
		cartridge:   cart,
		prgBanks8K:  prgBanks,
		prgBank:     0,
	}

	m.updateOffsets()
	return m
}

// ReadPRG maps CPU reads to banked PRG-ROM across $6000-$FFFF.
func (m *Mapper40) ReadPRG(addr uint16) byte {
	if addr < mmc40Window6000 {
		return 0
	}

	window := int((addr - mmc40Window6000) / mmc40PRGBankSize)
	if window >= mmc40PRGWindowCount {
		return 0
	}

	offset := m.prgOffsets[window] + int(addr&mmc40PRGWindowMask)
	if offset >= len(m.cartridge.PRG) {
		return 0
	}

	return m.cartridge.PRG[offset]
}

// WritePRG handles CPU writes to control registers ($8000-$FFFF).
func (m *Mapper40) WritePRG(addr uint16, data byte) {
	if addr < mmc40RegDisableIRQ {
		return
	}

	switch addr & mmc40RegisterMask {
	case mmc40RegDisableIRQ:
		m.irqEnabled = false
		m.irqCounter = 0
		m.irqPending = false
	case mmc40RegEnableIRQ:
		m.irqEnabled = true
	case mmc40RegSelectBank:
		m.prgBank = int(data & mmc40BankSelectMask)
		m.updateOffsets()
	}
}

// StepCPU advances the internal 4096-cycle M2 counter when IRQ is enabled.
func (m *Mapper40) StepCPU(cycles int) {
	if !m.irqEnabled {
		return
	}

	m.irqCounter += cycles
	if m.irqCounter >= mmc40IRQThreshold {
		m.irqEnabled = false
		m.irqPending = true
	}
}

// IRQPending reports whether the mapper is asserting a hardware IRQ line.
func (m *Mapper40) IRQPending() bool {
	return m.irqPending
}

func (m *Mapper40) updateOffsets() {
	// Layout:
	// Window 0 ($6000-$7FFF): fixed to bank (prgBanks8K - 2)
	// Window 1 ($8000-$9FFF): fixed to bank (prgBanks8K - 4)
	// Window 2 ($A000-$BFFF): fixed to bank (prgBanks8K - 3)
	// Window 3 ($C000-$DFFF): selectable bank via $E000
	// Window 4 ($E000-$FFFF): fixed to last bank (prgBanks8K - 1)
	bank6000 := (m.prgBanks8K - 2) % m.prgBanks8K
	bank8000 := (m.prgBanks8K - 4) % m.prgBanks8K
	bankA000 := (m.prgBanks8K - 3) % m.prgBanks8K
	bankC000 := m.prgBank % m.prgBanks8K
	bankE000 := (m.prgBanks8K - 1) % m.prgBanks8K

	m.prgOffsets[0] = bank6000 * mmc40PRGBankSize
	m.prgOffsets[1] = bank8000 * mmc40PRGBankSize
	m.prgOffsets[2] = bankA000 * mmc40PRGBankSize
	m.prgOffsets[3] = bankC000 * mmc40PRGBankSize
	m.prgOffsets[4] = bankE000 * mmc40PRGBankSize
}

// SaveState serializes Mapper 40 internal register and IRQ state.
func (m *Mapper40) SaveState(w io.Writer) error {
	state := struct {
		PRGBank    int32
		IRQCounter int32
		IRQEnabled bool
		IRQPending bool
	}{
		PRGBank:    int32(m.prgBank & 0x7FFFFFFF),
		IRQCounter: int32(m.irqCounter & 0x7FFFFFFF),
		IRQEnabled: m.irqEnabled,
		IRQPending: m.irqPending,
	}
	if err := binary.Write(w, binary.LittleEndian, state); err != nil {
		return fmt.Errorf("write mapper40 state: %w", err)
	}
	return nil
}

// LoadState restores Mapper 40 internal register and IRQ state.
func (m *Mapper40) LoadState(r io.Reader) error {
	var state struct {
		PRGBank    int32
		IRQCounter int32
		IRQEnabled bool
		IRQPending bool
	}
	if err := binary.Read(r, binary.LittleEndian, &state); err != nil {
		return fmt.Errorf("read mapper40 state: %w", err)
	}
	m.prgBank = int(state.PRGBank)
	m.irqCounter = int(state.IRQCounter)
	m.irqEnabled = state.IRQEnabled
	m.irqPending = state.IRQPending
	m.updateOffsets()
	return nil
}

// Reset clears pending IRQ state on console reset.
func (m *Mapper40) Reset() {
	m.irqPending = false
}
