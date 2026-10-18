// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	// Sunsoft 5B clock divider: 5B is clocked at half the CPU frequency (fCPU / 2).
	s5bClockDivisor = 2

	// Tone and Noise period counters decrement every 16 5B clocks (32 CPU cycles).
	s5bPrescalerClocks = 16

	// Envelope period counter decrements every 256 5B clocks (512 CPU cycles).
	s5bEnvelopePrescaler = 256

	s5bRegisterCount = 16

	s5bRegToneALow   = 0x00
	s5bRegToneAHigh  = 0x01
	s5bRegToneBLow   = 0x02
	s5bRegToneBHigh  = 0x03
	s5bRegToneCLow   = 0x04
	s5bRegToneCHigh  = 0x05
	s5bRegNoise      = 0x06
	s5bRegMixer      = 0x07
	s5bRegVolA       = 0x08
	s5bRegVolB       = 0x09
	s5bRegVolC       = 0x0A
	s5bRegEnvLow     = 0x0B
	s5bRegEnvHigh    = 0x0C
	s5bRegEnvShape   = 0x0D

	s5bMaskPeriodHigh = 0x0F
	s5bMaskPeriodLow  = 0xFF
	s5bMaskNoise      = 0x1F
	s5bMaskVol        = 0x0F
	s5bMaskEnvFlag    = 0x10
	s5bMaskEnvShape   = 0x0F

	s5bMixerDisableToneA  = 0x01
	s5bMixerDisableToneB  = 0x02
	s5bMixerDisableToneC  = 0x04
	s5bMixerDisableNoiseA = 0x08
	s5bMixerDisableNoiseB = 0x10
	s5bMixerDisableNoiseC = 0x20

	s5bEnvShapeHold      = 0x01
	s5bEnvShapeAlternate = 0x02
	s5bEnvShapeAttack    = 0x04
	s5bEnvShapeContinue  = 0x08

	s5bNoiseInitialLFSR = 0x00010000
	s5bNoiseTapBit      = 3
	s5bNoiseFeedbackMask = 0x01

	s5bVolumeSteps    = 16
	s5bMaxAudioOutput = 3.0
	// s5bMixScale normalizes the sum of 3 channels (max 3.0) to match APU expansion audio level (~0.25).
	s5bMixScale = float32(0.25 / s5bMaxAudioOutput)
)

// ym2149DAC contains canonical logarithmic voltage levels (0..1.0) for the 16 volume steps.
var ym2149DAC = [s5bVolumeSteps]float32{
	0.0,
	0.007812, 0.011048, 0.015625, 0.022097,
	0.031250, 0.044194, 0.062500, 0.088388,
	0.125000, 0.176777, 0.250000, 0.353553,
	0.500000, 0.707107, 1.000000,
}

// Sunsoft5BAudio implements the sound generator embedded in the Sunsoft 5B ASIC
// (a Yamaha YM2149 / General Instrument AY-3-8910 clone).
// Provides 3 square wave channels, a 17-bit LFSR noise generator, and hardware envelope.
type Sunsoft5BAudio struct {
	regs            [s5bRegisterCount]byte
	selectedReg     byte
	clockAccumulator int
	prescaler       int
	envPrescaler    int

	// Tone generators (A, B, C)
	tonePeriod [3]int
	toneTimer  [3]int
	toneOutput [3]bool

	// Noise generator
	noisePeriod int
	noiseTimer  int
	noiseLFSR   uint32
	noiseOutput bool

	// Envelope generator
	envPeriod    int
	envTimer     int
	envVolume    int
	envAttack    bool
	envAlternate bool
	envHold      bool
	envContinue  bool
	envHolding   bool
}

// NewSunsoft5BAudio creates and initializes a new Sunsoft 5B audio core.
func NewSunsoft5BAudio() *Sunsoft5BAudio {
	a := &Sunsoft5BAudio{
		noiseLFSR: s5bNoiseInitialLFSR,
	}
	for i := range a.tonePeriod {
		a.tonePeriod[i] = 1
		a.toneTimer[i] = 1
	}
	a.noisePeriod = 1
	a.noiseTimer = 1
	a.envPeriod = 1
	a.envTimer = 1
	return a
}

// SelectRegister sets the currently active sound register index (0..15).
func (a *Sunsoft5BAudio) SelectRegister(reg byte) {
	a.selectedReg = reg & 0x0F
}

// WriteData writes a byte to the currently selected sound register.
func (a *Sunsoft5BAudio) WriteData(data byte) {
	a.regs[a.selectedReg] = data

	switch a.selectedReg {
	case s5bRegToneALow, s5bRegToneAHigh:
		a.tonePeriod[0] = (int(a.regs[s5bRegToneAHigh]&s5bMaskPeriodHigh) << 8) | int(a.regs[s5bRegToneALow])
	case s5bRegToneBLow, s5bRegToneBHigh:
		a.tonePeriod[1] = (int(a.regs[s5bRegToneBHigh]&s5bMaskPeriodHigh) << 8) | int(a.regs[s5bRegToneBLow])
	case s5bRegToneCLow, s5bRegToneCHigh:
		a.tonePeriod[2] = (int(a.regs[s5bRegToneCHigh]&s5bMaskPeriodHigh) << 8) | int(a.regs[s5bRegToneCLow])
	case s5bRegNoise:
		a.noisePeriod = int(data & s5bMaskNoise)
	case s5bRegEnvLow, s5bRegEnvHigh:
		a.envPeriod = (int(a.regs[s5bRegEnvHigh]) << 8) | int(a.regs[s5bRegEnvLow])
	case s5bRegEnvShape:
		a.resetEnvelope(data)
	}
}

func (a *Sunsoft5BAudio) resetEnvelope(shape byte) {
	a.envHold = (shape & s5bEnvShapeHold) != 0
	a.envAlternate = (shape & s5bEnvShapeAlternate) != 0
	a.envAttack = (shape & s5bEnvShapeAttack) != 0
	a.envContinue = (shape & s5bEnvShapeContinue) != 0
	a.envHolding = false

	a.envVolume = s5bVolumeSteps - 1
	if a.envAttack {
		a.envVolume = 0
	}
	a.envTimer = a.envPeriod
}

// StepAudio advances the 5B audio synthesis by the given CPU cycles.
func (a *Sunsoft5BAudio) StepAudio(cpuCycles int) {
	if cpuCycles <= 0 {
		return
	}

	a.clockAccumulator += cpuCycles
	// 5B clock runs at half CPU frequency (1 tick per 2 CPU cycles)
	ticks := a.clockAccumulator / s5bClockDivisor
	a.clockAccumulator %= s5bClockDivisor

	for range ticks {
		a.stepToneAndNoise()
		a.stepEnvelope()
	}
}

func (a *Sunsoft5BAudio) stepToneAndNoise() {
	a.prescaler++
	if a.prescaler < s5bPrescalerClocks {
		return
	}
	a.prescaler = 0

	// Step tone generators A, B, C
	for ch := range 3 {
		period := a.tonePeriod[ch]
		if period == 0 {
			period = 1
		}
		a.toneTimer[ch]--
		if a.toneTimer[ch] <= 0 {
			a.toneTimer[ch] = period
			a.toneOutput[ch] = !a.toneOutput[ch]
		}
	}

	// Step noise generator
	noisePeriod := a.noisePeriod
	if noisePeriod == 0 {
		noisePeriod = 1
	}
	a.noiseTimer--
	if a.noiseTimer <= 0 {
		a.noiseTimer = noisePeriod
		// 17-bit LFSR: feedback = bit 0 XOR bit 3
		fb := (a.noiseLFSR & s5bNoiseFeedbackMask) ^ ((a.noiseLFSR >> s5bNoiseTapBit) & s5bNoiseFeedbackMask)
		a.noiseLFSR = (a.noiseLFSR >> 1) | (fb << 16)
		a.noiseOutput = (a.noiseLFSR & s5bNoiseFeedbackMask) != 0
	}
}

func (a *Sunsoft5BAudio) stepEnvelope() {
	a.envPrescaler++
	if a.envPrescaler < s5bEnvelopePrescaler {
		return
	}
	a.envPrescaler = 0

	if a.envHolding {
		return
	}

	envPeriod := a.envPeriod
	if envPeriod == 0 {
		envPeriod = 1
	}

	a.envTimer--
	if a.envTimer > 0 {
		return
	}
	a.envTimer = envPeriod

	if a.envAttack {
		a.envVolume++
		if a.envVolume >= s5bVolumeSteps {
			a.handleEnvelopeOverflow()
		}
		return
	}
	a.envVolume--
	if a.envVolume < 0 {
		a.handleEnvelopeUnderflow()
	}
}

func (a *Sunsoft5BAudio) handleEnvelopeOverflow() {
	if !a.envContinue {
		a.envVolume = 0
		a.envHolding = true
		return
	}
	if a.envHold {
		a.envVolume = s5bVolumeSteps - 1
		if a.envAlternate {
			a.envVolume = 0
		}
		a.envHolding = true
		return
	}
	a.envVolume = 0
	if a.envAlternate {
		a.envAttack = false
		a.envVolume = s5bVolumeSteps - 1
	}
}

func (a *Sunsoft5BAudio) handleEnvelopeUnderflow() {
	if !a.envContinue {
		a.envVolume = 0
		a.envHolding = true
		return
	}
	if a.envHold {
		a.envVolume = 0
		if a.envAlternate {
			a.envVolume = s5bVolumeSteps - 1
		}
		a.envHolding = true
		return
	}
	a.envVolume = s5bVolumeSteps - 1
	if a.envAlternate {
		a.envAttack = true
		a.envVolume = 0
	}
}

// AudioOutput returns the combined normalized audio sample of the 3 channels.
func (a *Sunsoft5BAudio) AudioOutput() float32 {
	mixer := a.regs[s5bRegMixer]

	var sum float32
	// Channel A
	volA := a.channelVolume(s5bRegVolA)
	toneA := (mixer&s5bMixerDisableToneA) != 0 || a.toneOutput[0]
	noiseA := (mixer&s5bMixerDisableNoiseA) != 0 || a.noiseOutput
	if toneA && noiseA {
		sum += ym2149DAC[volA]
	}

	// Channel B
	volB := a.channelVolume(s5bRegVolB)
	toneB := (mixer&s5bMixerDisableToneB) != 0 || a.toneOutput[1]
	noiseB := (mixer&s5bMixerDisableNoiseB) != 0 || a.noiseOutput
	if toneB && noiseB {
		sum += ym2149DAC[volB]
	}

	// Channel C
	volC := a.channelVolume(s5bRegVolC)
	toneC := (mixer&s5bMixerDisableToneC) != 0 || a.toneOutput[2]
	noiseC := (mixer&s5bMixerDisableNoiseC) != 0 || a.noiseOutput
	if toneC && noiseC {
		sum += ym2149DAC[volC]
	}

	return sum * s5bMixScale
}

func (a *Sunsoft5BAudio) channelVolume(reg byte) int {
	val := a.regs[reg]
	if (val & s5bMaskEnvFlag) != 0 {
		return a.envVolume
	}
	return int(val & s5bMaskVol)
}

// SaveState serializes the Sunsoft 5B audio synthesis state.
func (a *Sunsoft5BAudio) SaveState(w io.Writer) error {
	var tonePeriod, toneTimer [3]int32
	for i := range 3 {
		tonePeriod[i] = int32(a.tonePeriod[i] & 0x7FFFFFFF)
		toneTimer[i] = int32(a.toneTimer[i] & 0x7FFFFFFF)
	}
	state := struct {
		Regs             [s5bRegisterCount]byte
		SelectedReg      byte
		ClockAccumulator int32
		Prescaler        int32
		EnvPrescaler     int32
		TonePeriod       [3]int32
		ToneTimer        [3]int32
		ToneOutput       [3]bool
		NoisePeriod      int32
		NoiseTimer       int32
		NoiseLFSR        uint32
		NoiseOutput      bool
		EnvPeriod        int32
		EnvTimer         int32
		EnvVolume        int32
		EnvAttack        bool
		EnvAlternate     bool
		EnvHold          bool
		EnvContinue      bool
		EnvHolding       bool
	}{
		Regs:             a.regs,
		SelectedReg:      a.selectedReg,
		ClockAccumulator: int32(a.clockAccumulator & 0x7FFFFFFF),
		Prescaler:        int32(a.prescaler & 0x7FFFFFFF),
		EnvPrescaler:     int32(a.envPrescaler & 0x7FFFFFFF),
		TonePeriod:       tonePeriod,
		ToneTimer:        toneTimer,
		ToneOutput:       a.toneOutput,
		NoisePeriod:      int32(a.noisePeriod & 0x7FFFFFFF),
		NoiseTimer:       int32(a.noiseTimer & 0x7FFFFFFF),
		NoiseLFSR:        a.noiseLFSR,
		NoiseOutput:      a.noiseOutput,
		EnvPeriod:        int32(a.envPeriod & 0x7FFFFFFF),
		EnvTimer:         int32(a.envTimer & 0x7FFFFFFF),
		EnvVolume:        int32(a.envVolume & 0x7FFFFFFF),
		EnvAttack:        a.envAttack,
		EnvAlternate:     a.envAlternate,
		EnvHold:          a.envHold,
		EnvContinue:      a.envContinue,
		EnvHolding:       a.envHolding,
	}

	if err := binary.Write(w, binary.LittleEndian, state); err != nil {
		return fmt.Errorf("write sunsoft5b audio state: %w", err)
	}
	return nil
}

// LoadState restores the Sunsoft 5B audio synthesis state.
func (a *Sunsoft5BAudio) LoadState(r io.Reader) error {
	var state struct {
		Regs             [s5bRegisterCount]byte
		SelectedReg      byte
		ClockAccumulator int32
		Prescaler        int32
		EnvPrescaler     int32
		TonePeriod       [3]int32
		ToneTimer        [3]int32
		ToneOutput       [3]bool
		NoisePeriod      int32
		NoiseTimer       int32
		NoiseLFSR        uint32
		NoiseOutput      bool
		EnvPeriod        int32
		EnvTimer         int32
		EnvVolume        int32
		EnvAttack        bool
		EnvAlternate     bool
		EnvHold          bool
		EnvContinue      bool
		EnvHolding       bool
	}
	if err := binary.Read(r, binary.LittleEndian, &state); err != nil {
		return fmt.Errorf("read sunsoft5b audio state: %w", err)
	}
	a.regs = state.Regs
	a.selectedReg = state.SelectedReg
	a.clockAccumulator = int(state.ClockAccumulator)
	a.prescaler = int(state.Prescaler)
	a.envPrescaler = int(state.EnvPrescaler)
	for i := range 3 {
		a.tonePeriod[i] = int(state.TonePeriod[i])
		a.toneTimer[i] = int(state.ToneTimer[i])
	}
	a.toneOutput = state.ToneOutput
	a.noisePeriod = int(state.NoisePeriod)
	a.noiseTimer = int(state.NoiseTimer)
	a.noiseLFSR = state.NoiseLFSR
	a.noiseOutput = state.NoiseOutput
	a.envPeriod = int(state.EnvPeriod)
	a.envTimer = int(state.EnvTimer)
	a.envVolume = int(state.EnvVolume)
	a.envAttack = state.EnvAttack
	a.envAlternate = state.EnvAlternate
	a.envHold = state.EnvHold
	a.envContinue = state.EnvContinue
	a.envHolding = state.EnvHolding
	return nil
}

// Reset silences all 3 square waves and noise channels on console reset.
func (a *Sunsoft5BAudio) Reset() {
	a.regs = [s5bRegisterCount]byte{}
	for i := range a.tonePeriod {
		a.tonePeriod[i] = 1
		a.toneTimer[i] = 1
	}
	a.noisePeriod = 1
	a.noiseTimer = 1
	a.envPeriod = 1
	a.envTimer = 1
	a.noiseLFSR = s5bNoiseInitialLFSR
	a.selectedReg = 0
	a.clockAccumulator = 0
	a.prescaler = 0
	a.envPrescaler = 0
	a.envVolume = 0
	a.envHolding = false
}
