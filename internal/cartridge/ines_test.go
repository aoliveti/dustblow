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
	testHeaderSize  = 16
	testPRGUnit16KB = 16384
	testPRGUnit32KB = 32768
	testCHRUnit8KB  = 8192
	testPartialSize = 256

	// Hardware configuration test flags.
	testFlagHorizontalMirror byte = 0x00
	testFlagVerticalMirror   byte = 0x01
	testFlagTrainerPresent   byte = 0x04
	testFlagFourScreen          byte = 0x08
	testMapper0ID               byte = 0x00
	testMapper3ID               byte = 0x03
	testMapper3Nibble           byte = 0x30
	testMapper4ID               byte = 0x04
	testMapper4Nibble           byte = 0x40
	testMapper5ID               byte = 0x05
	testMapper5Nibble           byte = 0x50
	testMapper7ID               byte = 0x07
	testMapper7Nibble           byte = 0x70
	testMapper9ID               byte = 0x09
	testMapper9Nibble           byte = 0x90
	testMapper10ID              byte = 10
	testMapper10Nibble          byte = 0xA0
	testMapper34ID              byte = 34
	testMapper34Byte6           byte = 0x20
	testMapper34Byte7           byte = 0x20
	testMapper40ID              byte = 40
	testMapper40Byte6           byte = 0x80
	testMapper40Byte7           byte = 0x20
	testMapper66ID              byte = 66
	testMapper66Byte6           byte = 0x20
	testMapper66Byte7           byte = 0x40
	testUnsupportedMapperNibble byte = 0xD0

	// 6502 CPU Test addresses and opcodes.
	testAddressPRGBase   uint16 = 0x8000
	testAddressPRGMirror uint16 = 0xC000
	testAddressResetLow  uint16 = 0xFFFC
	testAddressResetHigh uint16 = 0xFFFD
	testAddressCHRBase   uint16 = 0x0000
	testAddressCHRAM     uint16 = 0x0100

	testOpcodeLDA byte = 0xA9

	samplePRGFirstBankByte  byte = 0x11
	samplePRGSecondBankByte byte = 0x22
	sampleLDAOperand        byte = 0x42
	sampleCHRPatternByte    byte = 0x55
	sampleTestWriteByte     byte = 0xFE

	testResetVectorLowByte  byte = 0x00
	testResetVectorHighByte byte = 0x80
)

func makeHeader(prg16k, chr8k, hwConfig, extFlags byte) []byte {
	header := make([]byte, testHeaderSize)
	header[0] = 'N'
	header[1] = 'E'
	header[2] = 'S'
	header[3] = 0x1A
	header[4] = prg16k
	header[5] = chr8k
	header[6] = hwConfig
	header[7] = extFlags
	return header
}

func TestCartridge_HeaderValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		data        []byte
		expectedErr error
	}{
		{
			name:        "too short header",
			data:        []byte{'N', 'E', 'S'},
			expectedErr: cartridge.ErrInvalidHeader,
		},
		{
			name:        "invalid magic bytes",
			data:        []byte{0x00, 0x01, 0x02, 0x03, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			expectedErr: cartridge.ErrInvalidHeader,
		},
		{
			name:        "truncated PRG ROM",
			data:        makeHeader(1, 1, testFlagHorizontalMirror, 0),
			expectedErr: cartridge.ErrTruncatedROM,
		},
		{
			name: "truncated CHR ROM",
			data: func() []byte {
				buf := makeHeader(1, 1, testFlagHorizontalMirror, 0)
				buf = append(buf, make([]byte, testPRGUnit16KB)...)
				return buf
			}(),
			expectedErr: cartridge.ErrTruncatedROM,
		},
		{
			name: "truncated trainer",
			data: func() []byte {
				buf := makeHeader(1, 1, testFlagTrainerPresent, 0)
				buf = append(buf, make([]byte, testPartialSize)...)
				return buf
			}(),
			expectedErr: cartridge.ErrTruncatedROM,
		},
		{
			name: "unsupported mapper",
			data: func() []byte {
				buf := makeHeader(1, 1, testUnsupportedMapperNibble, 0)
				buf = append(buf, make([]byte, testPRGUnit16KB+testCHRUnit8KB)...)
				return buf
			}(),
			expectedErr: cartridge.ErrUnsupportedMapper,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := cartridge.New(bytes.NewReader(tt.data))
			require.ErrorIs(t, err, tt.expectedErr)
		})
	}
}

func TestCartridge_FormatConfigurations(t *testing.T) {
	t.Parallel()

	t.Run("NROM-128 with horizontal mirroring and mirroring verification", func(t *testing.T) {
		t.Parallel()
		prg := make([]byte, testPRGUnit16KB)
		prg[0x0000] = testOpcodeLDA
		prg[0x0001] = sampleLDAOperand
		prg[0x3FFC] = testResetVectorLowByte
		prg[0x3FFD] = testResetVectorHighByte

		chr := make([]byte, testCHRUnit8KB)
		chr[0] = sampleCHRPatternByte

		rom := makeHeader(1, 1, testFlagHorizontalMirror, 0)
		rom = append(rom, prg...)
		rom = append(rom, chr...)

		cart, err := cartridge.New(bytes.NewReader(rom))
		require.NoError(t, err)
		require.NotNil(t, cart)

		assert.Equal(t, testMapper0ID, cart.MapperID)
		assert.Equal(t, cartridge.MirrorHorizontal, cart.Mirroring)
		assert.Equal(t, testOpcodeLDA, cart.ReadPRG(testAddressPRGBase))
		assert.Equal(t, testOpcodeLDA, cart.ReadPRG(testAddressPRGMirror))
		assert.Equal(t, testResetVectorLowByte, cart.ReadPRG(testAddressResetLow))
		assert.Equal(t, testResetVectorHighByte, cart.ReadPRG(testAddressResetHigh))
		assert.Equal(t, sampleCHRPatternByte, cart.ReadCHR(testAddressCHRBase))
	})

	t.Run("NROM-256 with vertical mirroring and unmirrored banks", func(t *testing.T) {
		t.Parallel()
		prg := make([]byte, testPRGUnit32KB)
		prg[0x0000] = samplePRGFirstBankByte
		prg[0x4000] = samplePRGSecondBankByte

		chr := make([]byte, testCHRUnit8KB)

		rom := makeHeader(2, 1, testFlagVerticalMirror, 0)
		rom = append(rom, prg...)
		rom = append(rom, chr...)

		cart, err := cartridge.New(bytes.NewReader(rom))
		require.NoError(t, err)
		require.NotNil(t, cart)

		assert.Equal(t, testMapper0ID, cart.MapperID)
		assert.Equal(t, cartridge.MirrorVertical, cart.Mirroring)
		assert.Equal(t, samplePRGFirstBankByte, cart.ReadPRG(testAddressPRGBase))
		assert.Equal(t, samplePRGSecondBankByte, cart.ReadPRG(testAddressPRGMirror))
	})

	t.Run("FourScreen mirroring flag", func(t *testing.T) {
		t.Parallel()
		rom := makeHeader(1, 1, testFlagFourScreen, 0)
		rom = append(rom, make([]byte, testPRGUnit16KB+testCHRUnit8KB)...)

		cart, err := cartridge.New(bytes.NewReader(rom))
		require.NoError(t, err)
		require.NotNil(t, cart)
		assert.Equal(t, cartridge.MirrorFourScreen, cart.Mirroring)
	})

	t.Run("CHR-RAM automatic allocation when CHR units is zero", func(t *testing.T) {
		t.Parallel()
		prg := make([]byte, testPRGUnit16KB)
		rom := makeHeader(1, 0, 0, 0)
		rom = append(rom, prg...)

		cart, err := cartridge.New(bytes.NewReader(rom))
		require.NoError(t, err)
		require.NotNil(t, cart)
		require.Len(t, cart.CHR, testCHRUnit8KB)

		cart.WriteCHR(testAddressCHRAM, sampleTestWriteByte)
		assert.Equal(t, sampleTestWriteByte, cart.ReadCHR(testAddressCHRAM))
	})

	t.Run("CNROM Mapper 3 instantiation", func(t *testing.T) {
		t.Parallel()
		rom := makeHeader(1, 2, testMapper3Nibble, 0)
		rom = append(rom, make([]byte, testPRGUnit16KB+2*testCHRUnit8KB)...)

		cart, err := cartridge.New(bytes.NewReader(rom))
		require.NoError(t, err)
		require.NotNil(t, cart)
		assert.Equal(t, testMapper3ID, cart.MapperID)
		assert.NotNil(t, cart.Mapper)
	})

	t.Run("MMC3 Mapper 4 instantiation", func(t *testing.T) {
		t.Parallel()
		rom := makeHeader(2, 4, testMapper4Nibble, 0)
		rom = append(rom, make([]byte, 2*testPRGUnit16KB+4*testCHRUnit8KB)...)

		cart, err := cartridge.New(bytes.NewReader(rom))
		require.NoError(t, err)
		require.NotNil(t, cart)
		assert.Equal(t, testMapper4ID, cart.MapperID)
		assert.NotNil(t, cart.Mapper)
	})

	t.Run("AxROM Mapper 7 instantiation", func(t *testing.T) {
		t.Parallel()
		rom := makeHeader(4, 0, testMapper7Nibble, 0)
		rom = append(rom, make([]byte, 4*testPRGUnit16KB)...)

		cart, err := cartridge.New(bytes.NewReader(rom))
		require.NoError(t, err)
		require.NotNil(t, cart)
		assert.Equal(t, testMapper7ID, cart.MapperID)
		assert.Equal(t, cartridge.MirrorSingle0, cart.Mirroring)
		assert.NotNil(t, cart.Mapper)
	})

	t.Run("SMB2J Mapper 40 instantiation", func(t *testing.T) {
		t.Parallel()
		rom := makeHeader(4, 1, testMapper40Byte6, testMapper40Byte7)
		rom = append(rom, make([]byte, 4*testPRGUnit16KB+testCHRUnit8KB)...)

		cart, err := cartridge.New(bytes.NewReader(rom))
		require.NoError(t, err)
		require.NotNil(t, cart)
		assert.Equal(t, testMapper40ID, cart.MapperID)
		assert.NotNil(t, cart.Mapper)
	})

	t.Run("MMC2 Mapper 9 instantiation", func(t *testing.T) {
		t.Parallel()
		rom := makeHeader(8, 16, testMapper9Nibble, 0)
		rom = append(rom, make([]byte, 8*testPRGUnit16KB+16*testCHRUnit8KB)...)

		cart, err := cartridge.New(bytes.NewReader(rom))
		require.NoError(t, err)
		require.NotNil(t, cart)
		assert.Equal(t, testMapper9ID, cart.MapperID)
		assert.NotNil(t, cart.Mapper)
	})

	t.Run("GxROM Mapper 66 instantiation", func(t *testing.T) {
		t.Parallel()
		rom := makeHeader(4, 4, testMapper66Byte6, testMapper66Byte7)
		rom = append(rom, make([]byte, 4*testPRGUnit16KB+4*testCHRUnit8KB)...)

		cart, err := cartridge.New(bytes.NewReader(rom))
		require.NoError(t, err)
		require.NotNil(t, cart)
		assert.Equal(t, testMapper66ID, cart.MapperID)
		assert.NotNil(t, cart.Mapper)
	})

	t.Run("MMC4 Mapper 10 instantiation", func(t *testing.T) {
		t.Parallel()
		rom := makeHeader(16, 16, testMapper10Nibble, 0)
		rom = append(rom, make([]byte, 16*testPRGUnit16KB+16*testCHRUnit8KB)...)

		cart, err := cartridge.New(bytes.NewReader(rom))
		require.NoError(t, err)
		require.NotNil(t, cart)
		assert.Equal(t, testMapper10ID, cart.MapperID)
		assert.NotNil(t, cart.Mapper)
	})

	t.Run("BNROM Mapper 34 instantiation", func(t *testing.T) {
		t.Parallel()
		rom := makeHeader(8, 0, testMapper34Byte6, testMapper34Byte7)
		rom = append(rom, make([]byte, 8*testPRGUnit16KB)...)

		cart, err := cartridge.New(bytes.NewReader(rom))
		require.NoError(t, err)
		require.NotNil(t, cart)
		assert.Equal(t, testMapper34ID, cart.MapperID)
		assert.NotNil(t, cart.Mapper)
	})

	t.Run("MMC5 Mapper 5 instantiation", func(t *testing.T) {
		t.Parallel()
		rom := makeHeader(16, 16, testMapper5Nibble, 0)
		rom = append(rom, make([]byte, 16*testPRGUnit16KB+16*testCHRUnit8KB)...)

		cart, err := cartridge.New(bytes.NewReader(rom))
		require.NoError(t, err)
		require.NotNil(t, cart)
		assert.Equal(t, testMapper5ID, cart.MapperID)
		assert.NotNil(t, cart.Mapper)
	})
}
