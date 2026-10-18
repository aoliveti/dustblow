// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bus_test

import (
	"testing"

	"github.com/dustblow/dustblow/internal/apu"
	"github.com/dustblow/dustblow/internal/bus"
	"github.com/dustblow/dustblow/internal/cartridge"
	"github.com/dustblow/dustblow/internal/joypad"
	"github.com/dustblow/dustblow/internal/ppu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAddressRAMZeroPage  uint16 = 0x0042
	testAddressRAMMirror2KB uint16 = 0x0842 // $0042 + 2KB ($0800)
	testAddressRAMMirror4KB uint16 = 0x1042 // $0042 + 4KB ($1000)
	testAddressRAMMirror6KB uint16 = 0x1842 // $0042 + 6KB ($1800)

	testAddressPPUCtrl   uint16 = 0x2000
	testAddressPPUMask   uint16 = 0x2001
	testAddressPPUStatus uint16 = 0x2002
	testAddressOAMDMA    uint16 = 0x4014
	testAddressAPUStatus uint16 = 0x4015
	testAddressJoypad1   uint16 = 0x4016
	testAddressJoypad2   uint16 = 0x4017
	testAddressPRGBase   uint16 = 0x8000
	testAddressPRGMirror uint16 = 0xC000
	sampleDMAPageBase    uint16 = 0x0200

	sampleRAMPayload     byte = 0x5A
	alternateRAMPayload  byte = 0xA5
	testDataZero         byte = 0x00
	sampleDMAPage        byte = 0x02
	sampleStrobeHigh     byte = 0x01
	sampleStrobeLow      byte = 0x00
	joypadButtonPressed  byte = 0x01
	joypadButtonReleased byte = 0x00

	prgBankSize16KB = 16384
	chrBankSize8KB  = 8192
)

func newTestCartridge() *cartridge.Cartridge {
	prg := make([]byte, prgBankSize16KB)
	prg[0] = sampleRAMPayload
	cart := &cartridge.Cartridge{
		PRG: prg,
		CHR: make([]byte, chrBankSize8KB),
	}
	cart.Mapper = cartridge.NewMapper0(cart)
	return cart
}

func TestBus_RAMAccessAndMirroring(t *testing.T) {
	t.Parallel()

	t.Run("read back from original address and all three mirrors", func(t *testing.T) {
		t.Parallel()
		b := bus.New(nil)
		b.Write(testAddressRAMZeroPage, sampleRAMPayload)
		assert.Equal(t, sampleRAMPayload, b.Read(testAddressRAMZeroPage))
		assert.Equal(t, sampleRAMPayload, b.Read(testAddressRAMMirror2KB))
		assert.Equal(t, sampleRAMPayload, b.Read(testAddressRAMMirror4KB))
		assert.Equal(t, sampleRAMPayload, b.Read(testAddressRAMMirror6KB))
	})

	t.Run("write to mirror updates original address", func(t *testing.T) {
		t.Parallel()
		bLocal := bus.New(nil)
		bLocal.Write(testAddressRAMMirror4KB, alternateRAMPayload)
		assert.Equal(t, alternateRAMPayload, bLocal.Read(testAddressRAMZeroPage))
		assert.Equal(t, alternateRAMPayload, bLocal.Peek(testAddressRAMZeroPage))
	})
}

func TestBus_CartridgeAccess(t *testing.T) {
	t.Parallel()

	t.Run("read PRG ROM", func(t *testing.T) {
		t.Parallel()
		b := bus.New(newTestCartridge())
		assert.Equal(t, sampleRAMPayload, b.Read(testAddressPRGBase))
		assert.Equal(t, sampleRAMPayload, b.Read(testAddressPRGMirror))
		assert.Equal(t, sampleRAMPayload, b.Peek(testAddressPRGBase))
	})

	t.Run("writes to PRG ROM are ignored", func(t *testing.T) {
		t.Parallel()
		b := bus.New(newTestCartridge())
		b.Write(testAddressPRGBase, alternateRAMPayload)
		assert.Equal(t, sampleRAMPayload, b.Read(testAddressPRGBase))
	})

	t.Run("nil cartridge safely returns zero", func(t *testing.T) {
		t.Parallel()
		bNil := bus.New(nil)
		assert.Equal(t, testDataZero, bNil.Read(testAddressPRGBase))
		assert.Equal(t, testDataZero, bNil.Peek(testAddressPRGBase))
		bNil.Write(testAddressPRGBase, sampleRAMPayload)
	})
}

func TestBus_PPUAndAPUAccess(t *testing.T) {
	t.Parallel()

	b := bus.New(nil)
	require.NotNil(t, b.PPU())

	// Write to PPUMASK through bus ($2001)
	b.Write(testAddressPPUMask, sampleRAMPayload)
	// Read open bus from $2000 through bus
	assert.Equal(t, sampleRAMPayload, b.Read(testAddressPPUCtrl))
	assert.Equal(t, sampleRAMPayload, b.Peek(testAddressPPUCtrl))

	// Test with nil PPU attached
	b.AttachPPU(nil)
	assert.Nil(t, b.PPU())
	assert.Equal(t, testDataZero, b.Read(testAddressPPUStatus))
	assert.Equal(t, testDataZero, b.Peek(testAddressPPUStatus))
	b.Write(testAddressPPUStatus, sampleRAMPayload)

	// APU tests
	assert.Equal(t, bus.OpenBusValue, b.Read(testAddressAPUStatus))
	assert.Equal(t, bus.OpenBusValue, b.Peek(testAddressAPUStatus))
	b.Write(testAddressAPUStatus, sampleRAMPayload)
}

func TestBus_OAMDMA(t *testing.T) {
	t.Parallel()

	t.Run("DMA transfer copies 256 bytes from CPU RAM to PPU OAM", func(t *testing.T) {
		t.Parallel()
		p := ppu.New(nil)
		b := bus.New(nil)
		b.AttachPPU(p)

		for i := range uint16(bus.OAMDMASize) {
			b.Write(sampleDMAPageBase+i, byte(i)^sampleRAMPayload)
		}

		b.Write(testAddressOAMDMA, sampleDMAPage)

		for i := range uint16(bus.OAMDMASize) {
			expected := byte(i) ^ sampleRAMPayload
			actual := p.ReadOAM(byte(i))
			assert.Equal(t, expected, actual)
		}
	})

	t.Run("DMA transfer with nil PPU does not panic", func(t *testing.T) {
		t.Parallel()
		b := bus.New(nil)
		b.Write(testAddressOAMDMA, sampleDMAPage)
	})
}

func TestBus_Controllers(t *testing.T) {
	t.Parallel()

	t.Run("strobe and sequential read on Port1 and Port2", func(t *testing.T) {
		t.Parallel()
		b := bus.New(nil)
		pad1 := joypad.New()
		pad2 := joypad.New()

		b.AttachController(joypad.Port1, pad1)
		b.AttachController(joypad.Port2, pad2)

		pad1.SetButton(joypad.ButtonA, true)
		pad2.SetButton(joypad.ButtonB, true)

		// Writing to $4016 strobes all connected controllers
		b.Write(testAddressJoypad1, sampleStrobeHigh)
		b.Write(testAddressJoypad1, sampleStrobeLow)

		// Port 1: first read is A (pressed)
		assert.Equal(t, joypadButtonPressed, b.Read(testAddressJoypad1))
		// Port 1: second read is B (released)
		assert.Equal(t, joypadButtonReleased, b.Read(testAddressJoypad1))

		// Port 2: first read is A (released)
		assert.Equal(t, joypadButtonReleased, b.Read(testAddressJoypad2))
		// Port 2: second read is B (pressed)
		assert.Equal(t, joypadButtonPressed, b.Read(testAddressJoypad2))
	})

	t.Run("peek does not advance controller shift registers", func(t *testing.T) {
		t.Parallel()
		b := bus.New(nil)
		pad1 := joypad.New()
		b.AttachController(joypad.Port1, pad1)
		pad1.SetButton(joypad.ButtonA, true)

		b.Write(testAddressJoypad1, sampleStrobeHigh)
		b.Write(testAddressJoypad1, sampleStrobeLow)

		assert.Equal(t, joypadButtonPressed, b.Peek(testAddressJoypad1))
		assert.Equal(t, joypadButtonPressed, b.Peek(testAddressJoypad1))
		assert.Equal(t, joypadButtonPressed, b.Read(testAddressJoypad1))
		assert.Equal(t, joypadButtonReleased, b.Peek(testAddressJoypad1))
	})

	t.Run("detached ports return open bus", func(t *testing.T) {
		t.Parallel()
		b := bus.New(nil)
		pad1 := joypad.New()

		// Unconnected initially
		assert.Equal(t, bus.OpenBusValue, b.Read(testAddressJoypad1))
		assert.Equal(t, bus.OpenBusValue, b.Peek(testAddressJoypad1))

		// Attach
		b.AttachController(joypad.Port1, pad1)
		pad1.SetButton(joypad.ButtonA, true)
		b.Write(testAddressJoypad1, sampleStrobeHigh)
		b.Write(testAddressJoypad1, sampleStrobeLow)
		assert.Equal(t, joypadButtonPressed, b.Read(testAddressJoypad1))

		// Detach
		b.DetachController(joypad.Port1)
		assert.Equal(t, bus.OpenBusValue, b.Read(testAddressJoypad1))
		assert.Equal(t, bus.OpenBusValue, b.Peek(testAddressJoypad1))
	})

	t.Run("out of range port operations do not panic", func(t *testing.T) {
		t.Parallel()
		b := bus.New(nil)
		invalidPort := joypad.Port(99)

		b.AttachController(invalidPort, joypad.New())
		b.DetachController(invalidPort)
	})
}

func TestBus_APUAttachment(t *testing.T) {
	t.Parallel()

	b := bus.New(nil)
	assert.Nil(t, b.APU())

	a := apu.New(nil)
	b.AttachAPU(a)
	assert.Equal(t, a, b.APU())
}
