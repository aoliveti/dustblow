// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package apu implements the Ricoh 2A03 Audio Processing Unit (APU) for the NES.
package apu

import (
	"io"
)

const (
	cpuFrequencyNTSC  = 1789773.0
	defaultSampleRate = 44100.0

	// Hardware register memory addresses ($4000-$4017)
	registerPulse1Control     uint16 = 0x4000
	registerPulse1Sweep       uint16 = 0x4001
	registerPulse1TimerLow    uint16 = 0x4002
	registerPulse1TimerHigh   uint16 = 0x4003
	registerPulse2Control     uint16 = 0x4004
	registerPulse2Sweep       uint16 = 0x4005
	registerPulse2TimerLow    uint16 = 0x4006
	registerPulse2TimerHigh   uint16 = 0x4007
	registerTriangleControl   uint16 = 0x4008
	registerTriangleTimerLow  uint16 = 0x400A
	registerTriangleTimerHigh uint16 = 0x400B
	registerNoiseControl      uint16 = 0x400C
	registerNoisePeriod       uint16 = 0x400E
	registerNoiseLength       uint16 = 0x400F
	registerDMCControl        uint16 = 0x4010
	registerDMCDirectLoad     uint16 = 0x4011
	registerDMCSampleAddress  uint16 = 0x4012
	registerDMCSampleLength   uint16 = 0x4013
	registerStatus            uint16 = 0x4015
	registerFrameCounter      uint16 = 0x4017

	// Status register ($4015) read flags
	statusPulse1   byte = 0x01
	statusPulse2   byte = 0x02
	statusTriangle byte = 0x04
	statusNoise    byte = 0x08
	statusDMC      byte = 0x10
	statusFrameIRQ byte = 0x40
	statusDMCIRQ   byte = 0x80

	// Status register ($4015) channel enable write masks
	enablePulse1Mask   byte = 0x01
	enablePulse2Mask   byte = 0x02
	enableTriangleMask byte = 0x04
	enableNoiseMask    byte = 0x08
	enableDMCMask      byte = 0x10
)

// ExpansionAudio generates extra sound from cartridge hardware (e.g. MMC5, VRC6, Sunsoft 5B).
type ExpansionAudio interface {
	StepAudio(cycles int)
	AudioOutput() float32
}

// APU represents the 2A03 audio generation hardware.
type APU struct {
	Pulse1         *Pulse
	Pulse2         *Pulse
	Triangle       *Triangle
	Noise          *Noise
	DMC            *DMC
	FrameCounter   *FrameCounter
	filterChain    *FilterChain
	buffer         *RingBuffer
	expansionAudio ExpansionAudio

	apuCycleOdd      bool
	cycleDebt        float64
	accumulator      float32
	sampleCount      uint32
	cyclesPerSample  float64
	frameSamples     []float32
	recordingEnabled bool
}

// AttachExpansionAudio connects an optional cartridge expansion sound synthesizer.
func (a *APU) AttachExpansionAudio(exp ExpansionAudio) {
	a.expansionAudio = exp
}

// New creates an initialized APU with memory reader callback for DMC DMA.
func New(readMemory func(addr uint16) byte) *APU {
	apu := &APU{
		Pulse1:          NewPulse(false),
		Pulse2:          NewPulse(true),
		Triangle:        NewTriangle(),
		Noise:           NewNoise(),
		DMC:             NewDMC(readMemory),
		filterChain:     NewFilterChain(defaultSampleRate),
		buffer:          NewRingBuffer(),
		cyclesPerSample: cpuFrequencyNTSC / defaultSampleRate,
		frameSamples:    make([]float32, 0, 800),
	}

	apu.FrameCounter = NewFrameCounter(apu.clockQuarterFrame, apu.clockHalfFrame)
	apu.DMC.SetGetCycleHook(apu.CurrentCycleIsGet)
	return apu
}

func (a *APU) clockQuarterFrame() {
	a.Pulse1.ClockQuarterFrame()
	a.Pulse2.ClockQuarterFrame()
	a.Triangle.ClockQuarterFrame()
	a.Noise.ClockQuarterFrame()
}

func (a *APU) clockHalfFrame() {
	a.Pulse1.ClockHalfFrame()
	a.Pulse2.ClockHalfFrame()
	a.Triangle.ClockHalfFrame()
	a.Noise.ClockHalfFrame()
}

// ReadRegister reads APU registers ($4015 status register).
func (a *APU) ReadRegister(addr uint16) byte {
	if addr == registerStatus {
		return a.readStatus()
	}
	return 0
}

func (a *APU) readStatus() byte {
	var status byte
	if a.Pulse1.lengthCounter > 0 {
		status |= statusPulse1
	}
	if a.Pulse2.lengthCounter > 0 {
		status |= statusPulse2
	}
	if a.Triangle.lengthCounter > 0 {
		status |= statusTriangle
	}
	if a.Noise.lengthCounter > 0 {
		status |= statusNoise
	}
	if a.DMC.bytesRemaining > 0 {
		status |= statusDMC
	}
	if a.FrameCounter.ReadStatus(a.CurrentCycleIsGet()) {
		status |= statusFrameIRQ
	}
	if a.DMC.irqPending {
		status |= statusDMCIRQ
	}

	return status
}

// CurrentCycleIsGet reports whether the current CPU cycle being executed is an APU get cycle.
func (a *APU) CurrentCycleIsGet() bool {
	return !a.apuCycleOdd
}

// SyncDMA aligns APU cycle parity so that the next CPU cycle following OAM DMA is a get cycle.
func (a *APU) SyncDMA() {
	a.apuCycleOdd = false
}

// WriteRegister writes to APU registers ($4000-$4017).
func (a *APU) WriteRegister(addr uint16, val byte) {
	switch {
	case addr >= registerPulse1Control && addr <= registerPulse1TimerHigh:
		a.writePulse1(addr, val)
	case addr >= registerPulse2Control && addr <= registerPulse2TimerHigh:
		a.writePulse2(addr, val)
	case addr >= registerTriangleControl && addr <= registerTriangleTimerHigh:
		a.writeTriangle(addr, val)
	case addr >= registerNoiseControl && addr <= registerNoiseLength:
		a.writeNoise(addr, val)
	case addr >= registerDMCControl && addr <= registerDMCSampleLength:
		a.writeDMC(addr, val)
	case addr == registerStatus:
		a.writeStatus(val)
	case addr == registerFrameCounter:
		a.FrameCounter.WriteControl(val, a.CurrentCycleIsGet())
	}
}

func (a *APU) writePulse1(addr uint16, val byte) {
	switch addr {
	case registerPulse1Control:
		a.Pulse1.WriteControl(val)
	case registerPulse1Sweep:
		a.Pulse1.WriteSweep(val)
	case registerPulse1TimerLow:
		a.Pulse1.WriteTimerLow(val)
	case registerPulse1TimerHigh:
		a.Pulse1.WriteTimerHigh(val)
	}
}

func (a *APU) writePulse2(addr uint16, val byte) {
	switch addr {
	case registerPulse2Control:
		a.Pulse2.WriteControl(val)
	case registerPulse2Sweep:
		a.Pulse2.WriteSweep(val)
	case registerPulse2TimerLow:
		a.Pulse2.WriteTimerLow(val)
	case registerPulse2TimerHigh:
		a.Pulse2.WriteTimerHigh(val)
	}
}

func (a *APU) writeTriangle(addr uint16, val byte) {
	switch addr {
	case registerTriangleControl:
		a.Triangle.WriteControl(val)
	case registerTriangleTimerLow:
		a.Triangle.WriteTimerLow(val)
	case registerTriangleTimerHigh:
		a.Triangle.WriteTimerHigh(val)
	}
}

func (a *APU) writeNoise(addr uint16, val byte) {
	switch addr {
	case registerNoiseControl:
		a.Noise.WriteControl(val)
	case registerNoisePeriod:
		a.Noise.WriteModeAndPeriod(val)
	case registerNoiseLength:
		a.Noise.WriteLength(val)
	}
}

func (a *APU) writeDMC(addr uint16, val byte) {
	switch addr {
	case registerDMCControl:
		a.DMC.WriteControl(val)
	case registerDMCDirectLoad:
		a.DMC.WriteDirectLoad(val)
	case registerDMCSampleAddress:
		a.DMC.WriteSampleAddress(val)
	case registerDMCSampleLength:
		a.DMC.WriteSampleLength(val)
	}
}

func (a *APU) writeStatus(val byte) {
	a.Pulse1.SetEnabled((val & enablePulse1Mask) != 0)
	a.Pulse2.SetEnabled((val & enablePulse2Mask) != 0)
	a.Triangle.SetEnabled((val & enableTriangleMask) != 0)
	a.Noise.SetEnabled((val & enableNoiseMask) != 0)
	a.DMC.SetEnabled((val & enableDMCMask) != 0)
}

// Step advances the APU by cpuCycles.
func (a *APU) Step(cpuCycles int) {
	for range cpuCycles {
		a.stepTimers()
		a.FrameCounter.Step(1)
		if a.expansionAudio != nil {
			a.expansionAudio.StepAudio(1)
		}
		a.accumulateSample()
	}
}

func (a *APU) stepTimers() {
	a.Triangle.StepTimer()
	a.Noise.StepTimer()
	a.DMC.StepTimer()

	a.apuCycleOdd = !a.apuCycleOdd
	if a.apuCycleOdd {
		a.Pulse1.StepTimer()
		a.Pulse2.StepTimer()
	}
}

// APUCycleOdd returns whether the APU is currently on an odd CPU cycle (second half of APU cycle).
func (a *APU) APUCycleOdd() bool {
	return a.apuCycleOdd
}


func (a *APU) accumulateSample() {
	sample := MixChannels(
		a.Pulse1.Output(),
		a.Pulse2.Output(),
		a.Triangle.Output(),
		a.Noise.Output(),
		a.DMC.Output(),
	)
	if a.expansionAudio != nil {
		sample += a.expansionAudio.AudioOutput()
	}

	a.accumulator += sample
	a.sampleCount++
	a.cycleDebt += 1.0

	if a.cycleDebt < a.cyclesPerSample {
		return
	}

	mean := a.accumulator / float32(a.sampleCount)
	filtered := a.filterChain.Step(mean)

	a.buffer.WriteSample(filtered)
	if a.recordingEnabled {
		a.frameSamples = append(a.frameSamples, filtered)
	}

	a.cycleDebt -= a.cyclesPerSample
	a.accumulator = 0
	a.sampleCount = 0
}

// IRQPending returns true if either the Frame Counter or DMC is requesting a CPU interrupt.
func (a *APU) IRQPending() bool {
	return a.FrameCounter.IRQPending() || a.DMC.irqPending
}

// TakeStallCycles returns and resets any CPU stall cycles incurred by DMC DMA transfers.
func (a *APU) TakeStallCycles() int {
	if a.DMC == nil {
		return 0
	}
	return a.DMC.TakeStallCycles()
}

// AudioStream returns an io.Reader providing real-time stereo 16-bit PCM audio.
func (a *APU) AudioStream() io.Reader {
	return a.buffer
}

// FlushAudio flushes any queued real-time audio bytes (used on pause or reset).
func (a *APU) FlushAudio() {
	a.buffer.Clear()
}

// Reset silences all audio channels, resets timers, frame counter, and flushes output buffers per NES specifications.
func (a *APU) Reset() {
	a.writeStatus(0)
	a.Pulse1.Reset()
	a.Pulse2.Reset()
	a.Triangle.Reset()
	a.Noise.Reset()
	a.DMC.Reset()
	a.FrameCounter.Reset()
	a.filterChain.Reset()
	a.buffer.Clear()
	a.accumulator = 0
	a.sampleCount = 0
	a.cycleDebt = 0
	a.apuCycleOdd = false
}

// SetRecording enables or disables frame audio sample capture for recording and streaming.
func (a *APU) SetRecording(enabled bool) {
	a.recordingEnabled = enabled
	if !enabled {
		a.frameSamples = a.frameSamples[:0]
	}
}

// IsRecording reports whether frame audio sample capture is currently active.
func (a *APU) IsRecording() bool {
	return a.recordingEnabled
}

// TakeFrameSamples returns and clears the accumulated audio samples for the current frame.
// Used for synchronized video/audio recording and streaming.
func (a *APU) TakeFrameSamples() []float32 {
	if len(a.frameSamples) == 0 {
		return nil
	}
	samples := make([]float32, len(a.frameSamples))
	copy(samples, a.frameSamples)
	a.frameSamples = a.frameSamples[:0]
	return samples
}
