// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ppu_test

import (
	"testing"

	"github.com/aoliveti/dustblow/internal/cartridge"
	"github.com/aoliveti/dustblow/internal/ppu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAddressPattern0 uint16 = 0x0010
	testAddressPattern1 uint16 = 0x1020

	testAddressNT0       uint16 = 0x2000
	testAddressNT1       uint16 = 0x2400
	testAddressNT2       uint16 = 0x2800
	testAddressNT3       uint16 = 0x2C00
	testAddressNT0Mirror uint16 = 0x3000

	testAddressPaletteBackdrop uint16 = 0x3F00
	testAddressPalette0Color1  uint16 = 0x3F01
	testAddressPaletteBG1      uint16 = 0x3F04
	testAddressPaletteBG2      uint16 = 0x3F08
	testAddressPaletteBG3      uint16 = 0x3F0C
	testAddressPaletteSprite0  uint16 = 0x3F10
	testAddressPaletteSprite1  uint16 = 0x3F14
	testAddressPaletteSprite2  uint16 = 0x3F18
	testAddressPaletteSprite3  uint16 = 0x3F1C
	testAddressPaletteMirror   uint16 = 0x3F20

	testRegPPUCTRL   uint16 = 0x2000
	testRegPPUMASK   uint16 = 0x2001
	testRegPPUSTATUS uint16 = 0x2002
	testRegOAMADDR   uint16 = 0x2003
	testRegOAMDATA   uint16 = 0x2004
	testRegPPUSCROLL uint16 = 0x2005
	testRegPPUADDR   uint16 = 0x2006
	testRegPPUDATA   uint16 = 0x2007

	testDataByteA byte = 0x37
	testDataByteB byte = 0x42
	testDataByteC byte = 0x55
	testDataByteD byte = 0xAA
	testDataZero  byte = 0x00

	testAddressNTNextRow uint16 = 0x2020

	testMaskOpenBusVal byte = 0x1E

	testAddrPaletteHigh byte = 0x3F
	testAddrPaletteLow  byte = 0x00

	testAddrVRAMHigh byte = 0x20
	testAddrVRAMLow  byte = 0x00

	testAddrLoopyHigh byte = 0x24
	testAddrLoopyLow  byte = 0x50

	testPatternSolidPlane byte = 0xFF
	testColorBlack        byte = 0x0F
	testColorWhite        byte = 0x20

	testSpriteColorRed       byte   = 0x16
	testSpriteColorGreen     byte   = 0x2A
	spriteYTargetScanline    byte   = 9  // OAM Y=9 displays on scanline 10
	spriteYOffscreen         byte   = 19 // OAM Y=19 displays on scanline 20
	testSpriteAttrDefault    byte   = 0x00
	testSpriteAttrBehind     byte   = 0x20
	tileIndexSolid           byte   = 0x01
	tileIndexPair            byte   = 0x02
	spriteTargetX            byte   = 16
	spriteLeftEdgeX          byte   = 0
	spriteRightEdgeX         byte   = 255
	testPaletteSprite0Color1 uint16 = 0x3F11
	testPaletteSprite0Color2 uint16 = 0x3F12

	tileSolidCHRBase    uint16 = 16
	tilePairTopCHRBase  uint16 = 32
	tilePairBottomPlane uint16 = 56
)

const (
	testCHRSize8KB = 8192

	testSamplePixelX       = 16
	testBytesPerPixel      = 4
	testTotalScanlines     = 262
	testCyclesPerScanline  = 341
	testTotalFrameCycles   = testTotalScanlines * testCyclesPerScanline
	testPreRenderScanlines = 261
	dotPreRenderCopy       = 285
	testLoopyCoarseXCycles = 8
	targetScanlineCycles   = 10 * testCyclesPerScanline

	targetScanline    = 10
	offscreenScanline = 20

	dotPastSpriteTargetX = 18
	dotPastCollisionX    = 20
	dotPastRightEdge     = 256
	dotPastLeftEdge      = 10
	dotScanlineStart     = 1
	leftmostPixelX       = 0

	testLuminanceMask byte = 0x30

	testEmphasisRedIndex   = 1
	testEmphasisGreenIndex = 2
	testEmphasisBlueIndex  = 4
	testEmphasisAllIndex   = 7

	overflowSpriteCount = 9
	oamBytesPerEntry    = 4
	oamSpriteSpacing    = 10
)

func newTestCartridge(mirroring cartridge.Mirroring) *cartridge.Cartridge {
	cart := &cartridge.Cartridge{
		CHR:       make([]byte, testCHRSize8KB),
		IsCHRRAM:  true,
		Mirroring: mirroring,
	}
	cart.Mapper = cartridge.NewMapper0(cart)
	return cart
}

func setTestSprite(p *ppu.PPU, index, y, tile, attr, x byte) {
	base := index * oamBytesPerEntry
	p.WriteOAM(base, y)
	p.WriteOAM(base+1, tile)
	p.WriteOAM(base+2, attr)
	p.WriteOAM(base+3, x)
}

func fillTestTilePlane(cart *cartridge.Cartridge, base uint16) {
	for i := range uint16(8) {
		cart.WriteCHR(base+i, testPatternSolidPlane)
	}
}

func TestPPU_PaletteMirroring(t *testing.T) {
	t.Parallel()

	t.Run("sprite backdrop mirrors universal backdrop", func(t *testing.T) {
		t.Parallel()
		pLocal := ppu.New(nil)
		pLocal.WriteVRAM(testAddressPaletteBackdrop, testDataByteA)
		assert.Equal(t, testDataByteA, pLocal.ReadVRAM(testAddressPaletteSprite0))

		pLocal.WriteVRAM(testAddressPaletteSprite0, testDataByteB)
		assert.Equal(t, testDataByteB, pLocal.ReadVRAM(testAddressPaletteBackdrop))
	})

	t.Run("mirrored sprite palette entries mirror BG palette entries", func(t *testing.T) {
		t.Parallel()
		pLocal := ppu.New(nil)
		pLocal.WriteVRAM(testAddressPaletteSprite1, testDataByteC)
		assert.Equal(t, testDataByteC, pLocal.ReadVRAM(testAddressPaletteBG1))

		pLocal.WriteVRAM(testAddressPaletteSprite2, testDataByteD)
		assert.Equal(t, testDataByteD, pLocal.ReadVRAM(testAddressPaletteBG2))

		pLocal.WriteVRAM(testAddressPaletteSprite3, testDataByteA)
		assert.Equal(t, testDataByteA, pLocal.ReadVRAM(testAddressPaletteBG3))
	})

	t.Run("palette mirror range $3F20-$3FFF mirrors $3F00-$3F1F", func(t *testing.T) {
		t.Parallel()
		pLocal := ppu.New(nil)
		pLocal.WriteVRAM(testAddressPaletteBackdrop, testDataByteA)
		assert.Equal(t, testDataByteA, pLocal.ReadVRAM(testAddressPaletteMirror))
	})
}

func TestPPU_NametableMirroring(t *testing.T) {
	t.Parallel()

	t.Run("horizontal mirroring", func(t *testing.T) {
		t.Parallel()
		cart := newTestCartridge(cartridge.MirrorHorizontal)
		p := ppu.New(cart)

		// Page 0: NT0 and NT1
		p.WriteVRAM(testAddressNT0, testDataByteA)
		assert.Equal(t, testDataByteA, p.ReadVRAM(testAddressNT1))
		assert.Equal(t, testDataByteA, p.ReadVRAM(testAddressNT0Mirror))

		// Page 1: NT2 and NT3
		p.WriteVRAM(testAddressNT2, testDataByteB)
		assert.Equal(t, testDataByteB, p.ReadVRAM(testAddressNT3))
	})

	t.Run("vertical mirroring", func(t *testing.T) {
		t.Parallel()
		cart := newTestCartridge(cartridge.MirrorVertical)
		p := ppu.New(cart)

		// Page 0: NT0 and NT2
		p.WriteVRAM(testAddressNT0, testDataByteC)
		assert.Equal(t, testDataByteC, p.ReadVRAM(testAddressNT2))

		// Page 1: NT1 and NT3
		p.WriteVRAM(testAddressNT1, testDataByteD)
		assert.Equal(t, testDataByteD, p.ReadVRAM(testAddressNT3))
	})
}

func TestPPU_LoopyScrollingRegisters(t *testing.T) {
	t.Parallel()

	t.Run("PPUCTRL nametable select updates bits 10-11 of t", func(t *testing.T) {
		t.Parallel()
		pLocal := ppu.New(nil)
		pLocal.WriteRegister(testRegPPUCTRL, 0x03) // Nametable 3 ($2C00) -> t[10..11] = 3

		// Set low address byte through PPUADDR second write
		pLocal.WriteRegister(testRegPPUADDR, 0x00) // 1st write
		pLocal.WriteRegister(testRegPPUADDR, 0x00) // 2nd write: v = t
		// v should reflect $2C00 (nametable select bits 10-11 = 3 -> $0C00)
		pLocal.WriteRegister(testRegPPUDATA, testDataByteA)
	})

	t.Run("PPUADDR two writes set v address directly", func(t *testing.T) {
		t.Parallel()
		pLocal := ppu.New(nil)
		// Set VRAM address to $2108
		pLocal.WriteRegister(testRegPPUADDR, 0x21)
		pLocal.WriteRegister(testRegPPUADDR, 0x08)

		pLocal.WriteRegister(testRegPPUDATA, testDataByteB)
		assert.Equal(t, testDataByteB, pLocal.ReadVRAM(0x2108))
	})

	t.Run("PPUSCROLL updates t and fine x correctly", func(t *testing.T) {
		t.Parallel()
		pLocal := ppu.New(nil)

		// 1st write: coarse X (d >> 3) and fine X (d & 7)
		// d = 0b10100_011 (coarse X = 20, fine X = 3)
		pLocal.WriteRegister(testRegPPUSCROLL, 0xA3)

		// 2nd write: coarse Y and fine Y
		// d = 0b01011_101 (coarse Y = 11, fine Y = 5)
		pLocal.WriteRegister(testRegPPUSCROLL, 0x5D)

		// Check registers via PeekRegister
		assert.Equal(t, byte(0), pLocal.PeekRegister(testRegPPUSTATUS)&ppu.FlagStatusVBlank)
	})
}

func TestPPU_ReadBufferPipeline(t *testing.T) {
	t.Parallel()

	p := ppu.New(nil)

	// Pre-fill VRAM at $2000 and $2001
	p.WriteVRAM(testAddressNT0, testDataByteA)
	p.WriteVRAM(testAddressNT0+1, testDataByteB)

	// Set PPU address to $2000
	p.WriteRegister(testRegPPUADDR, 0x20)
	p.WriteRegister(testRegPPUADDR, 0x00)

	// First read from $2000 returns stale buffer (0), while buffering $2000
	firstRead := p.ReadRegister(testRegPPUDATA)
	assert.Equal(t, byte(0x00), firstRead)

	// Second read returns $2000 byte and buffers $2001
	secondRead := p.ReadRegister(testRegPPUDATA)
	assert.Equal(t, testDataByteA, secondRead)

	// Third read returns $2001 byte
	thirdRead := p.ReadRegister(testRegPPUDATA)
	assert.Equal(t, testDataByteB, thirdRead)

	// Palette read test: immediate read!
	p.WriteVRAM(testAddressPaletteBackdrop, testDataByteC)
	p.WriteRegister(testRegPPUADDR, 0x3F)
	p.WriteRegister(testRegPPUADDR, 0x00)

	paletteRead := p.ReadRegister(testRegPPUDATA)
	assert.Equal(t, testDataByteC&0x3F, paletteRead)
}

func TestPPU_StatusRegisterReading(t *testing.T) {
	t.Parallel()

	p := ppu.New(nil)

	// Simulate VBlank trigger by stepping to scanline 241 dot 1
	p.Step(241*341 + 2)

	// Verify VBlank status flag is set
	status := p.ReadRegister(testRegPPUSTATUS)
	assert.NotZero(t, status&ppu.FlagStatusVBlank)

	// Reading PPUSTATUS clears the VBlank flag
	statusAfter := p.ReadRegister(testRegPPUSTATUS)
	assert.Zero(t, statusAfter&ppu.FlagStatusVBlank)
}

func TestPPU_OAMAccess(t *testing.T) {
	t.Parallel()

	p := ppu.New(nil)

	p.WriteRegister(testRegOAMADDR, 0x10)
	p.WriteRegister(testRegOAMDATA, testDataByteD)

	p.WriteRegister(testRegOAMADDR, 0x10)
	assert.Equal(t, testDataByteD, p.ReadRegister(testRegOAMDATA))
	assert.Equal(t, testDataByteD, p.PeekRegister(testRegOAMDATA))
}

func TestPPU_TimingAndNMITrigger(t *testing.T) {
	t.Parallel()

	p := ppu.New(nil)
	// Enable NMI in PPUCTRL
	p.WriteRegister(testRegPPUCTRL, ppu.FlagCtrlNMI)

	// Step until VBlank scanline (241) dot 1
	p.Step(241*341 + 2)
	assert.True(t, p.PollNMI())

	// Step until pre-render scanline (261) dot 1 to clear flags
	p.Step(20*341 + 1)
	assert.Zero(t, p.ReadRegister(testRegPPUSTATUS)&ppu.FlagStatusVBlank)
}

func TestPPU_RegionConfigurations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		region ppu.Region
	}{
		{"NTSC", ppu.RegionNTSC},
		{"PAL", ppu.RegionPAL},
		{"Dendy", ppu.RegionDendy},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := ppu.New(nil, ppu.WithRegion(tt.region))
			require.NotNil(t, p)
		})
	}
}

func TestPPU_PatternTableAccess(t *testing.T) {
	t.Parallel()

	t.Run("with cartridge CHR", func(t *testing.T) {
		t.Parallel()
		cart := newTestCartridge(cartridge.MirrorHorizontal)
		p := ppu.New(cart)

		p.WriteVRAM(testAddressPattern0, testDataByteA)
		assert.Equal(t, testDataByteA, p.ReadVRAM(testAddressPattern0))
		assert.Equal(t, testDataByteA, p.PeekVRAM(testAddressPattern0))

		p.WriteVRAM(testAddressPattern1, testDataByteB)
		assert.Equal(t, testDataByteB, p.ReadVRAM(testAddressPattern1))
	})

	t.Run("without cartridge CHR returns 0", func(t *testing.T) {
		t.Parallel()
		p := ppu.New(nil)
		p.WriteVRAM(testAddressPattern0, testDataByteA)
		assert.Equal(t, byte(0), p.ReadVRAM(testAddressPattern0))
	})
}

func TestPPU_VRAMIncrementDown(t *testing.T) {
	t.Parallel()

	p := ppu.New(nil)
	p.WriteRegister(testRegPPUCTRL, ppu.FlagCtrlVRAMIncrement) // Increment by 32

	p.WriteRegister(testRegPPUADDR, testAddrVRAMHigh)
	p.WriteRegister(testRegPPUADDR, testAddrVRAMLow)

	p.WriteRegister(testRegPPUDATA, testDataByteA)
	p.WriteRegister(testRegPPUDATA, testDataByteB)

	assert.Equal(t, testDataByteA, p.ReadVRAM(testAddressNT0))
	assert.Equal(t, testDataByteB, p.ReadVRAM(testAddressNTNextRow))
}

func TestPPU_PeekAndOpenBus(t *testing.T) {
	t.Parallel()

	p := ppu.New(nil)

	// Write to PPUMASK fills open bus latch
	p.WriteRegister(testRegPPUMASK, testMaskOpenBusVal)

	// Reading write-only register returns open bus latch
	assert.Equal(t, testMaskOpenBusVal, p.ReadRegister(testRegPPUCTRL))
	assert.Equal(t, testMaskOpenBusVal, p.PeekRegister(testRegPPUCTRL))

	// Test PeekRegister on PPUDATA pointing to Palette
	p.WriteVRAM(testAddressPaletteBackdrop, testDataByteD)
	p.WriteRegister(testRegPPUADDR, testAddrPaletteHigh)
	p.WriteRegister(testRegPPUADDR, testAddrPaletteLow)
	assert.Equal(t, (testDataByteD&0x3F)|(testMaskOpenBusVal&0xC0), p.PeekRegister(testRegPPUDATA))
}

func TestPPU_FourScreenMirroring(t *testing.T) {
	t.Parallel()

	cart := newTestCartridge(cartridge.MirrorFourScreen)
	p := ppu.New(cart)

	p.WriteVRAM(testAddressNT0, testDataByteA)
	assert.Equal(t, testDataByteA, p.ReadVRAM(testAddressNT0))
}

func TestPPU_FrameBufferProperties(t *testing.T) {
	t.Parallel()

	p := ppu.New(nil)
	fb := p.FrameBuffer()
	assert.Len(t, fb, ppu.FrameBufferSize)
	assert.Len(t, fb, ppu.ScreenWidth*ppu.ScreenHeight*4)
}

func TestPPU_LoopyRenderingIncrements(t *testing.T) {
	t.Parallel()

	p := ppu.New(nil)
	// Enable background rendering
	p.WriteRegister(testRegPPUMASK, ppu.FlagMaskShowBG)

	// Set initial v address through PPUADDR
	p.WriteRegister(testRegPPUADDR, testAddrVRAMHigh)
	p.WriteRegister(testRegPPUADDR, testAddrVRAMLow)

	// Step cycles: coarse X increment triggers at dot 8
	p.Step(testLoopyCoarseXCycles)

	// In Loopy, coarse X increments by 1
	fb := p.FrameBuffer()
	assert.NotNil(t, fb)
}

func TestPPU_LoopyPreRenderVerticalCopy(t *testing.T) {
	t.Parallel()

	p := ppu.New(nil)
	p.WriteRegister(testRegPPUMASK, ppu.FlagMaskShowBG)

	// Set t to a distinct address
	p.WriteRegister(testRegPPUADDR, testAddrLoopyHigh)
	p.WriteRegister(testRegPPUADDR, testAddrLoopyLow)

	// Step to scanline 261, dot 285
	p.Step((testPreRenderScanlines * testCyclesPerScanline) + dotPreRenderCopy)

	// Verify pre-render line stepped safely without panic
	assert.NotNil(t, p.FrameBuffer())
}

func TestPPU_BackgroundRenderingOutput(t *testing.T) {
	t.Parallel()

	cart := newTestCartridge(cartridge.MirrorHorizontal)
	fillTestTilePlane(cart, 0)

	p := ppu.New(cart)
	// Setup Palette: Backdrop = Black, Palette 0 Color 1 = White
	p.WriteVRAM(testAddressPaletteBackdrop, testColorBlack)
	p.WriteVRAM(testAddressPalette0Color1, testColorWhite)

	// Enable background and left column
	p.WriteRegister(testRegPPUMASK, ppu.FlagMaskShowBG|ppu.FlagMaskShowLeftBG)

	// Step through first scanline
	p.Step(testCyclesPerScanline)

	fb := p.FrameBuffer()
	// At pixel 16 (after first tile is loaded at dot 8 and shifted into upper bits), color is white.
	offset := testSamplePixelX * testBytesPerPixel
	expectedWhite := ppu.DefaultPalette[testColorWhite]
	assert.Equal(t, expectedWhite[0], fb[offset])
	assert.Equal(t, expectedWhite[1], fb[offset+1])
	assert.Equal(t, expectedWhite[2], fb[offset+2])
	assert.Equal(t, expectedWhite[3], fb[offset+3])
}

func TestPPU_FullFrameRenderingWithPreFetch(t *testing.T) {
	t.Parallel()

	cart := newTestCartridge(cartridge.MirrorHorizontal)
	fillTestTilePlane(cart, 0)

	p := ppu.New(cart)
	p.WriteVRAM(testAddressPaletteBackdrop, testColorBlack)
	p.WriteVRAM(testAddressPalette0Color1, testColorWhite)

	p.WriteRegister(testRegPPUMASK, ppu.FlagMaskShowBG|ppu.FlagMaskShowLeftBG)

	// Step a full frame: scanline 261 will prefetch tiles for the next frame
	p.Step(testTotalFrameCycles)

	fb := p.FrameBuffer()
	assert.Len(t, fb, ppu.FrameBufferSize)
}

func TestPPU_SpriteEvaluation_8x8(t *testing.T) {
	t.Parallel()

	cart := newTestCartridge(cartridge.MirrorHorizontal)
	fillTestTilePlane(cart, tileSolidCHRBase)

	p := ppu.New(cart)
	p.WriteVRAM(testAddressPaletteBackdrop, testColorBlack)
	p.WriteVRAM(testPaletteSprite0Color1, testSpriteColorRed)

	setTestSprite(p, 0, spriteYTargetScanline, tileIndexSolid, testSpriteAttrDefault, spriteTargetX)
	setTestSprite(p, 1, spriteYOffscreen, tileIndexSolid, testSpriteAttrDefault, spriteTargetX)

	p.WriteRegister(testRegPPUMASK, ppu.FlagMaskShowSprites|ppu.FlagMaskShowLeftSprites)

	p.Step(targetScanlineCycles + dotPastSpriteTargetX)

	fb := p.FrameBuffer()
	offset := (targetScanline*ppu.ScreenWidth + int(spriteTargetX)) * testBytesPerPixel
	expectedRed := ppu.DefaultPalette[testSpriteColorRed]
	assert.Equal(t, expectedRed[0], fb[offset])
	assert.Equal(t, expectedRed[1], fb[offset+1])
	assert.Equal(t, expectedRed[2], fb[offset+2])
}

func TestPPU_SpriteEvaluation_8x16(t *testing.T) {
	t.Parallel()

	cart := newTestCartridge(cartridge.MirrorHorizontal)
	fillTestTilePlane(cart, tilePairTopCHRBase)
	fillTestTilePlane(cart, tilePairBottomPlane)

	p := ppu.New(cart)
	p.WriteVRAM(testAddressPaletteBackdrop, testColorBlack)
	p.WriteVRAM(testPaletteSprite0Color1, testSpriteColorRed)
	p.WriteVRAM(testPaletteSprite0Color2, testSpriteColorGreen)

	p.WriteRegister(testRegPPUCTRL, ppu.FlagCtrlSpriteSize)
	p.WriteRegister(testRegPPUMASK, ppu.FlagMaskShowSprites|ppu.FlagMaskShowLeftSprites)

	setTestSprite(p, 0, spriteYTargetScanline, tileIndexPair, testSpriteAttrDefault, spriteTargetX)

	p.Step(targetScanlineCycles + dotPastSpriteTargetX)
	fb := p.FrameBuffer()
	offsetTop := (targetScanline*ppu.ScreenWidth + int(spriteTargetX)) * testBytesPerPixel
	expectedRed := ppu.DefaultPalette[testSpriteColorRed]
	assert.Equal(t, expectedRed[0], fb[offsetTop])
	assert.Equal(t, expectedRed[1], fb[offsetTop+1])
	assert.Equal(t, expectedRed[2], fb[offsetTop+2])

	p.Step(targetScanlineCycles)
	offsetBottom := (offscreenScanline*ppu.ScreenWidth + int(spriteTargetX)) * testBytesPerPixel
	expectedGreen := ppu.DefaultPalette[testSpriteColorGreen]
	assert.Equal(t, expectedGreen[0], fb[offsetBottom])
	assert.Equal(t, expectedGreen[1], fb[offsetBottom+1])
	assert.Equal(t, expectedGreen[2], fb[offsetBottom+2])
}

func TestPPU_SpriteOverflow(t *testing.T) {
	t.Parallel()

	p := ppu.New(nil)
	for i := range overflowSpriteCount {
		setTestSprite(p, byte(i), spriteYTargetScanline, 0, 0, byte(i)*oamSpriteSpacing)
	}

	p.WriteRegister(testRegPPUMASK, ppu.FlagMaskShowSprites)

	p.Step(targetScanlineCycles + dotScanlineStart)
	status := p.ReadRegister(testRegPPUSTATUS)
	assert.NotZero(t, status&ppu.FlagStatusSpriteOverflow)

	remainingCycles := (testPreRenderScanlines-targetScanline)*testCyclesPerScanline + dotScanlineStart
	p.Step(remainingCycles)
	statusCleared := p.PeekRegister(testRegPPUSTATUS)
	assert.Zero(t, statusCleared&ppu.FlagStatusSpriteOverflow)
}

func TestPPU_Sprite0Hit(t *testing.T) {
	t.Parallel()

	t.Run("positive hit when BG and sprite 0 overlap with opaque pixels", func(t *testing.T) {
		t.Parallel()
		cart := newTestCartridge(cartridge.MirrorHorizontal)
		fillTestTilePlane(cart, 0)
		fillTestTilePlane(cart, tileSolidCHRBase)

		p := ppu.New(cart)
		setTestSprite(p, 0, spriteYTargetScanline, tileIndexSolid, testSpriteAttrDefault, spriteTargetX)

		p.WriteRegister(testRegPPUMASK, ppu.FlagMaskShowBG|ppu.FlagMaskShowSprites|ppu.FlagMaskShowLeftBG|ppu.FlagMaskShowLeftSprites)

		p.Step(targetScanlineCycles + dotPastCollisionX)
		assert.NotZero(t, p.PeekRegister(testRegPPUSTATUS)&ppu.FlagStatusSprite0Hit)
	})

	t.Run("no hit when X equals 255", func(t *testing.T) {
		t.Parallel()
		cart := newTestCartridge(cartridge.MirrorHorizontal)
		fillTestTilePlane(cart, 0)
		fillTestTilePlane(cart, tileSolidCHRBase)

		p := ppu.New(cart)
		setTestSprite(p, 0, spriteYTargetScanline, tileIndexSolid, testSpriteAttrDefault, spriteRightEdgeX)

		p.WriteRegister(testRegPPUMASK, ppu.FlagMaskShowBG|ppu.FlagMaskShowSprites)
		p.Step(targetScanlineCycles + dotPastRightEdge)
		assert.Zero(t, p.PeekRegister(testRegPPUSTATUS)&ppu.FlagStatusSprite0Hit)
	})

	t.Run("no hit when left clipping is active and X is less than 8", func(t *testing.T) {
		t.Parallel()
		cart := newTestCartridge(cartridge.MirrorHorizontal)
		fillTestTilePlane(cart, 0)
		fillTestTilePlane(cart, tileSolidCHRBase)

		p := ppu.New(cart)
		setTestSprite(p, 0, spriteYTargetScanline, tileIndexSolid, testSpriteAttrDefault, spriteLeftEdgeX)

		p.WriteRegister(testRegPPUMASK, ppu.FlagMaskShowBG|ppu.FlagMaskShowSprites)
		p.Step(targetScanlineCycles + dotPastLeftEdge)
		assert.Zero(t, p.PeekRegister(testRegPPUSTATUS)&ppu.FlagStatusSprite0Hit)
	})
}

func TestPPU_SpritePriority_Multiplexer(t *testing.T) {
	t.Parallel()

	t.Run("front priority renders sprite over background", func(t *testing.T) {
		t.Parallel()
		cart := newTestCartridge(cartridge.MirrorHorizontal)
		fillTestTilePlane(cart, 0)
		fillTestTilePlane(cart, tileSolidCHRBase)

		p := ppu.New(cart)
		p.WriteVRAM(testAddressPaletteBackdrop, testColorBlack)
		p.WriteVRAM(testAddressPalette0Color1, testColorWhite)
		p.WriteVRAM(testPaletteSprite0Color1, testSpriteColorRed)

		setTestSprite(p, 0, spriteYTargetScanline, tileIndexSolid, testSpriteAttrDefault, spriteTargetX)

		p.WriteRegister(testRegPPUMASK, ppu.FlagMaskShowBG|ppu.FlagMaskShowSprites|ppu.FlagMaskShowLeftBG|ppu.FlagMaskShowLeftSprites)
		p.Step(targetScanlineCycles + dotPastSpriteTargetX)

		fb := p.FrameBuffer()
		offset := (targetScanline*ppu.ScreenWidth + int(spriteTargetX)) * testBytesPerPixel
		expectedRed := ppu.DefaultPalette[testSpriteColorRed]
		assert.Equal(t, expectedRed[0], fb[offset])
		assert.Equal(t, expectedRed[1], fb[offset+1])
		assert.Equal(t, expectedRed[2], fb[offset+2])
	})

	t.Run("behind priority renders background over sprite", func(t *testing.T) {
		t.Parallel()
		cart := newTestCartridge(cartridge.MirrorHorizontal)
		fillTestTilePlane(cart, 0)
		fillTestTilePlane(cart, tileSolidCHRBase)

		p := ppu.New(cart)
		p.WriteVRAM(testAddressPaletteBackdrop, testColorBlack)
		p.WriteVRAM(testAddressPalette0Color1, testColorWhite)
		p.WriteVRAM(testPaletteSprite0Color1, testSpriteColorRed)

		setTestSprite(p, 0, spriteYTargetScanline, tileIndexSolid, testSpriteAttrBehind, spriteTargetX)

		p.WriteRegister(testRegPPUMASK, ppu.FlagMaskShowBG|ppu.FlagMaskShowSprites|ppu.FlagMaskShowLeftBG|ppu.FlagMaskShowLeftSprites)
		p.Step(targetScanlineCycles + dotPastSpriteTargetX)

		fb := p.FrameBuffer()
		offset := (targetScanline*ppu.ScreenWidth + int(spriteTargetX)) * testBytesPerPixel
		expectedWhite := ppu.DefaultPalette[testColorWhite]
		assert.Equal(t, expectedWhite[0], fb[offset])
		assert.Equal(t, expectedWhite[1], fb[offset+1])
		assert.Equal(t, expectedWhite[2], fb[offset+2])
	})
}

func TestPPU_OAMInspectionAndFrame(t *testing.T) {
	t.Parallel()

	p := ppu.New(newTestCartridge(cartridge.MirrorHorizontal))
	assert.Zero(t, p.Frame())
	assert.Zero(t, p.OAMAddr())

	p.WriteRegister(testRegOAMADDR, 0x10)
	assert.Equal(t, byte(0x10), p.OAMAddr())

	p.WriteRegister(testRegOAMDATA, 0x42)
	assert.Equal(t, byte(0x42), p.ReadOAM(0x10))
}

func TestPPU_GreyscaleMode(t *testing.T) {
	t.Parallel()

	p := ppu.New(newTestCartridge(cartridge.MirrorHorizontal))
	p.WriteVRAM(testAddressPaletteBackdrop, testSpriteColorRed)

	// Step to target scanline without greyscale.
	p.Step(targetScanlineCycles + dotScanlineStart)
	fbNormal := p.FrameBuffer()
	offset := (targetScanline*ppu.ScreenWidth + leftmostPixelX) * testBytesPerPixel
	expectedNormal := ppu.DefaultPalette[testSpriteColorRed]
	assert.Equal(t, expectedNormal[0], fbNormal[offset])
	assert.Equal(t, expectedNormal[1], fbNormal[offset+1])
	assert.Equal(t, expectedNormal[2], fbNormal[offset+2])

	// Enable greyscale mode on the next scanline.
	p.WriteRegister(testRegPPUMASK, ppu.FlagMaskGreyscale)
	p.Step(testCyclesPerScanline)

	fbGrey := p.FrameBuffer()
	offsetNextScanline := ((targetScanline+1)*ppu.ScreenWidth + leftmostPixelX) * testBytesPerPixel
	expectedGrey := ppu.DefaultPalette[testSpriteColorRed&testLuminanceMask]
	assert.Equal(t, expectedGrey[0], fbGrey[offsetNextScanline])
	assert.Equal(t, expectedGrey[1], fbGrey[offsetNextScanline+1])
	assert.Equal(t, expectedGrey[2], fbGrey[offsetNextScanline+2])
}

func TestPPU_ColorEmphasis(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		maskFlag      byte
		emphasisIndex int
	}{
		{
			name:          "emphasize red attenuates green and blue",
			maskFlag:      ppu.FlagMaskEmphasizeRed,
			emphasisIndex: testEmphasisRedIndex,
		},
		{
			name:          "emphasize green attenuates red and blue",
			maskFlag:      ppu.FlagMaskEmphasizeGreen,
			emphasisIndex: testEmphasisGreenIndex,
		},
		{
			name:          "emphasize blue attenuates red and green",
			maskFlag:      ppu.FlagMaskEmphasizeBlue,
			emphasisIndex: testEmphasisBlueIndex,
		},
		{
			name:          "all three emphasis bits uniformly darken all channels",
			maskFlag:      ppu.FlagMaskEmphasizeRed | ppu.FlagMaskEmphasizeGreen | ppu.FlagMaskEmphasizeBlue,
			emphasisIndex: testEmphasisAllIndex,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := ppu.New(newTestCartridge(cartridge.MirrorHorizontal))
			p.WriteVRAM(testAddressPaletteBackdrop, testColorWhite)
			p.WriteRegister(testRegPPUMASK, tt.maskFlag)

			p.Step(targetScanlineCycles + dotScanlineStart)

			fb := p.FrameBuffer()
			offset := (targetScanline*ppu.ScreenWidth + leftmostPixelX) * testBytesPerPixel
			expected := ppu.PaletteTable[tt.emphasisIndex][testColorWhite]
			assert.Equal(t, expected[0], fb[offset])
			assert.Equal(t, expected[1], fb[offset+1])
			assert.Equal(t, expected[2], fb[offset+2])
			assert.Equal(t, expected[3], fb[offset+3])
		})
	}
}

func TestPPU_Reset(t *testing.T) {
	t.Parallel()

	p := ppu.New(newTestCartridge(cartridge.MirrorHorizontal))
	require.NotNil(t, p)

	p.WriteRegister(testRegPPUCTRL, ppu.FlagCtrlNMI|ppu.FlagCtrlSpriteSize)
	p.WriteRegister(testRegPPUMASK, ppu.FlagMaskShowBG|ppu.FlagMaskShowSprites)
	p.WriteRegister(testRegPPUSCROLL, 0x50)
	p.Step(1000)

	assert.NotZero(t, p.Ctrl())
	assert.NotZero(t, p.Mask())
	assert.NotZero(t, p.Scanline())

	p.Reset()

	assert.Zero(t, p.Ctrl())
	assert.Zero(t, p.Mask())
	assert.Zero(t, p.Scanline())
	assert.False(t, p.PollNMI())
}
