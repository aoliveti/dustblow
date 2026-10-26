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
	testMMC2PRGBanks16 = 16 // 128 KB total PRG-ROM
	testMMC2CHRBanks32 = 32 // 128 KB total CHR-ROM

	testBankSize8KB = 0x2000
	testBankSize4KB = 0x1000

	addressPRGSwitchable             uint16 = 0x8000
	addressPRGFixedPenultimateMinus1 uint16 = 0xA000
	addressPRGFixedPenultimate       uint16 = 0xC000
	addressPRGFixedLast              uint16 = 0xE000

	addressCHRWindow0   uint16 = 0x0000
	addressCHRWindow1   uint16 = 0x1000
	addressTrigger0FD   uint16 = 0x0FD8
	addressTrigger0FE   uint16 = 0x0FE8
	addressTrigger1FD   uint16 = 0x1FD8
	addressTrigger1FE   uint16 = 0x1FE8
	addressNonTrigger0  uint16 = 0x0FC0
	addressNonTrigger1  uint16 = 0x1FC0
	addressCHROutOfBounds uint16 = 0x2000

	addressRegPRGBank uint16 = 0xA000
	addressRegCHR0FD  uint16 = 0xB000
	addressRegCHR0FE  uint16 = 0xC000
	addressRegCHR1FD  uint16 = 0xD000
	addressRegCHR1FE  uint16 = 0xE000
	addressRegMirror  uint16 = 0xF000

	targetSwitchablePRGBank byte = 5
	targetCHRBank0FD        byte = 2
	targetCHRBank0FE        byte = 3
	targetCHRBank1FD        byte = 4
	targetCHRBank1FE        byte = 5

	mirrorVerticalBit   byte = 0x00
	mirrorHorizontalBit byte = 0x01
)

func createTestMMC2Cartridge() (*cartridge.Cartridge, *cartridge.Mapper9) {
	prg := make([]byte, testMMC2PRGBanks16*testBankSize8KB)
	for i := range testMMC2PRGBanks16 {
		bankStart := i * testBankSize8KB
		fillVal := byte(i)
		for j := range testBankSize8KB {
			prg[bankStart+j] = fillVal
		}
	}

	chr := make([]byte, testMMC2CHRBanks32*testBankSize4KB)
	for i := range testMMC2CHRBanks32 {
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

	mapper := cartridge.NewMapper9(cart)
	cart.Mapper = mapper
	return cart, mapper
}

func TestMapper9_InitialPowerOnMapping(t *testing.T) {
	t.Parallel()

	_, mapper := createTestMMC2Cartridge()
	require.NotNil(t, mapper)

	// At power on:
	// $8000-$9FFF maps to bank 0.
	assert.Equal(t, byte(0), mapper.ReadPRG(addressPRGSwitchable))

	// $A000-$BFFF maps to penultimate-1 bank (16 - 3 = 13).
	assert.Equal(t, byte(13), mapper.ReadPRG(addressPRGFixedPenultimateMinus1))

	// $C000-$DFFF maps to penultimate bank (16 - 2 = 14).
	assert.Equal(t, byte(14), mapper.ReadPRG(addressPRGFixedPenultimate))

	// $E000-$FFFF maps to last bank (16 - 1 = 15).
	assert.Equal(t, byte(15), mapper.ReadPRG(addressPRGFixedLast))

	// CHR latches default to $FE for both windows.
	// Initial bank registers are 0, so reading $0000 and $1000 should read bank 0.
	assert.Equal(t, byte(0), mapper.ReadCHR(addressCHRWindow0))
	assert.Equal(t, byte(0), mapper.ReadCHR(addressCHRWindow1))
}

func TestMapper9_PRGBankSwitching(t *testing.T) {
	t.Parallel()

	_, mapper := createTestMMC2Cartridge()

	// Switch $8000-$9FFF to target PRG bank.
	mapper.WritePRG(addressRegPRGBank, targetSwitchablePRGBank)
	assert.Equal(t, targetSwitchablePRGBank, mapper.ReadPRG(addressPRGSwitchable))

	// Upper banks must remain fixed.
	assert.Equal(t, byte(13), mapper.ReadPRG(addressPRGFixedPenultimateMinus1))
	assert.Equal(t, byte(14), mapper.ReadPRG(addressPRGFixedPenultimate))
	assert.Equal(t, byte(15), mapper.ReadPRG(addressPRGFixedLast))
}

func TestMapper9_CHRLatchSwitchingWindow0(t *testing.T) {
	t.Parallel()

	_, mapper := createTestMMC2Cartridge()

	// Set Bank FD = 2, Bank FE = 3 for Window 0 ($0000-$0FFF).
	mapper.WritePRG(addressRegCHR0FD, targetCHRBank0FD)
	mapper.WritePRG(addressRegCHR0FE, targetCHRBank0FE)

	// Initial latch is $FE: reads from window 0 return data from bank FE (3).
	assert.Equal(t, targetCHRBank0FE, mapper.ReadCHR(addressCHRWindow0))
	assert.Equal(t, targetCHRBank0FE, mapper.ReadCHR(addressNonTrigger0))

	// Read from trigger range $0FD0-$0FDF.
	// The current read must still return bank FE (latch flips AFTER read).
	valOnTrigger := mapper.ReadCHR(addressTrigger0FD)
	assert.Equal(t, targetCHRBank0FE, valOnTrigger)

	// Subsequent reads from window 0 must now return bank FD (2).
	assert.Equal(t, targetCHRBank0FD, mapper.ReadCHR(addressCHRWindow0))

	// Read from trigger range $0FE0-$0FEF.
	// The current read must return bank FD (latch flips AFTER read).
	valOnSwitchBack := mapper.ReadCHR(addressTrigger0FE)
	assert.Equal(t, targetCHRBank0FD, valOnSwitchBack)

	// Subsequent reads from window 0 must now return bank FE (3).
	assert.Equal(t, targetCHRBank0FE, mapper.ReadCHR(addressCHRWindow0))
}

func TestMapper9_CHRLatchSwitchingWindow1(t *testing.T) {
	t.Parallel()

	_, mapper := createTestMMC2Cartridge()

	// Set Bank FD = 4, Bank FE = 5 for Window 1 ($1000-$1FFF).
	mapper.WritePRG(addressRegCHR1FD, targetCHRBank1FD)
	mapper.WritePRG(addressRegCHR1FE, targetCHRBank1FE)

	// Initial latch is $FE: reads from window 1 return data from bank FE (5).
	assert.Equal(t, targetCHRBank1FE, mapper.ReadCHR(addressCHRWindow1))
	assert.Equal(t, targetCHRBank1FE, mapper.ReadCHR(addressNonTrigger1))

	// Read from trigger range $1FD0-$1FDF.
	// Current read returns bank FE (5), but sets latch 1 to $FD.
	valOnTrigger := mapper.ReadCHR(addressTrigger1FD)
	assert.Equal(t, targetCHRBank1FE, valOnTrigger)

	// Subsequent reads from window 1 must return bank FD (4).
	assert.Equal(t, targetCHRBank1FD, mapper.ReadCHR(addressCHRWindow1))

	// Read from trigger range $1FE0-$1FEF.
	// Current read returns bank FD (4), but sets latch 1 to $FE.
	valOnSwitchBack := mapper.ReadCHR(addressTrigger1FE)
	assert.Equal(t, targetCHRBank1FD, valOnSwitchBack)

	// Subsequent reads from window 1 must return bank FE (5).
	assert.Equal(t, targetCHRBank1FE, mapper.ReadCHR(addressCHRWindow1))
}

func TestMapper9_MirroringControl(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestMMC2Cartridge()

	// Writing 0 to $F000 selects vertical mirroring.
	mapper.WritePRG(addressRegMirror, mirrorVerticalBit)
	assert.Equal(t, cartridge.MirrorVertical, cart.Mirroring)

	// Writing 1 to $F000 selects horizontal mirroring.
	mapper.WritePRG(addressRegMirror, mirrorHorizontalBit)
	assert.Equal(t, cartridge.MirrorHorizontal, cart.Mirroring)
}

func TestMapper9_SRAMReadWrite(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestMMC2Cartridge()

	assert.Equal(t, byte(0), mapper.ReadPRG(addressSRAMBase))
	mapper.WritePRG(addressSRAMBase, sampleSRAMByte)
	assert.Equal(t, sampleSRAMByte, mapper.ReadPRG(addressSRAMBase))
	assert.True(t, cart.IsSRAMDirty())
}

func TestMapper9_CHRWriteAndOutOfBounds(t *testing.T) {
	t.Parallel()

	_, mapper := createTestMMC2Cartridge()

	// Write to CHR memory and verify latch trigger on write.
	mapper.WritePRG(addressRegCHR0FD, targetCHRBank0FD)
	mapper.WritePRG(addressRegCHR0FE, targetCHRBank0FE)

	// Writing to $0FD8 sets latch 0 to $FD.
	mapper.WriteCHR(addressTrigger0FD, samplePatternByte)
	// Subsequent read should now be in bank FD.
	assert.Equal(t, targetCHRBank0FD, mapper.ReadCHR(addressCHRWindow0))

	// Out of bounds reads and writes must not panic or modify state unexpectedly.
	assert.Equal(t, byte(0), mapper.ReadCHR(addressCHROutOfBounds))
	mapper.WriteCHR(addressCHROutOfBounds, samplePatternByte)
	assert.Equal(t, byte(0), mapper.ReadPRG(addressUnmapped))
}

func TestMapper9_SaveAndLoadState(t *testing.T) {
	t.Parallel()

	_, mapper1 := createTestMMC2Cartridge()
	mapper1.WritePRG(addressRegPRGBank, targetSwitchablePRGBank)
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

	cart2, mapper2 := createTestMMC2Cartridge()
	err = mapper2.LoadState(&buf)
	require.NoError(t, err)

	assert.Equal(t, cartridge.MirrorHorizontal, cart2.Mirroring)
	assert.Equal(t, targetSwitchablePRGBank, mapper2.ReadPRG(addressPRGSwitchable))
	assert.Equal(t, targetCHRBank0FD, mapper2.ReadCHR(addressCHRWindow0))
}
