// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge

const (
	vrc6AudioMaxOutput = 61.0
	// vrc6AudioMixScale normalizes the 0..61 sum to approximately match APU expansion audio levels (~0.25 max).
	vrc6AudioMixScale = float32(0.25 / vrc6AudioMaxOutput)

	vrc6PulseStepMask    = 0x0F
	vrc6SawtoothSteps    = 14
	vrc6SawtoothRateMask = 0x3F

	vrc6PulseDutyShift   = 4
	vrc6PulseDutyMask    = 0x07
	vrc6PulseVolumeMask  = 0x0F
	vrc6PulseModeMask    = 0x80
	vrc6PulseEnableMask  = 0x80
	vrc6FreqHighMask     = 0x0F
	vrc6FreqHighShift    = 8
	vrc6GlobalHaltMask   = 0x01
)

type vrc6Pulse struct {
	volume  byte
	duty    byte
	mode    bool
	enabled bool
	period  int
	timer   int
	step    int
}

func (p *vrc6Pulse) writeControl(data byte) {
	p.volume = data & vrc6PulseVolumeMask
	p.duty = (data >> vrc6PulseDutyShift) & vrc6PulseDutyMask
	p.mode = (data & vrc6PulseModeMask) != 0
}

func (p *vrc6Pulse) writePeriodLow(data byte) {
	p.period = (p.period & 0x0F00) | int(data)
}

func (p *vrc6Pulse) writePeriodHigh(data byte) {
	p.period = (p.period & 0x00FF) | (int(data&vrc6FreqHighMask) << vrc6FreqHighShift)
	p.enabled = (data & vrc6PulseEnableMask) != 0
	if !p.enabled {
		p.step = 0
	}
}

func (p *vrc6Pulse) stepTimer() {
	if !p.enabled {
		return
	}
	p.timer--
	if p.timer <= 0 {
		p.timer = p.period + 1
		p.step = (p.step - 1) & vrc6PulseStepMask
	}
}

func (p *vrc6Pulse) output() byte {
	if !p.enabled {
		return 0
	}
	if p.mode || p.step <= int(p.duty) {
		return p.volume
	}
	return 0
}

type vrc6Sawtooth struct {
	accumRate byte
	enabled   bool
	period    int
	timer     int
	step      int
	accum     byte
}

func (s *vrc6Sawtooth) writeRate(data byte) {
	s.accumRate = data & vrc6SawtoothRateMask
}

func (s *vrc6Sawtooth) writePeriodLow(data byte) {
	s.period = (s.period & 0x0F00) | int(data)
}

func (s *vrc6Sawtooth) writePeriodHigh(data byte) {
	s.period = (s.period & 0x00FF) | (int(data&vrc6FreqHighMask) << vrc6FreqHighShift)
	s.enabled = (data & vrc6PulseEnableMask) != 0
	if !s.enabled {
		s.step = 0
		s.accum = 0
	}
}

func (s *vrc6Sawtooth) stepTimer() {
	if !s.enabled {
		return
	}
	s.timer--
	if s.timer <= 0 {
		s.timer = s.period + 1
		s.step = (s.step + 1) % vrc6SawtoothSteps
		if s.step == 0 {
			s.accum = 0
			return
		}
		if s.step%2 == 0 {
			s.accum += s.accumRate
		}
	}
}

func (s *vrc6Sawtooth) output() byte {
	if !s.enabled {
		return 0
	}
	return s.accum >> 3
}

// VRC6Audio synthesizes the 3-channel expansion audio of the Konami VRC6 ASIC.
type VRC6Audio struct {
	pulse1   vrc6Pulse
	pulse2   vrc6Pulse
	sawtooth vrc6Sawtooth
	halt     bool
}

// NewVRC6Audio constructs a new VRC6 expansion audio synthesizer.
func NewVRC6Audio() *VRC6Audio {
	return &VRC6Audio{}
}

// WriteRegister handles audio register writes.
func (a *VRC6Audio) WriteRegister(reg uint16, data byte) {
	switch reg {
	case 0x9000:
		a.pulse1.writeControl(data)
	case 0x9001:
		a.pulse1.writePeriodLow(data)
	case 0x9002:
		a.pulse1.writePeriodHigh(data)
	case 0x9003:
		a.halt = (data & vrc6GlobalHaltMask) != 0
	case 0xA000:
		a.pulse2.writeControl(data)
	case 0xA001:
		a.pulse2.writePeriodLow(data)
	case 0xA002:
		a.pulse2.writePeriodHigh(data)
	case 0xB000:
		a.sawtooth.writeRate(data)
	case 0xB001:
		a.sawtooth.writePeriodLow(data)
	case 0xB002:
		a.sawtooth.writePeriodHigh(data)
	}
}

// StepAudio advances the audio synthesis timers by the specified CPU cycles.
func (a *VRC6Audio) StepAudio(cycles int) {
	if a.halt || cycles <= 0 {
		return
	}

	for range cycles {
		a.pulse1.stepTimer()
		a.pulse2.stepTimer()
		a.sawtooth.stepTimer()
	}
}

// AudioOutput returns the combined normalized audio sample of the 3 channels.
func (a *VRC6Audio) AudioOutput() float32 {
	sum := int(a.pulse1.output()) + int(a.pulse2.output()) + int(a.sawtooth.output())
	return float32(sum) * vrc6AudioMixScale
}

// Reset silences all 3 expansion audio channels on console reset.
func (a *VRC6Audio) Reset() {
	a.pulse1 = vrc6Pulse{}
	a.pulse2 = vrc6Pulse{}
	a.sawtooth = vrc6Sawtooth{}
	a.halt = false
}
