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
	testM71PRGSize128K = 128 * 1024 // 8 banks of 16KB
	testM71CHRSize8K   = 8 * 1024
	testM71BankSize16K = 16 * 1024

	testM71AddrBank0     = 0x8000
	testM71AddrBank0Mid  = 0x9000
	testM71AddrFixedBank = 0xC000
	testM71AddrFixedMid  = 0xE000

	testM71RegMirroringLow  = 0x9000
	testM71RegMirroringHigh = 0x9FFF
	testM71RegBankSwitch    = 0xC000

	testM71Bank0Index = 0
	testM71Bank1Index = 1
	testM71Bank2Index = 2
	testM71Bank3Index = 3
	testM71BankLast   = 7

	testM71MirrorBitPage0 = 0x00
	testM71MirrorBitPage1 = 0x10

	testM71TestByte1 = 0x42
	testM71TestByte2 = 0xA5
	testM71CHRAddr   = 0x0123
)

func newTestMapper71(t *testing.T) (*cartridge.Cartridge, *cartridge.Mapper71) {
	t.Helper()
	prg := make([]byte, testM71PRGSize128K)
	for i := range prg {
		bank := i / testM71BankSize16K
		prg[i] = byte(bank)
	}

	chr := make([]byte, testM71CHRSize8K)
	cart := &cartridge.Cartridge{
		PRG:       prg,
		CHR:       chr,
		Mirroring: cartridge.MirrorVertical,
	}
	m := cartridge.NewMapper71(cart)
	cart.Mapper = m
	return cart, m
}

func TestMapper71_PowerOnDefaults(t *testing.T) {
	t.Parallel()
	_, m := newTestMapper71(t)

	// Bank 0 should be at $8000
	assert.Equal(t, byte(testM71Bank0Index), m.ReadPRG(testM71AddrBank0))
	assert.Equal(t, byte(testM71Bank0Index), m.ReadPRG(testM71AddrBank0Mid))

	// Last bank (7) should be at $C000
	assert.Equal(t, byte(testM71BankLast), m.ReadPRG(testM71AddrFixedBank))
	assert.Equal(t, byte(testM71BankLast), m.ReadPRG(testM71AddrFixedMid))
}

func TestMapper71_PRGBankSwitching(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		writeAddr    uint16
		bankSelect   byte
		expectedBank byte
	}{
		{
			name:         "switch to bank 1 at $C000",
			writeAddr:    testM71RegBankSwitch,
			bankSelect:   byte(testM71Bank1Index),
			expectedBank: byte(testM71Bank1Index),
		},
		{
			name:         "switch to bank 2 at $D000",
			writeAddr:    0xD000,
			bankSelect:   byte(testM71Bank2Index),
			expectedBank: byte(testM71Bank2Index),
		},
		{
			name:         "switch to bank 3 at $FFFF",
			writeAddr:    0xFFFF,
			bankSelect:   byte(testM71Bank3Index),
			expectedBank: byte(testM71Bank3Index),
		},
		{
			name:         "masking bank index beyond ROM size",
			writeAddr:    testM71RegBankSwitch,
			bankSelect:   0x09, // 9 % 8 = 1
			expectedBank: byte(testM71Bank1Index),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, m := newTestMapper71(t)
			m.WritePRG(tt.writeAddr, tt.bankSelect)
			assert.Equal(t, tt.expectedBank, m.ReadPRG(testM71AddrBank0))
			// Upper bank must remain fixed to bank 7
			assert.Equal(t, byte(testM71BankLast), m.ReadPRG(testM71AddrFixedBank))
		})
	}
}

func TestMapper71_MirroringControl(t *testing.T) {
	t.Parallel()
	cart, m := newTestMapper71(t)

	// Initially vertical
	assert.Equal(t, cartridge.MirrorVertical, cart.Mirroring)

	// Write $00 to $9000 -> Single-screen Page 0
	m.WritePRG(testM71RegMirroringLow, testM71MirrorBitPage0)
	assert.Equal(t, cartridge.MirrorSingle0, cart.Mirroring)

	// Write $10 to $9FFF -> Single-screen Page 1
	m.WritePRG(testM71RegMirroringHigh, testM71MirrorBitPage1)
	assert.Equal(t, cartridge.MirrorSingle1, cart.Mirroring)

	// Write $00 with noise in lower bits -> Single-screen Page 0
	m.WritePRG(0x9555, 0xEF&^testM71MirrorBitPage1)
	assert.Equal(t, cartridge.MirrorSingle0, cart.Mirroring)
}

func TestMapper71_CHRReadWrite(t *testing.T) {
	t.Parallel()
	_, m := newTestMapper71(t)

	m.WriteCHR(testM71CHRAddr, testM71TestByte1)
	assert.Equal(t, byte(testM71TestByte1), m.ReadCHR(testM71CHRAddr))

	m.WriteCHR(testM71CHRAddr, testM71TestByte2)
	assert.Equal(t, byte(testM71TestByte2), m.ReadCHR(testM71CHRAddr))
}

func TestMapper71_SaveLoadState(t *testing.T) {
	t.Parallel()
	cart, m := newTestMapper71(t)

	// Configure bank 3 and single screen 1
	m.WritePRG(testM71RegBankSwitch, byte(testM71Bank3Index))
	m.WritePRG(testM71RegMirroringLow, testM71MirrorBitPage1)

	var buf bytes.Buffer
	require.NoError(t, m.SaveState(&buf))

	// Mutate
	m.WritePRG(testM71RegBankSwitch, byte(testM71Bank1Index))
	m.WritePRG(testM71RegMirroringLow, testM71MirrorBitPage0)
	assert.Equal(t, byte(testM71Bank1Index), m.ReadPRG(testM71AddrBank0))
	assert.Equal(t, cartridge.MirrorSingle0, cart.Mirroring)

	// Restore
	require.NoError(t, m.LoadState(&buf))
	assert.Equal(t, byte(testM71Bank3Index), m.ReadPRG(testM71AddrBank0))
	assert.Equal(t, cartridge.MirrorSingle1, cart.Mirroring)
}
