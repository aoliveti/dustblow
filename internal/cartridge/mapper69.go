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
	mmc69PRGBankSize8K = 0x2000 // 8 KB PRG bank size
	mmc69CHRBankSize1K = 0x0400 // 1 KB CHR bank size

	mmc69PRGWindowMask8K = 0x1FFF // 8 KB PRG window mask ($8000-$9FFF, etc.)
	mmc69CHRWindowMask1K = 0x03FF // 1 KB CHR window mask ($0000-$03FF, etc.)

	mmc69PRGRAMBase = 0x6000
	mmc69PRGRAMEnd  = 0x7FFF
	mmc69PRGROMBase = 0x8000

	mmc69PRGWindowCount = 4 // Windows at $8000, $A000, $C000, $E000
	mmc69CHRWindowCount = 8 // Windows at $0000, $0400, $0800, $0C00, $1000, $1400, $1800, $1C00

	mmc69RegCommand     = 0x8000 // $8000-$9FFF: Command register
	mmc69RegData        = 0xA000 // $A000-$BFFF: Data / parameter register
	mmc69RegSoundSelect = 0xC000 // $C000-$DFFF: 5B sound register select
	mmc69RegSoundData   = 0xE000 // $E000-$FFFF: 5B sound register write

	mmc69CommandMask = 0x0F

	// Commands 0x00-0x07: 1KB CHR bank select for windows 0-7
	mmc69CmdCHR0 = 0x00
	mmc69CmdCHR7 = 0x07

	// Command 0x08: PRG-RAM / ROM select at $6000-$7FFF
	mmc69CmdPRGRAMROM = 0x08

	// Command 0x09-0x0B: 8KB PRG-ROM banks at $8000, $A000, $C000
	mmc69CmdPRG0 = 0x09
	mmc69CmdPRG1 = 0x0A
	mmc69CmdPRG2 = 0x0B

	// Command 0x0C: Nametable mirroring
	mmc69CmdMirroring = 0x0C

	// Command 0x0D: IRQ control
	mmc69CmdIRQControl = 0x0D

	// Command 0x0E-0x0F: 16-bit IRQ counter low / high
	mmc69CmdIRQLow  = 0x0E
	mmc69CmdIRQHigh = 0x0F

	// Command 0x08 bit flags
	mmc69RAMEnableMask = 0x40 // Bit 6: PRG-RAM enable
	mmc69RAMSelectMask = 0x80 // Bit 7: 1 = PRG-RAM, 0 = PRG-ROM
	mmc69BankMask6Bit  = 0x3F // Bits 5-0: Bank index

	// Command 0x0C mirroring modes
	mmc69MirrorVertical   = 0x00
	mmc69MirrorHorizontal = 0x01
	mmc69MirrorSingle0    = 0x02
	mmc69MirrorSingle1    = 0x03
	mmc69MirrorModeMask   = 0x03

	// Command 0x0D IRQ control flags
	mmc69IRQEnableMask    = 0x01 // Bit 0: assert IRQ on underflow
	mmc69IRQCountdownMask = 0x80 // Bit 7: counter decrements every CPU cycle

	minMMC69PRGBanks8K = 4
	minMMC69CHRBanks1K = 8
)

// Mapper69 implements the Sunsoft FME-7 / 5B ASIC mapper (iNES Mapper 69).
// Supports games like Batman: Return of the Joker, Gimmick!, and Hebereke (Ufouria: The Saga).
type Mapper69 struct {
	cartridge         *Cartridge
	prgBanks8K        int
	chrBanks1K        int
	command           byte
	prgOffsets        [mmc69PRGWindowCount]int
	chrOffsets        [mmc69CHRWindowCount]int
	ramOffset         int
	isRAMSelected     bool
	isRAMEnabled      bool
	irqEnabled        bool
	irqCounterRunning bool
	irqCounter        int
	irqPending        bool
	audio             *Sunsoft5BAudio
}

// NewMapper69 constructs and initializes a new Mapper69 (Sunsoft FME-7 / 5B) instance.
func NewMapper69(cart *Cartridge) *Mapper69 {
	prgBanks := max(len(cart.PRG)/mmc69PRGBankSize8K, minMMC69PRGBanks8K)
	chrBanks := max(len(cart.CHR)/mmc69CHRBankSize1K, minMMC69CHRBanks1K)

	if len(cart.SRAM) == 0 {
		cart.SRAM = make([]byte, SRAMSize)
	}

	audio := NewSunsoft5BAudio()
	cart.SetExpansionAudio(audio)

	m := &Mapper69{
		cartridge:  cart,
		prgBanks8K: prgBanks,
		chrBanks1K: chrBanks,
		audio:      audio,
	}

	// The upper 8KB bank ($E000-$FFFF) is permanently fixed to the last 8KB PRG-ROM bank.
	m.prgOffsets[3] = (prgBanks - 1) * mmc69PRGBankSize8K

	return m
}

// ReadPRG maps CPU reads across $6000-$FFFF (PRG-RAM/ROM window and banked PRG-ROM).
func (m *Mapper69) ReadPRG(addr uint16) byte {
	switch {
	case addr >= mmc69PRGRAMBase && addr <= mmc69PRGRAMEnd:
		if m.isRAMSelected {
			if !m.isRAMEnabled || len(m.cartridge.SRAM) == 0 {
				return 0
			}
			idx := (m.ramOffset + int(addr-mmc69PRGRAMBase)) % len(m.cartridge.SRAM)
			return m.cartridge.SRAM[idx]
		}
		if len(m.cartridge.PRG) == 0 {
			return 0
		}
		idx := (m.ramOffset + int(addr-mmc69PRGRAMBase)) % len(m.cartridge.PRG)
		return m.cartridge.PRG[idx]

	case addr >= mmc69PRGROMBase:
		if len(m.cartridge.PRG) == 0 {
			return 0
		}
		window := (addr - mmc69PRGROMBase) / mmc69PRGBankSize8K
		offset := addr & mmc69PRGWindowMask8K
		idx := (m.prgOffsets[window] + int(offset)) % len(m.cartridge.PRG)
		return m.cartridge.PRG[idx]

	default:
		return 0
	}
}

// WritePRG maps CPU writes to command, parameter, and PRG-RAM registers.
func (m *Mapper69) WritePRG(addr uint16, data byte) {
	switch {
	case addr >= mmc69PRGRAMBase && addr <= mmc69PRGRAMEnd:
		if m.isRAMSelected && m.isRAMEnabled && len(m.cartridge.SRAM) > 0 {
			idx := (m.ramOffset + int(addr-mmc69PRGRAMBase)) % len(m.cartridge.SRAM)
			m.cartridge.SRAM[idx] = data
			m.cartridge.MarkSRAMDirty()
		}

	case addr >= mmc69RegCommand && addr <= 0x9FFF:
		m.command = data & mmc69CommandMask

	case addr >= mmc69RegData && addr <= 0xBFFF:
		m.writeRegister(m.command, data)

	case addr >= mmc69RegSoundSelect && addr <= 0xDFFF:
		m.audio.SelectRegister(data)

	case addr >= mmc69RegSoundData:
		m.audio.WriteData(data)
	}
}

func (m *Mapper69) writeRegister(cmd byte, data byte) {
	switch {
	case cmd <= mmc69CmdCHR7:
		bank := int(data) % m.chrBanks1K
		m.chrOffsets[cmd] = bank * mmc69CHRBankSize1K

	case cmd == mmc69CmdPRGRAMROM:
		m.isRAMEnabled = (data & mmc69RAMEnableMask) != 0
		m.isRAMSelected = (data & mmc69RAMSelectMask) != 0
		bank := int(data & mmc69BankMask6Bit)
		m.ramOffset = (bank % m.prgBanks8K) * mmc69PRGBankSize8K
		if m.isRAMSelected {
			m.ramOffset = bank * SRAMSize
		}

	case cmd >= mmc69CmdPRG0 && cmd <= mmc69CmdPRG2:
		window := int(cmd - mmc69CmdPRG0)
		bank := int(data & mmc69BankMask6Bit) % m.prgBanks8K
		m.prgOffsets[window] = bank * mmc69PRGBankSize8K

	case cmd == mmc69CmdMirroring:
		m.applyMirroring(data & mmc69MirrorModeMask)

	case cmd == mmc69CmdIRQControl:
		m.irqEnabled = (data & mmc69IRQEnableMask) != 0
		m.irqCounterRunning = (data & mmc69IRQCountdownMask) != 0
		m.irqPending = false // Writing register $0D clears any active IRQ assertion

	case cmd == mmc69CmdIRQLow:
		m.irqCounter = (m.irqCounter & 0xFF00) | int(data)

	case cmd == mmc69CmdIRQHigh:
		m.irqCounter = (m.irqCounter & 0x00FF) | (int(data) << 8)
	}
}

func (m *Mapper69) applyMirroring(mode byte) {
	switch mode {
	case mmc69MirrorVertical:
		m.cartridge.Mirroring = MirrorVertical
	case mmc69MirrorHorizontal:
		m.cartridge.Mirroring = MirrorHorizontal
	case mmc69MirrorSingle0:
		m.cartridge.Mirroring = MirrorSingle0
	case mmc69MirrorSingle1:
		m.cartridge.Mirroring = MirrorSingle1
	}
}

// ReadCHR maps PPU reads in $0000-$1FFF to banked 1KB CHR-ROM/RAM.
func (m *Mapper69) ReadCHR(addr uint16) byte {
	if len(m.cartridge.CHR) == 0 {
		return 0
	}
	window := (addr / mmc69CHRBankSize1K) & 7
	offset := addr & mmc69CHRWindowMask1K
	idx := (m.chrOffsets[window] + int(offset)) % len(m.cartridge.CHR)
	return m.cartridge.CHR[idx]
}

// WriteCHR maps PPU writes in $0000-$1FFF to banked 1KB CHR-RAM.
func (m *Mapper69) WriteCHR(addr uint16, data byte) {
	if len(m.cartridge.CHR) == 0 {
		return
	}
	window := (addr / mmc69CHRBankSize1K) & 7
	offset := addr & mmc69CHRWindowMask1K
	idx := (m.chrOffsets[window] + int(offset)) % len(m.cartridge.CHR)
	m.cartridge.CHR[idx] = data
}

// StepCPU decrements the 16-bit IRQ down-counter by elapsed CPU cycles and advances 5B audio synthesis.
func (m *Mapper69) StepCPU(cycles int) {
	if m.audio != nil && cycles > 0 {
		m.audio.StepAudio(cycles)
	}

	if !m.irqCounterRunning || cycles <= 0 {
		return
	}

	m.irqCounter -= cycles
	if m.irqCounter < 0 {
		if m.irqEnabled {
			m.irqPending = true
		}
		m.irqCounter &= 0xFFFF
	}
}

// Audio returns the attached Sunsoft 5B audio synthesis core.
func (m *Mapper69) Audio() *Sunsoft5BAudio {
	return m.audio
}

// IRQPending reports whether the FME-7 IRQ line is currently asserted.
func (m *Mapper69) IRQPending() bool {
	return m.irqPending
}

// SaveState serializes the Sunsoft FME-7 / 5B internal mapper state.
func (m *Mapper69) SaveState(w io.Writer) error {
	mirror := byte(MirrorVertical)
	if m.cartridge != nil {
		mirror = byte(m.cartridge.Mirroring)
	}
	var prgOffsets [mmc69PRGWindowCount]int32
	for i, off := range m.prgOffsets {
		prgOffsets[i] = int32(off & 0x7FFFFFFF)
	}
	var chrOffsets [mmc69CHRWindowCount]int32
	for i, off := range m.chrOffsets {
		chrOffsets[i] = int32(off & 0x7FFFFFFF)
	}

	state := struct {
		Command           byte
		PRGOffsets        [mmc69PRGWindowCount]int32
		CHROffsets        [mmc69CHRWindowCount]int32
		RAMOffset         int32
		IsRAMSelected     bool
		IsRAMEnabled      bool
		IRQEnabled        bool
		IRQCounterRunning bool
		IRQCounter        int32
		IRQPending        bool
		Mirror            byte
	}{
		Command:           m.command,
		PRGOffsets:        prgOffsets,
		CHROffsets:        chrOffsets,
		RAMOffset:         int32(m.ramOffset & 0x7FFFFFFF),
		IsRAMSelected:     m.isRAMSelected,
		IsRAMEnabled:      m.isRAMEnabled,
		IRQEnabled:        m.irqEnabled,
		IRQCounterRunning: m.irqCounterRunning,
		IRQCounter:        int32(m.irqCounter & 0x7FFFFFFF),
		IRQPending:        m.irqPending,
		Mirror:            mirror,
	}

	if err := binary.Write(w, binary.LittleEndian, state); err != nil {
		return fmt.Errorf("write mapper69 state: %w", err)
	}

	if m.audio != nil {
		if err := m.audio.SaveState(w); err != nil {
			return fmt.Errorf("write mapper69 audio state: %w", err)
		}
	}
	return nil
}

// LoadState restores the Sunsoft FME-7 / 5B internal mapper state.
func (m *Mapper69) LoadState(r io.Reader) error {
	var state struct {
		Command           byte
		PRGOffsets        [mmc69PRGWindowCount]int32
		CHROffsets        [mmc69CHRWindowCount]int32
		RAMOffset         int32
		IsRAMSelected     bool
		IsRAMEnabled      bool
		IRQEnabled        bool
		IRQCounterRunning bool
		IRQCounter        int32
		IRQPending        bool
		Mirror            byte
	}

	if err := binary.Read(r, binary.LittleEndian, &state); err != nil {
		return fmt.Errorf("read mapper69 state: %w", err)
	}

	m.command = state.Command
	for i, off := range state.PRGOffsets {
		m.prgOffsets[i] = int(off)
	}
	for i, off := range state.CHROffsets {
		m.chrOffsets[i] = int(off)
	}
	m.ramOffset = int(state.RAMOffset)
	m.isRAMSelected = state.IsRAMSelected
	m.isRAMEnabled = state.IsRAMEnabled
	m.irqEnabled = state.IRQEnabled
	m.irqCounterRunning = state.IRQCounterRunning
	m.irqCounter = int(state.IRQCounter)
	m.irqPending = state.IRQPending
	if m.cartridge != nil {
		m.cartridge.Mirroring = Mirroring(state.Mirror)
	}

	if m.audio != nil {
		if err := m.audio.LoadState(r); err != nil {
			return fmt.Errorf("read mapper69 audio state: %w", err)
		}
	}
	return nil
}

// Reset silences expansion audio and clears pending IRQs on console reset.
func (m *Mapper69) Reset() {
	m.irqPending = false
	if m.audio != nil {
		m.audio.Reset()
	}
}
