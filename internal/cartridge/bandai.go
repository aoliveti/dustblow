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
	bandaiPRGBankSize = 0x4000 // 16KB
	bandaiCHRBankSize = 0x0400 // 1KB

	bandaiRegCHR0     = 0x00
	bandaiRegCHR7     = 0x07
	bandaiRegPRG      = 0x08
	bandaiRegMirror   = 0x09
	bandaiRegIRQCtrl  = 0x0A
	bandaiRegIRQLow   = 0x0B
	bandaiRegIRQHigh  = 0x0C
	bandaiRegEEPROM   = 0x0D
	bandaiEEPROMRead  = 0x04
	bandaiEEPROMSDAIn = 0x02
	bandaiEEPROMSCLIn = 0x01
)

// MapperBandai implements the Bandai FCG-1, FCG-2 and LZ93D50 ASICs (Mappers 16, 153, 159).
type MapperBandai struct {
	cartridge *Cartridge
	mapperID  byte
	eeprom    *I2CEEPROM

	prgBanksCount int
	chrBanksCount int

	prgBank  int
	chrBanks [8]int

	irqCounter uint16
	irqLatch   uint16
	irqEnabled bool
	irqPending bool

	hasSRAM   bool
	hasEEPROM bool
}

// NewMapperBandai constructs and initializes a new Bandai mapper instance.
func NewMapperBandai(cart *Cartridge, mapperID byte) *MapperBandai {
	prgBanks := max(len(cart.PRG)/bandaiPRGBankSize, 1)
	chrBanks := len(cart.CHR) / bandaiCHRBankSize

	m := &MapperBandai{
		cartridge:     cart,
		mapperID:      mapperID,
		prgBanksCount: prgBanks,
		chrBanksCount: chrBanks,
		prgBank:       0,
	}

	switch mapperID {
	case 153:
		// Famicom Jump II uses 8KB battery SRAM
		m.hasSRAM = true
		if len(cart.SRAM) == 0 {
			cart.SRAM = make([]byte, SRAMSize)
		}
	case 159:
		// LZ93D50 with 128-byte 24C01 EEPROM
		m.hasEEPROM = true
		m.eeprom = NewI2CEEPROM(cart, 128)
	case 16:
		// LZ93D50 with 256-byte 24C02 EEPROM (or no EEPROM)
		m.hasEEPROM = true
		m.eeprom = NewI2CEEPROM(cart, 256)
	}

	cart.BindMapper(m)
	return m
}

// ReadPRG maps CPU reads across $6000-$FFFF.
func (m *MapperBandai) ReadPRG(addr uint16) byte {
	switch {
	case addr >= 0x6000 && addr <= 0x7FFF:
		if m.hasSRAM {
			return m.cartridge.ReadSRAM(addr)
		}
		if m.hasEEPROM && m.eeprom != nil {
			if m.eeprom.ReadSDA() {
				return 0x01
			}
			return 0x00
		}
		return 0

	case addr >= 0x8000 && addr <= 0xBFFF:
		bank := m.prgBank % m.prgBanksCount
		return m.cartridge.PRG[bank*bandaiPRGBankSize+int(addr-0x8000)]

	case addr >= 0xC000:
		bank := (m.prgBanksCount - 1) * bandaiPRGBankSize
		return m.cartridge.PRG[bank+int(addr-0xC000)]

	default:
		return 0
	}
}

// WritePRG maps CPU writes across $6000-$FFFF.
func (m *MapperBandai) WritePRG(addr uint16, data byte) {
	if addr >= 0x6000 && addr <= 0x7FFF {
		if m.hasSRAM {
			m.cartridge.WriteSRAM(addr, data)
			return
		}
		// On FCG-1/2, registers are mapped to $6000-$7FFF
		m.writeRegister(addr&0x000F, data)
		return
	}

	if addr >= 0x8000 {
		m.writeRegister(addr&0x000F, data)
	}
}

func (m *MapperBandai) writeRegister(reg uint16, data byte) {
	switch {
	case reg <= bandaiRegCHR7:
		m.chrBanks[reg] = int(data)

	case reg == bandaiRegPRG:
		m.prgBank = int(data & 0x0F)

	case reg == bandaiRegMirror:
		switch data & 0x03 {
		case 0:
			m.cartridge.Mirroring = MirrorVertical
		case 1:
			m.cartridge.Mirroring = MirrorHorizontal
		case 2:
			m.cartridge.Mirroring = MirrorSingle0
		case 3:
			m.cartridge.Mirroring = MirrorSingle1
		}

	case reg == bandaiRegIRQCtrl:
		m.irqEnabled = (data & 0x01) != 0
		if m.irqEnabled {
			m.irqCounter = m.irqLatch
		}
		m.irqPending = false

	case reg == bandaiRegIRQLow:
		m.irqLatch = (m.irqLatch & 0xFF00) | uint16(data)

	case reg == bandaiRegIRQHigh:
		m.irqLatch = (m.irqLatch & 0x00FF) | (uint16(data) << 8)

	case reg == bandaiRegEEPROM:
		if m.hasEEPROM && m.eeprom != nil {
			scl := (data & bandaiEEPROMSCLIn) != 0
			sda := (data & bandaiEEPROMSDAIn) != 0
			m.eeprom.WriteLines(scl, sda)
		}
	}
}

// ReadCHR maps PPU reads across $0000-$1FFF to banked 1KB CHR-ROM/RAM.
func (m *MapperBandai) ReadCHR(addr uint16) byte {
	if len(m.cartridge.CHR) == 0 || addr >= 0x2000 {
		return 0
	}
	slot := addr / bandaiCHRBankSize
	offset := addr % bandaiCHRBankSize
	bank := m.chrBanks[slot]
	if m.chrBanksCount > 0 {
		bank %= m.chrBanksCount
	}
	idx := bank*bandaiCHRBankSize + int(offset)
	return m.cartridge.CHR[idx%len(m.cartridge.CHR)]
}

// WriteCHR maps PPU writes across $0000-$1FFF to banked 1KB CHR-RAM.
func (m *MapperBandai) WriteCHR(addr uint16, data byte) {
	if len(m.cartridge.CHR) == 0 || addr >= 0x2000 {
		return
	}
	slot := addr / bandaiCHRBankSize
	offset := addr % bandaiCHRBankSize
	bank := m.chrBanks[slot]
	if m.chrBanksCount > 0 {
		bank %= m.chrBanksCount
	}
	idx := bank*bandaiCHRBankSize + int(offset)
	m.cartridge.CHR[idx%len(m.cartridge.CHR)] = data
}

// IRQPending reports whether the Bandai IRQ down-counter reached zero.
func (m *MapperBandai) IRQPending() bool {
	return m.irqPending
}

// StepCPU decrements the Bandai IRQ down-counter by the given number of CPU clock cycles.
func (m *MapperBandai) StepCPU(cycles int) {
	if !m.irqEnabled || cycles <= 0 {
		return
	}

	if cycles >= int(m.irqCounter) {
		m.irqCounter = 0
		m.irqPending = true
		m.irqEnabled = false
		return
	}

	m.irqCounter -= uint16(cycles & 0xFFFF)
}

// SaveState serializes the Bandai mapper state.
func (m *MapperBandai) SaveState(w io.Writer) error {
	var chrBanks [8]int32
	for i, b := range m.chrBanks {
		chrBanks[i] = int32(b & 0x7FFFFFFF)
	}

	mirror := byte(MirrorVertical)
	if m.cartridge != nil {
		mirror = byte(m.cartridge.Mirroring)
	}

	state := struct {
		PRGBank    int32
		CHRBanks   [8]int32
		IRQCounter uint16
		IRQLatch   uint16
		IRQEnabled bool
		IRQPending bool
		Mirror     byte
	}{
		PRGBank:    int32(m.prgBank & 0x7FFFFFFF),
		CHRBanks:   chrBanks,
		IRQCounter: m.irqCounter,
		IRQLatch:   m.irqLatch,
		IRQEnabled: m.irqEnabled,
		IRQPending: m.irqPending,
		Mirror:     mirror,
	}

	if err := binary.Write(w, binary.LittleEndian, state); err != nil {
		return fmt.Errorf("write bandai state: %w", err)
	}

	if m.hasEEPROM && m.eeprom != nil {
		if err := m.eeprom.SaveState(w); err != nil {
			return fmt.Errorf("write bandai eeprom state: %w", err)
		}
	}
	return nil
}

// LoadState restores the Bandai mapper state.
func (m *MapperBandai) LoadState(r io.Reader) error {
	var state struct {
		PRGBank    int32
		CHRBanks   [8]int32
		IRQCounter uint16
		IRQLatch   uint16
		IRQEnabled bool
		IRQPending bool
		Mirror     byte
	}

	if err := binary.Read(r, binary.LittleEndian, &state); err != nil {
		return fmt.Errorf("read bandai state: %w", err)
	}

	m.prgBank = int(state.PRGBank)
	for i, b := range state.CHRBanks {
		m.chrBanks[i] = int(b)
	}
	m.irqCounter = state.IRQCounter
	m.irqLatch = state.IRQLatch
	m.irqEnabled = state.IRQEnabled
	m.irqPending = state.IRQPending
	if m.cartridge != nil {
		m.cartridge.Mirroring = Mirroring(state.Mirror)
	}

	if m.hasEEPROM && m.eeprom != nil {
		if err := m.eeprom.LoadState(r); err != nil {
			return fmt.Errorf("read bandai eeprom state: %w", err)
		}
	}
	return nil
}

// Reset clears pending IRQ state on console reset.
func (m *MapperBandai) Reset() {
	m.irqPending = false
}
