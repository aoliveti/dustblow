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
	mmc5PRGBankSize8K  = 0x2000 // 8 KB PRG bank size
	mmc5PRGBankSize16K = 0x4000 // 16 KB PRG bank size
	mmc5PRGBankSize32K = 0x8000 // 32 KB PRG bank size

	mmc5CHRBankSize1K = 0x0400 // 1 KB CHR bank size
	mmc5CHRBankSize2K = 0x0800 // 2 KB CHR bank size
	mmc5CHRBankSize4K = 0x1000 // 4 KB CHR bank size
	mmc5CHRBankSize8K = 0x2000 // 8 KB CHR bank size

	mmc5PRGWindowMask8K  = 0x1FFF
	mmc5CHRWindowMask1K  = 0x03FF
	mmc5NametableWrap    = 0x0FFF
	mmc5NametablePage    = 0x0400
	mmc5AttrTableOffset  = 0x03C0

	mmc5ExRAMSize    = 1024
	mmc5ExRAMBase    = 0x5C00
	mmc5ExRAMEnd     = 0x5FFF
	mmc5PRGRAMBase   = 0x6000
	mmc5PRGRAMEnd    = 0x7FFF
	mmc5PRGRAMSize64 = 64 * 1024

	mmc5PRGWindowCount = 4
	mmc5CHRWindowCount = 8

	mmc5RegPRGMode      = 0x5100
	mmc5RegCHRMode      = 0x5101
	mmc5RegRAMProtect1  = 0x5102
	mmc5RegRAMProtect2  = 0x5103
	mmc5RegExRAMMode    = 0x5104
	mmc5RegNametableMap = 0x5105
	mmc5RegFillTile     = 0x5106
	mmc5RegFillPalette  = 0x5107
	mmc5RegPRGRAMBank   = 0x5113
	mmc5RegPRGBank0     = 0x5114
	mmc5RegPRGBank1     = 0x5115
	mmc5RegPRGBank2     = 0x5116
	mmc5RegPRGBank3     = 0x5117
	mmc5RegCHRSprite0   = 0x5120
	mmc5RegCHRSprite7   = 0x5127
	mmc5RegCHRBGBase    = 0x5128
	mmc5RegCHRBGEnd     = 0x512B
	mmc5RegCHRHigh      = 0x5130
	mmc5RegSplitMode    = 0x5200
	mmc5RegSplitScroll  = 0x5201
	mmc5RegSplitBank    = 0x5202
	mmc5RegIRQCompare   = 0x5203
	mmc5RegIRQStatus    = 0x5204
	mmc5RegMultA        = 0x5205
	mmc5RegMultB        = 0x5206

	mmc5Protect1Enable = 0x02
	mmc5Protect2Enable = 0x01

	mmc5MaskIRQPending = 0x80
	mmc5MaskInFrame    = 0x40
	mmc5MaskPRGBit7    = 0x80
	mmc5MaskPRGBankROM = 0x7F
	mmc5MaskPRGBankRAM = 0x07
	mmc5MaskCHRHigh    = 0x03

	mmc5ScanlineVisibleCount   = 240
	mmc5NametableQuadrantShift = 2
	mmc5NametableModeMask      = 0x03

	minMMC5PRGBanks8K = 4
	minMMC5CHRBanks1K = 8

	mmc5RegModeMask = 0x03

	// ExRAM modes ($5104)
	mmc5ExRAMModeNametable    = 0 // %00: ExRAM used as extra nametable
	mmc5ExRAMModeExtendedAttr = 1 // %01: Extended Attribute Mode
	mmc5ExRAMModeReadWrite    = 2 // %10: Work RAM ($5C00-$5FFF read/write)
	mmc5ExRAMModeReadOnly     = 3 // %11: Work RAM ($5C00-$5FFF read-only)

	// Nametable sources ($5105)
	mmc5NametableSourceCIRAM0 = 0 // CIRAM Page 0 ($2000)
	mmc5NametableSourceCIRAM1 = 1 // CIRAM Page 1 ($2400)
	mmc5NametableSourceExRAM  = 2 // MMC5 Internal ExRAM
	mmc5NametableSourceFill   = 3 // Fill-mode tile & attribute

	// PRG banking modes ($5100)
	mmc5PRGMode32K   = 0 // One 32KB PRG bank
	mmc5PRGMode16K   = 1 // Two 16KB PRG banks
	mmc5PRGMode16K8K = 2 // One 16KB bank and two 8KB PRG banks
	mmc5PRGMode8K    = 3 // Four 8KB PRG banks

	// CHR banking modes ($5101)
	mmc5CHRMode8K = 0 // 8KB CHR bank
	mmc5CHRMode4K = 1 // 4KB CHR banks
	mmc5CHRMode2K = 2 // 2KB CHR banks
	mmc5CHRMode1K = 3 // 1KB CHR banks

	// Last CHR register set written
	mmc5CHRSetSprites = 0 // Set A ($5120-$5127) used for sprites or BG
	mmc5CHRSetBG      = 1 // Set B ($5128-$512B) used for BG

	// Extended Attribute Mode bit masks and shifts
	mmc5ExAttrCHRBankMask  = 0x3F // Bits 5-0: 4KB CHR bank index
	mmc5ExAttrPaletteShift = 6    // Bits 7-6: 8x8 tile palette index
	mmc5ExAttrPaletteMask  = 0x03
	mmc5ExAttrCHRHighShift = 6    // $5130 high bits shifted above 6-bit bank index
	mmc5CHRWindowMask4K    = 0x0FFF
)

// Mapper5 implements the Nintendo MMC5 / ExROM ASIC mapper (iNES Mapper 5).
// Supports games like Castlevania III (USA), Nobunaga's Ambition II, Romance of the Three Kingdoms II,
// Uncharted Waters, Gemfire, L'Empereur, Laser Invasion, and Just Breed.
type Mapper5 struct {
	cartridge     *Cartridge
	prgBanks8K    int
	chrBanks1K    int
	prgMode       int
	chrMode       int
	ramProtect1   byte
	ramProtect2   byte
	exRAMMode     byte
	nametableMap  byte
	fillTile      byte
	fillPalette   byte
	prgRAMBank    byte
	prgRegs       [mmc5PRGWindowCount]byte
	chrRegsA      [mmc5CHRWindowCount]int
	chrRegsB      [4]int
	chrHigh       int
	lastCHRSet    byte
	isSpriteFetch bool
	exRAMLatch    byte
	exRAMLatched  bool
	multA           byte
	multB           byte
	irqCompare      int
	irqEnabled      bool
	irqPending      bool
	inFrame         bool
	exRAM           [mmc5ExRAMSize]byte
	prgRAM          []byte
	prgOffsets      [mmc5PRGWindowCount]int
	prgIsRAM        [mmc5PRGWindowCount]bool
	chrOffsetsA     [mmc5CHRWindowCount]int
	chrOffsetsB     [mmc5CHRWindowCount]int
	audio           *MMC5Audio
}

// NewMapper5 creates and initializes a new Mapper5 (MMC5) instance.
func NewMapper5(cart *Cartridge) *Mapper5 {
	prgBanks := max(len(cart.PRG)/mmc5PRGBankSize8K, minMMC5PRGBanks8K)
	chrBanks := max(len(cart.CHR)/mmc5CHRBankSize1K, minMMC5CHRBanks1K)

	ramSize := max(mmc5PRGRAMSize64, len(cart.SRAM))
	ram := make([]byte, ramSize)
	if len(cart.SRAM) > 0 {
		copy(ram, cart.SRAM)
	}
	cart.SRAM = ram

	m := &Mapper5{
		cartridge:   cart,
		prgBanks8K:  prgBanks,
		chrBanks1K:  chrBanks,
		prgMode:     mmc5PRGMode8K, // Default power-on mode: 8KB banks
		chrMode:     mmc5CHRMode1K, // Default power-on mode: 1KB banks
		prgRAM:      ram,
		prgRegs:     [mmc5PRGWindowCount]byte{0, 0, 0, 0xFF}, // Last bank fixed to ROM $FF
		audio:       NewMMC5Audio(),
	}

	cart.SetExpansionAudio(m.audio)
	m.updatePRGOffsets()
	m.updateCHROffsets()
	return m
}

// SetSpriteFetch receives notification of whether the PPU is actively fetching sprite tiles.
func (m *Mapper5) SetSpriteFetch(active bool) {
	m.isSpriteFetch = active
}

// ReadPRG maps CPU reads across $4020-$FFFF (registers, ExRAM, PRG-RAM, and PRG-ROM).
func (m *Mapper5) ReadPRG(addr uint16) byte {
	if val, ok := m.audio.ReadRegister(addr); ok {
		return val
	}

	switch {
	case addr == mmc5RegIRQStatus:
		var status byte
		if m.irqPending {
			status |= mmc5MaskIRQPending
		}
		if m.inFrame {
			status |= mmc5MaskInFrame
		}
		m.irqPending = false // Reading $5204 clears the IRQ pending flag
		return status

	case addr == mmc5RegMultA:
		product := uint16(m.multA) * uint16(m.multB)
		return byte(product & 0xFF)

	case addr == mmc5RegMultB:
		product := uint16(m.multA) * uint16(m.multB)
		return byte((product >> 8) & 0xFF)

	case addr >= mmc5ExRAMBase && addr <= mmc5ExRAMEnd:
		// ExRAM CPU reads: modes 2 and 3 provide normal read access.
		if m.exRAMMode == mmc5ExRAMModeReadWrite || m.exRAMMode == mmc5ExRAMModeReadOnly {
			return m.exRAM[addr-mmc5ExRAMBase]
		}
		return 0

	case addr >= mmc5PRGRAMBase && addr <= mmc5PRGRAMEnd:
		return m.readPRGRAM(int(m.prgRAMBank&mmc5MaskPRGBankRAM), addr-mmc5PRGRAMBase)

	case addr >= 0x8000:
		window := int((addr - 0x8000) / mmc5PRGBankSize8K)
		offset := addr & mmc5PRGWindowMask8K
		if m.prgIsRAM[window] {
			val := m.readPRGRAM(m.prgOffsets[window], offset)
			m.audio.ReadPRGTrigger(addr, val)
			return val
		}
		var val byte
		romOffset := m.prgOffsets[window] + int(offset)
		if romOffset < len(m.cartridge.PRG) {
			val = m.cartridge.PRG[romOffset]
		}
		m.audio.ReadPRGTrigger(addr, val)
		return val

	default:
		return 0
	}
}

func (m *Mapper5) readPRGRAM(bank int, offset uint16) byte {
	idx := (bank * mmc5PRGBankSize8K) + int(offset)
	if idx < len(m.prgRAM) {
		return m.prgRAM[idx]
	}
	return 0
}

func (m *Mapper5) writePRGRAM(bank int, offset uint16, data byte) {
	if m.ramProtect1 != mmc5Protect1Enable || m.ramProtect2 != mmc5Protect2Enable {
		return
	}
	idx := (bank * mmc5PRGBankSize8K) + int(offset)
	if idx < len(m.prgRAM) {
		m.prgRAM[idx] = data
		m.cartridge.MarkSRAMDirty()
	}
}

// WritePRG maps CPU writes to MMC5 registers, ExRAM, and PRG-RAM.
func (m *Mapper5) WritePRG(addr uint16, data byte) {
	if m.audio != nil && m.audio.WriteRegister(addr, data) {
		return
	}
	if m.writeCHRRegister(addr, data) || m.writeControlRegister(addr, data) {
		return
	}
	m.writeMemory(addr, data)
}

func (m *Mapper5) writeCHRRegister(addr uint16, data byte) bool {
	switch {
	case addr >= mmc5RegCHRSprite0 && addr <= mmc5RegCHRSprite7:
		idx := int(addr - mmc5RegCHRSprite0)
		m.chrRegsA[idx] = int(data) | (m.chrHigh << 8)
		m.lastCHRSet = mmc5CHRSetSprites
		m.updateCHROffsets()
		return true

	case addr >= mmc5RegCHRBGBase && addr <= mmc5RegCHRBGEnd:
		idx := int(addr - mmc5RegCHRBGBase)
		m.chrRegsB[idx] = int(data) | (m.chrHigh << 8)
		m.lastCHRSet = mmc5CHRSetBG
		m.updateCHROffsets()
		return true

	case addr == mmc5RegCHRHigh:
		m.chrHigh = int(data & mmc5MaskCHRHigh)
		return true

	default:
		return false
	}
}

func (m *Mapper5) writeControlRegister(addr uint16, data byte) bool {
	switch {
	case addr == mmc5RegPRGMode:
		m.prgMode = int(data & mmc5RegModeMask)
		m.updatePRGOffsets()
		return true

	case addr == mmc5RegCHRMode:
		m.chrMode = int(data & mmc5RegModeMask)
		m.updateCHROffsets()
		return true

	case addr == mmc5RegRAMProtect1:
		m.ramProtect1 = data & mmc5RegModeMask
		return true

	case addr == mmc5RegRAMProtect2:
		m.ramProtect2 = data & mmc5RegModeMask
		return true

	case addr == mmc5RegExRAMMode:
		m.exRAMMode = data & mmc5RegModeMask
		if m.exRAMMode != mmc5ExRAMModeExtendedAttr {
			m.exRAMLatched = false
		}
		return true

	case addr == mmc5RegNametableMap:
		m.nametableMap = data
		return true

	case addr == mmc5RegFillTile:
		m.fillTile = data
		return true

	case addr == mmc5RegFillPalette:
		m.fillPalette = data & mmc5RegModeMask
		return true

	case addr == mmc5RegPRGRAMBank:
		m.prgRAMBank = data & mmc5MaskPRGBankRAM
		return true

	case addr >= mmc5RegPRGBank0 && addr <= mmc5RegPRGBank3:
		m.prgRegs[addr-mmc5RegPRGBank0] = data
		m.updatePRGOffsets()
		return true

	case addr == mmc5RegIRQCompare:
		m.irqCompare = int(data)
		return true

	case addr == mmc5RegIRQStatus:
		m.irqEnabled = (data & mmc5MaskIRQPending) != 0
		return true

	case addr == mmc5RegMultA:
		m.multA = data
		return true

	case addr == mmc5RegMultB:
		m.multB = data
		return true

	default:
		return false
	}
}

func (m *Mapper5) writeMemory(addr uint16, data byte) {
	switch {
	case addr >= mmc5ExRAMBase && addr <= mmc5ExRAMEnd:
		if m.exRAMMode != mmc5ExRAMModeReadOnly {
			m.exRAM[addr-mmc5ExRAMBase] = data
		}

	case addr >= mmc5PRGRAMBase && addr <= mmc5PRGRAMEnd:
		m.writePRGRAM(int(m.prgRAMBank&mmc5MaskPRGBankRAM), addr-mmc5PRGRAMBase, data)

	case addr >= 0x8000:
		window := int((addr - 0x8000) / mmc5PRGBankSize8K)
		if m.prgIsRAM[window] {
			m.writePRGRAM(m.prgOffsets[window], addr&mmc5PRGWindowMask8K, data)
		}
	}
}

// ReadCHR maps PPU reads to banked CHR-ROM/RAM using Set A or Set B, or ExRAM Extended Attributes.
func (m *Mapper5) ReadCHR(addr uint16) byte {
	if len(m.cartridge.CHR) == 0 {
		return 0
	}

	// Extended Attribute Mode (mode 1): applies to background tile fetches only.
	if !m.isSpriteFetch && m.exRAMMode == mmc5ExRAMModeExtendedAttr && m.exRAMLatched {
		bank4K := int(m.exRAMLatch&mmc5ExAttrCHRBankMask) | (m.chrHigh << mmc5ExAttrCHRHighShift)
		base := bank4K * mmc5CHRBankSize4K
		offset := int(addr & mmc5CHRWindowMask4K)
		chrIdx := (base + offset) % len(m.cartridge.CHR)
		return m.cartridge.CHR[chrIdx]
	}

	window := int(addr / mmc5CHRBankSize1K)
	offset := int(addr & mmc5CHRWindowMask1K)

	base := m.chrOffsetsB[window]
	if m.isSpriteFetch || m.lastCHRSet == mmc5CHRSetSprites {
		base = m.chrOffsetsA[window]
	}

	chrIdx := (base + offset) % len(m.cartridge.CHR)
	return m.cartridge.CHR[chrIdx]
}

// WriteCHR maps PPU writes to CHR-RAM if present.
func (m *Mapper5) WriteCHR(addr uint16, data byte) {
	if len(m.cartridge.CHR) == 0 {
		return
	}
	window := int(addr / mmc5CHRBankSize1K)
	offset := int(addr & mmc5CHRWindowMask1K)

	base := m.chrOffsetsB[window]
	if m.isSpriteFetch || m.lastCHRSet == mmc5CHRSetSprites {
		base = m.chrOffsetsA[window]
	}

	chrIdx := (base + offset) % len(m.cartridge.CHR)
	m.cartridge.CHR[chrIdx] = data
}

// ReadNametable intercepts PPU nametable reads ($2000-$3EFF) according to $5105 mapping.
func (m *Mapper5) ReadNametable(addr uint16, vram []byte) (byte, bool) {
	normalized := (addr - 0x2000) & mmc5NametableWrap
	quadrant := normalized / mmc5NametablePage
	offset := normalized & (mmc5NametablePage - 1)
	mode := (m.nametableMap >> (quadrant * mmc5NametableQuadrantShift)) & mmc5NametableModeMask

	// In Extended Attribute Mode (mode 1), nametable tile fetches latch the ExRAM byte for 8x8 attributes and 4KB CHR bank.
	if offset < mmc5AttrTableOffset && m.exRAMMode == mmc5ExRAMModeExtendedAttr {
		m.exRAMLatch = m.exRAM[offset]
		m.exRAMLatched = true
	}
	if offset >= mmc5AttrTableOffset && m.exRAMMode == mmc5ExRAMModeExtendedAttr && m.exRAMLatched {
		// PPU reads attribute byte: return 2-bit palette replicated into all 4 pairs
		pal := (m.exRAMLatch >> mmc5ExAttrPaletteShift) & mmc5ExAttrPaletteMask
		attr := (pal << 6) | (pal << 4) | (pal << 2) | pal
		return attr, true
	}

	switch mode {
	case mmc5NametableSourceCIRAM0: // CIRAM Page 0
		if int(offset) < len(vram) {
			return vram[offset], true
		}
		return 0, true

	case mmc5NametableSourceCIRAM1: // CIRAM Page 1
		vramOffset := int(mmc5NametablePage + offset)
		if vramOffset < len(vram) {
			return vram[vramOffset], true
		}
		return 0, true

	case mmc5NametableSourceExRAM: // MMC5 Internal ExRAM
		if m.exRAMMode == mmc5ExRAMModeNametable || m.exRAMMode == mmc5ExRAMModeExtendedAttr {
			return m.exRAM[offset], true
		}
		return 0, true

	case mmc5NametableSourceFill: // Fill-Mode Tile & Attribute
		if offset < mmc5AttrTableOffset {
			return m.fillTile, true
		}
		pal := m.fillPalette & mmc5ExAttrPaletteMask
		attr := (pal << 6) | (pal << 4) | (pal << 2) | pal
		return attr, true

	default:
		return 0, false
	}
}

// WriteNametable intercepts PPU nametable writes ($2000-$3EFF) according to $5105 mapping.
func (m *Mapper5) WriteNametable(addr uint16, val byte, vram []byte) bool {
	normalized := (addr - 0x2000) & mmc5NametableWrap
	quadrant := normalized / mmc5NametablePage
	offset := normalized & (mmc5NametablePage - 1)
	mode := (m.nametableMap >> (quadrant * mmc5NametableQuadrantShift)) & mmc5NametableModeMask

	switch mode {
	case mmc5NametableSourceCIRAM0: // CIRAM Page 0
		if int(offset) < len(vram) {
			vram[offset] = val
		}
		return true

	case mmc5NametableSourceCIRAM1: // CIRAM Page 1
		vramOffset := int(mmc5NametablePage + offset)
		if vramOffset < len(vram) {
			vram[vramOffset] = val
		}
		return true

	case mmc5NametableSourceExRAM: // MMC5 Internal ExRAM
		if m.exRAMMode == mmc5ExRAMModeNametable || m.exRAMMode == mmc5ExRAMModeExtendedAttr {
			m.exRAM[offset] = val
		}
		return true

	case mmc5NametableSourceFill: // Fill-Mode is write-protected
		return true

	default:
		return false
	}
}

// ClockScanline advances the MMC5 scanline IRQ counter.
func (m *Mapper5) ClockScanline(scanline int) {
	if scanline >= 0 && scanline < mmc5ScanlineVisibleCount {
		m.inFrame = true
		if scanline == m.irqCompare && m.irqEnabled {
			m.irqPending = true
		}
		return
	}

	m.inFrame = false
}

// IRQPending reports whether an MMC5 scanline or PCM IRQ is currently asserted.
func (m *Mapper5) IRQPending() bool {
	return m.irqPending || (m.audio != nil && m.audio.IRQPending())
}

func (m *Mapper5) updatePRGOffsets() {
	switch m.prgMode {
	case mmc5PRGMode32K:
		bank := int((m.prgRegs[3] & 0x7C) >> 2) % (m.prgBanks8K / 4)
		base := bank * mmc5PRGBankSize32K
		for i := range mmc5PRGWindowCount {
			m.prgOffsets[i] = base + (i * mmc5PRGBankSize8K)
			m.prgIsRAM[i] = false
		}

	case mmc5PRGMode16K:
		m.applyPRGBank16K(0, m.prgRegs[1])
		m.applyPRGBank16K(2, m.prgRegs[3]|mmc5MaskPRGBit7) // Upper 16KB always ROM

	case mmc5PRGMode16K8K:
		m.applyPRGBank16K(0, m.prgRegs[1])
		m.applyPRGBank8K(2, m.prgRegs[2])
		m.applyPRGBank8K(3, m.prgRegs[3]|mmc5MaskPRGBit7) // $E000-$FFFF always ROM

	case mmc5PRGMode8K:
		m.applyPRGBank8K(0, m.prgRegs[0])
		m.applyPRGBank8K(1, m.prgRegs[1])
		m.applyPRGBank8K(2, m.prgRegs[2])
		m.applyPRGBank8K(3, m.prgRegs[3]|mmc5MaskPRGBit7) // $E000-$FFFF always ROM
	}
}

func (m *Mapper5) applyPRGBank16K(window int, reg byte) {
	if (reg & mmc5MaskPRGBit7) == 0 {
		// RAM
		bank := int(reg & 0x06)
		m.prgOffsets[window] = bank
		m.prgOffsets[window+1] = bank + 1
		m.prgIsRAM[window] = true
		m.prgIsRAM[window+1] = true
		return
	}

	// ROM
	bank := int(reg&0x7E) % (m.prgBanks8K &^ 1)
	m.prgOffsets[window] = bank * mmc5PRGBankSize8K
	m.prgOffsets[window+1] = (bank + 1) * mmc5PRGBankSize8K
	m.prgIsRAM[window] = false
	m.prgIsRAM[window+1] = false
}

func (m *Mapper5) applyPRGBank8K(window int, reg byte) {
	if (reg & mmc5MaskPRGBit7) == 0 {
		// RAM
		m.prgOffsets[window] = int(reg & mmc5MaskPRGBankRAM)
		m.prgIsRAM[window] = true
		return
	}

	// ROM
	bank := int(reg&mmc5MaskPRGBankROM) % m.prgBanks8K
	m.prgOffsets[window] = bank * mmc5PRGBankSize8K
	m.prgIsRAM[window] = false
}

func (m *Mapper5) updateCHROffsets() {
	m.updateCHROffsetsA()
	m.updateCHROffsetsB()
}

func (m *Mapper5) updateCHROffsetsA() {
	switch m.chrMode {
	case mmc5CHRMode8K:
		base := (m.chrRegsA[7] &^ 7) * mmc5CHRBankSize1K
		for i := range mmc5CHRWindowCount {
			m.chrOffsetsA[i] = base + (i * mmc5CHRBankSize1K)
		}

	case mmc5CHRMode4K:
		base0 := (m.chrRegsA[3] &^ 3) * mmc5CHRBankSize1K
		base1 := (m.chrRegsA[7] &^ 3) * mmc5CHRBankSize1K
		for i := range 4 {
			m.chrOffsetsA[i] = base0 + (i * mmc5CHRBankSize1K)
			m.chrOffsetsA[i+4] = base1 + (i * mmc5CHRBankSize1K)
		}

	case mmc5CHRMode2K:
		for pair := range 4 {
			regIdx := (pair * 2) + 1
			base := (m.chrRegsA[regIdx] &^ 1) * mmc5CHRBankSize1K
			m.chrOffsetsA[pair*2] = base
			m.chrOffsetsA[(pair*2)+1] = base + mmc5CHRBankSize1K
		}

	case mmc5CHRMode1K:
		for i := range mmc5CHRWindowCount {
			m.chrOffsetsA[i] = m.chrRegsA[i] * mmc5CHRBankSize1K
		}
	}
}

func (m *Mapper5) updateCHROffsetsB() {
	switch m.chrMode {
	case mmc5CHRMode8K:
		base := (m.chrRegsB[3] &^ 7) * mmc5CHRBankSize1K
		for i := range mmc5CHRWindowCount {
			m.chrOffsetsB[i] = base + (i * mmc5CHRBankSize1K)
		}

	case mmc5CHRMode4K:
		base := (m.chrRegsB[3] &^ 3) * mmc5CHRBankSize1K
		for i := range mmc5CHRWindowCount {
			m.chrOffsetsB[i] = base + ((i % 4) * mmc5CHRBankSize1K)
		}

	case mmc5CHRMode2K:
		base0 := (m.chrRegsB[1] &^ 1) * mmc5CHRBankSize1K
		base1 := (m.chrRegsB[3] &^ 1) * mmc5CHRBankSize1K
		for half := range 2 {
			offset := half * 4
			m.chrOffsetsB[offset] = base0
			m.chrOffsetsB[offset+1] = base0 + mmc5CHRBankSize1K
			m.chrOffsetsB[offset+2] = base1
			m.chrOffsetsB[offset+3] = base1 + mmc5CHRBankSize1K
		}

	case mmc5CHRMode1K:
		for i := range mmc5CHRWindowCount {
			m.chrOffsetsB[i] = m.chrRegsB[i%4] * mmc5CHRBankSize1K
		}
	}
}

// SaveState serializes the MMC5 internal mapper, ExRAM, and audio synthesis state.
func (m *Mapper5) SaveState(w io.Writer) error {
	var chrRegsA [mmc5CHRWindowCount]int32
	for i, r := range m.chrRegsA {
		chrRegsA[i] = int32(r & 0x7FFFFFFF)
	}
	var chrRegsB [4]int32
	for i, r := range m.chrRegsB {
		chrRegsB[i] = int32(r & 0x7FFFFFFF)
	}

	state := struct {
		PRGMode       int32
		CHRMode       int32
		RAMProtect1   byte
		RAMProtect2   byte
		ExRAMMode     byte
		NametableMap  byte
		FillTile      byte
		FillPalette   byte
		PRGRAMBank    byte
		PRGRegs       [mmc5PRGWindowCount]byte
		CHRRegsA      [mmc5CHRWindowCount]int32
		CHRRegsB      [4]int32
		CHRHigh       int32
		LastCHRSet    byte
		IsSpriteFetch bool
		ExRAMLatch    byte
		ExRAMLatched  bool
		MultA         byte
		MultB         byte
		IRQCompare    int32
		IRQEnabled    bool
		IRQPending    bool
		InFrame       bool
		ExRAM         [mmc5ExRAMSize]byte
	}{
		PRGMode:       int32(m.prgMode & 0x7FFFFFFF),
		CHRMode:       int32(m.chrMode & 0x7FFFFFFF),
		RAMProtect1:   m.ramProtect1,
		RAMProtect2:   m.ramProtect2,
		ExRAMMode:     m.exRAMMode,
		NametableMap:  m.nametableMap,
		FillTile:      m.fillTile,
		FillPalette:   m.fillPalette,
		PRGRAMBank:    m.prgRAMBank,
		PRGRegs:       m.prgRegs,
		CHRRegsA:      chrRegsA,
		CHRRegsB:      chrRegsB,
		CHRHigh:       int32(m.chrHigh & 0x7FFFFFFF),
		LastCHRSet:    m.lastCHRSet,
		IsSpriteFetch: m.isSpriteFetch,
		ExRAMLatch:    m.exRAMLatch,
		ExRAMLatched:  m.exRAMLatched,
		MultA:         m.multA,
		MultB:         m.multB,
		IRQCompare:    int32(m.irqCompare & 0x7FFFFFFF),
		IRQEnabled:    m.irqEnabled,
		IRQPending:    m.irqPending,
		InFrame:       m.inFrame,
		ExRAM:         m.exRAM,
	}

	if err := binary.Write(w, binary.LittleEndian, state); err != nil {
		return fmt.Errorf("write mapper5 state: %w", err)
	}

	if m.audio != nil {
		if err := m.audio.SaveState(w); err != nil {
			return fmt.Errorf("write mapper5 audio state: %w", err)
		}
	}
	return nil
}

// LoadState restores the MMC5 internal mapper, ExRAM, and audio synthesis state.
func (m *Mapper5) LoadState(r io.Reader) error {
	var state struct {
		PRGMode       int32
		CHRMode       int32
		RAMProtect1   byte
		RAMProtect2   byte
		ExRAMMode     byte
		NametableMap  byte
		FillTile      byte
		FillPalette   byte
		PRGRAMBank    byte
		PRGRegs       [mmc5PRGWindowCount]byte
		CHRRegsA      [mmc5CHRWindowCount]int32
		CHRRegsB      [4]int32
		CHRHigh       int32
		LastCHRSet    byte
		IsSpriteFetch bool
		ExRAMLatch    byte
		ExRAMLatched  bool
		MultA         byte
		MultB         byte
		IRQCompare    int32
		IRQEnabled    bool
		IRQPending    bool
		InFrame       bool
		ExRAM         [mmc5ExRAMSize]byte
	}

	if err := binary.Read(r, binary.LittleEndian, &state); err != nil {
		return fmt.Errorf("read mapper5 state: %w", err)
	}

	m.prgMode = int(state.PRGMode)
	m.chrMode = int(state.CHRMode)
	m.ramProtect1 = state.RAMProtect1
	m.ramProtect2 = state.RAMProtect2
	m.exRAMMode = state.ExRAMMode
	m.nametableMap = state.NametableMap
	m.fillTile = state.FillTile
	m.fillPalette = state.FillPalette
	m.prgRAMBank = state.PRGRAMBank
	m.prgRegs = state.PRGRegs
	for i, reg := range state.CHRRegsA {
		m.chrRegsA[i] = int(reg)
	}
	for i, reg := range state.CHRRegsB {
		m.chrRegsB[i] = int(reg)
	}
	m.chrHigh = int(state.CHRHigh)
	m.lastCHRSet = state.LastCHRSet
	m.isSpriteFetch = state.IsSpriteFetch
	m.exRAMLatch = state.ExRAMLatch
	m.exRAMLatched = state.ExRAMLatched
	m.multA = state.MultA
	m.multB = state.MultB
	m.irqCompare = int(state.IRQCompare)
	m.irqEnabled = state.IRQEnabled
	m.irqPending = state.IRQPending
	m.inFrame = state.InFrame
	m.exRAM = state.ExRAM

	m.updatePRGOffsets()
	m.updateCHROffsets()

	if m.audio != nil {
		if err := m.audio.LoadState(r); err != nil {
			return fmt.Errorf("read mapper5 audio state: %w", err)
		}
	}
	return nil
}

// Reset silences expansion audio and clears pending IRQs on console reset.
func (m *Mapper5) Reset() {
	m.irqPending = false
	if m.audio != nil {
		m.audio.Reset()
	}
}
