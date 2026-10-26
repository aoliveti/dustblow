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
	testMMC4PRGBanks16 = 16 // 256 KB total PRG-ROM (Fire Emblem)
	testMMC4CHRBanks32 = 32 // 128 KB total CHR-ROM

	testBankSize16KB = 0x4000

	addressPRGWindow16KSwitchable uint16 = 0x8000
	addressPRGWindow16KFixedLast   uint16 = 0xC000
	addressPRGWindow16KFixedVector uint16 = 0xFFFC

	targetSwitchableMMC4PRGBank byte = 7
)

func createTestMMC4Cartridge() (*cartridge.Cartridge, *cartridge.Mapper10) {
	prg := make([]byte, testMMC4PRGBanks16*testBankSize16KB)
	for i := range testMMC4PRGBanks16 {
		bankStart := i * testBankSize16KB
		fillVal := byte(i)
		for j := range testBankSize16KB {
			prg[bankStart+j] = fillVal
		}
	}

	chr := make([]byte, testMMC4CHRBanks32*testBankSize4KB)
	for i := range testMMC4CHRBanks32 {
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

	mapper := cartridge.NewMapper10(cart)
	cart.Mapper = mapper
	return cart, mapper
}

func TestMapper10_InitialPowerOnMapping(t *testing.T) {
	t.Parallel()

	_, mapper := createTestMMC4Cartridge()
	require.NotNil(t, mapper)

	// At power on:
	// $8000-$BFFF maps to bank 0.
	assert.Equal(t, byte(0), mapper.ReadPRG(addressPRGWindow16KSwitchable))

	// $C000-$FFFF maps to the last 16 KB bank (16 - 1 = 15).
	assert.Equal(t, byte(15), mapper.ReadPRG(addressPRGWindow16KFixedLast))
	assert.Equal(t, byte(15), mapper.ReadPRG(addressPRGWindow16KFixedVector))

	// CHR latches default to $FE for both windows. Initial bank registers are 0.
	assert.Equal(t, byte(0), mapper.ReadCHR(addressCHRWindow0))
	assert.Equal(t, byte(0), mapper.ReadCHR(addressCHRWindow1))
}

func TestMapper10_PRGBankSwitching(t *testing.T) {
	t.Parallel()

	_, mapper := createTestMMC4Cartridge()

	// Switch $8000-$BFFF to target PRG bank.
	mapper.WritePRG(addressRegPRGBank, targetSwitchableMMC4PRGBank)
	assert.Equal(t, targetSwitchableMMC4PRGBank, mapper.ReadPRG(addressPRGWindow16KSwitchable))

	// $C000-$FFFF must remain fixed to bank 15.
	assert.Equal(t, byte(15), mapper.ReadPRG(addressPRGWindow16KFixedLast))
	assert.Equal(t, byte(15), mapper.ReadPRG(addressPRGWindow16KFixedVector))
}

func TestMapper10_CHRLatchSwitchingWindow0(t *testing.T) {
	t.Parallel()

	_, mapper := createTestMMC4Cartridge()

	// Set Bank FD = 2, Bank FE = 3 for Window 0 ($0000-$0FFF).
	mapper.WritePRG(addressRegCHR0FD, targetCHRBank0FD)
	mapper.WritePRG(addressRegCHR0FE, targetCHRBank0FE)

	// Initial latch is $FE: reads from window 0 return data from bank FE (3).
	assert.Equal(t, targetCHRBank0FE, mapper.ReadCHR(addressCHRWindow0))
	assert.Equal(t, targetCHRBank0FE, mapper.ReadCHR(addressNonTrigger0))

	// Read from trigger range $0FD0-$0FDF. Current read returns bank FE (3), flips latch to $FD.
	assert.Equal(t, targetCHRBank0FE, mapper.ReadCHR(addressTrigger0FD))

	// Subsequent reads from window 0 must now return bank FD (2).
	assert.Equal(t, targetCHRBank0FD, mapper.ReadCHR(addressCHRWindow0))

	// Read from trigger range $0FE0-$0FEF. Current read returns bank FD (2), flips latch to $FE.
	assert.Equal(t, targetCHRBank0FD, mapper.ReadCHR(addressTrigger0FE))

	// Subsequent reads from window 0 must now return bank FE (3).
	assert.Equal(t, targetCHRBank0FE, mapper.ReadCHR(addressCHRWindow0))
}

func TestMapper10_CHRLatchSwitchingWindow1(t *testing.T) {
	t.Parallel()

	_, mapper := createTestMMC4Cartridge()

	// Set Bank FD = 4, Bank FE = 5 for Window 1 ($1000-$1FFF).
	mapper.WritePRG(addressRegCHR1FD, targetCHRBank1FD)
	mapper.WritePRG(addressRegCHR1FE, targetCHRBank1FE)

	// Initial latch is $FE: reads from window 1 return data from bank FE (5).
	assert.Equal(t, targetCHRBank1FE, mapper.ReadCHR(addressCHRWindow1))
	assert.Equal(t, targetCHRBank1FE, mapper.ReadCHR(addressNonTrigger1))

	// Read from trigger range $1FD0-$1FDF. Flips latch 1 to $FD after read.
	assert.Equal(t, targetCHRBank1FE, mapper.ReadCHR(addressTrigger1FD))

	// Subsequent reads from window 1 must return bank FD (4).
	assert.Equal(t, targetCHRBank1FD, mapper.ReadCHR(addressCHRWindow1))

	// Read from trigger range $1FE0-$1FEF. Flips latch 1 to $FE after read.
	assert.Equal(t, targetCHRBank1FD, mapper.ReadCHR(addressTrigger1FE))

	// Subsequent reads from window 1 must return bank FE (5).
	assert.Equal(t, targetCHRBank1FE, mapper.ReadCHR(addressCHRWindow1))
}

func TestMapper10_MirroringControl(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestMMC4Cartridge()

	// Writing 0 to $F000 selects vertical mirroring.
	mapper.WritePRG(addressRegMirror, mirrorVerticalBit)
	assert.Equal(t, cartridge.MirrorVertical, cart.Mirroring)

	// Writing 1 to $F000 selects horizontal mirroring.
	mapper.WritePRG(addressRegMirror, mirrorHorizontalBit)
	assert.Equal(t, cartridge.MirrorHorizontal, cart.Mirroring)
}

func TestMapper10_SRAMReadWrite(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestMMC4Cartridge()

	assert.Equal(t, byte(0), mapper.ReadPRG(addressSRAMBase))
	mapper.WritePRG(addressSRAMBase, sampleSRAMByte)
	assert.Equal(t, sampleSRAMByte, mapper.ReadPRG(addressSRAMBase))
	assert.True(t, cart.IsSRAMDirty())
}

func TestMapper10_CHRWriteAndOutOfBounds(t *testing.T) {
	t.Parallel()

	_, mapper := createTestMMC4Cartridge()

	// Write to CHR memory and verify latch trigger on write.
	mapper.WritePRG(addressRegCHR0FD, targetCHRBank0FD)
	mapper.WritePRG(addressRegCHR0FE, targetCHRBank0FE)

	// Writing to $0FD8 sets latch 0 to $FD.
	mapper.WriteCHR(addressTrigger0FD, samplePatternByte)
	assert.Equal(t, targetCHRBank0FD, mapper.ReadCHR(addressCHRWindow0))

	// Out of bounds addresses.
	assert.Equal(t, byte(0), mapper.ReadCHR(addressCHROutOfBounds))
	mapper.WriteCHR(addressCHROutOfBounds, samplePatternByte)
	assert.Equal(t, byte(0), mapper.ReadPRG(addressUnmapped))
}

func TestMapper10_SaveAndLoadState(t *testing.T) {
	t.Parallel()

	_, mapper1 := createTestMMC4Cartridge()
	mapper1.WritePRG(addressRegPRGBank, targetSwitchableMMC4PRGBank)
	mapper1.WritePRG(addressRegCHR0FD, targetCHRBank0FD)
	mapper1.WritePRG(addressRegCHR0FE, targetCHRBank0FE)
	mapper1.WritePRG(addressRegCHR1FD, targetCHRBank1FD)
	mapper1.WritePRG(addressRegCHR1FE, targetCHRBank1FE)
	mapper1.WritePRG(addressRegMirror, mirrorHorizontalBit)

	// Trigger latch 0 to $FD by reading trigger
	_ = mapper1.ReadCHR(addressTrigger0FD)
	assert.Equal(t, targetCHRBank0FD, mapper1.ReadCHR(addressCHRWindow0))

	var buf bytes.Buffer
	err := mapper1.SaveState(&buf)
	require.NoError(t, err)

	cart2, mapper2 := createTestMMC4Cartridge()
	err = mapper2.LoadState(&buf)
	require.NoError(t, err)

	assert.Equal(t, cartridge.MirrorHorizontal, cart2.Mirroring)
	assert.Equal(t, targetSwitchableMMC4PRGBank, mapper2.ReadPRG(addressPRGWindow16KSwitchable))
	assert.Equal(t, targetCHRBank0FD, mapper2.ReadCHR(addressCHRWindow0))
}
