// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ppu implements the Ricoh 2C02 Picture Processing Unit (PPU) for the NES.
// It manages video memory (2KB VRAM, 256B OAM, 32B Palette RAM), Loopy scrolling state machines,
// scanline and dot rasterization, and 32-bit RGBA background frame rendering.
package ppu

import "github.com/dustblow/dustblow/internal/cartridge"

// Region represents the television broadcast standard governing PPU timing and scanlines.
type Region byte

const (
	// RegionNTSC is the North American and Japanese standard (60Hz, 262 scanlines).
	RegionNTSC Region = iota
	// RegionPAL is the European and Australian standard (50Hz, 312 scanlines).
	RegionPAL
	// RegionDendy is the Eastern European hybrid standard (50Hz, 312 scanlines, 3:1 clock ratio).
	RegionDendy
)

const (
	cycleIdleDot           = 0
	scanlineLastCycle      = 340
	cycleOddFrameSkip      = 339
	tileFetchCycleInterval = 8
	cycleVisibleStart      = 1
	cycleEventTrigger      = 1
	cycleVBlankClear       = 2
	cycleSpriteEvalStart   = 65
	cycleHorizontalCopy    = 257
	cycleSpriteFetchStart  = 257
	cycleScanlineClock     = 260
	cycleSpriteFetchEnd    = 320
	cyclePreFetchStart     = 321
	cyclePreFetchEnd       = 336
	cycleVerticalCopyStart = 280
	cycleVerticalCopyEnd   = 304
)

type timing struct {
	totalScanlines int
	vblankStart    int
	preRenderLine  int
	hasOddSkip     bool
}

var regionTimings = [...]timing{
	RegionNTSC: {
		totalScanlines: 262,
		vblankStart:    241,
		preRenderLine:  261,
		hasOddSkip:     true,
	},
	RegionPAL: {
		totalScanlines: 312,
		vblankStart:    241,
		preRenderLine:  311,
		hasOddSkip:     false,
	},
	RegionDendy: {
		totalScanlines: 312,
		vblankStart:    241,
		preRenderLine:  311,
		hasOddSkip:     false,
	},
}

// Option configures the PPU during initialization.
type Option func(p *PPU)

// WithRegion configures the television broadcast standard for PPU timing.
func WithRegion(r Region) Option {
	return func(p *PPU) {
		p.region = r
		p.timing = regionTimings[r]
	}
}

// PPU represents the Ricoh 2C02 Picture Processing Unit.
// Field order is organized to eliminate padding and satisfy 64-bit alignment.
type PPU struct {
	cartridge           *cartridge.Cartridge
	frameBuffer         [FrameBufferSize]byte
	vram                [VRAMSize]byte
	oam                 [OAMSize]byte
	palette             [PaletteSize]byte
	scanlineSprites     [maxSpritesPerScanline]scanlineSprite
	nextScanlineSprites [maxSpritesPerScanline]scanlineSprite
	secondarySprites    [maxSpritesPerScanline]secondaryOAMSprite
	timing              timing
	frame               uint64
	scanline            int
	cycle               int
	scanlineSpriteCount int
	nextScanlineCount   int
	secondaryCount      int
	bgShiftPatternLow   uint16
	bgShiftPatternHigh  uint16
	bgShiftAttrLow      uint16
	bgShiftAttrHigh     uint16
	v                   uint16
	t                   uint16
	ctrl                byte
	mask                byte
	status              byte
	oamAddr             byte
	spriteEvalOAMAddr   byte
	openBus             byte
	readBuffer          byte
	x                   byte
	region              Region
	openBusDecayFrames  int
	w                   bool
	nmiCurrent           bool
	nmiPrevious          bool
	nmiEdge              bool
	suppressVBlank        bool
	oamCorruptionPending  bool
	oamCorruptionRow      byte
	spriteOverflowPending bool
	spriteOverflowCycle   int
}

// New creates and initializes a new 2C02 PPU instance.
func New(cart *cartridge.Cartridge, opts ...Option) *PPU {
	p := &PPU{
		cartridge: cart,
		region:    RegionNTSC,
		timing:    regionTimings[RegionNTSC],
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Reset restores PPU control, rendering registers, and scanline counters to reset defaults.
func (p *PPU) Reset() {
	p.ctrl = 0
	p.mask = 0
	p.w = false
	p.scanline = 0
	p.cycle = 0
	p.nmiCurrent = false
	p.nmiPrevious = false
	p.nmiEdge = false
	p.suppressVBlank = false
	p.oamCorruptionPending = false
	p.oamCorruptionRow = 0
}

// PollNMI returns true if a rising edge on the NMI interrupt line occurred.
func (p *PPU) PollNMI() bool {
	edge := p.nmiEdge
	p.nmiEdge = false
	return edge
}

// Ctrl returns the current value of PPUCTRL ($2000).
func (p *PPU) Ctrl() byte {
	return p.ctrl
}

// Mask returns the current value of PPUMASK ($2001).
func (p *PPU) Mask() byte {
	return p.mask
}

// Scanline returns the current raster scanline index.
func (p *PPU) Scanline() int {
	return p.scanline
}

// VRAM returns the internal 2KB video RAM.
func (p *PPU) VRAM() [VRAMSize]byte {
	return p.vram
}

// VRAMAddr returns the current 14-bit VRAM address (v register).
func (p *PPU) VRAMAddr() uint16 {
	return p.v & maskVRAM14Bit
}

// TAddr returns the temporary VRAM address (t register).
func (p *PPU) TAddr() uint16 {
	return p.t & maskVRAM14Bit
}

// Status returns the current PPU status register without side-effects.
func (p *PPU) Status() byte {
	return p.status
}

// Cycle returns the current scanline dot/cycle index.
func (p *PPU) Cycle() int {
	return p.cycle
}

// Frame returns the total count of completed video frames since power-on.
func (p *PPU) Frame() uint64 {
	return p.frame
}

// Step advances the PPU state machine by the specified number of clock cycles.
func (p *PPU) Step(cycles int) {
	for range cycles {
		p.stepCycle()
	}
}

func (p *PPU) stepCycle() {
	p.advanceCycleCounter()

	renderingEnabled := (p.mask & (FlagMaskShowBG | FlagMaskShowSprites)) != 0

	isPreRender := p.scanline == p.timing.preRenderLine
	isVisibleLine := p.scanline < ScreenHeight
	isRenderLine := isVisibleLine || isPreRender

	p.handleOddFrameSkip(isPreRender, renderingEnabled)
	if p.oamCorruptionPending && renderingEnabled && isRenderLine {
		p.applyOAMCorruption()
		p.oamCorruptionPending = false
	}
	p.handleSpriteEvaluation(isRenderLine, renderingEnabled)
	p.processPixelPipeline(renderingEnabled, isRenderLine, isVisibleLine, isPreRender)
	p.handleTimingEvents(isPreRender)
	p.handleScanlineClock(isRenderLine, renderingEnabled)
}

func (p *PPU) handleScanlineClock(isRenderLine, renderingEnabled bool) {
	if isRenderLine && renderingEnabled && p.cycle == cycleScanlineClock && p.cartridge != nil {
		p.cartridge.ClockScanline(p.scanline)
	}
}

func (p *PPU) handleSpriteEvaluation(isRenderLine, renderingEnabled bool) {
	if !isRenderLine || !renderingEnabled {
		if p.cycle == cycleSpriteFetchEnd {
			p.scanlineSpriteCount = 0
			p.nextScanlineCount = 0
		}
		return
	}

	if p.cycle == cycleSpriteEvalStart {
		p.spriteEvalOAMAddr = p.oamAddr
		targetScanline := p.scanline + 1
		if p.scanline == p.timing.preRenderLine {
			targetScanline = 0
		}
		p.secondaryCount = 0
		p.nextScanlineCount = 0
		if targetScanline < ScreenHeight {
			p.evaluateSpritesForScanline(targetScanline, p.spriteEvalOAMAddr)
		}
	}

	if p.cycle >= cycleSpriteFetchStart && p.cycle <= cycleSpriteFetchEnd {
		offset := (p.cycle - cycleSpriteFetchStart) % spriteFetchSlotCycles
		if offset == spriteFetchSlotOffset {
			idx := (p.cycle - cycleSpriteFetchStart) / spriteFetchSlotCycles
			if idx < p.secondaryCount {
				p.fetchSecondarySprite(idx)
			}
		}
		if p.cycle == cycleSpriteFetchEnd {
			p.scanlineSprites = p.nextScanlineSprites
			p.scanlineSpriteCount = p.nextScanlineCount
		}
	}

	if p.spriteOverflowPending && p.cycle >= p.spriteOverflowCycle {
		p.status |= FlagStatusSpriteOverflow
		p.spriteOverflowPending = false
	}
}

func (p *PPU) advanceCycleCounter() {
	p.cycle++
	if p.cycle <= scanlineLastCycle {
		return
	}
	p.cycle = 0
	p.scanline++
	if p.scanline < p.timing.totalScanlines {
		return
	}
	p.scanline = 0
	p.frame++
	if p.openBusDecayFrames > 0 {
		p.openBusDecayFrames--
		if p.openBusDecayFrames == 0 {
			p.openBus = 0
		}
	}
}

func (p *PPU) handleOddFrameSkip(isPreRender, renderingEnabled bool) {
	if isPreRender && p.cycle == cycleOddFrameSkip && (p.frame%2 != 0) && renderingEnabled && p.timing.hasOddSkip {
		p.cycle = scanlineLastCycle
	}
}

func (p *PPU) processPixelPipeline(renderingEnabled, isRenderLine, isVisibleLine, isPreRender bool) {
	if !renderingEnabled {
		if isVisibleLine && p.cycle >= cycleVisibleStart && p.cycle <= ScreenWidth {
			p.renderPixel()
		}
		return
	}

	if !isRenderLine {
		return
	}

	p.processVisibleDots(isVisibleLine)
	p.processFetchDots(isPreRender)
}

func (p *PPU) processVisibleDots(isVisibleLine bool) {
	if p.cycle < cycleVisibleStart || p.cycle > ScreenWidth {
		return
	}

	if isVisibleLine {
		p.renderPixel()
	}
	p.shiftBackgroundRegisters()

	if p.cycle%tileFetchCycleInterval == 0 {
		p.loadNextTile()
		p.incrementCoarseX()
	}

	if p.cycle == ScreenWidth {
		p.incrementY()
	}
}

func (p *PPU) processFetchDots(isPreRender bool) {
	if p.cycle >= cycleSpriteFetchStart && p.cycle <= cycleSpriteFetchEnd {
		p.oamAddr = 0
	}

	switch {
	case p.cycle == cycleHorizontalCopy:
		p.copyHorizontal()
	case p.cycle >= cyclePreFetchStart && p.cycle <= cyclePreFetchEnd:
		p.shiftBackgroundRegisters()
		if p.cycle%tileFetchCycleInterval == 0 {
			p.loadNextTile()
			p.incrementCoarseX()
		}
	case isPreRender && p.cycle >= cycleVerticalCopyStart && p.cycle <= cycleVerticalCopyEnd:
		p.copyVertical()
	}
}

func (p *PPU) handleTimingEvents(isPreRender bool) {
	// Scanline 241, dot 1: Set VBlank flag and assert NMI if enabled in PPUCTRL.
	if p.scanline == p.timing.vblankStart && p.cycle == cycleEventTrigger {
		p.triggerVBlankEvent()
	}

	// Pre-render scanline, dot 1: Clear VBlank flag, Sprite 0 Hit, and Sprite Overflow.
	if isPreRender && p.cycle == cycleEventTrigger {
		p.status &^= FlagStatusVBlank | FlagStatusSprite0Hit | FlagStatusSpriteOverflow
		p.nmiCurrent = false
		p.nmiPrevious = false
		p.nmiEdge = false
	}
}

func (p *PPU) triggerVBlankEvent() {
	if p.suppressVBlank {
		p.suppressVBlank = false
		return
	}
	p.status |= FlagStatusVBlank
	if (p.ctrl & FlagCtrlNMI) != 0 {
		p.nmiCurrent = true
		if !p.nmiPrevious {
			p.nmiEdge = true
		}
		p.nmiPrevious = true
	}
}

// State holds the complete serializable state of the PPU for save state snapshots.
type State struct {
	VRAM               [VRAMSize]byte
	OAM                [OAMSize]byte
	Palette            [PaletteSize]byte
	Frame              uint64
	Scanline           int
	Cycle              int
	V                  uint16
	T                  uint16
	BGShiftPatternLow  uint16
	BGShiftPatternHigh uint16
	BGShiftAttrLow     uint16
	BGShiftAttrHigh    uint16
	Ctrl               byte
	Mask               byte
	Status             byte
	OAMAddr            byte
	SpriteEvalOAMAddr  byte
	ReadBuffer         byte
	X                  byte
	W                  bool
	NMICurrent         bool
	NMIPrevious        bool
	NMIEdge            bool
	SuppressVBlank        bool
	OAMCorruptionPending  bool
	OAMCorruptionRow      byte
	SpriteOverflowPending bool
	SpriteOverflowCycle   int
	SecondarySprites      [maxSpritesPerScanline]secondaryOAMSprite
	SecondaryCount        int
}

func (p *PPU) applyOAMCorruption() {
	dst := int(p.oamCorruptionRow) * oamRowBytes
	if dst+oamRowBytes <= len(p.oam) {
		copy(p.oam[dst:dst+oamRowBytes], p.oam[:oamRowBytes])
	}
}

// State returns an immutable snapshot copy of the internal PPU state.
func (p *PPU) State() State {
	return State{
		VRAM:                  p.vram,
		OAM:                   p.oam,
		Palette:               p.palette,
		Frame:                 p.frame,
		Scanline:              p.scanline,
		Cycle:                 p.cycle,
		V:                     p.v,
		T:                     p.t,
		BGShiftPatternLow:     p.bgShiftPatternLow,
		BGShiftPatternHigh:    p.bgShiftPatternHigh,
		BGShiftAttrLow:        p.bgShiftAttrLow,
		BGShiftAttrHigh:       p.bgShiftAttrHigh,
		Ctrl:                  p.ctrl,
		Mask:                  p.mask,
		Status:                p.status,
		OAMAddr:               p.oamAddr,
		SpriteEvalOAMAddr:     p.spriteEvalOAMAddr,
		ReadBuffer:            p.readBuffer,
		X:                     p.x,
		W:                     p.w,
		NMICurrent:            p.nmiCurrent,
		NMIPrevious:           p.nmiPrevious,
		NMIEdge:               p.nmiEdge,
		SuppressVBlank:        p.suppressVBlank,
		OAMCorruptionPending:  p.oamCorruptionPending,
		OAMCorruptionRow:      p.oamCorruptionRow,
		SpriteOverflowPending: p.spriteOverflowPending,
		SpriteOverflowCycle:   p.spriteOverflowCycle,
		SecondarySprites:      p.secondarySprites,
		SecondaryCount:        p.secondaryCount,
	}
}

// RestoreState restores the internal PPU state from a snapshot.
func (p *PPU) RestoreState(s State) {
	p.vram = s.VRAM
	p.oam = s.OAM
	p.palette = s.Palette
	p.frame = s.Frame
	p.scanline = s.Scanline
	p.cycle = s.Cycle
	p.v = s.V
	p.t = s.T
	p.bgShiftPatternLow = s.BGShiftPatternLow
	p.bgShiftPatternHigh = s.BGShiftPatternHigh
	p.bgShiftAttrLow = s.BGShiftAttrLow
	p.bgShiftAttrHigh = s.BGShiftAttrHigh
	p.ctrl = s.Ctrl
	p.mask = s.Mask
	p.status = s.Status
	p.oamAddr = s.OAMAddr
	p.spriteEvalOAMAddr = s.SpriteEvalOAMAddr
	p.readBuffer = s.ReadBuffer
	p.x = s.X
	p.w = s.W
	p.nmiCurrent = s.NMICurrent
	p.nmiPrevious = s.NMIPrevious
	p.nmiEdge = s.NMIEdge
	p.suppressVBlank = s.SuppressVBlank
	p.oamCorruptionPending = s.OAMCorruptionPending
	p.oamCorruptionRow = s.OAMCorruptionRow
	p.spriteOverflowPending = s.SpriteOverflowPending
	p.spriteOverflowCycle = s.SpriteOverflowCycle
	p.secondarySprites = s.SecondarySprites
	p.secondaryCount = s.SecondaryCount
}
