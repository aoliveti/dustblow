// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"testing"

	"github.com/aoliveti/dustblow/internal/cartridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMMC5Audio_PulseChannelOutputAndStatus(t *testing.T) {
	t.Parallel()

	audio := cartridge.NewMMC5Audio()
	require.NotNil(t, audio)

	// Initially disabled: status $5015 returns 0
	val, ok := audio.ReadRegister(0x5015)
	assert.True(t, ok)
	assert.Equal(t, byte(0x00), val)
	assert.InDelta(t, float32(0.0), audio.AudioOutput(), 1e-6)

	// Enable Pulse 1 and Pulse 2 via $5015
	ok = audio.WriteRegister(0x5015, 0x03)
	assert.True(t, ok)

	// Configure Pulse 1: 50% duty (0x80), constant volume 10 (0x1A)
	ok = audio.WriteRegister(0x5000, 0x9A)
	assert.True(t, ok)
	// Period low: 4
	ok = audio.WriteRegister(0x5002, 0x04)
	assert.True(t, ok)
	// Period high: 0, length load index 0 (10 ticks)
	ok = audio.WriteRegister(0x5003, 0x00)
	assert.True(t, ok)

	// Status should report Pulse 1 active (bit 0)
	val, ok = audio.ReadRegister(0x5015)
	assert.True(t, ok)
	assert.Equal(t, byte(0x01), val)

	// Step timers and verify pulse waveform produces non-zero output
	hasNonZero := false
	for range 50 {
		audio.StepAudio(2)
		if audio.AudioOutput() > 0 {
			hasNonZero = true
			break
		}
	}
	assert.True(t, hasNonZero)

	// Disable Pulse 1 via $5015
	audio.WriteRegister(0x5015, 0x00)
	val, ok = audio.ReadRegister(0x5015)
	assert.True(t, ok)
	assert.Equal(t, byte(0x00), val)
}

func TestMMC5Audio_PCMWriteModeAndIRQ(t *testing.T) {
	t.Parallel()

	audio := cartridge.NewMMC5Audio()
	require.NotNil(t, audio)

	// Configure PCM: Write Mode (bit 0 = 0), IRQ Enable (bit 7 = 1) -> $80
	ok := audio.WriteRegister(0x5010, 0x80)
	assert.True(t, ok)
	assert.False(t, audio.IRQPending())

	// Write non-zero DAC value to $5011
	ok = audio.WriteRegister(0x5011, 0x7F) // ~127
	assert.True(t, ok)
	assert.False(t, audio.IRQPending())
	assert.Greater(t, audio.AudioOutput(), float32(0.1))

	// Write 0x00 to $5011: DAC does not change, IRQ trips
	audio.WriteRegister(0x5011, 0x00)
	assert.True(t, audio.IRQPending())

	// Read $5010: should report bit 7 set and acknowledge IRQ
	val, ok := audio.ReadRegister(0x5010)
	assert.True(t, ok)
	assert.Equal(t, byte(0x81), val) // IRQ bit 7 + MMC5A revision bit 0
	assert.False(t, audio.IRQPending())

	// Subsequent read to $5010 should have IRQ bit cleared
	val, ok = audio.ReadRegister(0x5010)
	assert.True(t, ok)
	assert.Equal(t, byte(0x01), val)
}

func TestMMC5Audio_PCMReadModeTrigger(t *testing.T) {
	t.Parallel()

	audio := cartridge.NewMMC5Audio()
	require.NotNil(t, audio)

	// Configure PCM: Read Mode (bit 0 = 1), IRQ Enable (bit 7 = 1) -> $81
	ok := audio.WriteRegister(0x5010, 0x81)
	assert.True(t, ok)

	// Direct write to $5011 in read mode is ignored
	audio.WriteRegister(0x5011, 0xFF)
	assert.InDelta(t, float32(0.0), audio.AudioOutput(), 1e-6)

	// Trigger read from PRG $8000-$BFFF with non-zero byte
	audio.ReadPRGTrigger(0x8000, 0x64) // DAC = 100
	assert.False(t, audio.IRQPending())
	assert.Greater(t, audio.AudioOutput(), float32(0.1))

	// Out of range read ($C000) does not affect PCM
	audio.ReadPRGTrigger(0xC000, 0x00)
	assert.False(t, audio.IRQPending())

	// Trigger read from $9000 with 0x00: trips IRQ
	audio.ReadPRGTrigger(0x9000, 0x00)
	assert.True(t, audio.IRQPending())

	// Acknowledge via $5010 read
	val, ok := audio.ReadRegister(0x5010)
	assert.True(t, ok)
	assert.Equal(t, byte(0x81), val)
	assert.False(t, audio.IRQPending())
}

func TestMMC5Audio_EnvelopeDecayAndLengthCounter(t *testing.T) {
	t.Parallel()

	audio := cartridge.NewMMC5Audio()
	audio.WriteRegister(0x5015, 0x01) // Enable Pulse 1

	// Duty 50% (0x80), decay envelope with period 2 (0x02), no loop
	audio.WriteRegister(0x5000, 0x82)
	audio.WriteRegister(0x5002, 0x01)
	// Length load index 3 -> 2 ticks
	audio.WriteRegister(0x5003, 0x18)

	val, _ := audio.ReadRegister(0x5015)
	assert.Equal(t, byte(0x01), val)

	// Advance 240Hz frame divider cycles twice (~7457 * 2 cycles)
	audio.StepAudio(7457 * 3)

	// Length counter should have expired
	val, _ = audio.ReadRegister(0x5015)
	assert.Equal(t, byte(0x00), val)
}
