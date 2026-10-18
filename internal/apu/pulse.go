// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package apu

// lengthTable holds the standard duration values for APU length counters.
var lengthTable = [32]byte{
	10, 254, 20, 2, 40, 4, 80, 6, 160, 8, 60, 10, 14, 12, 26, 14,
	12, 16, 24, 18, 48, 20, 96, 22, 192, 24, 72, 26, 16, 28, 32, 30,
}

// dutyTable stores the 8-step waveform outputs for the 4 duty cycles.
var dutyTable = [4][8]byte{
	{0, 1, 0, 0, 0, 0, 0, 0}, // 12.5%
	{0, 1, 1, 0, 0, 0, 0, 0}, // 25.0%
	{0, 1, 1, 1, 1, 0, 0, 0}, // 50.0%
	{1, 0, 0, 1, 1, 1, 1, 1}, // 25.0% negated
}

const (
	minPulseTimerPeriod uint16 = 8
	maxPulseTimerPeriod uint16 = 0x07FF

	envelopeMaxDecay byte = 15

	pulseDutyMask        byte = 0x03
	pulseLengthHaltMask  byte = 0x20
	pulseConstVolumeMask byte = 0x10
	pulseVolumeMask      byte = 0x0F

	sweepEnableMask byte = 0x80
	sweepPeriodMask byte = 0x07
	sweepNegateMask byte = 0x08
	sweepShiftMask  byte = 0x07

	pulseTimerHighMask   byte = 0x07
	pulseLengthIndexMask byte = 0x1F
	pulseStepMask        byte = 0x07
)

// Envelope generates decaying or constant volume levels.
type Envelope struct {
	startFlag   bool
	loopFlag    bool
	constVolume bool
	volume      byte
	divider     byte
	decayLevel  byte
}

// Clock advances the envelope generator by one quarter-frame tick.
func (e *Envelope) Clock() {
	if e.startFlag {
		e.startFlag = false
		e.decayLevel = envelopeMaxDecay
		e.divider = e.volume
		return
	}

	if e.divider > 0 {
		e.divider--
		return
	}

	e.divider = e.volume
	if e.decayLevel > 0 {
		e.decayLevel--
		return
	}
	if e.loopFlag {
		e.decayLevel = envelopeMaxDecay
	}
}

// Output returns the current volume level (0 to 15).
func (e *Envelope) Output() byte {
	if e.constVolume {
		return e.volume
	}
	return e.decayLevel
}

// Sweep modulates the pulse channel's frequency up or down.
type Sweep struct {
	isChannel2 bool
	enabled    bool
	negate     bool
	reloadFlag bool
	shift      byte
	period     byte
	divider    byte
}

// TargetPeriod calculates the candidate timer period produced by the shifter.
func (s *Sweep) TargetPeriod(currentPeriod uint16) uint16 {
	change := currentPeriod >> s.shift
	if !s.negate {
		return currentPeriod + change
	}
	if s.isChannel2 {
		// Pulse 2 uses two's complement (-change)
		if change > currentPeriod {
			return 0
		}
		return currentPeriod - change
	}
	// Pulse 1 uses one's complement (-change - 1)
	if change+1 > currentPeriod {
		return 0
	}
	return currentPeriod - change - 1
}

// IsMuting returns true if the pulse channel must be silenced due to frequency bounds.
// Note: According to Nesdev, muting applies continuously even when sweep is disabled.
func (s *Sweep) IsMuting(currentPeriod uint16) bool {
	if currentPeriod < minPulseTimerPeriod {
		return true
	}
	return s.TargetPeriod(currentPeriod) > maxPulseTimerPeriod
}

// Clock updates the target period on half-frame ticks.
func (s *Sweep) Clock(currentPeriod *uint16) {
	if s.divider == 0 && s.enabled && s.shift > 0 && !s.IsMuting(*currentPeriod) {
		*currentPeriod = s.TargetPeriod(*currentPeriod)
	}

	if s.divider == 0 || s.reloadFlag {
		s.divider = s.period
		s.reloadFlag = false
		return
	}
	s.divider--
}

// Pulse implements the Pulse 1 and Pulse 2 sound generators.
type Pulse struct {
	isChannel2    bool
	enabled       bool
	lengthHalt    bool
	duty          byte
	step          byte
	rawPeriod     uint16
	timerDivider  uint16
	lengthCounter byte
	envelope      Envelope
	sweep         Sweep
}

// NewPulse creates a new Pulse configured for Pulse 1 (false) or Pulse 2 (true).
func NewPulse(isChannel2 bool) *Pulse {
	return &Pulse{
		isChannel2: isChannel2,
		sweep: Sweep{
			isChannel2: isChannel2,
		},
	}
}

// WriteControl handles writes to $4000 (Pulse 1) or $4004 (Pulse 2).
func (p *Pulse) WriteControl(val byte) {
	p.duty = (val >> 6) & pulseDutyMask
	p.lengthHalt = (val & pulseLengthHaltMask) != 0
	p.envelope.loopFlag = p.lengthHalt
	p.envelope.constVolume = (val & pulseConstVolumeMask) != 0
	p.envelope.volume = val & pulseVolumeMask
}

// WriteSweep handles writes to $4001 (Pulse 1) or $4005 (Pulse 2).
func (p *Pulse) WriteSweep(val byte) {
	p.sweep.enabled = (val & sweepEnableMask) != 0
	p.sweep.period = (val >> 4) & sweepPeriodMask
	p.sweep.negate = (val & sweepNegateMask) != 0
	p.sweep.shift = val & sweepShiftMask
	p.sweep.reloadFlag = true
}

// WriteTimerLow handles writes to $4002 (Pulse 1) or $4006 (Pulse 2).
func (p *Pulse) WriteTimerLow(val byte) {
	p.rawPeriod = (p.rawPeriod & 0x0700) | uint16(val)
}

// WriteTimerHigh handles writes to $4003 (Pulse 1) or $4007 (Pulse 2).
func (p *Pulse) WriteTimerHigh(val byte) {
	p.rawPeriod = (p.rawPeriod & 0x00FF) | (uint16(val&pulseTimerHighMask) << 8)
	if p.enabled {
		p.lengthCounter = lengthTable[(val>>3)&pulseLengthIndexMask]
	}
	p.step = 0
	p.envelope.startFlag = true
}

// StepTimer is clocked every APU cycle (every 2 CPU cycles).
func (p *Pulse) StepTimer() {
	if p.timerDivider == 0 {
		p.timerDivider = p.rawPeriod
		p.step = (p.step + 1) & pulseStepMask
		return
	}
	p.timerDivider--
}

// ClockQuarterFrame clocks the envelope.
func (p *Pulse) ClockQuarterFrame() {
	p.envelope.Clock()
}

// ClockHalfFrame clocks length counter and sweep unit.
func (p *Pulse) ClockHalfFrame() {
	if !p.lengthHalt && p.lengthCounter > 0 {
		p.lengthCounter--
	}
	p.sweep.Clock(&p.rawPeriod)
}

// SetEnabled enables or disables the channel from $4015 write.
func (p *Pulse) SetEnabled(enabled bool) {
	p.enabled = enabled
	if !enabled {
		p.lengthCounter = 0
	}
}

// Output returns the current sample amplitude (0 to 15) to send to the mixer.
func (p *Pulse) Output() byte {
	if !p.enabled || p.lengthCounter == 0 {
		return 0
	}
	if p.sweep.IsMuting(p.rawPeriod) {
		return 0
	}
	if dutyTable[p.duty][p.step] == 0 {
		return 0
	}
	return p.envelope.Output()
}

// Reset silences the pulse channel and resets internal counters and sweep state.
func (p *Pulse) Reset() {
	p.enabled = false
	p.lengthHalt = false
	p.duty = 0
	p.step = 0
	p.rawPeriod = 0
	p.timerDivider = 0
	p.lengthCounter = 0
	p.envelope = Envelope{}
	p.sweep.enabled = false
	p.sweep.shift = 0
	p.sweep.period = 0
	p.sweep.negate = false
	p.sweep.divider = 0
	p.sweep.reloadFlag = false
}
