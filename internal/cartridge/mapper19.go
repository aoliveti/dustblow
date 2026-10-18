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
	n163PRGBankSize     = 0x2000 // 8KB
	n163CHRBankSize     = 0x0400 // 1KB
	n163NametableCIRAM  = 0xE0
	n163IRQTarget       = 0x7FFF
	n163SoundDisableBit = 0x40
)

// Mapper19 implements the Namco 163 (and Namco 129) ASIC.
type Mapper19 struct {
	cartridge *Cartridge
	audio     *Namco163Audio

	prgBanksCount int
	chrBanksCount int

	// PRG bank selection ($8000-$9FFF, $A000-$BFFF, $C000-$DFFF)
	prgBank0 int
	prgBank1 int
	prgBank2 int

	// CHR 1KB bank selection (8 windows: $0000-$1FFF)
	chrBanks [8]int

	// Nametable 1KB bank selection (4 windows: $2000-$2FFF)
	ntBanks [4]byte

	// Internal sound RAM addressing port ($F800-$FFFF and $4800-$4FFF)
	ramAddr          byte
	ramAutoIncrement bool

	// CPU-cycle IRQ counter ($5000-$5FFF)
	irqCounter uint16
	irqEnabled bool
	irqPending bool

	// Sound disable flag ($E000 bit 6)
	soundDisabled bool
}

// NewMapper19 constructs a new Namco 163 mapper instance.
func NewMapper19(cart *Cartridge) *Mapper19 {
	prgBanks := max(len(cart.PRG)/n163PRGBankSize, 1)
	chrBanks := len(cart.CHR) / n163CHRBankSize

	audio := NewNamco163Audio()

	m := &Mapper19{
		cartridge:     cart,
		audio:         audio,
		prgBanksCount: prgBanks,
		chrBanksCount: chrBanks,
		prgBank0:      0,
		prgBank1:      1 % prgBanks,
		prgBank2:      2 % prgBanks,
		ntBanks:       [4]byte{n163NametableCIRAM, n163NametableCIRAM + 1, n163NametableCIRAM, n163NametableCIRAM + 1},
	}

	cart.BindMapper(m)
	cart.SetExpansionAudio(audio)
	return m
}

// ReadPRG maps CPU reads across $4800-$FFFF.
func (m *Mapper19) ReadPRG(addr uint16) byte {
	switch {
	case addr >= 0x4800 && addr <= 0x4FFF:
		val := m.audio.ReadRAM(m.ramAddr)
		if m.ramAutoIncrement {
			m.ramAddr = (m.ramAddr + 1) & 0x7F
		}
		return val

	case addr >= 0x5000 && addr <= 0x57FF:
		return byte(m.irqCounter & 0xFF)

	case addr >= 0x5800 && addr <= 0x5FFF:
		high := byte((m.irqCounter >> 8) & 0x7F)
		if m.irqEnabled {
			high |= 0x80
		}
		return high

	case addr >= 0x6000 && addr <= 0x7FFF:
		return m.cartridge.ReadSRAM(addr)

	case addr >= 0x8000 && addr <= 0x9FFF:
		bank := m.prgBank0 % m.prgBanksCount
		return m.cartridge.PRG[bank*n163PRGBankSize+int(addr-0x8000)]

	case addr >= 0xA000 && addr <= 0xBFFF:
		bank := m.prgBank1 % m.prgBanksCount
		return m.cartridge.PRG[bank*n163PRGBankSize+int(addr-0xA000)]

	case addr >= 0xC000 && addr <= 0xDFFF:
		bank := m.prgBank2 % m.prgBanksCount
		return m.cartridge.PRG[bank*n163PRGBankSize+int(addr-0xC000)]

	case addr >= 0xE000:
		bank := (m.prgBanksCount - 1) * n163PRGBankSize
		return m.cartridge.PRG[bank+int(addr-0xE000)]

	default:
		return 0
	}
}

// WritePRG maps CPU writes across $4800-$FFFF.
func (m *Mapper19) WritePRG(addr uint16, data byte) {
	switch {
	case addr >= 0x4800 && addr <= 0x4FFF:
		m.audio.WriteRAM(m.ramAddr, data)
		if m.ramAutoIncrement {
			m.ramAddr = (m.ramAddr + 1) & 0x7F
		}

	case addr >= 0x5000 && addr <= 0x57FF:
		m.irqCounter = (m.irqCounter & 0x7F00) | uint16(data)
		m.irqPending = false

	case addr >= 0x5800 && addr <= 0x5FFF:
		m.irqCounter = (m.irqCounter & 0x00FF) | (uint16(data&0x7F) << 8)
		m.irqEnabled = (data & 0x80) != 0
		m.irqPending = false

	case addr >= 0x6000 && addr <= 0x7FFF:
		m.cartridge.WriteSRAM(addr, data)

	case addr >= 0x8000 && addr <= 0xBFFF:
		slot := (addr - 0x8000) / 0x0800
		if slot < 8 {
			m.chrBanks[slot] = int(data)
		}

	case addr >= 0xC000 && addr <= 0xDFFF:
		slot := (addr - 0xC000) / 0x0800
		if slot < 4 {
			m.ntBanks[slot] = data
		}

	case addr >= 0xE000 && addr <= 0xE7FF:
		m.soundDisabled = (data & n163SoundDisableBit) != 0
		m.audio.SetSoundDisabled(m.soundDisabled)
		m.prgBank0 = int(data & 0x3F)

	case addr >= 0xE800 && addr <= 0xEFFF:
		m.prgBank1 = int(data & 0x3F)

	case addr >= 0xF000 && addr <= 0xF7FF:
		m.prgBank2 = int(data & 0x3F)

	case addr >= 0xF800:
		m.ramAddr = data & 0x7F
		m.ramAutoIncrement = (data & 0x80) != 0
	}
}

// ReadCHR maps PPU reads across $0000-$1FFF to banked 1KB CHR-ROM/RAM.
func (m *Mapper19) ReadCHR(addr uint16) byte {
	if len(m.cartridge.CHR) == 0 || addr >= 0x2000 {
		return 0
	}
	slot := addr / n163CHRBankSize
	offset := addr % n163CHRBankSize
	bank := m.chrBanks[slot]
	if m.chrBanksCount > 0 {
		bank %= m.chrBanksCount
	}
	idx := bank*n163CHRBankSize + int(offset)
	return m.cartridge.CHR[idx%len(m.cartridge.CHR)]
}

// WriteCHR maps PPU writes across $0000-$1FFF to banked 1KB CHR-RAM.
func (m *Mapper19) WriteCHR(addr uint16, data byte) {
	if len(m.cartridge.CHR) == 0 || addr >= 0x2000 {
		return
	}
	slot := addr / n163CHRBankSize
	offset := addr % n163CHRBankSize
	bank := m.chrBanks[slot]
	if m.chrBanksCount > 0 {
		bank %= m.chrBanksCount
	}
	idx := bank*n163CHRBankSize + int(offset)
	m.cartridge.CHR[idx%len(m.cartridge.CHR)] = data
}

// ReadNametable maps PPU nametable reads ($2000-$2FFF) through Namco 163 registers ($C000-$DFFF).
// If register value is >= $E0, it routes to console CIRAM page (0 or 1).
// If register value is < $E0, it routes to 1KB CHR-ROM page.
func (m *Mapper19) ReadNametable(addr uint16, vram []byte) (byte, bool) {
	if addr < 0x2000 || addr >= 0x3000 {
		return 0, false
	}

	slot := (addr - 0x2000) / 0x0400
	offset := addr % 0x0400
	regVal := m.ntBanks[slot]

	if regVal >= n163NametableCIRAM {
		ciramPage := int(regVal & 0x01)
		ciramAddr := ciramPage*0x0400 + int(offset)
		if ciramAddr < len(vram) {
			return vram[ciramAddr], true
		}
		return 0, true
	}

	// Route to CHR-ROM
	if len(m.cartridge.CHR) > 0 {
		chrIdx := (int(regVal)*n163CHRBankSize + int(offset)) % len(m.cartridge.CHR)
		return m.cartridge.CHR[chrIdx], true
	}

	return 0, true
}

// WriteNametable maps PPU nametable writes ($2000-$2FFF) through Namco 163 registers ($C000-$DFFF).
func (m *Mapper19) WriteNametable(addr uint16, val byte, vram []byte) bool {
	if addr < 0x2000 || addr >= 0x3000 {
		return false
	}

	slot := (addr - 0x2000) / 0x0400
	offset := addr % 0x0400
	regVal := m.ntBanks[slot]

	if regVal >= n163NametableCIRAM {
		ciramPage := int(regVal & 0x01)
		ciramAddr := ciramPage*0x0400 + int(offset)
		if ciramAddr < len(vram) {
			vram[ciramAddr] = val
		}
		return true
	}

	// Writes to CHR-ROM are ignored
	return true
}

// IRQPending reports whether the Namco 163 has triggered a maskable interrupt request.
func (m *Mapper19) IRQPending() bool {
	return m.irqPending
}

// StepCPU advances the CPU cycle up-counter on the Namco 163.
func (m *Mapper19) StepCPU(cycles int) {
	if !m.irqEnabled || cycles <= 0 {
		return
	}

	target := uint32(n163IRQTarget)
	newVal := uint32(m.irqCounter) + uint32(cycles&0x7FFFFFFF)
	if newVal >= target {
		m.irqCounter = n163IRQTarget
		m.irqPending = true
		return
	}
	m.irqCounter = uint16(newVal)
}

// StepAudio advances the expansion audio synthesizer.
func (m *Mapper19) StepAudio(cycles int) {
	if m.audio != nil {
		m.audio.StepAudio(cycles)
	}
}

// AudioOutput returns the currently synthesized Namco 163 audio sample.
func (m *Mapper19) AudioOutput() float32 {
	if m.audio != nil {
		return m.audio.AudioOutput()
	}
	return 0
}

// SaveState serializes the Namco 163 internal state.
func (m *Mapper19) SaveState(w io.Writer) error {
	var chrBanks [8]int32
	for i, b := range m.chrBanks {
		chrBanks[i] = int32(b & 0x7FFFFFFF)
	}

	state := struct {
		PRGBank0         int32
		PRGBank1         int32
		PRGBank2         int32
		CHRBanks         [8]int32
		NTBanks          [4]byte
		RAMAddr          byte
		RAMAutoIncrement bool
		IRQCounter       uint16
		IRQEnabled       bool
		IRQPending       bool
		SoundDisabled    bool
	}{
		PRGBank0:         int32(m.prgBank0 & 0x7FFFFFFF),
		PRGBank1:         int32(m.prgBank1 & 0x7FFFFFFF),
		PRGBank2:         int32(m.prgBank2 & 0x7FFFFFFF),
		CHRBanks:         chrBanks,
		NTBanks:          m.ntBanks,
		RAMAddr:          m.ramAddr,
		RAMAutoIncrement: m.ramAutoIncrement,
		IRQCounter:       m.irqCounter,
		IRQEnabled:       m.irqEnabled,
		IRQPending:       m.irqPending,
		SoundDisabled:    m.soundDisabled,
	}

	if err := binary.Write(w, binary.LittleEndian, state); err != nil {
		return fmt.Errorf("write mapper19 state: %w", err)
	}

	if m.audio != nil {
		if err := m.audio.SaveState(w); err != nil {
			return fmt.Errorf("write mapper19 audio: %w", err)
		}
	}
	return nil
}

// LoadState restores the Namco 163 internal state.
func (m *Mapper19) LoadState(r io.Reader) error {
	var state struct {
		PRGBank0         int32
		PRGBank1         int32
		PRGBank2         int32
		CHRBanks         [8]int32
		NTBanks          [4]byte
		RAMAddr          byte
		RAMAutoIncrement bool
		IRQCounter       uint16
		IRQEnabled       bool
		IRQPending       bool
		SoundDisabled    bool
	}

	if err := binary.Read(r, binary.LittleEndian, &state); err != nil {
		return fmt.Errorf("read mapper19 state: %w", err)
	}

	m.prgBank0 = int(state.PRGBank0)
	m.prgBank1 = int(state.PRGBank1)
	m.prgBank2 = int(state.PRGBank2)
	for i, b := range state.CHRBanks {
		m.chrBanks[i] = int(b)
	}
	m.ntBanks = state.NTBanks
	m.ramAddr = state.RAMAddr
	m.ramAutoIncrement = state.RAMAutoIncrement
	m.irqCounter = state.IRQCounter
	m.irqEnabled = state.IRQEnabled
	m.irqPending = state.IRQPending
	m.soundDisabled = state.SoundDisabled

	if m.audio != nil {
		m.audio.SetSoundDisabled(m.soundDisabled)
		if err := m.audio.LoadState(r); err != nil {
			return fmt.Errorf("read mapper19 audio: %w", err)
		}
	}
	return nil
}

// Reset silences expansion audio and clears pending IRQs on console reset.
func (m *Mapper19) Reset() {
	m.irqPending = false
	if m.audio != nil {
		m.audio.Reset()
	}
}
