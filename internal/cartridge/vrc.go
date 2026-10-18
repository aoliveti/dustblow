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
	vrcPRGBankSize8K  = 8 * 1024
	vrcCHRBankSize1K  = 1 * 1024
	vrcPRGWindowCount = 4
	vrcCHRWindowCount = 8

	minVRCPRGBanks8K = 4
	minVRCCHRBanks1K = 8

	vrcPRGRAMBase uint16 = 0x6000
	vrcPRGRAMEnd  uint16 = 0x7FFF
	vrcPRGROMBase uint16 = 0x8000

	vrcRegBaseShift     = 12
	vrcRegBaseGroupMask = 0x07

	vrcBasePRGSelect0 = 0 // $8000
	vrcBaseControl    = 1 // $9000
	vrcBasePRGSelect1 = 2 // $A000
	vrcBaseCHR01      = 3 // $B000
	vrcBaseCHR23      = 4 // $C000
	vrcBaseCHR45      = 5 // $D000
	vrcBaseCHR67      = 6 // $E000
	vrcBaseIRQ        = 7 // $F000

	vrcPRGBankMask5Bit = 0x1F
	vrcCHRLowMask      = 0x0F
	vrcCHRHighMaskVRC4 = 0x1F
	vrcCHRHighMaskVRC2 = 0x0F

	vrcWRAMEnableMask = 0x01
	vrcSwapModeMask   = 0x02

	vrcIRQEnableAckMask  = 0x01
	vrcIRQEnableMask     = 0x02
	vrcIRQModeCycleMask  = 0x04
	vrcCounterTripValue  = 0xFF
	vrcPrescalerInitial  = 341
	vrcPrescalerStep     = 3
	vrcPrescalerReload   = 341
	vrcOpenBusHighMask   = 0xFE
	vrcWaiWaiMirrorMagic = 0xFF
)

type vrcRegMode int

const (
	vrcRegMode21Auto vrcRegMode = iota
	vrcRegMode21a
	vrcRegMode21c
	vrcRegMode22
	vrcRegMode23Auto
	vrcRegMode23f
	vrcRegMode23e
	vrcRegMode25Auto
	vrcRegMode25b
	vrcRegMode25d
)

// MapperVRC implements the Konami VRC2 and VRC4 ASIC mapper family (Mappers 21, 22, 23, 25).
type MapperVRC struct {
	cartridge    *Cartridge
	mapperID     byte
	submapper    byte
	regMode      vrcRegMode
	isVRC2       bool
	isVRC2a      bool
	prgBanks8K   int
	chrBanks1K   int
	chrHighMask  byte
	prgOffsets   [vrcPRGWindowCount]int
	chrOffsets   [vrcCHRWindowCount]int
	prgBank0     int
	prgBank1     int
	prgSwapMode  int
	wramEnabled  bool
	latch        byte
	chrLow       [vrcCHRWindowCount]byte
	chrHigh      [vrcCHRWindowCount]byte
	irqLatch     byte
	irqCounter   byte
	irqEnabled   bool
	irqEnableAck bool
	irqModeCycle bool
	irqPrescaler int
	irqPending   bool
}

// NewMapperVRC constructs a new Konami VRC2/VRC4 mapper instance.
func NewMapperVRC(cart *Cartridge, mapperID, submapper byte) *MapperVRC {
	prgBanks := max(len(cart.PRG)/vrcPRGBankSize8K, minVRCPRGBanks8K)
	chrBanks := max(len(cart.CHR)/vrcCHRBankSize1K, minVRCCHRBanks1K)

	if len(cart.SRAM) == 0 {
		cart.SRAM = make([]byte, SRAMSize)
	}

	m := &MapperVRC{
		cartridge:    cart,
		mapperID:     mapperID,
		submapper:    submapper,
		prgBanks8K:   prgBanks,
		chrBanks1K:   chrBanks,
		chrHighMask:  vrcCHRHighMaskVRC4,
		irqPrescaler: vrcPrescalerInitial,
	}

	m.configureVariant(mapperID, submapper)
	m.updatePRG()
	for slot := range vrcCHRWindowCount {
		m.updateCHR(slot)
	}

	return m
}

func (m *MapperVRC) configureVariant(mapperID, submapper byte) {
	switch mapperID {
	case mapperVRC4a4c:
		m.configureVRC4a4c(submapper)
	case mapperVRC2a:
		m.isVRC2 = true
		m.isVRC2a = true
		m.regMode = vrcRegMode22
		m.chrHighMask = vrcCHRHighMaskVRC2
	case mapperVRC2b4e:
		m.configureVRC2b4e(submapper)
	case mapperVRC2c4b:
		m.configureVRC2c4b(submapper)
	}
}

func (m *MapperVRC) configureVRC4a4c(submapper byte) {
	switch submapper {
	case 1:
		m.regMode = vrcRegMode21a
	case 2:
		m.regMode = vrcRegMode21c
	default:
		m.regMode = vrcRegMode21Auto
	}
}

func (m *MapperVRC) configureVRC2b4e(submapper byte) {
	switch submapper {
	case 1:
		m.regMode = vrcRegMode23f
	case 2:
		m.regMode = vrcRegMode23e
	case 3:
		m.isVRC2 = true
		m.regMode = vrcRegMode23f
		m.chrHighMask = vrcCHRHighMaskVRC2
	default:
		m.regMode = vrcRegMode23Auto
	}
}

func (m *MapperVRC) configureVRC2c4b(submapper byte) {
	switch submapper {
	case 1:
		m.regMode = vrcRegMode25b
	case 2:
		m.regMode = vrcRegMode25d
	case 3:
		m.isVRC2 = true
		m.regMode = vrcRegMode25b
		m.chrHighMask = vrcCHRHighMaskVRC2
	default:
		m.regMode = vrcRegMode25Auto
	}
}

func (m *MapperVRC) decodeReg(addr uint16) byte {
	var r0, r1 byte
	switch m.regMode {
	case vrcRegMode21a:
		r0 = byte((addr >> 1) & 1)
		r1 = byte((addr >> 2) & 1)
	case vrcRegMode21c:
		r0 = byte((addr >> 6) & 1)
		r1 = byte((addr >> 7) & 1)
	case vrcRegMode21Auto:
		r0 = byte((addr>>1)&1) | byte((addr>>6)&1)
		r1 = byte((addr>>2)&1) | byte((addr>>7)&1)
	case vrcRegMode22:
		r0 = byte((addr >> 1) & 1)
		r1 = byte(addr & 1)
	case vrcRegMode23f:
		r0 = byte(addr & 1)
		r1 = byte((addr >> 1) & 1)
	case vrcRegMode23e:
		r0 = byte((addr >> 2) & 1)
		r1 = byte((addr >> 3) & 1)
	case vrcRegMode23Auto:
		r0 = byte(addr&1) | byte((addr>>2)&1)
		r1 = byte((addr>>1)&1) | byte((addr>>3)&1)
	case vrcRegMode25b:
		r0 = byte((addr >> 1) & 1)
		r1 = byte(addr & 1)
	case vrcRegMode25d:
		r0 = byte((addr >> 3) & 1)
		r1 = byte((addr >> 2) & 1)
	case vrcRegMode25Auto:
		r0 = byte((addr>>1)&1) | byte((addr>>3)&1)
		r1 = byte(addr&1) | byte((addr>>2)&1)
	}
	return (r1 << 1) | r0
}

func (m *MapperVRC) updatePRG() {
	bankLast := m.prgBanks8K - 1
	bankSecondLast := max(m.prgBanks8K-2, 0)
	bank0 := m.prgBank0 % m.prgBanks8K
	bank1 := m.prgBank1 % m.prgBanks8K

	if m.prgSwapMode == 0 {
		m.prgOffsets[0] = bank0 * vrcPRGBankSize8K
		m.prgOffsets[1] = bank1 * vrcPRGBankSize8K
		m.prgOffsets[2] = bankSecondLast * vrcPRGBankSize8K
		m.prgOffsets[3] = bankLast * vrcPRGBankSize8K
		return
	}

	m.prgOffsets[0] = bankSecondLast * vrcPRGBankSize8K
	m.prgOffsets[1] = bank1 * vrcPRGBankSize8K
	m.prgOffsets[2] = bank0 * vrcPRGBankSize8K
	m.prgOffsets[3] = bankLast * vrcPRGBankSize8K
}

func (m *MapperVRC) updateCHR(slot int) {
	val := int(m.chrHigh[slot])<<4 | int(m.chrLow[slot])
	if m.isVRC2a {
		val >>= 1
	}
	m.chrOffsets[slot] = (val % m.chrBanks1K) * vrcCHRBankSize1K
}

// ReadPRG maps CPU reads across $6000-$FFFF.
func (m *MapperVRC) ReadPRG(addr uint16) byte {
	switch {
	case addr >= vrcPRGRAMBase && addr <= vrcPRGRAMEnd:
		if m.isVRC2 {
			if m.cartridge.HasBattery {
				return m.cartridge.SRAM[int(addr-vrcPRGRAMBase)%len(m.cartridge.SRAM)]
			}
			return (byte(addr>>8) & vrcOpenBusHighMask) | m.latch
		}
		if m.wramEnabled && len(m.cartridge.SRAM) > 0 {
			return m.cartridge.SRAM[int(addr-vrcPRGRAMBase)%len(m.cartridge.SRAM)]
		}
		return (byte(addr>>8) & vrcOpenBusHighMask) | m.latch

	case addr >= vrcPRGROMBase:
		window := (addr - vrcPRGROMBase) / vrcPRGBankSize8K
		offset := (addr - vrcPRGROMBase) % vrcPRGBankSize8K
		idx := m.prgOffsets[window] + int(offset)
		if idx < len(m.cartridge.PRG) {
			return m.cartridge.PRG[idx]
		}
		return 0

	default:
		return 0
	}
}

// WritePRG maps CPU writes across $6000-$FFFF.
func (m *MapperVRC) WritePRG(addr uint16, data byte) {
	switch {
	case addr >= vrcPRGRAMBase && addr <= vrcPRGRAMEnd:
		m.latch = data & 1
		if (m.isVRC2 && m.cartridge.HasBattery) || (!m.isVRC2 && m.wramEnabled) {
			m.cartridge.SRAM[int(addr-vrcPRGRAMBase)%len(m.cartridge.SRAM)] = data
			m.cartridge.MarkSRAMDirty()
		}

	case addr >= vrcPRGROMBase:
		m.writeRegister(addr, data)
	}
}

func (m *MapperVRC) writeRegister(addr uint16, data byte) {
	baseGroup := (addr >> vrcRegBaseShift) & vrcRegBaseGroupMask
	reg := m.decodeReg(addr)

	switch baseGroup {
	case vrcBasePRGSelect0:
		m.prgBank0 = int(data & vrcPRGBankMask5Bit)
		m.updatePRG()

	case vrcBaseControl:
		if reg == 0 {
			m.applyMirroring(data)
			return
		}
		if reg == 2 && !m.isVRC2 {
			m.wramEnabled = (data & vrcWRAMEnableMask) != 0
			m.prgSwapMode = int((data & vrcSwapModeMask) >> 1)
			m.updatePRG()
		}

	case vrcBasePRGSelect1:
		m.prgBank1 = int(data & vrcPRGBankMask5Bit)
		m.updatePRG()

	case vrcBaseCHR01, vrcBaseCHR23, vrcBaseCHR45, vrcBaseCHR67:
		slot := int(baseGroup-vrcBaseCHR01)*2 + int(reg>>1)
		if (reg & 1) == 0 {
			m.chrLow[slot] = data & vrcCHRLowMask
			m.updateCHR(slot)
			break
		}
		m.chrHigh[slot] = data & m.chrHighMask
		m.updateCHR(slot)

	case vrcBaseIRQ:
		if !m.isVRC2 {
			m.writeIRQ(reg, data)
		}
	}
}

func (m *MapperVRC) writeIRQ(reg, data byte) {
	switch reg {
	case 0:
		m.irqLatch = (m.irqLatch & 0xF0) | (data & 0x0F)
	case 1:
		m.irqLatch = (m.irqLatch & 0x0F) | ((data & 0x0F) << 4)
	case 2:
		m.irqEnableAck = (data & vrcIRQEnableAckMask) != 0
		m.irqEnabled = (data & vrcIRQEnableMask) != 0
		m.irqModeCycle = (data & vrcIRQModeCycleMask) != 0
		if m.irqEnabled {
			m.irqCounter = m.irqLatch
		}
		m.irqPrescaler = vrcPrescalerInitial
		m.irqPending = false
	case 3:
		m.irqPending = false
		m.irqEnabled = m.irqEnableAck
	}
}

func (m *MapperVRC) applyMirroring(data byte) {
	if m.isVRC2 {
		if (data & 0x01) == 0 {
			m.cartridge.Mirroring = MirrorVertical
			return
		}
		m.cartridge.Mirroring = MirrorHorizontal
		return
	}

	if data == vrcWaiWaiMirrorMagic {
		m.cartridge.Mirroring = MirrorHorizontal
		return
	}

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
}

// ReadCHR maps PPU reads across $0000-$1FFF to banked 1KB CHR-ROM/RAM.
func (m *MapperVRC) ReadCHR(addr uint16) byte {
	if len(m.cartridge.CHR) == 0 {
		return 0
	}
	slot := (addr / vrcCHRBankSize1K) & 7
	offset := addr % vrcCHRBankSize1K
	idx := (m.chrOffsets[slot] + int(offset)) % len(m.cartridge.CHR)
	return m.cartridge.CHR[idx]
}

// WriteCHR maps PPU writes across $0000-$1FFF to banked 1KB CHR-RAM.
func (m *MapperVRC) WriteCHR(addr uint16, data byte) {
	if len(m.cartridge.CHR) == 0 {
		return
	}
	slot := (addr / vrcCHRBankSize1K) & 7
	offset := addr % vrcCHRBankSize1K
	idx := (m.chrOffsets[slot] + int(offset)) % len(m.cartridge.CHR)
	m.cartridge.CHR[idx] = data
}

// StepCPU decrements the scanline prescaler or cycle counter and clocks the IRQ counter.
func (m *MapperVRC) StepCPU(cycles int) {
	if m.isVRC2 || !m.irqEnabled || cycles <= 0 {
		return
	}

	for range cycles {
		if m.irqModeCycle {
			m.clockIRQ()
			continue
		}

		m.irqPrescaler -= vrcPrescalerStep
		if m.irqPrescaler <= 0 {
			m.irqPrescaler += vrcPrescalerReload
			m.clockIRQ()
		}
	}
}

func (m *MapperVRC) clockIRQ() {
	if m.irqCounter == vrcCounterTripValue {
		m.irqCounter = m.irqLatch
		m.irqPending = true
		return
	}
	m.irqCounter++
}

// IRQPending reports whether the VRC4 IRQ line is currently asserted.
func (m *MapperVRC) IRQPending() bool {
	return m.irqPending
}

// SaveState serializes Konami VRC2/VRC4 internal register state.
func (m *MapperVRC) SaveState(w io.Writer) error {
	mirror := byte(MirrorVertical)
	if m.cartridge != nil {
		mirror = byte(m.cartridge.Mirroring)
	}
	state := struct {
		RegMode      int32
		PRGBank0     int32
		PRGBank1     int32
		PRGSwapMode  int32
		WRAMEnabled  bool
		Latch        byte
		CHRLow       [vrcCHRWindowCount]byte
		CHRHigh      [vrcCHRWindowCount]byte
		IRQLatch     byte
		IRQCounter   byte
		IRQEnabled   bool
		IRQEnableAck bool
		IRQModeCycle bool
		IRQPrescaler int32
		IRQPending   bool
		Mirror       byte
	}{
		RegMode:      int32(m.regMode & 0x7FFFFFFF),
		PRGBank0:     int32(m.prgBank0 & 0x7FFFFFFF),
		PRGBank1:     int32(m.prgBank1 & 0x7FFFFFFF),
		PRGSwapMode:  int32(m.prgSwapMode & 0x7FFFFFFF),
		WRAMEnabled:  m.wramEnabled,
		Latch:        m.latch,
		CHRLow:       m.chrLow,
		CHRHigh:      m.chrHigh,
		IRQLatch:     m.irqLatch,
		IRQCounter:   m.irqCounter,
		IRQEnabled:   m.irqEnabled,
		IRQEnableAck: m.irqEnableAck,
		IRQModeCycle: m.irqModeCycle,
		IRQPrescaler: int32(m.irqPrescaler & 0x7FFFFFFF),
		IRQPending:   m.irqPending,
		Mirror:       mirror,
	}
	if err := binary.Write(w, binary.LittleEndian, state); err != nil {
		return fmt.Errorf("write mapper vrc state: %w", err)
	}
	return nil
}

// LoadState restores Konami VRC2/VRC4 internal register state.
func (m *MapperVRC) LoadState(r io.Reader) error {
	var state struct {
		RegMode      int32
		PRGBank0     int32
		PRGBank1     int32
		PRGSwapMode  int32
		WRAMEnabled  bool
		Latch        byte
		CHRLow       [vrcCHRWindowCount]byte
		CHRHigh      [vrcCHRWindowCount]byte
		IRQLatch     byte
		IRQCounter   byte
		IRQEnabled   bool
		IRQEnableAck bool
		IRQModeCycle bool
		IRQPrescaler int32
		IRQPending   bool
		Mirror       byte
	}
	if err := binary.Read(r, binary.LittleEndian, &state); err != nil {
		return fmt.Errorf("read mapper vrc state: %w", err)
	}
	m.regMode = vrcRegMode(state.RegMode)
	m.prgBank0 = int(state.PRGBank0)
	m.prgBank1 = int(state.PRGBank1)
	m.prgSwapMode = int(state.PRGSwapMode)
	m.wramEnabled = state.WRAMEnabled
	m.latch = state.Latch
	m.chrLow = state.CHRLow
	m.chrHigh = state.CHRHigh
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
	for slot := range vrcCHRWindowCount {
		m.updateCHR(slot)
	}
	return nil
}

// Reset clears pending IRQ state on console reset.
func (m *MapperVRC) Reset() {
	m.irqPending = false
}
