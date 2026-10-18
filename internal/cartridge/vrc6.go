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
	vrc6PRGBank16KSize = 16 * 1024
	vrc6PRGBank8KSize  = 8 * 1024
	vrc6CHRBank1KSize  = 1 * 1024
	vrc6CHRWindowCount = 8

	minVRC6PRGBanks8K  = 4
	minVRC6PRGBanks16K = 2
	minVRC6CHRBanks1K  = 8

	vrc6PRGRAMBase uint16 = 0x6000
	vrc6PRGRAMEnd  uint16 = 0x7FFF
	vrc6PRGROMBase uint16 = 0x8000

	vrc6RegGroupShift = 12
	vrc6RegGroupMask  = 0x0F

	vrc6GroupPRG16K    = 0x8
	vrc6GroupPulse1    = 0x9
	vrc6GroupPulse2    = 0xA
	vrc6GroupSawtooth  = 0xB
	vrc6GroupPRG8K     = 0xC
	vrc6GroupCHR0123   = 0xD
	vrc6GroupCHR4567   = 0xE
	vrc6GroupIRQ       = 0xF

	vrc6PRG16KMask     = 0x0F
	vrc6PRG8KMask      = 0x1F

	vrc6RAMEnableMask  = 0x80
	vrc6MirroringShift = 2
	vrc6MirroringMask  = 0x03

	vrc6IRQEnableAckMask = 0x01
	vrc6IRQEnableMask    = 0x02
	vrc6IRQModeCycleMask = 0x04
	vrc6CounterTripValue = 0xFF
	vrc6PrescalerInitial = 341
	vrc6PrescalerStep    = 3
	vrc6PrescalerReload  = 341

	vrc6Window16KOffset = 0x8000
	vrc6Window8KOffset  = 0xC000
	vrc6WindowLastOffset = 0xE000
)

// MapperVRC6 implements the Konami VRC6 ASIC mapper (iNES Mapper 24 and 26).
type MapperVRC6 struct {
	cartridge    *Cartridge
	mapperID     byte
	isVRC6b      bool // true for Mapper 26 (A0 and A1 swapped)
	prgBanks16K  int
	prgBanks8K   int
	chrBanks1K   int
	prgBank16K   int
	prgBank8K    int
	prgOffset16K int
	prgOffset8K  int
	prgOffsetLast int
	chrOffsets   [vrc6CHRWindowCount]int
	ramEnabled   bool
	irqLatch     byte
	irqCounter   byte
	irqEnabled   bool
	irqEnableAck bool
	irqModeCycle bool
	irqPrescaler int
	irqPending   bool
	audio        *VRC6Audio
}

// NewMapperVRC6 constructs and initializes a new VRC6 mapper instance.
func NewMapperVRC6(cart *Cartridge, mapperID byte) *MapperVRC6 {
	prg16K := max(len(cart.PRG)/vrc6PRGBank16KSize, minVRC6PRGBanks16K)
	prg8K := max(len(cart.PRG)/vrc6PRGBank8KSize, minVRC6PRGBanks8K)
	chr1K := max(len(cart.CHR)/vrc6CHRBank1KSize, minVRC6CHRBanks1K)

	if len(cart.SRAM) == 0 {
		cart.SRAM = make([]byte, SRAMSize)
	}

	m := &MapperVRC6{
		cartridge:     cart,
		mapperID:      mapperID,
		isVRC6b:       mapperID == mapperVRC6b,
		prgBanks16K:   prg16K,
		prgBanks8K:    prg8K,
		chrBanks1K:    chr1K,
		prgOffsetLast: (prg8K - 1) * vrc6PRGBank8KSize,
		irqPrescaler:  vrc6PrescalerInitial,
		audio:         NewVRC6Audio(),
	}

	cart.SetExpansionAudio(m.audio)
	m.updatePRG()

	for i := range vrc6CHRWindowCount {
		m.chrOffsets[i] = (i % m.chrBanks1K) * vrc6CHRBank1KSize
	}

	return m
}

func (m *MapperVRC6) decodeReg(addr uint16) byte {
	if m.isVRC6b {
		// Mapper 26: A0 is CPU A1, A1 is CPU A0
		r0 := byte((addr >> 1) & 1)
		r1 := byte(addr & 1)
		return (r1 << 1) | r0
	}
	// Mapper 24: A0 is CPU A0, A1 is CPU A1
	return byte(addr & 0x03)
}

func (m *MapperVRC6) updatePRG() {
	m.prgOffset16K = (m.prgBank16K % m.prgBanks16K) * vrc6PRGBank16KSize
	m.prgOffset8K = (m.prgBank8K % m.prgBanks8K) * vrc6PRGBank8KSize
}

// ReadPRG maps CPU reads across $6000-$FFFF.
func (m *MapperVRC6) ReadPRG(addr uint16) byte {
	switch {
	case addr >= vrc6PRGRAMBase && addr <= vrc6PRGRAMEnd:
		if (m.ramEnabled || m.cartridge.HasBattery) && len(m.cartridge.SRAM) > 0 {
			return m.cartridge.SRAM[int(addr-vrc6PRGRAMBase)%len(m.cartridge.SRAM)]
		}
		return 0

	case addr >= vrc6Window16KOffset && addr < vrc6Window8KOffset:
		offset := int(addr - vrc6Window16KOffset)
		idx := m.prgOffset16K + offset
		if idx < len(m.cartridge.PRG) {
			return m.cartridge.PRG[idx]
		}
		return 0

	case addr >= vrc6Window8KOffset && addr < vrc6WindowLastOffset:
		offset := int(addr - vrc6Window8KOffset)
		idx := m.prgOffset8K + offset
		if idx < len(m.cartridge.PRG) {
			return m.cartridge.PRG[idx]
		}
		return 0

	case addr >= vrc6WindowLastOffset:
		offset := int(addr - vrc6WindowLastOffset)
		idx := m.prgOffsetLast + offset
		if idx < len(m.cartridge.PRG) {
			return m.cartridge.PRG[idx]
		}
		return 0

	default:
		return 0
	}
}

// WritePRG maps CPU writes across $6000-$FFFF.
func (m *MapperVRC6) WritePRG(addr uint16, data byte) {
	switch {
	case addr >= vrc6PRGRAMBase && addr <= vrc6PRGRAMEnd:
		if (m.ramEnabled || m.cartridge.HasBattery) && len(m.cartridge.SRAM) > 0 {
			m.cartridge.SRAM[int(addr-vrc6PRGRAMBase)%len(m.cartridge.SRAM)] = data
			m.cartridge.MarkSRAMDirty()
		}

	case addr >= vrc6PRGROMBase:
		m.writeRegister(addr, data)
	}
}

func (m *MapperVRC6) writeRegister(addr uint16, data byte) {
	group := (addr >> vrc6RegGroupShift) & vrc6RegGroupMask
	reg := m.decodeReg(addr)

	switch group {
	case vrc6GroupPRG16K:
		m.prgBank16K = int(data & vrc6PRG16KMask)
		m.updatePRG()

	case vrc6GroupPulse1:
		m.audio.WriteRegister(0x9000|uint16(reg), data)

	case vrc6GroupPulse2:
		m.audio.WriteRegister(0xA000|uint16(reg), data)

	case vrc6GroupSawtooth:
		if reg == 3 {
			m.handleBankingStyle(data)
			return
		}
		m.audio.WriteRegister(0xB000|uint16(reg), data)

	case vrc6GroupPRG8K:
		m.prgBank8K = int(data & vrc6PRG8KMask)
		m.updatePRG()

	case vrc6GroupCHR0123:
		slot := int(reg)
		m.chrOffsets[slot] = (int(data) % m.chrBanks1K) * vrc6CHRBank1KSize

	case vrc6GroupCHR4567:
		slot := 4 + int(reg)
		m.chrOffsets[slot] = (int(data) % m.chrBanks1K) * vrc6CHRBank1KSize

	case vrc6GroupIRQ:
		m.handleIRQWrite(reg, data)
	}
}

func (m *MapperVRC6) handleBankingStyle(data byte) {
	m.ramEnabled = (data & vrc6RAMEnableMask) != 0

	mirrorMode := (data >> vrc6MirroringShift) & vrc6MirroringMask
	switch mirrorMode {
	case 0:
		m.cartridge.Mirroring = MirrorVertical
	case 1:
		m.cartridge.Mirroring = MirrorHorizontal
	case 2:
		m.cartridge.Mirroring = MirrorSingle0
	case 3:
		m.cartridge.Mirroring = MirrorSingle1
	}
}

func (m *MapperVRC6) handleIRQWrite(reg byte, data byte) {
	switch reg {
	case 0:
		m.irqLatch = data
	case 1:
		m.irqEnableAck = (data & vrc6IRQEnableAckMask) != 0
		m.irqEnabled = (data & vrc6IRQEnableMask) != 0
		m.irqModeCycle = (data & vrc6IRQModeCycleMask) != 0
		if m.irqEnabled {
			m.irqCounter = m.irqLatch
		}
		m.irqPrescaler = vrc6PrescalerInitial
		m.irqPending = false
	case 2:
		m.irqPending = false
		m.irqEnabled = m.irqEnableAck
	}
}

// ReadCHR maps PPU reads across $0000-$1FFF to banked 1KB CHR-ROM/RAM.
func (m *MapperVRC6) ReadCHR(addr uint16) byte {
	if len(m.cartridge.CHR) == 0 {
		return 0
	}
	slot := (addr / vrc6CHRBank1KSize) & 7
	offset := addr % vrc6CHRBank1KSize
	idx := (m.chrOffsets[slot] + int(offset)) % len(m.cartridge.CHR)
	return m.cartridge.CHR[idx]
}

// WriteCHR maps PPU writes across $0000-$1FFF to banked 1KB CHR-RAM.
func (m *MapperVRC6) WriteCHR(addr uint16, data byte) {
	if len(m.cartridge.CHR) == 0 {
		return
	}
	slot := (addr / vrc6CHRBank1KSize) & 7
	offset := addr % vrc6CHRBank1KSize
	idx := (m.chrOffsets[slot] + int(offset)) % len(m.cartridge.CHR)
	m.cartridge.CHR[idx] = data
}

// StepCPU decrements the scanline prescaler or cycle counter and clocks the IRQ counter.
func (m *MapperVRC6) StepCPU(cycles int) {
	if !m.irqEnabled || cycles <= 0 {
		return
	}

	for range cycles {
		if m.irqModeCycle {
			m.clockIRQ()
			continue
		}

		m.irqPrescaler -= vrc6PrescalerStep
		if m.irqPrescaler <= 0 {
			m.irqPrescaler += vrc6PrescalerReload
			m.clockIRQ()
		}
	}
}

func (m *MapperVRC6) clockIRQ() {
	if m.irqCounter == vrc6CounterTripValue {
		m.irqCounter = m.irqLatch
		m.irqPending = true
		return
	}
	m.irqCounter++
}

// IRQPending reports whether the VRC6 IRQ line is currently asserted.
func (m *MapperVRC6) IRQPending() bool {
	return m.irqPending
}

// SaveState serializes Konami VRC6 internal register and audio state.
func (m *MapperVRC6) SaveState(w io.Writer) error {
	mirror := byte(MirrorVertical)
	if m.cartridge != nil {
		mirror = byte(m.cartridge.Mirroring)
	}
	var chrOffsets [vrc6CHRWindowCount]int32
	for i, off := range m.chrOffsets {
		chrOffsets[i] = int32(off & 0x7FFFFFFF)
	}

	state := struct {
		PRGBank16K   int32
		PRGBank8K    int32
		CHROffsets   [vrc6CHRWindowCount]int32
		RAMEnabled   bool
		IRQLatch     byte
		IRQCounter   byte
		IRQEnabled   bool
		IRQEnableAck bool
		IRQModeCycle bool
		IRQPrescaler int32
		IRQPending   bool
		Mirror       byte
		// Pulse1
		P1Vol     byte
		P1Duty    byte
		P1Mode    bool
		P1Enabled bool
		P1Period  int32
		P1Timer   int32
		P1Step    int32
		// Pulse2
		P2Vol     byte
		P2Duty    byte
		P2Mode    bool
		P2Enabled bool
		P2Period  int32
		P2Timer   int32
		P2Step    int32
		// Sawtooth
		SawAccumRate byte
		SawEnabled   bool
		SawPeriod    int32
		SawTimer     int32
		SawStep      int32
		SawAccum     byte
		AudioHalt    bool
	}{
		PRGBank16K:   int32(m.prgBank16K & 0x7FFFFFFF),
		PRGBank8K:    int32(m.prgBank8K & 0x7FFFFFFF),
		CHROffsets:   chrOffsets,
		RAMEnabled:   m.ramEnabled,
		IRQLatch:     m.irqLatch,
		IRQCounter:   m.irqCounter,
		IRQEnabled:   m.irqEnabled,
		IRQEnableAck: m.irqEnableAck,
		IRQModeCycle: m.irqModeCycle,
		IRQPrescaler: int32(m.irqPrescaler & 0x7FFFFFFF),
		IRQPending:   m.irqPending,
		Mirror:       mirror,
		P1Vol:        m.audio.pulse1.volume,
		P1Duty:       m.audio.pulse1.duty,
		P1Mode:       m.audio.pulse1.mode,
		P1Enabled:    m.audio.pulse1.enabled,
		P1Period:     int32(m.audio.pulse1.period & 0x7FFFFFFF),
		P1Timer:      int32(m.audio.pulse1.timer & 0x7FFFFFFF),
		P1Step:       int32(m.audio.pulse1.step & 0x7FFFFFFF),
		P2Vol:        m.audio.pulse2.volume,
		P2Duty:       m.audio.pulse2.duty,
		P2Mode:       m.audio.pulse2.mode,
		P2Enabled:    m.audio.pulse2.enabled,
		P2Period:     int32(m.audio.pulse2.period & 0x7FFFFFFF),
		P2Timer:      int32(m.audio.pulse2.timer & 0x7FFFFFFF),
		P2Step:       int32(m.audio.pulse2.step & 0x7FFFFFFF),
		SawAccumRate: m.audio.sawtooth.accumRate,
		SawEnabled:   m.audio.sawtooth.enabled,
		SawPeriod:    int32(m.audio.sawtooth.period & 0x7FFFFFFF),
		SawTimer:     int32(m.audio.sawtooth.timer & 0x7FFFFFFF),
		SawStep:      int32(m.audio.sawtooth.step & 0x7FFFFFFF),
		SawAccum:     m.audio.sawtooth.accum,
		AudioHalt:    m.audio.halt,
	}

	if err := binary.Write(w, binary.LittleEndian, state); err != nil {
		return fmt.Errorf("write mapper vrc6 state: %w", err)
	}
	return nil
}

// LoadState restores Konami VRC6 internal register and audio state.
func (m *MapperVRC6) LoadState(r io.Reader) error {
	var state struct {
		PRGBank16K   int32
		PRGBank8K    int32
		CHROffsets   [vrc6CHRWindowCount]int32
		RAMEnabled   bool
		IRQLatch     byte
		IRQCounter   byte
		IRQEnabled   bool
		IRQEnableAck bool
		IRQModeCycle bool
		IRQPrescaler int32
		IRQPending   bool
		Mirror       byte
		// Pulse1
		P1Vol     byte
		P1Duty    byte
		P1Mode    bool
		P1Enabled bool
		P1Period  int32
		P1Timer   int32
		P1Step    int32
		// Pulse2
		P2Vol     byte
		P2Duty    byte
		P2Mode    bool
		P2Enabled bool
		P2Period  int32
		P2Timer   int32
		P2Step    int32
		// Sawtooth
		SawAccumRate byte
		SawEnabled   bool
		SawPeriod    int32
		SawTimer     int32
		SawStep      int32
		SawAccum     byte
		AudioHalt    bool
	}
	if err := binary.Read(r, binary.LittleEndian, &state); err != nil {
		return fmt.Errorf("read mapper vrc6 state: %w", err)
	}
	m.prgBank16K = int(state.PRGBank16K)
	m.prgBank8K = int(state.PRGBank8K)
	for i, off := range state.CHROffsets {
		m.chrOffsets[i] = int(off)
	}
	m.ramEnabled = state.RAMEnabled
	m.irqLatch = state.IRQLatch
	m.irqCounter = state.IRQCounter
	m.irqEnabled = state.IRQEnabled
	m.irqEnableAck = state.IRQEnableAck
	m.irqModeCycle = state.IRQModeCycle
	m.irqPrescaler = int(state.IRQPrescaler)
	m.irqPending = state.IRQPending
	if m.cartridge != nil {
		m.cartridge.Mirroring = Mirroring(state.Mirror)
	}
	m.updatePRG()

	m.audio.pulse1.volume = state.P1Vol
	m.audio.pulse1.duty = state.P1Duty
	m.audio.pulse1.mode = state.P1Mode
	m.audio.pulse1.enabled = state.P1Enabled
	m.audio.pulse1.period = int(state.P1Period)
	m.audio.pulse1.timer = int(state.P1Timer)
	m.audio.pulse1.step = int(state.P1Step)

	m.audio.pulse2.volume = state.P2Vol
	m.audio.pulse2.duty = state.P2Duty
	m.audio.pulse2.mode = state.P2Mode
	m.audio.pulse2.enabled = state.P2Enabled
	m.audio.pulse2.period = int(state.P2Period)
	m.audio.pulse2.timer = int(state.P2Timer)
	m.audio.pulse2.step = int(state.P2Step)

	m.audio.sawtooth.accumRate = state.SawAccumRate
	m.audio.sawtooth.enabled = state.SawEnabled
	m.audio.sawtooth.period = int(state.SawPeriod)
	m.audio.sawtooth.timer = int(state.SawTimer)
	m.audio.sawtooth.step = int(state.SawStep)
	m.audio.sawtooth.accum = state.SawAccum
	m.audio.halt = state.AudioHalt

	return nil
}

// Reset silences expansion audio and clears pending IRQs on console reset.
func (m *MapperVRC6) Reset() {
	m.irqPending = false
	m.audio.Reset()
}
