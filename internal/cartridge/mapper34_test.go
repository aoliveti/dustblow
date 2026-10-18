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
	testBNROMPRGBanks4 = 4 // 128 KB total PRG-ROM (Deadly Towers)
	testNINAPRGBanks2  = 2 // 64 KB total PRG-ROM (Impossible Mission II)
	testNINACHRBanks16 = 16 // 64 KB total CHR-ROM

	addressBNROMPRGBase uint16 = 0x8000
	addressBNROMPRGEnd  uint16 = 0xFFFF

	addressNINARegPRG  uint16 = 0x7FFD
	addressNINARegCHR0 uint16 = 0x7FFE
	addressNINARegCHR1 uint16 = 0x7FFF

	targetBNROMPRGBank1 byte = 1
	targetBNROMPRGBank2 byte = 2
	targetBNROMPRGBank3 byte = 3

	targetNINACHRBank4 byte = 4
	targetNINACHRBank7 byte = 7
)

func createTestBNROMCartridge() (*cartridge.Cartridge, *cartridge.Mapper34) {
	prg := make([]byte, testBNROMPRGBanks4*testBankSize32KB)
	for i := range testBNROMPRGBanks4 {
		bankStart := i * testBankSize32KB
		fillVal := byte(i)
		for j := range testBankSize32KB {
			prg[bankStart+j] = fillVal
		}
	}

	cart := &cartridge.Cartridge{
		PRG:        prg,
		CHR:        make([]byte, testCHRRAMCapacity),
		SRAM:       make([]byte, cartridge.SRAMSize),
		HasBattery: true,
	}

	mapper := cartridge.NewMapper34(cart)
	cart.Mapper = mapper
	return cart, mapper
}

func createTestNINACartridge() *cartridge.Mapper34 {
	prg := make([]byte, testNINAPRGBanks2*testBankSize32KB)
	for i := range testNINAPRGBanks2 {
		bankStart := i * testBankSize32KB
		fillVal := byte(i)
		for j := range testBankSize32KB {
			prg[bankStart+j] = fillVal
		}
	}

	chr := make([]byte, testNINACHRBanks16*testBankSize4KB)
	for i := range testNINACHRBanks16 {
		bankStart := i * testBankSize4KB
		fillVal := byte(i)
		for j := range testBankSize4KB {
			chr[bankStart+j] = fillVal
		}
	}

	cart := &cartridge.Cartridge{
		PRG:        prg,
		CHR:        chr,
		SRAM:       make([]byte, cartridge.SRAMSize),
		HasBattery: true,
	}

	mapper := cartridge.NewMapper34(cart)
	cart.Mapper = mapper
	return mapper
}

func TestMapper34_BNROMInitialMapping(t *testing.T) {
	t.Parallel()

	_, mapper := createTestBNROMCartridge()
	require.NotNil(t, mapper)

	// At power on, bank 0 is mapped to $8000-$FFFF.
	assert.Equal(t, byte(0), mapper.ReadPRG(addressBNROMPRGBase))
	assert.Equal(t, byte(0), mapper.ReadPRG(addressBNROMPRGEnd))
}

func TestMapper34_BNROMPRGBankSwitching(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		writeVal     byte
		expectedBank byte
	}{
		{
			name:         "Switch to Bank 1",
			writeVal:     targetBNROMPRGBank1,
			expectedBank: targetBNROMPRGBank1,
		},
		{
			name:         "Switch to Bank 2",
			writeVal:     targetBNROMPRGBank2,
			expectedBank: targetBNROMPRGBank2,
		},
		{
			name:         "Switch to Bank 3",
			writeVal:     targetBNROMPRGBank3,
			expectedBank: targetBNROMPRGBank3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, mapper := createTestBNROMCartridge()

			mapper.WritePRG(addressBNROMPRGBase, tt.writeVal)
			assert.Equal(t, tt.expectedBank, mapper.ReadPRG(addressBNROMPRGBase))
			assert.Equal(t, tt.expectedBank, mapper.ReadPRG(addressBNROMPRGEnd))
		})
	}
}

func TestMapper34_NINABankSwitching(t *testing.T) {
	t.Parallel()

	mapper := createTestNINACartridge()
	require.NotNil(t, mapper)

	// Initial CHR bank 0 and 1
	assert.Equal(t, byte(0), mapper.ReadCHR(addressCHRWindow0))
	assert.Equal(t, byte(1), mapper.ReadCHR(addressCHRWindow1))

	// Switch PRG bank via $7FFD
	mapper.WritePRG(addressNINARegPRG, 1)
	assert.Equal(t, byte(1), mapper.ReadPRG(addressBNROMPRGBase))
	assert.Equal(t, byte(1), mapper.ReadPRG(addressBNROMPRGEnd))

	// Switch CHR Window 0 via $7FFE
	mapper.WritePRG(addressNINARegCHR0, targetNINACHRBank4)
	assert.Equal(t, targetNINACHRBank4, mapper.ReadCHR(addressCHRWindow0))

	// Switch CHR Window 1 via $7FFF
	mapper.WritePRG(addressNINARegCHR1, targetNINACHRBank7)
	assert.Equal(t, targetNINACHRBank7, mapper.ReadCHR(addressCHRWindow1))
}

func TestMapper34_SRAMReadWrite(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestBNROMCartridge()

	assert.Equal(t, byte(0), mapper.ReadPRG(addressSRAMBase))
	mapper.WritePRG(addressSRAMBase, sampleSRAMByte)
	assert.Equal(t, sampleSRAMByte, mapper.ReadPRG(addressSRAMBase))
	assert.True(t, cart.IsSRAMDirty())
}

func TestMapper34_CHRWriteAndOutOfBounds(t *testing.T) {
	t.Parallel()

	_, mapper := createTestBNROMCartridge()

	mapper.WriteCHR(addressCHRWindow0, samplePatternByte)
	assert.Equal(t, samplePatternByte, mapper.ReadCHR(addressCHRWindow0))

	assert.Equal(t, byte(0), mapper.ReadCHR(addressCHROutOfBounds))
	mapper.WriteCHR(addressCHROutOfBounds, samplePatternByte)
	assert.Equal(t, byte(0), mapper.ReadPRG(addressUnmapped))
	mapper.WritePRG(addressUnmapped, samplePatternByte)
}

func TestMapper34_SaveAndLoadState(t *testing.T) {
	t.Parallel()

	mapper1 := createTestNINACartridge()
	mapper1.WritePRG(addressNINARegPRG, 1)
	mapper1.WritePRG(addressNINARegCHR0, targetNINACHRBank4)
	mapper1.WritePRG(addressNINARegCHR1, targetNINACHRBank7)

	assert.Equal(t, byte(1), mapper1.ReadPRG(addressBNROMPRGBase))
	assert.Equal(t, targetNINACHRBank4, mapper1.ReadCHR(addressCHRWindow0))
	assert.Equal(t, targetNINACHRBank7, mapper1.ReadCHR(addressCHRWindow1))

	var buf bytes.Buffer
	err := mapper1.SaveState(&buf)
	require.NoError(t, err)

	mapper2 := createTestNINACartridge()
	err = mapper2.LoadState(&buf)
	require.NoError(t, err)

	assert.Equal(t, byte(1), mapper2.ReadPRG(addressBNROMPRGBase))
	assert.Equal(t, targetNINACHRBank4, mapper2.ReadCHR(addressCHRWindow0))
	assert.Equal(t, targetNINACHRBank7, mapper2.ReadCHR(addressCHRWindow1))
}
