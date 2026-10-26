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
	testUxROMPRGBanks8  = 8  // 128 KB total PRG (UNROM configuration)
	testUOROMPRGBanks16 = 16 // 256 KB total PRG (UOROM configuration)
	testCHRRAMCapacity  = 8192

	addressPRGLowerWindow uint16 = 0x8000
	addressPRGUpperWindow uint16 = 0xC000
	addressCHRRAMWindow   uint16 = 0x0000

	sampleTilePattern byte = 0x5C
	sampleSRAMPayload byte = 0x7E

	testOverflowBankSelect byte = 11
	testModuloExpectedBank byte = 3
	testTargetUOROMBank    byte = 14
)

func createTestUxROMCartridge(prgBanks int) (*cartridge.Cartridge, *cartridge.Mapper2) {
	prg := make([]byte, prgBanks*testPRGUnit16KB)
	for i := range prgBanks {
		bankStart := i * testPRGUnit16KB
		prg[bankStart] = byte(i)
	}

	cart := &cartridge.Cartridge{
		PRG:        prg,
		CHR:        make([]byte, testCHRRAMCapacity),
		SRAM:       make([]byte, cartridge.SRAMSize),
		HasBattery: true,
	}

	mapper := cartridge.NewMapper2(cart)
	cart.Mapper = mapper
	return cart, mapper
}

func TestMapper2_InitialPowerOnMapping(t *testing.T) {
	t.Parallel()

	_, mapper := createTestUxROMCartridge(testUxROMPRGBanks8)
	require.NotNil(t, mapper)

	// At power-on, lower window ($8000) maps to bank 0.
	assert.Equal(t, byte(0), mapper.ReadPRG(addressPRGLowerWindow))

	// Upper window ($C000) maps to the last bank (bank 7).
	expectedLastBank := byte(testUxROMPRGBanks8 - 1)
	assert.Equal(t, expectedLastBank, mapper.ReadPRG(addressPRGUpperWindow))
}

func TestMapper2_PRGBankSwitching(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		writeBank    byte
		expectedBank byte
	}{
		{
			name:         "switch to bank 1",
			writeBank:    1,
			expectedBank: 1,
		},
		{
			name:         "switch to bank 5",
			writeBank:    5,
			expectedBank: 5,
		},
		{
			name:         "switch to bank with high bits set wraps with modulo",
			writeBank:    testOverflowBankSelect,
			expectedBank: testModuloExpectedBank,
		},
		{
			name:         "switch back to bank 0",
			writeBank:    0,
			expectedBank: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, mapper := createTestUxROMCartridge(testUxROMPRGBanks8)
			mapper.WritePRG(addressPRGLowerWindow, tt.writeBank)

			assert.Equal(t, tt.expectedBank, mapper.ReadPRG(addressPRGLowerWindow))

			// Upper bank must strictly remain fixed to the last bank (bank 7).
			expectedLastBank := byte(testUxROMPRGBanks8 - 1)
			assert.Equal(t, expectedLastBank, mapper.ReadPRG(addressPRGUpperWindow))
		})
	}
}

func TestMapper2_UnmappedMemory(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestUxROMCartridge(testUxROMPRGBanks8)
	cart.SRAM = nil // Simulate cartridge without PRG-RAM

	assert.Equal(t, byte(0), mapper.ReadPRG(addressLowRAM))
	assert.Equal(t, byte(0), mapper.ReadPRG(addressCartridgeRAM))
}

func TestMapper2_SRAMReadWrite(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestUxROMCartridge(testUxROMPRGBanks8)

	mapper.WritePRG(addressCartridgeRAM, sampleSRAMPayload)
	assert.Equal(t, sampleSRAMPayload, mapper.ReadPRG(addressCartridgeRAM))
	assert.True(t, cart.IsSRAMDirty())
}

func TestMapper2_CHRRAMReadWrite(t *testing.T) {
	t.Parallel()

	_, mapper := createTestUxROMCartridge(testUxROMPRGBanks8)

	mapper.WriteCHR(addressCHRRAMWindow, sampleTilePattern)
	assert.Equal(t, sampleTilePattern, mapper.ReadCHR(addressCHRRAMWindow))

	// Out of bounds CHR read returns zero and write does not panic.
	assert.Equal(t, byte(0), mapper.ReadCHR(addressInvalidHighCHR))
	mapper.WriteCHR(addressInvalidHighCHR, sampleTilePattern)
}

func TestMapper2_UOROM_256KB(t *testing.T) {
	t.Parallel()

	expectedLastBank := byte(testUOROMPRGBanks16 - 1)

	_, mapper := createTestUxROMCartridge(testUOROMPRGBanks16)
	mapper.WritePRG(addressPRGLowerWindow, testTargetUOROMBank)

	assert.Equal(t, testTargetUOROMBank, mapper.ReadPRG(addressPRGLowerWindow))
	assert.Equal(t, expectedLastBank, mapper.ReadPRG(addressPRGUpperWindow))
}

func TestMapper2_SaveAndLoadState(t *testing.T) {
	t.Parallel()

	_, mapper1 := createTestUxROMCartridge(testUxROMPRGBanks8)
	mapper1.WritePRG(addressPRGLowerWindow, 5)
	assert.Equal(t, byte(5), mapper1.ReadPRG(addressPRGLowerWindow))

	var buf bytes.Buffer
	err := mapper1.SaveState(&buf)
	require.NoError(t, err)

	_, mapper2 := createTestUxROMCartridge(testUxROMPRGBanks8)
	err = mapper2.LoadState(&buf)
	require.NoError(t, err)

	assert.Equal(t, byte(5), mapper2.ReadPRG(addressPRGLowerWindow))
}
