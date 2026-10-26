// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package console_test

import (
	"bytes"
	"testing"

	"github.com/aoliveti/dustblow/internal/cartridge"
	"github.com/aoliveti/dustblow/internal/console"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestConsole() *console.Console {
	prg := make([]byte, 16384)
	chr := make([]byte, 8192)
	cart := &cartridge.Cartridge{
		PRG:       prg,
		CHR:       chr,
		SRAM:      make([]byte, cartridge.SRAMSize),
		Mirroring: cartridge.MirrorVertical,
	}
	cart.Mapper = cartridge.NewMapper0(cart)
	return console.New(cart)
}

func TestConsole_SaveAndLoadState(t *testing.T) {
	t.Parallel()

	c1 := newTestConsole()
	c1.CPU.A = 0x42
	c1.CPU.X = 0x13
	c1.CPU.Y = 0x37
	c1.CPU.PC = 0xC000
	c1.CPU.Cycles = 123456
	c1.Bus.Write(0x0100, 0xFE)
	c1.Bus.Write(0x6000, 0xAA) // Write to PRG-RAM (SRAM)

	data, err := c1.SaveState()
	require.NoError(t, err)
	require.NotEmpty(t, data)
	assert.Equal(t, []byte("DUST"), data[:4])

	c2 := newTestConsole()
	err = c2.LoadState(data)
	require.NoError(t, err)

	assert.Equal(t, byte(0x42), c2.CPU.A)
	assert.Equal(t, byte(0x13), c2.CPU.X)
	assert.Equal(t, byte(0x37), c2.CPU.Y)
	assert.Equal(t, uint16(0xC000), c2.CPU.PC)
	assert.Equal(t, uint64(123456), c2.CPU.Cycles)
	assert.Equal(t, byte(0xFE), c2.Bus.Read(0x0100))
	assert.Equal(t, byte(0xAA), c2.Bus.Read(0x6000))
}

func TestConsole_LoadState_Corrupted(t *testing.T) {
	t.Parallel()

	c := newTestConsole()

	// Empty data
	err := c.LoadState(nil)
	require.ErrorIs(t, err, console.ErrInvalidSnapshot)

	// Corrupted magic
	badMagic := []byte("FAIL\x01\x00\x00\x00")
	err = c.LoadState(badMagic)
	require.ErrorIs(t, err, console.ErrInvalidSnapshot)

	// Valid magic but future version
	futureVersion := bytes.Join([][]byte{
		[]byte("DUST"),
		{0x99, 0x00, 0x00, 0x00},
	}, nil)
	err = c.LoadState(futureVersion)
	require.ErrorIs(t, err, console.ErrUnsupportedVersion)
}

func TestConsole_SaveAndLoadState_WithMapperState(t *testing.T) {
	t.Parallel()

	const (
		bankCount     = 8
		bankSize16KB  = 16 * 1024
		switchBank3   = 3
		mirrorPage1Bit = 0x10
		addrBankSwitch = 0xC000
		addrMirrorReg  = 0x9000
		addrBank0Read  = 0x8000
	)

	prg := make([]byte, bankCount*bankSize16KB)
	for i := range prg {
		prg[i] = byte(i / bankSize16KB)
	}
	chr := make([]byte, 8192)
	cart1 := &cartridge.Cartridge{
		PRG:       prg,
		CHR:       chr,
		Mirroring: cartridge.MirrorVertical,
	}
	cart1.BindMapper(cartridge.NewMapper71(cart1))
	c1 := console.New(cart1)

	// Switch Mapper 71 bank 0 to bank 3 and single-screen mirroring page 1.
	c1.Bus.Write(addrBankSwitch, switchBank3)
	c1.Bus.Write(addrMirrorReg, mirrorPage1Bit)
	assert.Equal(t, byte(switchBank3), c1.Bus.Read(addrBank0Read))
	assert.Equal(t, cartridge.MirrorSingle1, cart1.Mirroring)

	data, err := c1.SaveState()
	require.NoError(t, err)

	cart2 := &cartridge.Cartridge{
		PRG:       prg,
		CHR:       chr,
		Mirroring: cartridge.MirrorVertical,
	}
	cart2.BindMapper(cartridge.NewMapper71(cart2))
	c2 := console.New(cart2)

	// Initially cart2 points to bank 0.
	assert.Equal(t, byte(0), c2.Bus.Read(addrBank0Read))

	err = c2.LoadState(data)
	require.NoError(t, err)

	// Mapper 71 bank and mirroring restored from snapshot.
	assert.Equal(t, byte(switchBank3), c2.Bus.Read(addrBank0Read))
	assert.Equal(t, cartridge.MirrorSingle1, cart2.Mirroring)
}

func TestConsole_SaveAndLoadState_WithMMC3(t *testing.T) {
	t.Parallel()

	const (
		bankCount8KB  = 32 // 256KB
		bankSize8KB   = 8 * 1024
		targetPRGBank = 5
	)

	prg := make([]byte, bankCount8KB*bankSize8KB)
	for i := range prg {
		prg[i] = byte(i / bankSize8KB)
	}
	chr := make([]byte, 128*1024)
	for i := range chr {
		chr[i] = byte(i / 1024)
	}

	cart1 := &cartridge.Cartridge{
		PRG:       prg,
		CHR:       chr,
		SRAM:      make([]byte, cartridge.SRAMSize),
		Mirroring: cartridge.MirrorVertical,
	}
	cart1.BindMapper(cartridge.NewMapper4(cart1))
	c1 := console.New(cart1)

	// Switch PRG bank at $8000 via MMC3 register 6
	c1.Bus.Write(0x8000, 0x06) // Select register 6 (PRG bank at $8000)
	c1.Bus.Write(0x8001, targetPRGBank)
	assert.Equal(t, byte(targetPRGBank), c1.Bus.Read(0x8000))

	// Switch mirroring to horizontal via $A000
	c1.Bus.Write(0xA000, 0x01)
	assert.Equal(t, cartridge.MirrorHorizontal, cart1.Mirroring)

	// Set IRQ latch to 12 and reload via $C000/$C001
	c1.Bus.Write(0xC000, 12)
	c1.Bus.Write(0xC001, 0)
	c1.Bus.Write(0xE001, 0) // enable IRQ

	data, err := c1.SaveState()
	require.NoError(t, err)

	cart2 := &cartridge.Cartridge{
		PRG:       prg,
		CHR:       chr,
		SRAM:      make([]byte, cartridge.SRAMSize),
		Mirroring: cartridge.MirrorVertical,
	}
	cart2.BindMapper(cartridge.NewMapper4(cart2))
	c2 := console.New(cart2)

	// Before loading, c2 PRG at $8000 is default bank 0
	assert.Equal(t, byte(0), c2.Bus.Read(0x8000))

	err = c2.LoadState(data)
	require.NoError(t, err)

	// After loading, PRG bank, mirroring, and MMC3 state restored
	assert.Equal(t, byte(targetPRGBank), c2.Bus.Read(0x8000))
	assert.Equal(t, cartridge.MirrorHorizontal, cart2.Mirroring)
}

