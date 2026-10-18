// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"bytes"
	"testing"

	"github.com/dustblow/dustblow/internal/cartridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testMMC5PRG128K = 128 * 1024
	testMMC5CHR128K = 128 * 1024
	testMMC5SRAM64K = 64 * 1024

	addrMMC5PRGMode      = 0x5100
	addrMMC5CHRMode      = 0x5101
	addrMMC5RAMProtect1  = 0x5102
	addrMMC5RAMProtect2  = 0x5103
	addrMMC5ExRAMMode    = 0x5104
	addrMMC5NametableMap = 0x5105
	addrMMC5FillTile     = 0x5106
	addrMMC5FillPalette  = 0x5107
	addrMMC5PRGRAMBank   = 0x5113
	addrMMC5PRGBank0     = 0x5114
	addrMMC5PRGBank1     = 0x5115
	addrMMC5PRGBank2     = 0x5116
	addrMMC5PRGBank3     = 0x5117
	addrMMC5CHRSprite0   = 0x5120
	addrMMC5CHRSprite7   = 0x5127
	addrMMC5CHRBGBase    = 0x5128
	addrMMC5CHRBGEnd     = 0x512B
	addrMMC5CHRHigh      = 0x5130
	addrMMC5IRQCompare   = 0x5203
	addrMMC5IRQStatus    = 0x5204
	addrMMC5MultA        = 0x5205
	addrMMC5MultB        = 0x5206
	addrMMC5ExRAMBase    = 0x5C00
	addrMMC5PRGRAMBase   = 0x6000

	protect1Enable = 0x02
	protect2Enable = 0x01

	exRAMModeNametable    = 0
	exRAMModeExtendedAttr = 1
	exRAMModeReadWrite    = 2
	exRAMModeReadOnly     = 3
)

func createTestMMC5Cartridge(t *testing.T) (*cartridge.Cartridge, cartridge.Mapper) {
	t.Helper()

	prg := make([]byte, testMMC5PRG128K)
	for i := range prg {
		prg[i] = byte(i / 0x2000) // Each 8KB bank contains its bank index
	}

	chr := make([]byte, testMMC5CHR128K)
	for i := range chr {
		chr[i] = byte(i / 0x0400) // Each 1KB bank contains its bank index
	}

	cart := &cartridge.Cartridge{
		PRG:        prg,
		CHR:        chr,
		SRAM:       make([]byte, testMMC5SRAM64K),
		MapperID:   5,
		HasBattery: true,
	}

	mapper := cartridge.NewMapper5(cart)
	cart.Mapper = mapper
	return cart, mapper
}

func TestMapper5_HardwareMultiplier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		factorA   byte
		factorB   byte
		wantLow   byte
		wantHigh  byte
	}{
		{
			name:     "Zero multiplication",
			factorA:  0,
			factorB:  42,
			wantLow:  0,
			wantHigh: 0,
		},
		{
			name:     "Small factors 3 x 5",
			factorA:  3,
			factorB:  5,
			wantLow:  15,
			wantHigh: 0,
		},
		{
			name:     "Large factors 255 x 255 (65025 = 0xFE01)",
			factorA:  0xFF,
			factorB:  0xFF,
			wantLow:  0x01,
			wantHigh: 0xFE,
		},
		{
			name:     "Medium factors 100 x 50 (5000 = 0x1388)",
			factorA:  100,
			factorB:  50,
			wantLow:  0x88,
			wantHigh: 0x13,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, mapper := createTestMMC5Cartridge(t)
			mapper.WritePRG(addrMMC5MultA, tt.factorA)
			mapper.WritePRG(addrMMC5MultB, tt.factorB)

			low := mapper.ReadPRG(addrMMC5MultA)
			high := mapper.ReadPRG(addrMMC5MultB)

			assert.Equal(t, tt.wantLow, low, "Product low byte mismatch")
			assert.Equal(t, tt.wantHigh, high, "Product high byte mismatch")
		})
	}
}

func TestMapper5_RAMProtection(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestMMC5Cartridge(t)

	// Writing to RAM without protection enabled should fail
	mapper.WritePRG(addrMMC5RAMProtect1, 0)
	mapper.WritePRG(addrMMC5RAMProtect2, 0)
	mapper.WritePRG(addrMMC5PRGRAMBase, 0xAA)
	assert.Equal(t, byte(0), mapper.ReadPRG(addrMMC5PRGRAMBase))
	assert.False(t, cart.IsSRAMDirty())

	// Setting only protection 1 should still fail
	mapper.WritePRG(addrMMC5RAMProtect1, protect1Enable)
	mapper.WritePRG(addrMMC5PRGRAMBase, 0xAA)
	assert.Equal(t, byte(0), mapper.ReadPRG(addrMMC5PRGRAMBase))

	// Setting both protection registers unlocks write access
	mapper.WritePRG(addrMMC5RAMProtect2, protect2Enable)
	mapper.WritePRG(addrMMC5PRGRAMBase, 0xAA)
	assert.Equal(t, byte(0xAA), mapper.ReadPRG(addrMMC5PRGRAMBase))
	assert.True(t, cart.IsSRAMDirty())
}

func TestMapper5_PRGBankingModes(t *testing.T) {
	t.Parallel()

	_, mapper := createTestMMC5Cartridge(t)

	// Mode 3: four 8KB banks
	mapper.WritePRG(addrMMC5PRGMode, 3)
	mapper.WritePRG(addrMMC5PRGBank0, 0x80|2) // Bank 2 ROM at $8000
	mapper.WritePRG(addrMMC5PRGBank1, 0x80|4) // Bank 4 ROM at $A000
	mapper.WritePRG(addrMMC5PRGBank2, 0x80|6) // Bank 6 ROM at $C000
	mapper.WritePRG(addrMMC5PRGBank3, 0x80|7) // Bank 7 ROM at $E000

	assert.Equal(t, byte(2), mapper.ReadPRG(0x8000))
	assert.Equal(t, byte(4), mapper.ReadPRG(0xA000))
	assert.Equal(t, byte(6), mapper.ReadPRG(0xC000))
	assert.Equal(t, byte(7), mapper.ReadPRG(0xE000))

	// Mode 0: one 32KB bank selected by $5117
	mapper.WritePRG(addrMMC5PRGMode, 0)
	mapper.WritePRG(addrMMC5PRGBank3, 0x80|(1<<2)) // 32KB bank 1 = 8KB banks 4, 5, 6, 7

	assert.Equal(t, byte(4), mapper.ReadPRG(0x8000))
	assert.Equal(t, byte(5), mapper.ReadPRG(0xA000))
	assert.Equal(t, byte(6), mapper.ReadPRG(0xC000))
	assert.Equal(t, byte(7), mapper.ReadPRG(0xE000))
}

func TestMapper5_ExRAM(t *testing.T) {
	t.Parallel()

	_, mapper := createTestMMC5Cartridge(t)

	// Mode 2: CPU Read/Write ordinary RAM at $5C00-$5FFF
	mapper.WritePRG(addrMMC5ExRAMMode, 2)
	mapper.WritePRG(addrMMC5ExRAMBase+10, 0x42)
	assert.Equal(t, byte(0x42), mapper.ReadPRG(addrMMC5ExRAMBase+10))

	// Mode 3: CPU Read-Only
	mapper.WritePRG(addrMMC5ExRAMMode, 3)
	mapper.WritePRG(addrMMC5ExRAMBase+10, 0x99) // Should be ignored
	assert.Equal(t, byte(0x42), mapper.ReadPRG(addrMMC5ExRAMBase+10))
}

func TestMapper5_NametableMappingAndFillMode(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestMMC5Cartridge(t)
	vram := make([]byte, 2048)
	vram[10] = 0x11
	vram[1024+10] = 0x22

	// Setup:
	// Quadrant 0 ($2000): CIRAM 0
	// Quadrant 1 ($2400): CIRAM 1
	// Quadrant 2 ($2800): ExRAM (mode 0)
	// Quadrant 3 ($2C00): Fill Mode (Tile $FE, Palette 2)
	mapper.WritePRG(addrMMC5ExRAMMode, 0)
	mapper.WritePRG(addrMMC5FillTile, 0xFE)
	mapper.WritePRG(addrMMC5FillPalette, 2)
	// Mapping byte: Q0=0, Q1=1, Q2=2, Q3=3 -> 0b11_10_01_00 = 0xE4
	mapper.WritePRG(addrMMC5NametableMap, 0xE4)

	// Test Quadrant 0 (CIRAM 0)
	val, ok := cart.ReadNametable(0x2000+10, vram)
	require.True(t, ok)
	assert.Equal(t, byte(0x11), val)

	// Test Quadrant 1 (CIRAM 1)
	val, ok = cart.ReadNametable(0x2400+10, vram)
	require.True(t, ok)
	assert.Equal(t, byte(0x22), val)

	// Test Quadrant 2 (ExRAM)
	cart.WriteNametable(0x2800+10, 0x33, vram)
	val, ok = cart.ReadNametable(0x2800+10, vram)
	require.True(t, ok)
	assert.Equal(t, byte(0x33), val)

	// Test Quadrant 3 (Fill Mode Tile)
	val, ok = cart.ReadNametable(0x2C00+10, vram)
	require.True(t, ok)
	assert.Equal(t, byte(0xFE), val)

	// Test Quadrant 3 (Fill Mode Attribute: palette 2 repeated -> 0b10101010 = 0xAA)
	val, ok = cart.ReadNametable(0x2C00+0x3C5, vram)
	require.True(t, ok)
	assert.Equal(t, byte(0xAA), val)
}

func TestMapper5_ScanlineIRQ(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestMMC5Cartridge(t)

	// Configure target scanline to 10 and enable IRQ
	mapper.WritePRG(addrMMC5IRQCompare, 10)
	mapper.WritePRG(addrMMC5IRQStatus, 0x80)

	// Pre-render / start of frame
	cart.ClockScanline(0)
	assert.False(t, cart.IRQPending())

	// Clock through scanlines 1 to 9
	for scanline := 1; scanline <= 9; scanline++ {
		cart.ClockScanline(scanline)
		assert.False(t, cart.IRQPending(), "IRQ should not trigger before scanline 10")
	}

	// Scanline 10: IRQ should trigger
	cart.ClockScanline(10)
	assert.True(t, cart.IRQPending(), "IRQ should trigger on scanline 10")

	// Read $5204: should return IRQ pending (bit 7) and in-frame (bit 6), and clear pending
	status := mapper.ReadPRG(addrMMC5IRQStatus)
	assert.Equal(t, byte(0xC0), status&0xC0)
	assert.False(t, cart.IRQPending(), "Reading $5204 should clear IRQ pending")
}

func TestMapper5_CHRDualSetSwitching(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestMMC5Cartridge(t)

	// 1KB Mode
	mapper.WritePRG(addrMMC5CHRMode, 3)

	// Set A: sprite banks 10..17
	for i := range 8 {
		mapper.WritePRG(uint16(addrMMC5CHRSprite0+i), byte(10+i))
	}
	// Set B: background banks 20..23
	for i := range 4 {
		mapper.WritePRG(uint16(addrMMC5CHRBGBase+i), byte(20+i))
	}

	// When isSpriteFetch is false and Set B was written last -> reads from Set B
	assert.Equal(t, byte(20), mapper.ReadCHR(0x0000))
	assert.Equal(t, byte(21), mapper.ReadCHR(0x0400))

	// When sprite fetch is active -> reads from Set A
	cart.SetSpriteFetch(true)
	assert.Equal(t, byte(10), mapper.ReadCHR(0x0000))
	assert.Equal(t, byte(11), mapper.ReadCHR(0x0400))

	cart.SetSpriteFetch(false)
	assert.Equal(t, byte(20), mapper.ReadCHR(0x0000))
}

func TestMapper5_ExRAMModes(t *testing.T) {
	t.Parallel()

	_, mapper := createTestMMC5Cartridge(t)

	const (
		testOffset = 0x0150
		payload1   = 0x42
		payload2   = 0x84
		payload3   = 0x99
		payload4   = 0xAA
	)

	// Mode 0: CPU can write to ExRAM, but CPU reads return 0 (open bus)
	mapper.WritePRG(addrMMC5ExRAMMode, exRAMModeNametable)
	mapper.WritePRG(addrMMC5ExRAMBase+testOffset, payload1)
	assert.Equal(t, byte(0), mapper.ReadPRG(addrMMC5ExRAMBase+testOffset), "Mode 0 CPU read must return 0")

	// Verify write succeeded by reading in Mode 2
	mapper.WritePRG(addrMMC5ExRAMMode, exRAMModeReadWrite)
	assert.Equal(t, byte(payload1), mapper.ReadPRG(addrMMC5ExRAMBase+testOffset), "Mode 2 must see write from Mode 0")

	// Mode 1: CPU can write to ExRAM, but CPU reads return 0
	mapper.WritePRG(addrMMC5ExRAMMode, exRAMModeExtendedAttr)
	mapper.WritePRG(addrMMC5ExRAMBase+testOffset, payload2)
	assert.Equal(t, byte(0), mapper.ReadPRG(addrMMC5ExRAMBase+testOffset), "Mode 1 CPU read must return 0")

	// Verify write succeeded by reading in Mode 2
	mapper.WritePRG(addrMMC5ExRAMMode, exRAMModeReadWrite)
	assert.Equal(t, byte(payload2), mapper.ReadPRG(addrMMC5ExRAMBase+testOffset), "Mode 2 must see write from Mode 1")

	// Mode 2: CPU can write and read directly
	mapper.WritePRG(addrMMC5ExRAMBase+testOffset, payload3)
	assert.Equal(t, byte(payload3), mapper.ReadPRG(addrMMC5ExRAMBase+testOffset), "Mode 2 read must return written value")

	// Mode 3: ExRAM is read-only for CPU writes
	mapper.WritePRG(addrMMC5ExRAMMode, exRAMModeReadOnly)
	mapper.WritePRG(addrMMC5ExRAMBase+testOffset, payload4)
	assert.Equal(t, byte(payload3), mapper.ReadPRG(addrMMC5ExRAMBase+testOffset), "Mode 3 must reject CPU writes")
}

func TestMapper5_ExtendedAttributeMode(t *testing.T) {
	t.Parallel()

	const (
		testMMC5CHR512K = 512 * 1024
		tileOffset      = 0x0010
		attrBaseOffset  = 0x03C0
		testTileID      = 0x3F
		chosenBank      = 5
		chosenPalette   = 2 // 0b10
		expectedAttr    = (chosenPalette << 6) | (chosenPalette << 4) | (chosenPalette << 2) | chosenPalette
		exRAMPayload    = (chosenPalette << 6) | chosenBank
		highBankBit     = 1
		combinedBank    = chosenBank | (highBankBit << 6)
	)

	chr := make([]byte, testMMC5CHR512K)
	for i := range chr {
		chr[i] = byte(i / 0x1000) // Each 4KB bank contains its 4KB bank index (0..127)
	}

	cart := &cartridge.Cartridge{
		PRG:      make([]byte, testMMC5PRG128K),
		CHR:      chr,
		SRAM:     make([]byte, testMMC5SRAM64K),
		MapperID: 5,
	}
	mapper := cartridge.NewMapper5(cart)
	cart.Mapper = mapper
	vram := make([]byte, 2048)

	// Set ExRAM Mode 1 (Extended Attribute Mode)
	mapper.WritePRG(addrMMC5ExRAMMode, exRAMModeExtendedAttr)

	// Map Quadrant 0 ($2000) to CIRAM 0
	mapper.WritePRG(addrMMC5NametableMap, 0x00)

	// Write tile ID to CIRAM 0 VRAM
	vram[tileOffset] = testTileID

	// Write Extended Attribute byte to ExRAM at tileOffset
	mapper.WritePRG(addrMMC5ExRAMBase+tileOffset, exRAMPayload)

	// 1. PPU fetches nametable tile byte at $2000 + tileOffset
	tileRead, ok := cart.ReadNametable(0x2000+tileOffset, vram)
	require.True(t, ok)
	assert.Equal(t, byte(testTileID), tileRead, "Nametable read must return tile from CIRAM")

	// 2. PPU fetches attribute byte (offset >= 0x03C0)
	attrRead, ok := cart.ReadNametable(0x2000+attrBaseOffset, vram)
	require.True(t, ok)
	assert.Equal(t, byte(expectedAttr), attrRead, "Attribute read must return 4-way replicated palette")

	// 3. PPU fetches background CHR pattern data: must read from 4KB bank 5
	cart.SetSpriteFetch(false)
	chrRead := mapper.ReadCHR(0x0000)
	assert.Equal(t, byte(chosenBank), chrRead, "Background CHR fetch must use ExRAM 4KB bank")

	// 4. Test CHR High register ($5130) contribution
	mapper.WritePRG(addrMMC5CHRHigh, highBankBit)
	chrReadHigh := mapper.ReadCHR(0x0000)
	assert.Equal(t, byte(combinedBank), chrReadHigh, "Background CHR fetch must include $5130 high bits")

	// 5. When sprite fetch is active, normal sprite banking must be used instead
	mapper.WritePRG(addrMMC5CHRHigh, 0)
	cart.SetSpriteFetch(true)
	spriteCHR := mapper.ReadCHR(0x0000)
	assert.NotEqual(t, byte(chosenBank), spriteCHR, "Sprite CHR fetch must bypass extended attribute mode")
}

func TestMapper5_SaveAndLoadState(t *testing.T) {
	t.Parallel()

	cart1, mapper1 := createTestMMC5Cartridge(t)

	// 1. Hardware multiplier
	mapper1.WritePRG(addrMMC5MultA, 0x12)
	mapper1.WritePRG(addrMMC5MultB, 0x34)
	expectedMultLow := mapper1.ReadPRG(addrMMC5MultA)
	expectedMultHigh := mapper1.ReadPRG(addrMMC5MultB)

	// 2. PRG Mode 3 (8KB banks) and map $8000 to bank 5
	mapper1.WritePRG(addrMMC5PRGMode, 3)
	mapper1.WritePRG(addrMMC5PRGBank0, 0x80|5)

	// 3. ExRAM in read/write mode and write test byte
	mapper1.WritePRG(addrMMC5RAMProtect1, protect1Enable)
	mapper1.WritePRG(addrMMC5RAMProtect2, protect2Enable)
	mapper1.WritePRG(addrMMC5ExRAMMode, exRAMModeReadWrite)
	mapper1.WritePRG(addrMMC5ExRAMBase+0x42, 0x77)

	// 4. IRQ compare line
	mapper1.WritePRG(addrMMC5IRQCompare, 10)
	mapper1.WritePRG(addrMMC5IRQStatus, 0x80) // enable IRQ

	// Step scanlines 0..10 -> IRQ fires on scanline 10
	for scanline := range 11 {
		cart1.ClockScanline(scanline)
	}
	assert.True(t, cart1.IRQPending())

	stateful1, ok := mapper1.(cartridge.StatefulMapper)
	require.True(t, ok)

	var buf bytes.Buffer
	err := stateful1.SaveState(&buf)
	require.NoError(t, err)

	cart2, mapper2 := createTestMMC5Cartridge(t)
	stateful2, ok := mapper2.(cartridge.StatefulMapper)
	require.True(t, ok)

	err = stateful2.LoadState(&buf)
	require.NoError(t, err)

	// Verify multiplier restored
	assert.Equal(t, expectedMultLow, mapper2.ReadPRG(addrMMC5MultA))
	assert.Equal(t, expectedMultHigh, mapper2.ReadPRG(addrMMC5MultB))

	// Verify PRG banking restored
	assert.Equal(t, byte(5), mapper2.ReadPRG(0x8000))

	// Verify ExRAM byte restored
	assert.Equal(t, byte(0x77), mapper2.ReadPRG(addrMMC5ExRAMBase+0x42))

	// Verify IRQ pending restored
	assert.True(t, cart2.IRQPending())
}

