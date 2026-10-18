// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ppu

const (
	// Register address offsets (modulo 8, $2000-$2007).
	regPPUCTRL   uint16 = 0
	regPPUMASK   uint16 = 1
	regPPUSTATUS uint16 = 2
	regOAMADDR   uint16 = 3
	regOAMDATA   uint16 = 4
	regPPUSCROLL uint16 = 5
	regPPUADDR   uint16 = 6
	regPPUDATA   uint16 = 7
)

// Bitmask flags for the PPUCTRL register ($2000).
const (
	// FlagCtrlBaseNametable masks the base nametable address ($2000, $2400, $2800, $2C00).
	FlagCtrlBaseNametable byte = 0x03
	// FlagCtrlVRAMIncrement selects VRAM increment mode (0: +1 across, 1: +32 down).
	FlagCtrlVRAMIncrement byte = 0x04
	// FlagCtrlSpritePattern selects sprite pattern table for 8x8 sprites (0: $0000, 1: $1000).
	FlagCtrlSpritePattern byte = 0x08
	// FlagCtrlBGPattern selects background pattern table (0: $0000, 1: $1000).
	FlagCtrlBGPattern byte = 0x10
	// FlagCtrlSpriteSize selects sprite size (0: 8x8, 1: 8x16).
	FlagCtrlSpriteSize byte = 0x20
	// FlagCtrlNMI enables generation of NMI at the start of vertical blanking.
	FlagCtrlNMI byte = 0x80
)

// Bitmask flags for the PPUMASK register ($2001).
const (
	// FlagMaskGreyscale forces monochrome display by clearing chrominance bits.
	FlagMaskGreyscale byte = 0x01
	// FlagMaskShowLeftBG enables background in leftmost 8 pixels.
	FlagMaskShowLeftBG byte = 0x02
	// FlagMaskShowLeftSprites enables sprites in leftmost 8 pixels.
	FlagMaskShowLeftSprites byte = 0x04
	// FlagMaskShowBG enables background rendering.
	FlagMaskShowBG byte = 0x08
	// FlagMaskShowSprites enables sprite rendering.
	FlagMaskShowSprites byte = 0x10
	// FlagMaskEmphasizeRed emphasizes red (darkens green and blue on NTSC).
	FlagMaskEmphasizeRed byte = 0x20
	// FlagMaskEmphasizeGreen emphasizes green (darkens red and blue on NTSC).
	FlagMaskEmphasizeGreen byte = 0x40
	// FlagMaskEmphasizeBlue emphasizes blue (darkens red and green on NTSC).
	FlagMaskEmphasizeBlue byte = 0x80
)

// Bitmask flags for the PPUSTATUS register ($2002).
const (
	// FlagStatusOpenBus masks the least significant 5 bits representing open bus.
	FlagStatusOpenBus byte = 0x1F
	// FlagStatusSpriteOverflow indicates more than 8 sprites on a scanline.
	FlagStatusSpriteOverflow byte = 0x20
	// FlagStatusSprite0Hit indicates collision between sprite 0 and non-transparent background.
	FlagStatusSprite0Hit byte = 0x40
	// FlagStatusVBlank indicates vertical blanking period is active.
	FlagStatusVBlank byte = 0x80
)

const (
	// Loopy scrolling bit manipulation masks.
	maskNametableSelect = 0x0C00 // Bits 10-11 of t/v
	maskCoarseX         = 0x001F // Bits 0-4 of t/v
	maskCoarseY         = 0x03E0 // Bits 5-9 of t/v
	maskFineY           = 0x7000 // Bits 12-14 of t/v
	maskFineX           = 0x07   // Bits 0-2 of x
	maskVRAM14Bit       = 0x3FFF // 14-bit PPU address space
	maskPPUADDRClearBit = 0x7F00 // Clears bit 14 on first write to PPUADDR
	maskPPUADDRHighByte = 0x3F   // 6 bits of high address in PPUADDR
	maskLowByte         = 0x00FF // Low 8 bits mask
	maskRegisterSelect  = 0x0007 // Mirroring mask for 8 PPU registers ($2000-$2007)

	shiftNametableSelect = 10
	shiftCoarseFromByte  = 3
	shiftCoarseY         = 5
	shiftFineY           = 12
	shiftAddressHighByte = 8
	shiftEmphasisBits    = 5

	maskGreyscale    byte = 0x30
	maskEmphasisBits byte = 0xE0

	vramStepAcross   uint16 = 1
	vramStepDown     uint16 = 32
	vramMirrorOffset uint16 = 0x1000

	openBusDecayFramesLimit = 45
)

// ReadRegister reads one of the 8 memory-mapped PPU I/O registers ($2000-$2007).
func (p *PPU) ReadRegister(addr uint16) byte {
	switch addr & maskRegisterSelect {
	case regPPUSTATUS:
		res := (p.status &^ FlagStatusOpenBus) | (p.openBus & FlagStatusOpenBus)
		if p.scanline == p.timing.preRenderLine && p.cycle == cycleIdleDot {
			// On pre-render dot 0, reading PPUSTATUS latches VBlank at the start of the CPU read
			// while sprite flags are sampled at the end. By the end of the read, dot 1 clears sprite
			// flags, returning VBlank=1 but Sprite0/Overflow=0.
			res &^= FlagStatusSprite0Hit | FlagStatusSpriteOverflow
		}

		if p.spriteOverflowPending && p.cycle >= p.spriteOverflowCycle {
			res |= FlagStatusSpriteOverflow
		}
		if p.scanline == p.timing.vblankStart {
			switch p.cycle {
			case 0:
				res &^= FlagStatusVBlank
				p.suppressVBlank = true
				p.nmiCurrent = false
				p.nmiEdge = false
			case 1:
				p.nmiCurrent = false
				p.nmiEdge = false
			}
		}

		// Reading PPUSTATUS clears the VBlank flag and resets the write toggle.
		p.status &^= FlagStatusVBlank
		p.w = false
		p.openBus = (res &^ FlagStatusOpenBus) | (p.openBus & FlagStatusOpenBus)
		p.openBusDecayFrames = openBusDecayFramesLimit
		return res

	case regOAMDATA:
		val := p.readOAMData()
		p.openBus = val
		p.openBusDecayFrames = openBusDecayFramesLimit
		return val

	case regPPUDATA:
		target := p.v & maskVRAM14Bit
		p.incrementVRAM()

		if target < paletteBase {
			buffered := p.readBuffer
			p.readBuffer = p.ReadVRAM(target)
			p.openBus = buffered
			p.openBusDecayFrames = openBusDecayFramesLimit
			return buffered
		}

		// Palette reads return immediately; underlying nametable mirror byte is buffered.
		p.readBuffer = p.ReadVRAM(target - vramMirrorOffset)
		palVal := p.ReadVRAM(target) & 0x3F
		if (p.mask & FlagMaskGreyscale) != 0 {
			palVal &= maskGreyscale
		}
		val := palVal | (p.openBus & 0xC0)
		p.openBus = val
		p.openBusDecayFrames = openBusDecayFramesLimit
		return val

	default:
		return p.openBus
	}
}

// WriteRegister writes data to one of the 8 memory-mapped PPU I/O registers ($2000-$2007).
func (p *PPU) WriteRegister(addr uint16, data byte) {
	p.openBus = data
	p.openBusDecayFrames = openBusDecayFramesLimit

	switch addr & maskRegisterSelect {
	case regPPUCTRL:
		nmiWasDisabled := (p.ctrl & FlagCtrlNMI) == 0
		p.ctrl = data
		// t: ...GH.. ........ <- d: ......GH (nametable select)
		p.t = (p.t &^ maskNametableSelect) | (uint16(data&FlagCtrlBaseNametable) << shiftNametableSelect)

		if (data & FlagCtrlNMI) == 0 {
			p.nmiCurrent = false
			p.nmiPrevious = false
			break
		}
		if nmiWasDisabled && (p.status&FlagStatusVBlank) != 0 {
			p.nmiCurrent = true
			p.nmiPrevious = true
			p.nmiEdge = true
		}

	case regPPUMASK:
		wasRendering := (p.mask & (FlagMaskShowBG | FlagMaskShowSprites)) != 0
		nowRendering := (data & (FlagMaskShowBG | FlagMaskShowSprites)) != 0
		if wasRendering && !nowRendering && p.scanline < p.timing.vblankStart {
			p.oamCorruptionPending = true
			rawRow := (p.cycle - 1) / 2
			if rawRow <= 0 {
				rawRow = defaultOAMCorruptionRow
			}
			if rawRow >= oamMaxRows {
				rawRow = oamMaxRows - 1
			}
			p.oamCorruptionRow = byte(rawRow)
		}
		p.mask = data

	case regOAMADDR:
		p.oamAddr = data

	case regOAMDATA:
		p.writeOAMData(data)

	case regPPUSCROLL:
		if !p.w {
			// First write: coarse X and fine X
			p.t = (p.t &^ maskCoarseX) | uint16(data>>shiftCoarseFromByte)
			p.x = data & maskFineX
			p.w = true
			break
		}
		// Second write: fine Y and coarse Y
		p.t = (p.t &^ (maskFineY | maskCoarseY)) |
			(uint16(data&maskFineX) << shiftFineY) |
			(uint16(data>>shiftCoarseFromByte) << shiftCoarseY)
		p.w = false

	case regPPUADDR:
		if !p.w {
			// First write: high 6 bits of address (bit 14 cleared)
			p.t = (p.t &^ maskPPUADDRClearBit) | (uint16(data&maskPPUADDRHighByte) << shiftAddressHighByte)
			p.w = true
			break
		}
		// Second write: low byte of address and latch v = t
		p.t = (p.t &^ maskLowByte) | uint16(data)
		p.v = p.t
		p.w = false

	case regPPUDATA:
		target := p.v & maskVRAM14Bit
		p.WriteVRAM(target, data)
		p.incrementVRAM()
	}
}

// PeekRegister passively reads a PPU register without altering write toggle or clearing VBlank.
func (p *PPU) PeekRegister(addr uint16) byte {
	switch addr & maskRegisterSelect {
	case regPPUSTATUS:
		return (p.status &^ FlagStatusOpenBus) | (p.openBus & FlagStatusOpenBus)
	case regOAMDATA:
		val := p.oam[p.oamAddr]
		if (p.oamAddr & maskOAMEntryByte) == oamOffsetAttr {
			val &= maskOAMAttrRead
		}
		return val
	case regPPUDATA:
		target := p.v & maskVRAM14Bit
		if target < paletteBase {
			return p.readBuffer
		}
		palVal := p.PeekVRAM(target) & 0x3F
		if (p.mask & FlagMaskGreyscale) != 0 {
			palVal &= maskGreyscale
		}
		return palVal | (p.openBus & 0xC0)
	default:
		return p.openBus
	}
}

func (p *PPU) readOAMData() byte {
	isRenderLine := p.scanline < ScreenHeight || p.scanline == p.timing.preRenderLine
	renderingEnabled := (p.mask & (FlagMaskShowBG | FlagMaskShowSprites)) != 0
	if isRenderLine && renderingEnabled {
		if (p.cycle >= cycleVisibleStart && p.cycle < cycleSpriteEvalStart) ||
			(p.cycle >= cycleSpriteFetchStart && p.cycle <= cycleSpriteFetchEnd) {
			return 0xFF
		}
		if p.cycle >= cycleSpriteEvalStart && p.cycle < cycleSpriteFetchStart {
			return p.readOAMByte(byte(p.cycle - cycleSpriteEvalStart))
		}
	}

	return p.readOAMByte(p.oamAddr)
}

func (p *PPU) readOAMByte(addr byte) byte {
	val := p.oam[addr]
	if (addr & maskOAMEntryByte) == oamOffsetAttr {
		val &= maskOAMAttrRead
	}
	return val
}

func (p *PPU) writeOAMData(data byte) {
	isRenderLine := p.scanline < ScreenHeight || p.scanline == p.timing.preRenderLine
	renderingEnabled := (p.mask & (FlagMaskShowBG | FlagMaskShowSprites)) != 0
	if isRenderLine && renderingEnabled {
		p.oamAddr = (p.oamAddr + bytesPerOAMEntry) & maskOAMAddrAlign
		return
	}
	p.oam[p.oamAddr] = data
	p.oamAddr++
}

func (p *PPU) incrementVRAM() {
	isRenderLine := p.scanline < ScreenHeight || p.scanline == p.timing.preRenderLine
	renderingEnabled := (p.mask & (FlagMaskShowBG | FlagMaskShowSprites)) != 0
	if isRenderLine && renderingEnabled {
		p.incrementCoarseX()
		p.incrementY()
		return
	}

	step := vramStepAcross
	if (p.ctrl & FlagCtrlVRAMIncrement) != 0 {
		step = vramStepDown
	}
	p.v += step
}
