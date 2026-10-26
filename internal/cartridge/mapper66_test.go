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
	testGxROMPRGBanks4 = 4 // 128 KB total PRG-ROM
	testGxROMCHRBanks4 = 4 // 32 KB total CHR-ROM

	addressGxROMPRGBase uint16 = 0x8000
	addressGxROMPRGEnd  uint16 = 0xFFFF
	addressGxROMCHRBase uint16 = 0x0000
	addressGxROMCHREnd  uint16 = 0x1FFF

	targetGxROMPRGBank1 byte = 1
	targetGxROMPRGBank2 byte = 2
	targetGxROMCHRBank1 byte = 1
	targetGxROMCHRBank3 byte = 3

	// Format: [..PP ..CC]
	payloadSwitchPRG1CHR1 byte = (1 << 4) | 1
	payloadSwitchPRG2CHR3 byte = (2 << 4) | 3
	payloadSwitchWrap     byte = (5 << 4) | 6 // 5 % 4 = 1, 6 % 4 = 2
)

func createTestGxROMCartridge() (*cartridge.Cartridge, *cartridge.Mapper66) {
	prg := make([]byte, testGxROMPRGBanks4*testBankSize32KB)
	for i := range testGxROMPRGBanks4 {
		bankStart := i * testBankSize32KB
		fillVal := byte(i)
		for j := range testBankSize32KB {
			prg[bankStart+j] = fillVal
		}
	}

	chr := make([]byte, testGxROMCHRBanks4*testBankSize8KB)
	for i := range testGxROMCHRBanks4 {
		bankStart := i * testBankSize8KB
		fillVal := byte(i)
		for j := range testBankSize8KB {
			chr[bankStart+j] = fillVal
		}
	}

	cart := &cartridge.Cartridge{
		PRG:        prg,
		CHR:        chr,
		SRAM:       make([]byte, cartridge.SRAMSize),
		HasBattery: true,
	}

	mapper := cartridge.NewMapper66(cart)
	cart.Mapper = mapper
	return cart, mapper
}

func TestMapper66_InitialPowerOnMapping(t *testing.T) {
	t.Parallel()

	_, mapper := createTestGxROMCartridge()
	require.NotNil(t, mapper)

	// At power on, bank 0 is selected for PRG and CHR.
	assert.Equal(t, byte(0), mapper.ReadPRG(addressGxROMPRGBase))
	assert.Equal(t, byte(0), mapper.ReadPRG(addressGxROMPRGEnd))
	assert.Equal(t, byte(0), mapper.ReadCHR(addressGxROMCHRBase))
	assert.Equal(t, byte(0), mapper.ReadCHR(addressGxROMCHREnd))
}

func TestMapper66_BankSwitching(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		writePayload    byte
		expectedPRGBank byte
		expectedCHRBank byte
	}{
		{
			name:            "Switch PRG to Bank 1 and CHR to Bank 1",
			writePayload:    payloadSwitchPRG1CHR1,
			expectedPRGBank: targetGxROMPRGBank1,
			expectedCHRBank: targetGxROMCHRBank1,
		},
		{
			name:            "Switch PRG to Bank 2 and CHR to Bank 3",
			writePayload:    payloadSwitchPRG2CHR3,
			expectedPRGBank: targetGxROMPRGBank2,
			expectedCHRBank: targetGxROMCHRBank3,
		},
		{
			name:            "Switch with Bank modulo wrap-around",
			writePayload:    payloadSwitchWrap,
			expectedPRGBank: targetGxROMPRGBank1,
			expectedCHRBank: targetGxROMPRGBank2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, mapper := createTestGxROMCartridge()

			mapper.WritePRG(addressGxROMPRGBase, tt.writePayload)
			assert.Equal(t, tt.expectedPRGBank, mapper.ReadPRG(addressGxROMPRGBase))
			assert.Equal(t, tt.expectedPRGBank, mapper.ReadPRG(addressGxROMPRGEnd))
			assert.Equal(t, tt.expectedCHRBank, mapper.ReadCHR(addressGxROMCHRBase))
			assert.Equal(t, tt.expectedCHRBank, mapper.ReadCHR(addressGxROMCHREnd))
		})
	}
}

func TestMapper66_SRAMReadWrite(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestGxROMCartridge()

	assert.Equal(t, byte(0), mapper.ReadPRG(addressSRAMBase))
	mapper.WritePRG(addressSRAMBase, sampleSRAMByte)
	assert.Equal(t, sampleSRAMByte, mapper.ReadPRG(addressSRAMBase))
	assert.True(t, cart.IsSRAMDirty())
}

func TestMapper66_CHRWriteAndOutOfBounds(t *testing.T) {
	t.Parallel()

	_, mapper := createTestGxROMCartridge()

	// Write to CHR RAM/ROM
	mapper.WriteCHR(addressGxROMCHRBase, samplePatternByte)
	assert.Equal(t, samplePatternByte, mapper.ReadCHR(addressGxROMCHRBase))

	// Out of bounds addresses
	assert.Equal(t, byte(0), mapper.ReadCHR(addressCHROutOfBounds))
	mapper.WriteCHR(addressCHROutOfBounds, samplePatternByte)
	assert.Equal(t, byte(0), mapper.ReadPRG(addressUnmapped))
	mapper.WritePRG(addressUnmapped, samplePatternByte)
}

func TestMapper66_SaveAndLoadState(t *testing.T) {
	t.Parallel()

	_, mapper1 := createTestGxROMCartridge()
	mapper1.WritePRG(addressGxROMPRGBase, payloadSwitchPRG2CHR3)
	assert.Equal(t, targetGxROMPRGBank2, mapper1.ReadPRG(addressGxROMPRGBase))
	assert.Equal(t, targetGxROMCHRBank3, mapper1.ReadCHR(addressGxROMCHRBase))

	var buf bytes.Buffer
	err := mapper1.SaveState(&buf)
	require.NoError(t, err)

	_, mapper2 := createTestGxROMCartridge()
	err = mapper2.LoadState(&buf)
	require.NoError(t, err)

	assert.Equal(t, targetGxROMPRGBank2, mapper2.ReadPRG(addressGxROMPRGBase))
	assert.Equal(t, targetGxROMCHRBank3, mapper2.ReadCHR(addressGxROMCHRBase))
}
