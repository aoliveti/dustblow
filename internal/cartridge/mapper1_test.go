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
	testMMC1PRGUnits16K = 4 // 64 KB total PRG
	testMMC1CHRUnits4K  = 4 // 16 KB total CHR

	addressPRGLowerBank uint16 = 0x8000
	addressPRGUpperBank uint16 = 0xC000
	addressCHRLowerBank uint16 = 0x0000
	addressCHRUpperBank uint16 = 0x1000

	addressMMC1Control  uint16 = 0x8000
	addressMMC1CHRBank0 uint16 = 0xA000
	addressMMC1CHRBank1 uint16 = 0xC000
	addressMMC1PRGBank  uint16 = 0xE000

	samplePatternByteA byte = 0xA1
	samplePatternByteB byte = 0xB2
	sampleRAMByte      byte = 0x42
)

func createTestMMC1Cartridge() (*cartridge.Cartridge, *cartridge.Mapper1) {
	prg := make([]byte, testMMC1PRGUnits16K*testPRGUnit16KB)
	for i := range testMMC1PRGUnits16K {
		bankStart := i * testPRGUnit16KB
		prg[bankStart] = byte(i)
	}

	chr := make([]byte, testMMC1CHRUnits4K*4096)
	for i := range testMMC1CHRUnits4K {
		bankStart := i * 4096
		chr[bankStart] = byte(i + 10)
	}

	cart := &cartridge.Cartridge{
		PRG:        prg,
		CHR:        chr,
		SRAM:       make([]byte, cartridge.SRAMSize),
		HasBattery: true,
	}

	mapper := cartridge.NewMapper1(cart)
	cart.Mapper = mapper
	return cart, mapper
}

func writeMMC1Register(m *cartridge.Mapper1, addr uint16, val byte) {
	for i := range 5 {
		bit := (val >> i) & 1
		m.WritePRG(addr, bit)
	}
}

func TestMapper1_ShiftRegisterReset(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestMMC1Cartridge()
	require.NotNil(t, mapper)

	// Write 3 bits to shift register
	mapper.WritePRG(addressPRGLowerBank, 0x01)
	mapper.WritePRG(addressPRGLowerBank, 0x00)
	mapper.WritePRG(addressPRGLowerBank, 0x01)

	// Write bit 7 to trigger synchronous reset
	mapper.WritePRG(addressPRGLowerBank, 0x80)

	// Default after reset: Control has 0x0C set (Fixed last 16KB bank mode)
	// Bank at $C000 should be the last bank (index 3)
	assert.Equal(t, byte(3), cart.ReadPRG(addressPRGUpperBank))
}

func TestMapper1_MirroringModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		controlVal        byte
		expectedMirroring cartridge.Mirroring
	}{
		{
			name:              "one screen lower",
			controlVal:        0x00,
			expectedMirroring: cartridge.MirrorSingle0,
		},
		{
			name:              "one screen upper",
			controlVal:        0x01,
			expectedMirroring: cartridge.MirrorSingle1,
		},
		{
			name:              "vertical mirroring",
			controlVal:        0x02,
			expectedMirroring: cartridge.MirrorVertical,
		},
		{
			name:              "horizontal mirroring",
			controlVal:        0x03,
			expectedMirroring: cartridge.MirrorHorizontal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cart, mapper := createTestMMC1Cartridge()
			writeMMC1Register(mapper, addressPRGLowerBank, tt.controlVal)
			assert.Equal(t, tt.expectedMirroring, cart.Mirroring)
		})
	}
}

func TestMapper1_PRGBankingModes(t *testing.T) {
	t.Parallel()

	t.Run("32KB mode switches both banks", func(t *testing.T) {
		t.Parallel()
		cart, mapper := createTestMMC1Cartridge()

		// Control: Mode 0 (32KB PRG mode)
		writeMMC1Register(mapper, 0x8000, 0x00)

		// PRG Bank: select bank 2 (32KB switch ignores bit 0, so selects 2 and 3)
		writeMMC1Register(mapper, 0xE000, 0x02)

		assert.Equal(t, byte(2), cart.ReadPRG(addressPRGLowerBank))
		assert.Equal(t, byte(3), cart.ReadPRG(addressPRGUpperBank))
	})

	t.Run("16KB fixed first switches upper bank", func(t *testing.T) {
		t.Parallel()
		cart, mapper := createTestMMC1Cartridge()

		// Control: Mode 2 (Fixed first bank at $8000, switchable $C000)
		writeMMC1Register(mapper, 0x8000, 0x08)

		// PRG Bank: select bank 2 at $C000
		writeMMC1Register(mapper, 0xE000, 0x02)

		assert.Equal(t, byte(0), cart.ReadPRG(addressPRGLowerBank))
		assert.Equal(t, byte(2), cart.ReadPRG(addressPRGUpperBank))
	})

	t.Run("16KB fixed last switches lower bank", func(t *testing.T) {
		t.Parallel()
		cart, mapper := createTestMMC1Cartridge()

		// Control: Mode 3 (Default: Switchable $8000, fixed last bank at $C000)
		writeMMC1Register(mapper, 0x8000, 0x0C)

		// PRG Bank: select bank 1 at $8000
		writeMMC1Register(mapper, 0xE000, 0x01)

		assert.Equal(t, byte(1), cart.ReadPRG(addressPRGLowerBank))
		assert.Equal(t, byte(3), cart.ReadPRG(addressPRGUpperBank))
	})
}

func TestMapper1_CHRBankingModes(t *testing.T) {
	t.Parallel()

	t.Run("8KB mode switches both 4KB windows", func(t *testing.T) {
		t.Parallel()
		cart, mapper := createTestMMC1Cartridge()

		// Control: 8KB CHR mode (bit 4 = 0)
		writeMMC1Register(mapper, 0x8000, 0x00)

		// CHR Bank 0: select bank 2 (ignores bit 0 in 8KB mode -> 2 and 3)
		writeMMC1Register(mapper, 0xA000, 0x02)

		assert.Equal(t, byte(12), cart.ReadCHR(addressCHRLowerBank))
		assert.Equal(t, byte(13), cart.ReadCHR(addressCHRUpperBank))
	})

	t.Run("4KB separate mode switches windows independently", func(t *testing.T) {
		t.Parallel()
		cart, mapper := createTestMMC1Cartridge()

		// Control: 4KB CHR mode (bit 4 = 1)
		writeMMC1Register(mapper, 0x8000, 0x10)

		// CHR Bank 0: select bank 1 (4K bank at $0000)
		writeMMC1Register(mapper, 0xA000, 0x01)
		// CHR Bank 1: select bank 3 (4K bank at $1000)
		writeMMC1Register(mapper, 0xC000, 0x03)

		assert.Equal(t, byte(11), cart.ReadCHR(addressCHRLowerBank))
		assert.Equal(t, byte(13), cart.ReadCHR(addressCHRUpperBank))
	})

	t.Run("CHR-RAM writes and reads work properly", func(t *testing.T) {
		t.Parallel()
		cart, _ := createTestMMC1Cartridge()
		cart.WriteCHR(0x0050, samplePatternByteA)
		cart.WriteCHR(0x1050, samplePatternByteB)

		assert.Equal(t, samplePatternByteA, cart.ReadCHR(0x0050))
		assert.Equal(t, samplePatternByteB, cart.ReadCHR(0x1050))
	})
}

func TestMapper1_SRAMAccess(t *testing.T) {
	t.Parallel()

	cart, _ := createTestMMC1Cartridge()

	assert.False(t, cart.IsSRAMDirty())

	// Write to PRG-RAM ($6000-$7FFF)
	cart.WritePRG(addressCartridgeRAM, sampleRAMByte)
	assert.Equal(t, sampleRAMByte, cart.ReadPRG(addressCartridgeRAM))
	assert.True(t, cart.IsSRAMDirty())

	cart.ClearSRAMDirty()
	assert.False(t, cart.IsSRAMDirty())
}

func TestMapper1_BoundsSafety(t *testing.T) {
	t.Parallel()

	_, mapper := createTestMMC1Cartridge()

	// Memory under $6000 returns zero
	assert.Equal(t, byte(0), mapper.ReadPRG(addressLowRAM))
	mapper.WritePRG(addressLowRAM, sampleRAMByte)

	// Memory >= $2000 for CHR returns zero
	assert.Equal(t, byte(0), mapper.ReadCHR(addressInvalidHighCHR))
	mapper.WriteCHR(addressInvalidHighCHR, sampleRAMByte)
}

func TestMapper1_SaveAndLoadState(t *testing.T) {
	t.Parallel()

	cart1, mapper1 := createTestMMC1Cartridge()
	// Configure non-default state:
	// 1) Set 4KB CHR mode + vertical mirroring
	writeMMC1Register(mapper1, addressMMC1Control, 0x1E)
	// 2) Select CHR bank 1 for lower 4K
	writeMMC1Register(mapper1, addressMMC1CHRBank0, 1)
	// 3) Select CHR bank 3 for upper 4K
	writeMMC1Register(mapper1, addressMMC1CHRBank1, 3)
	// 4) Select PRG bank 2
	writeMMC1Register(mapper1, addressMMC1PRGBank, 2)
	// 5) Shift in 2 bits into shift register without completing 5 writes
	mapper1.WritePRG(addressMMC1PRGBank, 1)
	mapper1.WritePRG(addressMMC1PRGBank, 0)

	expectedLowerCHR := cart1.ReadCHR(addressCHRLowerBank)
	expectedUpperCHR := cart1.ReadCHR(addressCHRUpperBank)
	expectedPRGLower := cart1.ReadPRG(addressPRGLowerBank)
	expectedPRGUpper := cart1.ReadPRG(addressPRGUpperBank)
	expectedMirror := cart1.Mirroring

	var buf bytes.Buffer
	err := mapper1.SaveState(&buf)
	require.NoError(t, err)

	cart2, mapper2 := createTestMMC1Cartridge()
	err = mapper2.LoadState(&buf)
	require.NoError(t, err)

	assert.Equal(t, expectedMirror, cart2.Mirroring)
	assert.Equal(t, expectedLowerCHR, cart2.ReadCHR(addressCHRLowerBank))
	assert.Equal(t, expectedUpperCHR, cart2.ReadCHR(addressCHRUpperBank))
	assert.Equal(t, expectedPRGLower, cart2.ReadPRG(addressPRGLowerBank))
	assert.Equal(t, expectedPRGUpper, cart2.ReadPRG(addressPRGUpperBank))

	// Continue shifting the remaining 3 bits on mapper2: it should complete register write
	// Remaining 3 bits: 1, 0, 0 -> bits: [1, 0, 1, 0, 0] = value 5 to PRG bank ($E000)
	mapper2.WritePRG(addressMMC1PRGBank, 1)
	mapper2.WritePRG(addressMMC1PRGBank, 0)
	mapper2.WritePRG(addressMMC1PRGBank, 0)

	// Since PRG bank 5 is written (5 % 4 = 1), bank 1 is now selected at lower PRG
	assert.Equal(t, byte(1), cart2.ReadPRG(addressPRGLowerBank))
}
