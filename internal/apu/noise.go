// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package apu

// noisePeriodTable stores the 16 NTSC timer divider periods for the noise channel.
var noisePeriodTable = [16]uint16{
	4, 8, 16, 32, 64, 96, 128, 160, 202, 254, 380, 508, 762, 1016, 2034, 4068,
}

const (
	lfsrLongModeTap  = 1
	lfsrShortModeTap = 6
	lfsrFeedbackBit  = 14

	noiseLengthHaltMask  byte = 0x20
	noiseConstVolumeMask byte = 0x10
	noiseVolumeMask      byte = 0x0F
	noiseModeFlagMask    byte = 0x80
	noisePeriodIndexMask byte = 0x0F
	noiseLengthIndexMask byte = 0x1F
)

// Noise generates pseudo-random white noise and metallic percussion.
type Noise struct {
	enabled       bool
	lengthHalt    bool
	modeFlag      bool
	periodIndex   byte
	timerDivider  uint16
	lengthCounter byte
	shiftRegister uint16
	envelope      Envelope
}

// NewNoise creates an initialized Noise with LFSR seeded to 1.
func NewNoise() *Noise {
	return &Noise{
		shiftRegister: 1,
	}
}

// WriteControl handles writes to $400C.
func (n *Noise) WriteControl(val byte) {
	n.lengthHalt = (val & noiseLengthHaltMask) != 0
	n.envelope.loopFlag = n.lengthHalt
	n.envelope.constVolume = (val & noiseConstVolumeMask) != 0
	n.envelope.volume = val & noiseVolumeMask
}

// WriteModeAndPeriod handles writes to $400E.
func (n *Noise) WriteModeAndPeriod(val byte) {
	n.modeFlag = (val & noiseModeFlagMask) != 0
	n.periodIndex = val & noisePeriodIndexMask
}

// WriteLength handles writes to $400F.
func (n *Noise) WriteLength(val byte) {
	if n.enabled {
		n.lengthCounter = lengthTable[(val>>3)&noiseLengthIndexMask]
	}
	n.envelope.startFlag = true
}

// StepTimer is clocked every APU cycle (every 2 CPU cycles).
func (n *Noise) StepTimer() {
	if n.timerDivider == 0 {
		n.timerDivider = noisePeriodTable[n.periodIndex]

		tap := lfsrLongModeTap
		if n.modeFlag {
			tap = lfsrShortModeTap
		}

		feedback := (n.shiftRegister & 0x01) ^ ((n.shiftRegister >> tap) & 0x01)
		n.shiftRegister = (n.shiftRegister >> 1) | (feedback << lfsrFeedbackBit)
		return
	}
	n.timerDivider--
}

// ClockQuarterFrame clocks the envelope.
func (n *Noise) ClockQuarterFrame() {
	n.envelope.Clock()
}

// ClockHalfFrame clocks the length counter.
func (n *Noise) ClockHalfFrame() {
	if !n.lengthHalt && n.lengthCounter > 0 {
		n.lengthCounter--
	}
}

// SetEnabled enables or disables the channel from $4015 write.
func (n *Noise) SetEnabled(enabled bool) {
	n.enabled = enabled
	if !enabled {
		n.lengthCounter = 0
	}
}

// Output returns the current sample amplitude (0 to 15) to send to the mixer.
func (n *Noise) Output() byte {
	if !n.enabled || n.lengthCounter == 0 || (n.shiftRegister&0x01) != 0 {
		return 0
	}
	return n.envelope.Output()
}

// Reset silences the noise channel, clears length counters, and reseeds the LFSR.
func (n *Noise) Reset() {
	n.enabled = false
	n.lengthHalt = false
	n.modeFlag = false
	n.periodIndex = 0
	n.timerDivider = 0
	n.lengthCounter = 0
	n.shiftRegister = 1
	n.envelope = Envelope{}
}
