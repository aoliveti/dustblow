// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"testing"

	"github.com/dustblow/dustblow/internal/cartridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	addressLowRAM          uint16 = 0x0000
	addressCartridgeRAM    uint16 = 0x6000
	addressInvalidHighPRG  uint16 = 0x9000
	addressInvalidHighCHR  uint16 = 0x2000
	testPRGTruncatedSize          = 1024
	testCHRTruncatedSize          = 512
)

func TestMapper0_PRGAccess(t *testing.T) {
	t.Parallel()

	prg := make([]byte, testPRGUnit16KB)
	prg[0] = testOpcodeLDA

	cart := &cartridge.Cartridge{
		PRG: prg,
		CHR: make([]byte, testCHRUnit8KB),
	}
	mapper := cartridge.NewMapper0(cart)
	require.NotNil(t, mapper)

	t.Run("unmapped memory below $8000 returns zero", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, byte(0), mapper.ReadPRG(addressLowRAM))
		assert.Equal(t, byte(0), mapper.ReadPRG(addressCartridgeRAM))
	})

	t.Run("writes to PRG ROM are ignored", func(t *testing.T) {
		t.Parallel()
		mapper.WritePRG(testAddressPRGBase, sampleTestWriteByte)
		assert.Equal(t, testOpcodeLDA, mapper.ReadPRG(testAddressPRGBase))

		// Verify Cartridge level WritePRG delegation.
		cart.Mapper = mapper
		cart.WritePRG(testAddressPRGBase, sampleTestWriteByte)
		assert.Equal(t, testOpcodeLDA, cart.ReadPRG(testAddressPRGBase))
	})
}

func TestMapper0_BoundsSafety(t *testing.T) {
	t.Parallel()

	t.Run("truncated slices prevent panic on out of bounds", func(t *testing.T) {
		t.Parallel()
		cart := &cartridge.Cartridge{
			PRG: make([]byte, testPRGTruncatedSize),
			CHR: make([]byte, testCHRTruncatedSize),
		}
		mapper := cartridge.NewMapper0(cart)
		require.NotNil(t, mapper)

		assert.Equal(t, byte(0), mapper.ReadPRG(addressInvalidHighPRG))
		assert.Equal(t, byte(0), mapper.ReadCHR(addressInvalidHighCHR))

		mapper.WriteCHR(addressInvalidHighCHR, sampleTestWriteByte)
		assert.Equal(t, byte(0), mapper.ReadCHR(addressInvalidHighCHR))
	})

	t.Run("empty PRG slice defaults safely to singlePRGBank", func(t *testing.T) {
		t.Parallel()
		cart := &cartridge.Cartridge{}
		mapper := cartridge.NewMapper0(cart)
		require.NotNil(t, mapper)
		assert.Equal(t, byte(0), mapper.ReadPRG(testAddressPRGBase))
	})
}

func TestCartridge_NilMapperGuards(t *testing.T) {
	t.Parallel()

	cart := &cartridge.Cartridge{
		Mapper: nil,
	}

	assert.Equal(t, byte(0), cart.ReadPRG(testAddressPRGBase))
	cart.WritePRG(testAddressPRGBase, sampleTestWriteByte)

	assert.Equal(t, byte(0), cart.ReadCHR(testAddressCHRBase))
	cart.WriteCHR(testAddressCHRBase, sampleTestWriteByte)
}
