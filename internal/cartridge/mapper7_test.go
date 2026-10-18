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
	testAxROMPRGBanks4 = 4 // 128 KB total PRG (AOROM 128KB)
	testAxROMPRGBanks8 = 8 // 256 KB total PRG (AOROM 256KB, Battletoads)

	testBankSize32KB = 32768

	addressPRGWindowStart uint16 = 0x8000
	addressPRGWindowMid   uint16 = 0xA000
	addressPRGWindowEnd   uint16 = 0xFFFF

	addressSRAMBase uint16 = 0x6000
	addressUnmapped uint16 = 0x5000

	mirrorSelectPage0 byte = 0x00 // Bit 4 = 0
	mirrorSelectPage1 byte = 0x10 // Bit 4 = 1

	samplePatternByte byte = 0x8A
	sampleSRAMByte    byte = 0x3F

	testTargetBank1 byte = 1
	testTargetBank2 byte = 2
	testTargetBank3 byte = 3
	testTargetBank7 byte = 7

	testOverflowBank byte = 13 // 13 % 4 = 1
	testWrappedBank  byte = 1
)

func createTestAxROMCartridge(prgBanks int) (*cartridge.Cartridge, *cartridge.Mapper7) {
	prg := make([]byte, prgBanks*testBankSize32KB)
	for i := range prgBanks {
		bankStart := i * testBankSize32KB
		prg[bankStart] = byte(i)
		prg[bankStart+0x2000] = byte(i)
		prg[bankStart+testBankSize32KB-1] = byte(i)
	}

	cart := &cartridge.Cartridge{
		PRG:        prg,
		CHR:        make([]byte, testCHRRAMCapacity),
		SRAM:       make([]byte, cartridge.SRAMSize),
		HasBattery: true,
	}

	mapper := cartridge.NewMapper7(cart)
	cart.Mapper = mapper
	return cart, mapper
}

func TestMapper7_InitialPowerOnMapping(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestAxROMCartridge(testAxROMPRGBanks4)
	require.NotNil(t, mapper)

	// At power-on, bank 0 is mapped at $8000-$FFFF.
	assert.Equal(t, byte(0), mapper.ReadPRG(addressPRGWindowStart))
	assert.Equal(t, byte(0), mapper.ReadPRG(addressPRGWindowEnd))

	// Initial mirroring is single-screen page 0.
	assert.Equal(t, cartridge.MirrorSingle0, cart.Mirroring)
}

func TestMapper7_PRGBankSwitching(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		writeVal     byte
		expectedBank byte
	}{
		{
			name:         "Switch to Bank 1",
			writeVal:     testTargetBank1,
			expectedBank: testTargetBank1,
		},
		{
			name:         "Switch to Bank 2",
			writeVal:     testTargetBank2,
			expectedBank: testTargetBank2,
		},
		{
			name:         "Switch to Bank 3",
			writeVal:     testTargetBank3,
			expectedBank: testTargetBank3,
		},
		{
			name:         "Switch to Bank with Mirror Page 1 bit set",
			writeVal:     testTargetBank2 | mirrorSelectPage1,
			expectedBank: testTargetBank2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, mapper := createTestAxROMCartridge(testAxROMPRGBanks4)
			mapper.WritePRG(addressPRGWindowStart, tt.writeVal)

			assert.Equal(t, tt.expectedBank, mapper.ReadPRG(addressPRGWindowStart))
			assert.Equal(t, tt.expectedBank, mapper.ReadPRG(addressPRGWindowMid))
			assert.Equal(t, tt.expectedBank, mapper.ReadPRG(addressPRGWindowEnd))
		})
	}
}

func TestMapper7_ModuloWrapping(t *testing.T) {
	t.Parallel()

	_, mapper := createTestAxROMCartridge(testAxROMPRGBanks4)

	// Write bank 13 on a 4-bank cartridge: 13 % 4 = 1
	mapper.WritePRG(addressPRGWindowStart, testOverflowBank)
	assert.Equal(t, testWrappedBank, mapper.ReadPRG(addressPRGWindowStart))
}

func TestMapper7_SingleScreenMirroringSwitch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		writeVal          byte
		expectedMirroring cartridge.Mirroring
	}{
		{
			name:              "Select Single-Screen Page 1",
			writeVal:          mirrorSelectPage1,
			expectedMirroring: cartridge.MirrorSingle1,
		},
		{
			name:              "Select Single-Screen Page 0",
			writeVal:          mirrorSelectPage0,
			expectedMirroring: cartridge.MirrorSingle0,
		},
		{
			name:              "Select Single-Screen Page 1 with Bank 3",
			writeVal:          testTargetBank3 | mirrorSelectPage1,
			expectedMirroring: cartridge.MirrorSingle1,
		},
		{
			name:              "Select Single-Screen Page 0 with Bank 3",
			writeVal:          testTargetBank3 | mirrorSelectPage0,
			expectedMirroring: cartridge.MirrorSingle0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cart, mapper := createTestAxROMCartridge(testAxROMPRGBanks8)
			mapper.WritePRG(addressPRGWindowStart, tt.writeVal)

			assert.Equal(t, tt.expectedMirroring, cart.Mirroring)
		})
	}
}

func TestMapper7_CHRRAMReadWrite(t *testing.T) {
	t.Parallel()

	_, mapper := createTestAxROMCartridge(testAxROMPRGBanks4)

	const testPatternAddress uint16 = 0x0450
	const outOfBoundsAddress uint16 = 0x2500

	mapper.WriteCHR(testPatternAddress, samplePatternByte)
	assert.Equal(t, samplePatternByte, mapper.ReadCHR(testPatternAddress))

	// Out of bounds reads and writes must be safe no-ops
	assert.Equal(t, byte(0), mapper.ReadCHR(outOfBoundsAddress))
	mapper.WriteCHR(outOfBoundsAddress, samplePatternByte)
}

func TestMapper7_PRGRAMReadWrite(t *testing.T) {
	t.Parallel()

	_, mapper := createTestAxROMCartridge(testAxROMPRGBanks4)

	mapper.WritePRG(addressSRAMBase, sampleSRAMByte)
	assert.Equal(t, sampleSRAMByte, mapper.ReadPRG(addressSRAMBase))

	// Unmapped address below $6000 returns 0
	assert.Equal(t, byte(0), mapper.ReadPRG(addressUnmapped))
	mapper.WritePRG(addressUnmapped, sampleSRAMByte)
}

func TestMapper7_SaveAndLoadState(t *testing.T) {
	t.Parallel()

	cart1, mapper1 := createTestAxROMCartridge(testAxROMPRGBanks8)
	mapper1.WritePRG(addressPRGWindowStart, testTargetBank3|mirrorSelectPage1)
	assert.Equal(t, testTargetBank3, mapper1.ReadPRG(addressPRGWindowStart))
	assert.Equal(t, cartridge.MirrorSingle1, cart1.Mirroring)

	var buf bytes.Buffer
	err := mapper1.SaveState(&buf)
	require.NoError(t, err)

	cart2, mapper2 := createTestAxROMCartridge(testAxROMPRGBanks8)
	err = mapper2.LoadState(&buf)
	require.NoError(t, err)

	assert.Equal(t, testTargetBank3, mapper2.ReadPRG(addressPRGWindowStart))
	assert.Equal(t, cartridge.MirrorSingle1, cart2.Mirroring)
}
