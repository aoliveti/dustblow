// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ppu

const (
	// ScreenWidth is the horizontal display resolution of the NES in pixels.
	ScreenWidth = 256
	// ScreenHeight is the vertical display resolution of the NES in pixels.
	ScreenHeight = 240
	// FrameBufferSize is the total byte size of the 32-bit RGBA framebuffer (256 * 240 * 4 = 245,760 bytes).
	FrameBufferSize = ScreenWidth * ScreenHeight * 4

	bytesPerPixel    = 4
	tilePixelWidth   = 8
	patternPlaneSize = 8
	bytesPerTile     = 16

	maskNametableHorizontal uint16 = 0x0400 // Bit 10 of v
	maskNametableVertical   uint16 = 0x0800 // Bit 11 of v
	maskCoarseYLimit        uint16 = 29     // Coarse Y wraps from 29 to 0
	maskCoarseYOverflow     uint16 = 31     // Coarse Y wraps without toggling nametable at 31
	maskAttributeBase       uint16 = 0x23C0 // Base offset of attribute table in nametable
	maskAttributeCoarseY    uint16 = 0x38   // (coarseY >> 2) << 3
	maskUpperByte           uint16 = 0xFF00
	maskLowerByteValues     uint16 = 0x00FF
	maskPaletteIndex        byte   = 0x3F
	maskSingleBit           byte   = 1
	maskPaletteQuadrant     byte   = 0x03

	fineYIncrement           uint16 = 0x1000
	coarseYShift                    = 5
	fineYShift                      = 12
	attributeCoarseYShift           = 4
	attributeCoarseXShift           = 2
	attributeQuadrantYShift         = 4
	maskAttributeQuadrantY   uint16 = 0x04
	maskAttributeQuadrantX   uint16 = 0x02

	bitplane1Shift         = 1
	attributeBit1Shift     = 1
	shiftRegisterSampleBit = 15

	maxSpritesPerScanline = 8
	maxTotalSprites       = 64
	bytesPerOAMEntry      = 4
	spriteSize8x8Height   = 8
	spriteSize8x16Height  = 16

	maskSpritePalette   byte   = 0x03
	maskSpritePriority  byte   = 0x20
	maskSpriteFlipH     byte   = 0x40
	maskSpriteFlipV     byte   = 0x80
	maskSpriteTile8x16  byte   = 0xFE
	maskSpriteRowOffset        = 0x07
	lastVisiblePixelX          = 255
	spritePaletteBase   uint16 = 0x3F10
	flipVertical8x8Max         = 7
	flipVertical8x16Max        = 15
	flipHorizontalMax          = 7
	shiftSpritePalette         = 2

	oamOffsetTile              = 1
	oamOffsetAttr              = 2
	oamOffsetCoordX            = 3
	spriteScanlineOffset       = 1
	sprite0Index               = 0
	oamBuggedAdvance           = 5
	maskOAMAddrAlign     byte  = 0xFC
	maskOAMEntryByte     byte  = 0x03
	maskOAMAttrRead      byte  = 0xE3
	maskSpriteTableSelect byte = 0x01
	colorTransparent      byte = 0

	spriteEvalMissCycles  = 2
	spriteEvalCopyCycles  = 8
	spriteEvalAssertDelay = 1

	spriteFetchSlotCycles = 8
	spriteFetchSlotOffset = 7
)

// secondaryOAMSprite stores an in-range sprite entry copied to secondary OAM (OAM2).
type secondaryOAMSprite struct {
	y         byte
	tile      byte
	attr      byte
	x         byte
	isSprite0 bool
}

// scanlineSprite stores the evaluated sprite data for the active scanline.
// Struct field layout is optimized for memory alignment.
type scanlineSprite struct {
	x         byte
	palette   byte
	priority  byte
	patternLo byte
	patternHi byte
	flipH     bool
	isSprite0 bool
}

// FrameBuffer returns the current 32-bit RGBA framebuffer slice.
func (p *PPU) FrameBuffer() []byte {
	return p.frameBuffer[:]
}

// incrementCoarseX advances horizontal tile coordinate in v.
// Upon reaching tile 31, it wraps to 0 and flips the horizontal nametable select (bit 10).
func (p *PPU) incrementCoarseX() {
	if (p.v & maskCoarseX) == maskCoarseX {
		p.v = (p.v &^ maskCoarseX) ^ maskNametableHorizontal
		return
	}
	p.v++
}

// incrementY advances vertical fine and coarse tile coordinates in v.
// Fine Y ranges 0-7; upon wrapping to 0, Coarse Y is incremented.
// Coarse Y 29 marks the end of visible nametable tiles and wraps to 0 while toggling
// the vertical nametable bit (bit 11). Values 30-31 wrap to 0 without toggling.
func (p *PPU) incrementY() {
	if (p.v & maskFineY) != maskFineY {
		p.v += fineYIncrement
		return
	}

	p.v &^= maskFineY
	y := (p.v & maskCoarseY) >> coarseYShift
	switch y {
	case maskCoarseYLimit:
		y = 0
		p.v ^= maskNametableVertical
	case maskCoarseYOverflow:
		y = 0
	default:
		y++
	}
	p.v = (p.v &^ maskCoarseY) | (y << coarseYShift)
}

// copyHorizontal transfers coarse X and nametable horizontal select from t to v at dot 257.
func (p *PPU) copyHorizontal() {
	mask := maskCoarseX | maskNametableHorizontal
	p.v = (p.v &^ mask) | (p.t & mask)
}

// copyVertical transfers fine Y, coarse Y, and nametable vertical select from t to v (dots 280-304).
func (p *PPU) copyVertical() {
	mask := maskFineY | maskCoarseY | maskNametableVertical
	p.v = (p.v &^ mask) | (p.t & mask)
}

// loadNextTile fetches nametable tile ID, attribute byte, and pattern table planes,
// feeding the lower 8 bits of the background shift registers.
func (p *PPU) loadNextTile() {
	// 1. Fetch Tile ID from Nametable
	nametableAddr := nametableBase | (p.v & nametableWrapMask)
	tileID := p.ReadVRAM(nametableAddr)

	// 2. Fetch Attribute Byte
	attrAddr := maskAttributeBase | (p.v & maskNametableSelect) |
		(((p.v & maskCoarseY) >> attributeCoarseYShift) & maskAttributeCoarseY) |
		((p.v & maskCoarseX) >> attributeCoarseXShift)
	attrByte := p.ReadVRAM(attrAddr)

	// Shift attribute to get 2-bit palette: quadrant select based on coarse X bit 1 and coarse Y bit 1.
	shift := ((p.v >> attributeQuadrantYShift) & maskAttributeQuadrantY) | (p.v & maskAttributeQuadrantX)
	palette := (attrByte >> shift) & maskPaletteQuadrant

	// 3. Fetch Pattern Table Low & High Planes
	var patternBaseAddr uint16
	if (p.ctrl & FlagCtrlBGPattern) != 0 {
		patternBaseAddr = patternTable1Base
	}
	fineY := (p.v & maskFineY) >> fineYShift
	tilePatternAddr := patternBaseAddr + (uint16(tileID) * bytesPerTile) + fineY

	plane0 := p.ReadVRAM(tilePatternAddr)
	plane1 := p.ReadVRAM(tilePatternAddr + patternPlaneSize)

	// 4. Load bottom 8 bits of shift registers
	p.bgShiftPatternLow = (p.bgShiftPatternLow & maskUpperByte) | uint16(plane0)
	p.bgShiftPatternHigh = (p.bgShiftPatternHigh & maskUpperByte) | uint16(plane1)

	var attr0, attr1 uint16
	if (palette & 0x01) != 0 {
		attr0 = maskLowerByteValues
	}
	if (palette & 0x02) != 0 {
		attr1 = maskLowerByteValues
	}
	p.bgShiftAttrLow = (p.bgShiftAttrLow & maskUpperByte) | attr0
	p.bgShiftAttrHigh = (p.bgShiftAttrHigh & maskUpperByte) | attr1
}

// shiftBackgroundRegisters clocks the 16-bit background pattern and attribute shift registers by 1 bit.
func (p *PPU) shiftBackgroundRegisters() {
	p.bgShiftPatternLow <<= 1
	p.bgShiftPatternHigh <<= 1
	p.bgShiftAttrLow <<= 1
	p.bgShiftAttrHigh <<= 1
}

// renderPixel executes the complete Ricoh 2C02 pixel composition pipeline for dot x on scanline y:
// 1. Samples the background pattern and attribute bitplanes from internal shift registers.
// 2. Samples the first non-transparent pixel from evaluated scanline sprites.
// 3. Evaluates Sprite 0 Hit collision conditions against Nesdev timing rules.
// 4. Resolves background vs sprite priority via the priority multiplexer.
// 5. Writes the 32-bit RGBA color into the output framebuffer.
func (p *PPU) renderPixel() {
	x := p.cycle - 1
	y := p.scanline

	if x < 0 || x >= ScreenWidth || y < 0 || y >= ScreenHeight {
		return
	}

	bgPixel, bgPalette := p.sampleBackgroundPixel(x)
	sprPixel, sprPalette, sprPriority, isSprite0 := p.sampleSpritePixel(x)
	p.checkSprite0Hit(x, bgPixel, sprPixel, isSprite0)

	pixelColor := p.multiplexPixel(bgPixel, bgPalette, sprPixel, sprPalette, sprPriority)
	offset := (y*ScreenWidth + x) * bytesPerPixel

	p.frameBuffer[offset] = pixelColor[0]
	p.frameBuffer[offset+1] = pixelColor[1]
	p.frameBuffer[offset+2] = pixelColor[2]
	p.frameBuffer[offset+3] = pixelColor[3]
}

// sampleBackgroundPixel extracts the 2-bit color index and 2-bit palette attribute for coordinate x
// using fine X scroll from the 16-bit pattern and attribute shift registers.
// Returns (0, 0) if background rendering is disabled, clipped in the leftmost 8 pixels, or transparent.
func (p *PPU) sampleBackgroundPixel(x int) (pixel byte, palette byte) {
	if (p.mask & FlagMaskShowBG) == 0 {
		return colorTransparent, 0
	}
	if x < tilePixelWidth && (p.mask&FlagMaskShowLeftBG) == 0 {
		return colorTransparent, 0
	}

	shift := shiftRegisterSampleBit - p.x
	bit0 := byte((p.bgShiftPatternLow >> shift) & uint16(maskSingleBit))
	bit1 := byte((p.bgShiftPatternHigh >> shift) & uint16(maskSingleBit))
	pixel = (bit1 << bitplane1Shift) | bit0
	if pixel == colorTransparent {
		return colorTransparent, 0
	}

	attr0 := byte((p.bgShiftAttrLow >> shift) & uint16(maskSingleBit))
	attr1 := byte((p.bgShiftAttrHigh >> shift) & uint16(maskSingleBit))
	palette = (attr1 << attributeBit1Shift) | attr0
	return pixel, palette
}

// sampleSpritePixel iterates over the secondary OAM sprite buffer for the active scanline from lowest
// to highest OAM index. Lower OAM indices take visual priority over higher indices.
// Respects horizontal flip (bit 6) by flipping the bit sampling direction.
func (p *PPU) sampleSpritePixel(x int) (pixel byte, palette byte, priority byte, isSprite0 bool) {
	if (p.mask & FlagMaskShowSprites) == 0 {
		return colorTransparent, 0, 0, false
	}
	if x < tilePixelWidth && (p.mask&FlagMaskShowLeftSprites) == 0 {
		return colorTransparent, 0, 0, false
	}

	for i := range p.scanlineSpriteCount {
		spr := &p.scanlineSprites[i]
		diff := x - int(spr.x)
		if diff < 0 || diff >= tilePixelWidth {
			continue
		}

		shift := flipHorizontalMax - diff
		if spr.flipH {
			shift = diff
		}

		bit0 := (spr.patternLo >> shift) & 1
		bit1 := (spr.patternHi >> shift) & 1
		color := (bit1 << bitplane1Shift) | bit0
		if color != colorTransparent {
			return color, spr.palette, spr.priority, spr.isSprite0
		}
	}

	return colorTransparent, 0, 0, false
}

// checkSprite0Hit validates the 6 hardware collision criteria defined by the Nesdev specification:
// 1. Both background and sprite 0 pixels are non-transparent (opaque, values 1-3).
// 2. Sprite 0 was evaluated on the active scanline.
// 3. Pixel x is strictly less than 255 (hardware quirk: collision cannot occur on the rightmost pixel).
// 4. Pixel x >= 8 OR both background and sprite leftmost 8-pixel rendering are enabled in PPUMASK.
// 5. Both background and sprite rendering are enabled in PPUMASK.
// If all conditions match, FlagStatusSprite0Hit is latched in PPUSTATUS ($2002 bit 6).
func (p *PPU) checkSprite0Hit(x int, bgPixel, sprPixel byte, isSprite0 bool) {
	if !isSprite0 || bgPixel == colorTransparent || sprPixel == colorTransparent {
		return
	}
	if x >= lastVisiblePixelX {
		return
	}
	if x < tilePixelWidth && (p.mask&FlagMaskShowLeftSprites) == 0 {
		return
	}
	if (p.mask&FlagMaskShowBG) == 0 || (p.mask&FlagMaskShowSprites) == 0 {
		return
	}
	p.status |= FlagStatusSprite0Hit
}

// resolveColor converts a hardware palette RAM color index into a 32-bit RGBA Color,
// applying greyscale luminance masking (PPUMASK bit 0) and color emphasis tints (PPUMASK bits 5-7).
func (p *PPU) resolveColor(rawIndex byte) Color {
	colorIndex := rawIndex & maskPaletteIndex
	if (p.mask & FlagMaskGreyscale) != 0 {
		colorIndex &= maskGreyscale
	}
	emphasis := (p.mask & maskEmphasisBits) >> shiftEmphasisBits
	return PaletteTable[emphasis][colorIndex]
}

// multiplexPixel resolves final pixel color between universal backdrop, background, and sprite:
// - Transparent BG + Transparent Sprite -> Universal Backdrop ($3F00).
// - Transparent BG + Opaque Sprite      -> Sprite Palette ($3F10-$3F1F).
// - Opaque BG + Transparent Sprite      -> Background Palette ($3F00-$3F0F).
// - Opaque BG + Opaque Sprite:
//   - Sprite Priority 0 (front)         -> Sprite Palette ($3F10-$3F1F).
//   - Sprite Priority 1 (behind)        -> Background Palette ($3F00-$3F0F).
func (p *PPU) multiplexPixel(bgPixel, bgPalette, sprPixel, sprPalette, sprPriority byte) Color {
	if bgPixel == colorTransparent && sprPixel == colorTransparent {
		return p.resolveColor(p.palette[0])
	}
	if bgPixel == colorTransparent {
		offset := spritePaletteBase + (uint16(sprPalette) << shiftSpritePalette) + uint16(sprPixel)
		return p.resolveColor(p.palette[mirrorPaletteAddress(offset)])
	}
	if sprPixel == colorTransparent {
		offset := paletteBase + (uint16(bgPalette) << shiftSpritePalette) + uint16(bgPixel)
		return p.resolveColor(p.palette[mirrorPaletteAddress(offset)])
	}

	if sprPriority == 0 {
		offset := spritePaletteBase + (uint16(sprPalette) << shiftSpritePalette) + uint16(sprPixel)
		return p.resolveColor(p.palette[mirrorPaletteAddress(offset)])
	}

	offset := paletteBase + (uint16(bgPalette) << shiftSpritePalette) + uint16(bgPixel)
	return p.resolveColor(p.palette[mirrorPaletteAddress(offset)])
}

// evaluateSpritesForScanline emulates hardware sprite evaluation from primary OAM (256 bytes) to
// secondary OAM (8 sprites):
// - Nesdev hardware quirk: sprite rendering is delayed by 1 scanline (OAM coordinate Y appears on scanlines Y+1..Y+height).
// - Evaluates up to 8 sprites per scanline starting from startAddr. If more than 8 sprites intersect the line,
//   FlagStatusSpriteOverflow ($2002 bit 5) is set.
func (p *PPU) evaluateSpritesForScanline(scanline int, startAddr byte) {
	spriteHeight := spriteSize8x8Height
	if (p.ctrl & FlagCtrlSpriteSize) != 0 {
		spriteHeight = spriteSize8x16Height
	}

	p.secondaryCount = 0
	p.nextScanlineCount = 0
	p.spriteOverflowPending = false
	p.spriteOverflowCycle = 0
	currAddr := startAddr
	wrapped := false
	evalCycle := cycleSpriteEvalStart

evalLoop:
	for candidate := range maxTotalSprites {
		spriteY := int(p.oam[currAddr])
		row := scanline - (spriteY + spriteScanlineOffset)
		inRange := row >= 0 && row < spriteHeight

		var nextAddr byte
		switch {
		case p.secondaryCount >= maxSpritesPerScanline:
			if inRange {
				p.spriteOverflowPending = true
				p.spriteOverflowCycle = evalCycle + spriteEvalAssertDelay
				break evalLoop
			}
			evalCycle += spriteEvalMissCycles
			nextAddr = currAddr + oamBuggedAdvance
		case !inRange:
			evalCycle += spriteEvalMissCycles
			nextAddr = (currAddr + bytesPerOAMEntry) & maskOAMAddrAlign
		default:
			evalCycle += spriteEvalCopyCycles
			nextAddr = p.appendSecondarySprite(currAddr, scanline, candidate == sprite0Index)
		}

		if nextAddr < currAddr {
			wrapped = true
		}
		if wrapped && (nextAddr>>shiftSpritePalette) >= (startAddr>>shiftSpritePalette) {
			break
		}
		currAddr = nextAddr
	}
}

func (p *PPU) appendSecondarySprite(currAddr byte, scanline int, isSprite0 bool) byte {
	p.secondarySprites[p.secondaryCount] = secondaryOAMSprite{
		y:         p.oam[currAddr],
		tile:      p.oam[currAddr+oamOffsetTile],
		attr:      p.oam[currAddr+oamOffsetAttr],
		x:         p.oam[currAddr+oamOffsetCoordX],
		isSprite0: isSprite0,
	}
	p.secondaryCount++

	spriteX := p.oam[currAddr+oamOffsetCoordX]
	spriteHeight := spriteSize8x8Height
	if (p.ctrl & FlagCtrlSpriteSize) != 0 {
		spriteHeight = spriteSize8x16Height
	}
	xRow := scanline - (int(spriteX) + spriteScanlineOffset)
	if xRow >= 0 && xRow < spriteHeight {
		return currAddr + bytesPerOAMEntry
	}
	return (currAddr + bytesPerOAMEntry) & maskOAMAddrAlign
}

func (p *PPU) fetchSecondarySprite(index int) {
	entry := p.secondarySprites[index]
	spriteHeight := spriteSize8x8Height
	if (p.ctrl & FlagCtrlSpriteSize) != 0 {
		spriteHeight = spriteSize8x16Height
	}
	targetScanline := p.scanline + 1
	if p.scanline == p.timing.preRenderLine {
		targetScanline = 0
	}
	row := targetScanline - (int(entry.y) + spriteScanlineOffset)

	if p.cartridge != nil {
		p.cartridge.SetSpriteFetch(true)
	}
	patternLo, patternHi := p.fetchSpriteTilePattern(entry.tile, entry.attr, row, spriteHeight)
	if p.cartridge != nil {
		p.cartridge.SetSpriteFetch(false)
	}

	p.nextScanlineSprites[index] = scanlineSprite{
		x:         entry.x,
		palette:   entry.attr & maskSpritePalette,
		priority:  entry.attr & maskSpritePriority,
		patternLo: patternLo,
		patternHi: patternHi,
		flipH:     (entry.attr & maskSpriteFlipH) != 0,
		isSprite0: entry.isSprite0,
	}
	p.nextScanlineCount = index + 1
}

func flipSpriteRow(row, height int, attr byte) int {
	if (attr & maskSpriteFlipV) == 0 {
		return row
	}
	if height == spriteSize8x8Height {
		return flipVertical8x8Max - (row & maskSpriteRowOffset)
	}
	return flipVertical8x16Max - row
}

func (p *PPU) sprite8x8TileAddr(tile byte, row int) uint16 {
	tableBase := patternTable0Base
	if (p.ctrl & FlagCtrlSpritePattern) != 0 {
		tableBase = patternTable1Base
	}
	return tableBase | (uint16(tile) * bytesPerTile) + uint16(row&maskSpriteRowOffset)
}

func (p *PPU) sprite8x16TileAddr(tile byte, row int) uint16 {
	tableBase := patternTable0Base
	if (tile & maskSpriteTableSelect) != 0 {
		tableBase = patternTable1Base
	}
	tileNum := tile & maskSpriteTile8x16
	if row >= tilePixelWidth {
		tileNum++
		row -= tilePixelWidth
	}
	return tableBase | (uint16(tileNum) * bytesPerTile) + uint16(row&maskSpriteRowOffset)
}

// fetchSpriteTilePattern fetches the low and high bitplanes from VRAM for the specified sprite tile row.
// In 8x8 mode:
// - PPUCTRL bit 3 selects pattern table ($0000 or $1000).
// In 8x16 mode:
// - Tile byte bit 0 selects pattern table ($0000 if 0, $1000 if 1).
// - Tile byte bits 7-1 select the consecutive tile pair (top tile even, bottom tile odd).
// Inverts the row offset if vertical flip (attribute bit 7) is enabled.
func (p *PPU) fetchSpriteTilePattern(tile, attr byte, row, height int) (byte, byte) {
	effectiveRow := flipSpriteRow(row, height, attr)
	tileAddr := p.sprite8x8TileAddr(tile, effectiveRow)
	if height != spriteSize8x8Height {
		tileAddr = p.sprite8x16TileAddr(tile, effectiveRow)
	}

	lo := p.ReadVRAM(tileAddr)
	hi := p.ReadVRAM(tileAddr + patternPlaneSize)
	return lo, hi
}
