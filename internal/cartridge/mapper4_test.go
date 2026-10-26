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
	testMMC3PRGBanks16 = 16 // 128 KB PRG-ROM (16 banchi da 8 KB)
	testMMC3PRGBanks32 = 32 // 256 KB PRG-ROM (32 banchi da 8 KB, SMB3 configuration)
	testMMC3CHRBanks32 = 32 // 32 KB CHR-ROM (32 banchi da 1 KB)
	testMMC3CHRBanks64 = 64 // 64 KB CHR-ROM (64 banchi da 1 KB)
	testMMC3PRGUnit8KB = 8 * 1024
	testMMC3CHRUnit1KB = 1024

	addrMMC3Window8000 uint16 = 0x8000
	addrMMC3WindowA000 uint16 = 0xA000
	addrMMC3WindowC000 uint16 = 0xC000
	addrMMC3WindowE000 uint16 = 0xE000

	addrMMC3BankSelect uint16 = 0x8000
	addrMMC3BankData   uint16 = 0x8001
	addrMMC3Mirroring  uint16 = 0xA000
	addrMMC3RAMProtect uint16 = 0xA001
	addrMMC3IRQLatch   uint16 = 0xC000
	addrMMC3IRQReload  uint16 = 0xC001
	addrMMC3IRQDisable uint16 = 0xE000
	addrMMC3IRQEnable  uint16 = 0xE001

	addrMMC3CHR0000 uint16 = 0x0000
	addrMMC3CHR0400 uint16 = 0x0400
	addrMMC3CHR0800 uint16 = 0x0800
	addrMMC3CHR0C00 uint16 = 0x0C00
	addrMMC3CHR1000 uint16 = 0x1000
	addrMMC3CHR1400 uint16 = 0x1400
	addrMMC3CHR1800 uint16 = 0x1800
	addrMMC3CHR1C00 uint16 = 0x1C00

	testCmdSelectR0 byte = 0
	testCmdSelectR1 byte = 1
	testCmdSelectR2 byte = 2
	testCmdSelectR3 byte = 3
	testCmdSelectR4 byte = 4
	testCmdSelectR5 byte = 5
	testCmdSelectR6 byte = 6
	testCmdSelectR7 byte = 7

	testPRGMode1Bit byte = 0x40
	testCHRInvertBit byte = 0x80

	testPayloadByte byte = 0x93
)

func createTestMMC3Cartridge(prgBanks, chrBanks int) (*cartridge.Cartridge, *cartridge.Mapper4) {
	prg := make([]byte, prgBanks*testMMC3PRGUnit8KB)
	for i := range prgBanks {
		bankStart := i * testMMC3PRGUnit8KB
		prg[bankStart] = byte(i + 1)
	}

	chr := make([]byte, chrBanks*testMMC3CHRUnit1KB)
	for i := range chrBanks {
		bankStart := i * testMMC3CHRUnit1KB
		chr[bankStart] = byte(i + 1)
	}

	cart := &cartridge.Cartridge{
		PRG:        prg,
		CHR:        chr,
		SRAM:       make([]byte, cartridge.SRAMSize),
		HasBattery: true,
	}

	mapper := cartridge.NewMapper4(cart)
	cart.BindMapper(mapper)
	return cart, mapper
}

func TestMapper4_InitialPowerOnMapping(t *testing.T) {
	t.Parallel()

	_, mapper := createTestMMC3Cartridge(testMMC3PRGBanks16, testMMC3CHRBanks32)
	require.NotNil(t, mapper)

	// In power-on mode 0:
	// $8000 maps to R6 (default bank 0 -> byte 1)
	assert.Equal(t, byte(1), mapper.ReadPRG(addrMMC3Window8000))
	// $A000 maps to R7 (default bank 1 -> byte 2)
	assert.Equal(t, byte(2), mapper.ReadPRG(addrMMC3WindowA000))
	// $C000 maps to penultimate bank (bank 14 -> byte 15)
	assert.Equal(t, byte(15), mapper.ReadPRG(addrMMC3WindowC000))
	// $E000 maps to last bank (bank 15 -> byte 16)
	assert.Equal(t, byte(16), mapper.ReadPRG(addrMMC3WindowE000))
}

func TestMapper4_PRGBankingModes(t *testing.T) {
	t.Parallel()

	t.Run("mode 0: $8000 switchable and $C000 fixed to penultimate", func(t *testing.T) {
		t.Parallel()
		_, mapper := createTestMMC3Cartridge(testMMC3PRGBanks16, testMMC3CHRBanks64)

		// Set R6 to bank 5.
		mapper.WritePRG(addrMMC3BankSelect, testCmdSelectR6)
		mapper.WritePRG(addrMMC3BankData, 5)

		// Set R7 to bank 9.
		mapper.WritePRG(addrMMC3BankSelect, testCmdSelectR7)
		mapper.WritePRG(addrMMC3BankData, 9)

		assert.Equal(t, byte(6), mapper.ReadPRG(addrMMC3Window8000))
		assert.Equal(t, byte(10), mapper.ReadPRG(addrMMC3WindowA000))
		assert.Equal(t, byte(15), mapper.ReadPRG(addrMMC3WindowC000))
		assert.Equal(t, byte(16), mapper.ReadPRG(addrMMC3WindowE000))
	})

	t.Run("mode 1: $C000 switchable and $8000 fixed to penultimate", func(t *testing.T) {
		t.Parallel()
		_, mapper := createTestMMC3Cartridge(testMMC3PRGBanks16, testMMC3CHRBanks64)

		// Enable PRG mode 1 (bit 6 = 1) and target R6.
		mapper.WritePRG(addrMMC3BankSelect, testPRGMode1Bit|testCmdSelectR6)
		mapper.WritePRG(addrMMC3BankData, 5)

		// $8000 is now fixed to penultimate bank (14 -> byte 15).
		assert.Equal(t, byte(15), mapper.ReadPRG(addrMMC3Window8000))
		// $C000 is now mapped to R6 (bank 5 -> byte 6).
		assert.Equal(t, byte(6), mapper.ReadPRG(addrMMC3WindowC000))
		// $E000 remains fixed to last bank (15 -> byte 16).
		assert.Equal(t, byte(16), mapper.ReadPRG(addrMMC3WindowE000))
	})

	t.Run("256 KB PRG ROM configuration", func(t *testing.T) {
		t.Parallel()
		_, mapper := createTestMMC3Cartridge(testMMC3PRGBanks32, testMMC3CHRBanks64)

		// Bank 30 is penultimate (byte 31), Bank 31 is last (byte 32).
		assert.Equal(t, byte(31), mapper.ReadPRG(addrMMC3WindowC000))
		assert.Equal(t, byte(32), mapper.ReadPRG(addrMMC3WindowE000))
	})
}

func TestMapper4_CHRBankingAndInversion(t *testing.T) {
	t.Parallel()

	t.Run("standard inversion 0: 2KB windows at $0000 and 1KB windows at $1000", func(t *testing.T) {
		t.Parallel()
		_, mapper := createTestMMC3Cartridge(testMMC3PRGBanks16, testMMC3CHRBanks64)

		// R0 = 8 (2KB bank at $0000: maps 8 and 9)
		mapper.WritePRG(addrMMC3BankSelect, testCmdSelectR0)
		mapper.WritePRG(addrMMC3BankData, 8)

		// R1 = 12 (2KB bank at $0800: maps 12 and 13)
		mapper.WritePRG(addrMMC3BankSelect, testCmdSelectR1)
		mapper.WritePRG(addrMMC3BankData, 12)

		// R2 = 20, R3 = 21, R4 = 22, R5 = 23 (1KB banks at $1000, $1400, $1800, $1C00)
		mapper.WritePRG(addrMMC3BankSelect, testCmdSelectR2)
		mapper.WritePRG(addrMMC3BankData, 20)
		mapper.WritePRG(addrMMC3BankSelect, testCmdSelectR3)
		mapper.WritePRG(addrMMC3BankData, 21)
		mapper.WritePRG(addrMMC3BankSelect, testCmdSelectR4)
		mapper.WritePRG(addrMMC3BankData, 22)
		mapper.WritePRG(addrMMC3BankSelect, testCmdSelectR5)
		mapper.WritePRG(addrMMC3BankData, 23)

		assert.Equal(t, byte(9), mapper.ReadCHR(addrMMC3CHR0000))
		assert.Equal(t, byte(10), mapper.ReadCHR(addrMMC3CHR0400))
		assert.Equal(t, byte(13), mapper.ReadCHR(addrMMC3CHR0800))
		assert.Equal(t, byte(14), mapper.ReadCHR(addrMMC3CHR0C00))
		assert.Equal(t, byte(21), mapper.ReadCHR(addrMMC3CHR1000))
		assert.Equal(t, byte(22), mapper.ReadCHR(addrMMC3CHR1400))
		assert.Equal(t, byte(23), mapper.ReadCHR(addrMMC3CHR1800))
		assert.Equal(t, byte(24), mapper.ReadCHR(addrMMC3CHR1C00))
	})

	t.Run("inversion 1: 1KB windows at $0000 and 2KB windows at $1000", func(t *testing.T) {
		t.Parallel()
		_, mapper := createTestMMC3Cartridge(testMMC3PRGBanks16, testMMC3CHRBanks64)

		mapper.WritePRG(addrMMC3BankSelect, testCmdSelectR0)
		mapper.WritePRG(addrMMC3BankData, 8)
		mapper.WritePRG(addrMMC3BankSelect, testCmdSelectR2)
		mapper.WritePRG(addrMMC3BankData, 20)

		// Enable CHR inversion (bit 7 = 1).
		mapper.WritePRG(addrMMC3BankSelect, testCHRInvertBit|testCmdSelectR0)

		// $0000 now maps R2 (bank 20 -> byte 21).
		assert.Equal(t, byte(21), mapper.ReadCHR(addrMMC3CHR0000))
		// $1000 now maps R0 (bank 8 -> byte 9).
		assert.Equal(t, byte(9), mapper.ReadCHR(addrMMC3CHR1000))
		// $1400 now maps R0+1 (bank 9 -> byte 10).
		assert.Equal(t, byte(10), mapper.ReadCHR(addrMMC3CHR1400))
	})

	t.Run("R0 and R1 ignore bit 0 for 2KB alignment", func(t *testing.T) {
		t.Parallel()
		_, mapper := createTestMMC3Cartridge(testMMC3PRGBanks16, testMMC3CHRBanks64)

		// Write odd bank index (5), bit 0 must be ignored resulting in bank 4.
		mapper.WritePRG(addrMMC3BankSelect, testCmdSelectR0)
		mapper.WritePRG(addrMMC3BankData, 5)

		assert.Equal(t, byte(5), mapper.ReadCHR(addrMMC3CHR0000))
		assert.Equal(t, byte(6), mapper.ReadCHR(addrMMC3CHR0400))
	})
}

func TestMapper4_Mirroring(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestMMC3Cartridge(testMMC3PRGBanks16, testMMC3CHRBanks64)

	// Write 0 to $A000 -> Vertical.
	mapper.WritePRG(addrMMC3Mirroring, 0)
	assert.Equal(t, cartridge.MirrorVertical, cart.Mirroring)

	// Write 1 to $A000 -> Horizontal.
	mapper.WritePRG(addrMMC3Mirroring, 1)
	assert.Equal(t, cartridge.MirrorHorizontal, cart.Mirroring)

	// FourScreen mirroring must be preserved.
	cart.Mirroring = cartridge.MirrorFourScreen
	mapper.WritePRG(addrMMC3Mirroring, 0)
	assert.Equal(t, cartridge.MirrorFourScreen, cart.Mirroring)
}

func TestMapper4_RAMProtectionAndSRAM(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestMMC3Cartridge(testMMC3PRGBanks16, testMMC3CHRBanks64)

	// By default, RAM is enabled and writeable.
	mapper.WritePRG(cartridge.SRAMBaseAddress, testPayloadByte)
	assert.Equal(t, testPayloadByte, mapper.ReadPRG(cartridge.SRAMBaseAddress))
	assert.True(t, cart.IsSRAMDirty())

	// Write protect RAM (bit 6 = 1, bit 7 = 1).
	mapper.WritePRG(addrMMC3RAMProtect, 0xC0)
	mapper.WritePRG(cartridge.SRAMBaseAddress, 0x00)
	// Write must be ignored.
	assert.Equal(t, testPayloadByte, mapper.ReadPRG(cartridge.SRAMBaseAddress))

	// Disable RAM (bit 7 = 0).
	mapper.WritePRG(addrMMC3RAMProtect, 0x00)
	assert.Equal(t, byte(0), mapper.ReadPRG(cartridge.SRAMBaseAddress))
}

func TestMapper4_IRQCounterOperation(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestMMC3Cartridge(testMMC3PRGBanks16, testMMC3CHRBanks64)
	require.False(t, cart.IRQPending())

	// Set latch to 2 and request reload.
	mapper.WritePRG(addrMMC3IRQLatch, 2)
	mapper.WritePRG(addrMMC3IRQReload, 0)
	mapper.WritePRG(addrMMC3IRQEnable, 0)

	// Scanline 1: counter reloads to latch (2).
	cart.ClockScanline(1)
	assert.False(t, cart.IRQPending())

	// Scanline 2: counter decrements to 1.
	cart.ClockScanline(2)
	assert.False(t, cart.IRQPending())

	// Scanline 3: counter decrements to 0 and asserts IRQ.
	cart.ClockScanline(3)
	assert.True(t, cart.IRQPending())

	// Scanline 4: counter at 0 reloads to latch (2), IRQ stays asserted.
	cart.ClockScanline(4)
	assert.True(t, cart.IRQPending())

	// Acknowledge and disable IRQ via $E000.
	mapper.WritePRG(addrMMC3IRQDisable, 0)
	assert.False(t, cart.IRQPending())
}

func TestMapper4_IRQDisabledDoesNotAssert(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestMMC3Cartridge(testMMC3PRGBanks16, testMMC3CHRBanks64)

	mapper.WritePRG(addrMMC3IRQLatch, 1)
	mapper.WritePRG(addrMMC3IRQReload, 0)
	mapper.WritePRG(addrMMC3IRQDisable, 0)

	cart.ClockScanline(0) // Reloads to 1
	cart.ClockScanline(1) // Decrements to 0, but disabled -> no IRQ
	assert.False(t, cart.IRQPending())
}

func TestMapper4_OutOfBoundsSafety(t *testing.T) {
	t.Parallel()

	_, mapper := createTestMMC3Cartridge(testMMC3PRGBanks16, testMMC3CHRBanks64)

	assert.Equal(t, byte(0), mapper.ReadPRG(0x4020))
	assert.Equal(t, byte(0), mapper.ReadCHR(0x2000))

	mapper.WritePRG(0x4020, 0xFF)
	mapper.WriteCHR(0x2000, 0xFF)
}

func TestMapper4_SaveAndLoadState(t *testing.T) {
	t.Parallel()

	cart1, m1 := createTestMMC3Cartridge(testMMC3PRGBanks16, testMMC3CHRBanks64)

	// Set R6 to bank 4, R7 to bank 5, PRG mode 1, CHR inversion
	m1.WritePRG(addrMMC3BankSelect, testPRGMode1Bit|testCHRInvertBit|testCmdSelectR6)
	m1.WritePRG(addrMMC3BankData, 4)
	m1.WritePRG(addrMMC3BankSelect, testPRGMode1Bit|testCHRInvertBit|testCmdSelectR7)
	m1.WritePRG(addrMMC3BankData, 5)

	// Set horizontal mirroring
	m1.WritePRG(addrMMC3Mirroring, 1)

	// Configure IRQ
	m1.WritePRG(addrMMC3IRQLatch, 10)
	m1.WritePRG(addrMMC3IRQReload, 0)
	m1.WritePRG(addrMMC3IRQEnable, 0)

	var buf bytes.Buffer
	err := m1.SaveState(&buf)
	require.NoError(t, err)

	cart2, m2 := createTestMMC3Cartridge(testMMC3PRGBanks16, testMMC3CHRBanks64)
	err = m2.LoadState(&buf)
	require.NoError(t, err)

	assert.Equal(t, m1.ReadPRG(0x8000), m2.ReadPRG(0x8000))
	assert.Equal(t, m1.ReadPRG(0xA000), m2.ReadPRG(0xA000))
	assert.Equal(t, m1.ReadPRG(0xC000), m2.ReadPRG(0xC000))
	assert.Equal(t, m1.ReadPRG(0xE000), m2.ReadPRG(0xE000))
	assert.Equal(t, cart1.Mirroring, cart2.Mirroring)

	// Verify IRQ ticking continues identically
	m2.ClockScanline(0)
	assert.False(t, m2.IRQPending())
}
