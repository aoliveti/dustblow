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
	regToneALow   = 0x00
	regToneAHigh  = 0x01
	regToneBLow   = 0x02
	regToneBHigh  = 0x03
	regToneCLow   = 0x04
	regToneCHigh  = 0x05
	regNoise      = 0x06
	regMixer      = 0x07
	regVolA       = 0x08
	regVolB       = 0x09
	regVolC       = 0x0A
	regEnvLow     = 0x0B
	regEnvHigh    = 0x0C
	regEnvShape   = 0x0D

	mixerDisableAllAudio = 0x3F
	mixerEnableAllTones  = 0x38 // Noise disabled (bits 3,4,5 = 1), Tone enabled (bits 0,1,2 = 0)

	maxVolumeFixed = 0x0F
	envModeFlag    = 0x10

	cpuCyclesPer5BTick     = 2
	prescalerStepClocks    = 16
	envPrescalerStepClocks = 256
)

func writeAudioReg(audio *cartridge.Sunsoft5BAudio, reg byte, val byte) {
	audio.SelectRegister(reg)
	audio.WriteData(val)
}

func TestSunsoft5BAudio_ToneFrequency(t *testing.T) {
	t.Parallel()

	audio := cartridge.NewSunsoft5BAudio()

	// Configure Channel A: period = 4 (Tone timer decrements every 16 5B ticks = 32 CPU cycles)
	writeAudioReg(audio, regToneALow, 0x04)
	writeAudioReg(audio, regToneAHigh, 0x00)
	// Enable Tone A only (disable tones B and C, disable all noise)
	writeAudioReg(audio, regMixer, 0x3E) // Tone A bit 0 = 0 (enabled), others disabled
	// Volume A = max
	writeAudioReg(audio, regVolA, maxVolumeFixed)

	initialSample := audio.AudioOutput()
	// Step until the output toggles so we are synchronized with the start of a period
	for audio.AudioOutput() == initialSample {
		audio.StepAudio(cpuCyclesPer5BTick)
	}

	edgeSample := audio.AudioOutput()
	assert.NotEqual(t, initialSample, edgeSample, "Output should toggle at least once")

	cyclesPerToggle := 4 * prescalerStepClocks * cpuCyclesPer5BTick

	const floatDelta = 1e-6

	// Step half of the period: output should not change
	audio.StepAudio(cyclesPerToggle / 2)
	assert.InDelta(t, float64(edgeSample), float64(audio.AudioOutput()), floatDelta, "Sample should not change before period expires")

	// Step remaining half of period: output should toggle
	audio.StepAudio(cyclesPerToggle / 2)
	toggledSample := audio.AudioOutput()
	assert.NotEqual(t, edgeSample, toggledSample, "Square wave flip-flop should toggle output")

	// Step another full period: output should flip back to edgeSample
	audio.StepAudio(cyclesPerToggle)
	assert.InDelta(t, float64(edgeSample), float64(audio.AudioOutput()), floatDelta, "Square wave flip-flop should complete cycle")
}

func TestSunsoft5BAudio_Mixer(t *testing.T) {
	t.Parallel()

	audio := cartridge.NewSunsoft5BAudio()

	// Set period to 1 for all channels
	writeAudioReg(audio, regToneALow, 0x01)
	writeAudioReg(audio, regToneBLow, 0x01)
	writeAudioReg(audio, regToneCLow, 0x01)

	// Set max volume for all channels
	writeAudioReg(audio, regVolA, maxVolumeFixed)
	writeAudioReg(audio, regVolB, maxVolumeFixed)
	writeAudioReg(audio, regVolC, maxVolumeFixed)

	// Step enough cycles so flip-flops are non-zero
	audio.StepAudio(128)

	// Disable all channels (tone and noise)
	writeAudioReg(audio, regMixer, mixerDisableAllAudio)
	// Even if mixer disables tone and noise, in AY-3-8910 disabled tone/noise lines are pulled HIGH.
	// Therefore, disabling tone and noise outputs continuous DC volume.
	dcOutput := audio.AudioOutput()
	assert.Positive(t, dcOutput, "Disabled tone and noise outputs DC high")

	// Set volume to 0: DC output must drop to 0
	writeAudioReg(audio, regVolA, 0)
	writeAudioReg(audio, regVolB, 0)
	writeAudioReg(audio, regVolC, 0)
	assert.Zero(t, audio.AudioOutput(), "Zero volume must yield zero audio output")
}

func TestSunsoft5BAudio_NoiseLFSR(t *testing.T) {
	t.Parallel()

	audio := cartridge.NewSunsoft5BAudio()

	// Noise period = 2
	writeAudioReg(audio, regNoise, 0x02)
	// Enable noise on Channel A only (disable tone A, disable B & C completely)
	// Mixer: Tone A disabled (bit 0 = 1), Noise A enabled (bit 3 = 0)
	writeAudioReg(audio, regMixer, 0x37) // 0011 0111
	writeAudioReg(audio, regVolA, maxVolumeFixed)

	noiseCyclesPerClock := 2 * prescalerStepClocks * cpuCyclesPer5BTick

	sawHigh := false
	sawLow := false

	// Step through multiple noise cycles to verify LFSR changes output
	for range 100 {
		audio.StepAudio(noiseCyclesPerClock)
		out := audio.AudioOutput()
		if out > 0 {
			sawHigh = true
			continue
		}
		sawLow = true
	}

	assert.True(t, sawHigh, "LFSR noise should produce high outputs")
	assert.True(t, sawLow, "LFSR noise should produce low outputs")
}

func TestSunsoft5BAudio_EnvelopeShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		shape        byte
		initialVol   byte
		expectDecay  bool
		expectRepeat bool
	}{
		{
			name:         "Decay and Hold Zero (Shape 0x00)",
			shape:        0x00, // Continue=0: decay down to 0 and hold
			initialVol:   15,
			expectDecay:  true,
			expectRepeat: false,
		},
		{
			name:         "Decay and Repeat Sawtooth (Shape 0x08)",
			shape:        0x08, // Continue=1, Attack=0, Alternate=0, Hold=0
			initialVol:   15,
			expectDecay:  true,
			expectRepeat: true,
		},
		{
			name:         "Attack and Hold Zero (Shape 0x04)",
			shape:        0x04, // Continue=0, Attack=1: ramp 0..15 then drop to 0 and hold
			initialVol:   0,
			expectDecay:  false,
			expectRepeat: false,
		},
		{
			name:         "Attack and Repeat Sawtooth (Shape 0x0C)",
			shape:        0x0C, // Continue=1, Attack=1, Alternate=0, Hold=0
			initialVol:   0,
			expectDecay:  false,
			expectRepeat: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			audio := cartridge.NewSunsoft5BAudio()

			// Envelope period = 1
			writeAudioReg(audio, regEnvLow, 0x01)
			writeAudioReg(audio, regEnvHigh, 0x00)

			// Channel A uses Envelope mode
			writeAudioReg(audio, regVolA, envModeFlag)
			// Mixer: DC high (disable tone and noise on channel A)
			writeAudioReg(audio, regMixer, mixerDisableAllAudio)

			// Trigger envelope shape
			writeAudioReg(audio, regEnvShape, tt.shape)

			// Check initial output aligns with expectDecay (max vs 0)
			initialSample := audio.AudioOutput()
			if tt.expectDecay {
				assert.Positive(t, initialSample, "Decay should start at max volume")
			}
			if !tt.expectDecay {
				assert.Zero(t, initialSample, "Attack should start at zero volume")
			}

			// Step envelope clock: 1 period = 256 5B ticks = 512 CPU cycles
			envStepCycles := envPrescalerStepClocks * cpuCyclesPer5BTick

			// Step 15 envelope steps to reach the end of the initial ramp (0..15 or 15..0)
			for range 15 {
				audio.StepAudio(envStepCycles)
			}

			rampEndSample := audio.AudioOutput()
			if tt.expectDecay {
				assert.Zero(t, rampEndSample, "Decay ramp should reach zero at step 15")
			}
			if !tt.expectDecay {
				assert.Positive(t, rampEndSample, "Attack ramp should reach max volume at step 15")
			}

			// Step the 16th step: underflow or overflow occurs
			audio.StepAudio(envStepCycles)
			wrappedSample := audio.AudioOutput()

			if !tt.expectRepeat {
				assert.Zero(t, wrappedSample, "Non-repeating envelope should hold at zero volume")
				return
			}
			if tt.expectDecay {
				assert.Positive(t, wrappedSample, "Repeating decay should wrap back to max volume")
				return
			}
			assert.Zero(t, wrappedSample, "Repeating attack should wrap back to zero volume")
		})
	}
}

func TestMapper69_ExpansionAudioIntegration(t *testing.T) {
	t.Parallel()

	cart, mapper := createTestFME7Cartridge(t)
	require.NotNil(t, cart.ExpansionAudio, "Mapper69 cartridge must have ExpansionAudio registered")

	// Write sound register select: $C000 -> Reg 0 (Tone A Low)
	mapper.WritePRG(0xC000, regToneALow)
	// Write sound data: $E000 -> 0x02
	mapper.WritePRG(0xE000, 0x02)

	// Reg 7 (Mixer) -> Enable tone A, disable noise and other tones
	mapper.WritePRG(0xC000, regMixer)
	mapper.WritePRG(0xE000, mixerEnableAllTones)

	// Reg 8 (Vol A) -> Max volume
	mapper.WritePRG(0xC000, regVolA)
	mapper.WritePRG(0xE000, maxVolumeFixed)

	steppable, ok := mapper.(cartridge.CPUSteppable)
	require.True(t, ok, "Mapper69 must implement CPUSteppable")

	// Step CPU cycles to advance audio
	steppable.StepCPU(512)

	sample := cart.ExpansionAudio().AudioOutput()
	assert.GreaterOrEqual(t, sample, float32(0.0), "Sample should be non-negative")
}
