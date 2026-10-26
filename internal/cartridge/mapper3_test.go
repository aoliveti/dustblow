// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"bytes"
	"testing"

	"github.com/aoliveti/dustblow/internal/cartridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testCNROMPRGBanksSingle = 1 // 16 KB PRG-ROM (CNROM-128)
	testCNROMPRGBanksDouble = 2 // 32 KB PRG-ROM (CNROM-256)

	testCNROMCHRBanks2 = 2 // 16 KB CHR-ROM (2 banks of 8 KB)
	testCNROMCHRBanks4 = 4 // 32 KB CHR-ROM (4 banks of 8 KB, standard)
	testCNROMCHRBanks8 = 8 // 64 KB CHR-ROM (8 banks of 8 KB, oversized)

	addressPRGStart    uint16 = 0x8000
	addressPRGMirror   uint16 = 0xC000
	addressCHRWindow   uint16 = 0x0000
	addressCHREnd      uint16 = 0x1FFF
	addressCHROverflow uint16 = 0x2000

	testOverflowBankValue byte = 13
	testExpectedWrapBank  byte = 1
	testOversizedBank     byte = 7
)

func createTestCNROMCartridge(prgBanks, chrBanks int) (*cartridge.Cartridge, *cartridge.Mapper3) {
	prg := make([]byte, prgBanks*testPRGUnit16KB)
	for i := range prgBanks {
		bankStart := i * testPRGUnit16KB
		prg[bankStart] = byte(i + 1)
	}

	chr := make([]byte, chrBanks*testCHRUnit8KB)
	for i := range chrBanks {
		bankStart := i * testCHRUnit8KB
		chr[bankStart] = byte(i + 1)
		chr[bankStart+int(addressCHREnd)] = byte((i + 1) * 2)
	}

	cart := &cartridge.Cartridge{
		PRG:        prg,
		CHR:        chr,
		SRAM:       make([]byte, cartridge.SRAMSize),
		HasBattery: true,
	}

	mapper := cartridge.NewMapper3(cart)
	cart.Mapper = mapper
	return cart, mapper
}

func TestMapper3_InitialPowerOnMapping(t *testing.T) {
	t.Parallel()

	_, mapper := createTestCNROMCartridge(testCNROMPRGBanksSingle, testCNROMCHRBanks4)
	require.NotNil(t, mapper)

	// At power-on, CHR bank 0 must be selected.
	assert.Equal(t, byte(1), mapper.ReadCHR(addressCHRWindow))
	assert.Equal(t, byte(2), mapper.ReadCHR(addressCHREnd))

	// PRG at $8000 must read bank 1 payload.
	assert.Equal(t, byte(1), mapper.ReadPRG(addressPRGStart))
	// 16 KB PRG mirrors to $C000.
	assert.Equal(t, byte(1), mapper.ReadPRG(addressPRGMirror))
}

func TestMapper3_CHRBankSwitching(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		chrBanks          int
		writeBank         byte
		expectedFirstByte byte
		expectedLastByte  byte
	}{
		{
			name:              "switch to bank 1 in 4-bank configuration",
			chrBanks:          testCNROMCHRBanks4,
			writeBank:         1,
			expectedFirstByte: 2,
			expectedLastByte:  4,
		},
		{
			name:              "switch to bank 3 in 4-bank configuration",
			chrBanks:          testCNROMCHRBanks4,
			writeBank:         3,
			expectedFirstByte: 4,
			expectedLastByte:  8,
		},
		{
			name:              "switch with bank overflow wraps with modulo",
			chrBanks:          testCNROMCHRBanks4,
			writeBank:         testOverflowBankValue,
			expectedFirstByte: testExpectedWrapBank + 1,
			expectedLastByte:  (testExpectedWrapBank + 1) * 2,
		},
		{
			name:              "switch to bank 7 in oversized 8-bank configuration",
			chrBanks:          testCNROMCHRBanks8,
			writeBank:         testOversizedBank,
			expectedFirstByte: testOversizedBank + 1,
			expectedLastByte:  (testOversizedBank + 1) * 2,
		},
		{
			name:              "switch back to bank 0",
			chrBanks:          testCNROMCHRBanks4,
			writeBank:         0,
			expectedFirstByte: 1,
			expectedLastByte:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, mapper := createTestCNROMCartridge(testCNROMPRGBanksSingle, tt.chrBanks)
			mapper.WritePRG(addressPRGStart, tt.writeBank)

			assert.Equal(t, tt.expectedFirstByte, mapper.ReadCHR(addressCHRWindow))
			assert.Equal(t, tt.expectedLastByte, mapper.ReadCHR(addressCHREnd))
		})
	}
}

func TestMapper3_PRGMirroring(t *testing.T) {
	t.Parallel()

	t.Run("16 KB PRG mirrors across C000 to FFFF", func(t *testing.T) {
		t.Parallel()

		_, mapper := createTestCNROMCartridge(testCNROMPRGBanksSingle, testCNROMCHRBanks2)
		assert.Equal(t, byte(1), mapper.ReadPRG(addressPRGStart))
		assert.Equal(t, byte(1), mapper.ReadPRG(addressPRGMirror))
	})

	t.Run("32 KB PRG maps linearly without mirroring", func(t *testing.T) {
		t.Parallel()

		_, mapper := createTestCNROMCartridge(testCNROMPRGBanksDouble, testCNROMCHRBanks2)
		assert.Equal(t, byte(1), mapper.ReadPRG(addressPRGStart))
		assert.Equal(t, byte(2), mapper.ReadPRG(addressPRGMirror))
	})
}

func TestMapper3_SRAMAccess(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestCNROMCartridge(testCNROMPRGBanksSingle, testCNROMCHRBanks2)

	assert.Equal(t, byte(0), mapper.ReadPRG(cartridge.SRAMBaseAddress))

	mapper.WritePRG(cartridge.SRAMBaseAddress, sampleSRAMPayload)
	assert.Equal(t, sampleSRAMPayload, mapper.ReadPRG(cartridge.SRAMBaseAddress))
	assert.True(t, cart.IsSRAMDirty())
}

func TestMapper3_CHRRAMWriteAndRead(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestCNROMCartridge(testCNROMPRGBanksSingle, testCNROMCHRBanks2)

	mapper.WriteCHR(addressCHRWindow, sampleTilePattern)
	assert.Equal(t, sampleTilePattern, mapper.ReadCHR(addressCHRWindow))
	assert.Equal(t, sampleTilePattern, cart.CHR[0])
}

func TestMapper3_OutOfBoundsSafety(t *testing.T) {
	t.Parallel()

	_, mapper := createTestCNROMCartridge(testCNROMPRGBanksSingle, testCNROMCHRBanks2)

	// CPU reads below $6000 return 0.
	assert.Equal(t, byte(0), mapper.ReadPRG(0x4020))

	// PPU reads at or above $2000 return 0.
	assert.Equal(t, byte(0), mapper.ReadCHR(addressCHROverflow))

	// Writes to out-of-range addresses do not crash.
	mapper.WritePRG(0x4020, 0xFF)
	mapper.WriteCHR(addressCHROverflow, 0xFF)
}

func TestMapper3_SaveAndLoadState(t *testing.T) {
	t.Parallel()

	_, mapper1 := createTestCNROMCartridge(testCNROMPRGBanksSingle, testCNROMCHRBanks4)
	mapper1.WritePRG(addressPRGStart, 2)
	assert.Equal(t, byte(3), mapper1.ReadCHR(addressCHRWindow))

	var buf bytes.Buffer
	err := mapper1.SaveState(&buf)
	require.NoError(t, err)

	_, mapper2 := createTestCNROMCartridge(testCNROMPRGBanksSingle, testCNROMCHRBanks4)
	err = mapper2.LoadState(&buf)
	require.NoError(t, err)

	assert.Equal(t, byte(3), mapper2.ReadCHR(addressCHRWindow))
}
