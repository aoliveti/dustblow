// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/dustblow/dustblow/internal/cartridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockMultiFeatureMapper struct {
	irqPending  bool
	cpuCycles   int
	lastScan    int
	spriteFetch bool
	savedValue  byte
}

func (m *mockMultiFeatureMapper) ReadPRG(_ uint16) byte       { return 0 }
func (m *mockMultiFeatureMapper) WritePRG(_ uint16, _ byte)   {}
func (m *mockMultiFeatureMapper) ReadCHR(_ uint16) byte       { return 0 }
func (m *mockMultiFeatureMapper) WriteCHR(_ uint16, _ byte)   {}
func (m *mockMultiFeatureMapper) IRQPending() bool            { return m.irqPending }
func (m *mockMultiFeatureMapper) StepCPU(cycles int)          { m.cpuCycles += cycles }
func (m *mockMultiFeatureMapper) ClockScanline(scanline int)  { m.lastScan = scanline }
func (m *mockMultiFeatureMapper) SetSpriteFetch(active bool)  { m.spriteFetch = active }
func (m *mockMultiFeatureMapper) SaveState(w io.Writer) error {
	_, err := w.Write([]byte{m.savedValue})
	return err
}
func (m *mockMultiFeatureMapper) LoadState(r io.Reader) error {
	buf := make([]byte, 1)
	_, err := io.ReadFull(r, buf)
	if err == nil {
		m.savedValue = buf[0]
	}
	return err
}

func TestCartridge_BindMapperAndInterfaceCaching(t *testing.T) {
	t.Parallel()

	cart := &cartridge.Cartridge{
		PRG: make([]byte, 0x4000),
		CHR: make([]byte, 0x2000),
	}

	mock := &mockMultiFeatureMapper{
		irqPending: true,
		savedValue: 0x42,
	}

	cart.BindMapper(mock)

	assert.True(t, cart.IRQPending())
	cart.StepCPU(12)
	assert.Equal(t, 12, mock.cpuCycles)

	cart.ClockScanline(240)
	assert.Equal(t, 240, mock.lastScan)

	cart.SetSpriteFetch(true)
	assert.True(t, mock.spriteFetch)

	var buf bytes.Buffer
	err := cart.SaveState(&buf)
	require.NoError(t, err)
	assert.Equal(t, []byte{0x42}, buf.Bytes())

	mock.savedValue = 0
	err = cart.LoadState(bytes.NewReader([]byte{0x99}))
	require.NoError(t, err)
	assert.Equal(t, byte(0x99), mock.savedValue)

	// Unbinding with nil should reset all cached interfaces safely.
	cart.BindMapper(nil)
	assert.False(t, cart.IRQPending())
	cart.StepCPU(100)
	cart.ClockScanline(10)
	cart.SetSpriteFetch(false)
	require.NoError(t, cart.SaveState(&buf))
	require.NoError(t, cart.LoadState(bytes.NewReader([]byte{1})))
}

func TestCartridge_DirectMapperAssignmentAutoBinds(t *testing.T) {
	t.Parallel()

	cart := &cartridge.Cartridge{
		PRG: make([]byte, 0x4000),
		CHR: make([]byte, 0x2000),
	}

	mock := &mockMultiFeatureMapper{
		irqPending: true,
	}

	// Directly mutate cart.Mapper without explicit BindMapper call.
	cart.Mapper = mock

	// Fast path should auto-bind on first access.
	assert.True(t, cart.IRQPending())

	cart.ClockScanline(123)
	assert.Equal(t, 123, mock.lastScan)
}

func TestCartridge_NilCartridgeSafety(t *testing.T) {
	t.Parallel()

	var cart *cartridge.Cartridge

	assert.False(t, cart.IRQPending())
	assert.Nil(t, cart.BatterySRAM())
	assert.False(t, cart.IsSRAMDirty())
	assert.Nil(t, cart.ExpansionAudio())

	// Invoking methods on nil cart should be safe no-ops.
	cart.StepCPU(10)
	cart.ClockScanline(5)
	cart.SetSpriteFetch(true)
	cart.MarkSRAMDirty()
	cart.ClearSRAMDirty()

	var buf bytes.Buffer
	require.NoError(t, cart.SaveState(&buf))
	require.NoError(t, cart.LoadState(bytes.NewReader(nil)))

	data, handled := cart.ReadNametable(0x2000, nil)
	assert.False(t, handled)
	assert.Equal(t, byte(0), data)
	assert.False(t, cart.WriteNametable(0x2000, 0, nil))
}

func TestCartridge_SRAMHelpers(t *testing.T) {
	t.Parallel()

	cart := &cartridge.Cartridge{
		SRAM:       make([]byte, cartridge.SRAMSize),
		HasBattery: true,
	}

	const (
		testSRAMAddr uint16 = 0x6123
		testSRAMData byte   = 0x5A
	)

	// Outside SRAM range
	assert.False(t, cart.WriteSRAM(0x5FFF, testSRAMData))
	assert.False(t, cart.WriteSRAM(0x8000, testSRAMData))
	assert.Equal(t, byte(0), cart.ReadSRAM(0x5FFF))
	assert.Equal(t, byte(0), cart.ReadSRAM(0x8000))

	// Within SRAM range
	assert.True(t, cart.WriteSRAM(testSRAMAddr, testSRAMData))
	assert.True(t, cart.IsSRAMDirty())
	assert.Equal(t, testSRAMData, cart.ReadSRAM(testSRAMAddr))
}

func TestCartridge_UnbankedCHR(t *testing.T) {
	t.Parallel()

	chr := make([]byte, 0x2000)
	cart := &cartridge.Cartridge{
		CHR:      chr,
		IsCHRRAM: true,
	}

	u := cartridge.NewUnbankedCHR(cart)

	const (
		testCHRAddr uint16 = 0x0542
		testCHRData byte   = 0xA7
	)

	u.WriteCHR(testCHRAddr, testCHRData)
	assert.Equal(t, testCHRData, u.ReadCHR(testCHRAddr))

	// Out of bounds reads and writes
	u.WriteCHR(0x2000, 0xFF)
	assert.Equal(t, byte(0), u.ReadCHR(0x2000))

	// CHR-ROM protection (writes ignored when IsCHRRAM is false)
	cart.IsCHRRAM = false
	u.WriteCHR(testCHRAddr, 0x55)
	assert.Equal(t, testCHRData, u.ReadCHR(testCHRAddr))
}

type mockResettableMapper struct {
	mockMultiFeatureMapper
	wasReset bool
}

func (m *mockResettableMapper) Reset() {
	m.wasReset = true
}

func TestCartridge_Reset(t *testing.T) {
	t.Parallel()

	cart := &cartridge.Cartridge{}
	mock := &mockResettableMapper{}
	cart.BindMapper(mock)

	assert.False(t, mock.wasReset)
	cart.Reset()
	assert.True(t, mock.wasReset)
}

func TestCartridge_NilGuards(t *testing.T) {
	t.Parallel()

	var cart *cartridge.Cartridge
	assert.Equal(t, byte(0), cart.ReadPRG(0x8000))
	cart.WritePRG(0x8000, 0x42)
	assert.Equal(t, byte(0), cart.ReadCHR(0x0000))
	cart.WriteCHR(0x0000, 0x42)
	assert.False(t, cart.IRQPending())
	cart.StepCPU(10)
	cart.ClockScanline(20)
	cart.SetSpriteFetch(true)
	assert.Nil(t, cart.ExpansionAudio())
	cart.Reset()
	assert.NoError(t, cart.SaveState(nil))
	assert.NoError(t, cart.LoadState(nil))
	val, ok := cart.ReadNametable(0x2000, nil)
	assert.False(t, ok)
	assert.Equal(t, byte(0), val)
	assert.False(t, cart.WriteNametable(0x2000, 0x55, nil))
}
