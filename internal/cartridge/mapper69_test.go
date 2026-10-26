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
	testFME7PRG256K = 256 * 1024
	testFME7CHR128K = 128 * 1024

	addrFME7RegCommand = 0x8000
	addrFME7RegData    = 0xA000
)

func createTestFME7Cartridge(t *testing.T) (*cartridge.Cartridge, cartridge.Mapper) {
	t.Helper()

	prg := make([]byte, testFME7PRG256K)
	for i := range prg {
		prg[i] = byte(i / 0x2000) // Each 8KB bank contains its 8KB bank index (0..31)
	}

	chr := make([]byte, testFME7CHR128K)
	for i := range chr {
		chr[i] = byte(i / 0x0400) // Each 1KB bank contains its 1KB bank index (0..127)
	}

	cart := &cartridge.Cartridge{
		PRG:        prg,
		CHR:        chr,
		SRAM:       make([]byte, 8192),
		MapperID:   69,
		HasBattery: true,
	}

	mapper := cartridge.NewMapper69(cart)
	cart.Mapper = mapper
	return cart, mapper
}

func writeFME7Command(mapper cartridge.Mapper, cmd, data byte) {
	mapper.WritePRG(addrFME7RegCommand, cmd)
	mapper.WritePRG(addrFME7RegData, data)
}

func TestMapper69_PRGBanking(t *testing.T) {
	t.Parallel()

	_, mapper := createTestFME7Cartridge(t)

	// Switch banks:
	// Window 0 ($8000-$9FFF) -> Bank 4
	// Window 1 ($A000-$BFFF) -> Bank 7
	// Window 2 ($C000-$DFFF) -> Bank 11
	// Window 3 ($E000-$FFFF) -> Fixed to last bank (31 for 256KB)
	writeFME7Command(mapper, 0x09, 4)
	writeFME7Command(mapper, 0x0A, 7)
	writeFME7Command(mapper, 0x0B, 11)

	assert.Equal(t, byte(4), mapper.ReadPRG(0x8000))
	assert.Equal(t, byte(4), mapper.ReadPRG(0x9FFF))
	assert.Equal(t, byte(7), mapper.ReadPRG(0xA000))
	assert.Equal(t, byte(7), mapper.ReadPRG(0xBFFF))
	assert.Equal(t, byte(11), mapper.ReadPRG(0xC000))
	assert.Equal(t, byte(11), mapper.ReadPRG(0xDFFF))
	assert.Equal(t, byte(31), mapper.ReadPRG(0xE000))
	assert.Equal(t, byte(31), mapper.ReadPRG(0xFFFF))
}

func TestMapper69_PRGRAMROMWindow(t *testing.T) {
	t.Parallel()

	_, mapper := createTestFME7Cartridge(t)

	const (
		payloadRAM = 0xA5
		testBank5  = 5
	)

	// Mode 1: PRG-ROM mapped to $6000-$7FFF (Bit 7 = 0)
	// Bank 5 mapped to $6000
	writeFME7Command(mapper, 0x08, testBank5)
	assert.Equal(t, byte(testBank5), mapper.ReadPRG(0x6000), "Should read PRG-ROM bank 5")

	// Writes to $6000-$7FFF when ROM is selected must have no effect
	mapper.WritePRG(0x6000, 0x99)
	assert.Equal(t, byte(testBank5), mapper.ReadPRG(0x6000), "ROM write must be ignored")

	// Mode 2: PRG-RAM enabled (Bit 7 = 1, Bit 6 = 1)
	writeFME7Command(mapper, 0x08, 0xC0) // RAM selected + RAM enabled
	mapper.WritePRG(0x6000, payloadRAM)
	assert.Equal(t, byte(payloadRAM), mapper.ReadPRG(0x6000), "RAM read must return written value")

	// Mode 3: PRG-RAM disabled (Bit 7 = 1, Bit 6 = 0)
	writeFME7Command(mapper, 0x08, 0x80) // RAM selected + RAM disabled
	assert.Equal(t, byte(0), mapper.ReadPRG(0x6000), "Disabled RAM read must return 0")
	mapper.WritePRG(0x6000, 0x55) // Write ignored
	writeFME7Command(mapper, 0x08, 0xC0) // Re-enable RAM
	assert.Equal(t, byte(payloadRAM), mapper.ReadPRG(0x6000), "Previous RAM data preserved")
}

func TestMapper69_CHRBanking(t *testing.T) {
	t.Parallel()

	_, mapper := createTestFME7Cartridge(t)

	// Map each of the 8 1KB windows to distinct banks
	for i := range 8 {
		writeFME7Command(mapper, byte(i), byte(20+i))
	}

	for i := range 8 {
		addr := uint16(i * 0x0400)
		assert.Equal(t, byte(20+i), mapper.ReadCHR(addr), "CHR window %d should read bank %d", i, 20+i)
	}
}

func TestMapper69_Mirroring(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestFME7Cartridge(t)

	writeFME7Command(mapper, 0x0C, 0x00)
	assert.Equal(t, cartridge.MirrorVertical, cart.Mirroring)

	writeFME7Command(mapper, 0x0C, 0x01)
	assert.Equal(t, cartridge.MirrorHorizontal, cart.Mirroring)

	writeFME7Command(mapper, 0x0C, 0x02)
	assert.Equal(t, cartridge.MirrorSingle0, cart.Mirroring)

	writeFME7Command(mapper, 0x0C, 0x03)
	assert.Equal(t, cartridge.MirrorSingle1, cart.Mirroring)
}

func TestMapper69_IRQCounter(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestFME7Cartridge(t)
	steppable, ok := mapper.(cartridge.CPUSteppable)
	require.True(t, ok, "Mapper69 must implement CPUSteppable")

	// Set 16-bit counter to 100 cycles
	writeFME7Command(mapper, 0x0E, 100) // Low byte
	writeFME7Command(mapper, 0x0F, 0)   // High byte

	// 1. Counter paused (Bit 7 = 0): StepCPU should not decrement counter
	writeFME7Command(mapper, 0x0D, 0x01) // Enabled, but counter paused (Bit 7 = 0)
	steppable.StepCPU(200)
	assert.False(t, cart.IRQPending(), "IRQ should not fire while counter is paused")

	// 2. Enable countdown (Bit 7 = 1, Bit 0 = 1):
	writeFME7Command(mapper, 0x0D, 0x81)
	steppable.StepCPU(50)
	assert.False(t, cart.IRQPending(), "IRQ should not fire before counter underflow (50 of 100 cycles elapsed)")

	// Step remaining 51 cycles -> underflow occurs
	steppable.StepCPU(51)
	assert.True(t, cart.IRQPending(), "IRQ should fire upon counter underflow")

	// 3. Acknowledge IRQ by writing command 0x0D
	writeFME7Command(mapper, 0x0D, 0x80) // IRQ disabled, counter running
	assert.False(t, cart.IRQPending(), "Writing register $0D must clear active IRQ")
}

func TestMapper69_SaveAndLoadState(t *testing.T) {
	t.Parallel()

	cart1, mapper1 := createTestFME7Cartridge(t)
	// Set PRG bank 4 at $8000
	writeFME7Command(mapper1, 0x09, 4)
	// Set mirroring horizontal
	writeFME7Command(mapper1, 0x0C, 0x01)
	// Set CHR bank 12 at window 0
	writeFME7Command(mapper1, 0x00, 12)
	// Configure IRQ counter to 100 cycles, running & enabled
	writeFME7Command(mapper1, 0x0E, 100)
	writeFME7Command(mapper1, 0x0F, 0)
	writeFME7Command(mapper1, 0x0D, 0x81)

	// Step 40 cycles
	steppable1 := mapper1.(cartridge.CPUSteppable)
	steppable1.StepCPU(40)
	assert.False(t, cart1.IRQPending())

	stateful1, ok := mapper1.(cartridge.StatefulMapper)
	require.True(t, ok)

	var buf bytes.Buffer
	err := stateful1.SaveState(&buf)
	require.NoError(t, err)

	cart2, mapper2 := createTestFME7Cartridge(t)
	stateful2, ok := mapper2.(cartridge.StatefulMapper)
	require.True(t, ok)

	err = stateful2.LoadState(&buf)
	require.NoError(t, err)

	assert.Equal(t, byte(4), mapper2.ReadPRG(0x8000))
	assert.Equal(t, cartridge.MirrorHorizontal, cart2.Mirroring)
	assert.Equal(t, byte(12), mapper2.ReadCHR(0x0000))
	assert.False(t, cart2.IRQPending())

	// Step remaining 61 cycles on restored mapper -> IRQ must fire!
	steppable2 := mapper2.(cartridge.CPUSteppable)
	steppable2.StepCPU(61)
	assert.True(t, cart2.IRQPending())
}
