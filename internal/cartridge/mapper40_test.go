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
	testMapper40PRGBanks8 = 8 // 64 KB PRG-ROM (SMB2J standard configuration)
	testMapper40CHRBanks1 = 1 // 8 KB CHR-RAM

	addrM40Window6000 uint16 = 0x6000
	addrM40Window8000 uint16 = 0x8000
	addrM40WindowA000 uint16 = 0xA000
	addrM40WindowC000 uint16 = 0xC000
	addrM40WindowE000 uint16 = 0xE000

	addrM40RegDisableIRQ uint16 = 0x8000
	addrM40RegEnableIRQ  uint16 = 0xA000
	addrM40RegSelectBank uint16 = 0xE000

	testM40SampleTile byte = 0x88
)

func createTestMapper40Cartridge() (*cartridge.Cartridge, *cartridge.Mapper40) {
	prg := make([]byte, testMapper40PRGBanks8*testMMC3PRGUnit8KB)
	for i := range testMapper40PRGBanks8 {
		bankStart := i * testMMC3PRGUnit8KB
		prg[bankStart] = byte(i + 1)
	}

	cart := &cartridge.Cartridge{
		PRG: prg,
		CHR: make([]byte, testMapper40CHRBanks1*testCHRUnit8KB),
	}

	mapper := cartridge.NewMapper40(cart)
	cart.Mapper = mapper
	return cart, mapper
}

func TestMapper40_InitialPowerOnMapping(t *testing.T) {
	t.Parallel()

	_, mapper := createTestMapper40Cartridge()
	require.NotNil(t, mapper)

	// In an 8-bank ROM:
	// $6000 maps to bank 6 -> byte 7
	assert.Equal(t, byte(7), mapper.ReadPRG(addrM40Window6000))
	// $8000 maps to bank 4 -> byte 5
	assert.Equal(t, byte(5), mapper.ReadPRG(addrM40Window8000))
	// $A000 maps to bank 5 -> byte 6
	assert.Equal(t, byte(6), mapper.ReadPRG(addrM40WindowA000))
	// $C000 maps to bank 0 -> byte 1
	assert.Equal(t, byte(1), mapper.ReadPRG(addrM40WindowC000))
	// $E000 maps to bank 7 -> byte 8
	assert.Equal(t, byte(8), mapper.ReadPRG(addrM40WindowE000))
}

func TestMapper40_BankSwitching(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		writeBank    byte
		expectedByte byte
	}{
		{
			name:         "switch to bank 1",
			writeBank:    1,
			expectedByte: 2,
		},
		{
			name:         "switch to bank 2",
			writeBank:    2,
			expectedByte: 3,
		},
		{
			name:         "switch to bank 3",
			writeBank:    3,
			expectedByte: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, mapper := createTestMapper40Cartridge()
			mapper.WritePRG(addrM40RegSelectBank, tt.writeBank)

			assert.Equal(t, tt.expectedByte, mapper.ReadPRG(addrM40WindowC000))
			// Fixed windows must remain unchanged.
			assert.Equal(t, byte(7), mapper.ReadPRG(addrM40Window6000))
			assert.Equal(t, byte(5), mapper.ReadPRG(addrM40Window8000))
			assert.Equal(t, byte(6), mapper.ReadPRG(addrM40WindowA000))
			assert.Equal(t, byte(8), mapper.ReadPRG(addrM40WindowE000))
		})
	}
}

func TestMapper40_IRQCycleCounter(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestMapper40Cartridge()
	require.False(t, cart.IRQPending())

	// Step cycles when disabled does nothing.
	cart.StepCPU(2000)
	assert.False(t, cart.IRQPending())

	// Enable IRQ.
	mapper.WritePRG(addrM40RegEnableIRQ, 0)

	// Advance 4095 cycles (not yet 4096).
	cart.StepCPU(4095)
	assert.False(t, cart.IRQPending())

	// Advance 1 cycle to reach 4096 -> IRQ triggers!
	cart.StepCPU(1)
	assert.True(t, cart.IRQPending())

	// Acknowledge and disable via write to $8000.
	mapper.WritePRG(addrM40RegDisableIRQ, 0)
	assert.False(t, cart.IRQPending())
}

func TestMapper40_CHRRAMReadWrite(t *testing.T) {
	t.Parallel()

	_, mapper := createTestMapper40Cartridge()

	mapper.WriteCHR(0x0000, testM40SampleTile)
	assert.Equal(t, testM40SampleTile, mapper.ReadCHR(0x0000))
}

func TestMapper40_OutOfBoundsSafety(t *testing.T) {
	t.Parallel()

	_, mapper := createTestMapper40Cartridge()

	assert.Equal(t, byte(0), mapper.ReadPRG(0x5FFF))
	assert.Equal(t, byte(0), mapper.ReadCHR(0x2000))

	mapper.WritePRG(0x5FFF, 0xFF)
	mapper.WriteCHR(0x2000, 0xFF)
}

func TestMapper40_SaveAndLoadState(t *testing.T) {
	t.Parallel()

	_, mapper1 := createTestMapper40Cartridge()
	mapper1.WritePRG(addrM40RegSelectBank, 2)
	assert.Equal(t, byte(3), mapper1.ReadPRG(addrM40WindowC000))

	// Enable IRQ and advance halfway
	mapper1.WritePRG(addrM40RegEnableIRQ, 0)
	mapper1.StepCPU(2000)
	assert.False(t, mapper1.IRQPending())

	var buf bytes.Buffer
	err := mapper1.SaveState(&buf)
	require.NoError(t, err)

	_, mapper2 := createTestMapper40Cartridge()
	err = mapper2.LoadState(&buf)
	require.NoError(t, err)

	assert.Equal(t, byte(3), mapper2.ReadPRG(addrM40WindowC000))
	assert.False(t, mapper2.IRQPending())

	// Step another 2096 cycles (2000 + 2096 = 4096) -> triggers IRQ!
	mapper2.StepCPU(2096)
	assert.True(t, mapper2.IRQPending())
}
