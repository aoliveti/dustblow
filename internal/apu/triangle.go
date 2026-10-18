// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package apu

// triangleTable holds the 32-step DAC quantized waveform values.
var triangleTable = [32]byte{
	15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0,
	0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15,
}

const (
	triangleControlFlagMask      byte   = 0x80
	triangleReloadValueMask      byte   = 0x7F
	triangleTimerHighMask        byte   = 0x07
	triangleLengthIndexMask      byte   = 0x1F
	triangleStepMask             byte   = 0x1F
	triangleUltrasonicMutePeriod uint16 = 2
	triangleNeutralDCOffset      byte   = 7 // Mid-scale DAC bias (0-15) for ultrasonic silencing without clicks
)

// Triangle generates a pseudo-triangle waveform.
type Triangle struct {
	enabled       bool
	controlFlag   bool
	reloadFlag    bool
	reloadValue   byte
	linearCounter byte
	lengthCounter byte
	rawPeriod     uint16
	timerDivider  uint16
	step          byte
}

// NewTriangle creates an initialized Triangle.
func NewTriangle() *Triangle {
	return &Triangle{}
}

// WriteControl handles writes to $4008.
func (t *Triangle) WriteControl(val byte) {
	t.controlFlag = (val & triangleControlFlagMask) != 0
	t.reloadValue = val & triangleReloadValueMask
}

// WriteTimerLow handles writes to $400A.
func (t *Triangle) WriteTimerLow(val byte) {
	t.rawPeriod = (t.rawPeriod & 0x0700) | uint16(val)
}

// WriteTimerHigh handles writes to $400B.
func (t *Triangle) WriteTimerHigh(val byte) {
	t.rawPeriod = (t.rawPeriod & 0x00FF) | (uint16(val&triangleTimerHighMask) << 8)
	if t.enabled {
		t.lengthCounter = lengthTable[(val>>3)&triangleLengthIndexMask]
	}
	t.reloadFlag = true
}

// StepTimer is clocked on EVERY CPU cycle (1:1 with CPU clock).
func (t *Triangle) StepTimer() {
	if t.timerDivider == 0 {
		t.timerDivider = t.rawPeriod
		// The sequencer advances only when both linear counter and length counter are non-zero,
		// and frequency is below ultrasonic range (period >= 2) to eliminate popping artifacts.
		if t.linearCounter > 0 && t.lengthCounter > 0 && t.rawPeriod >= triangleUltrasonicMutePeriod {
			t.step = (t.step + 1) & triangleStepMask
		}
		return
	}
	t.timerDivider--
}

// ClockQuarterFrame updates the linear counter.
func (t *Triangle) ClockQuarterFrame() {
	if t.reloadFlag {
		t.linearCounter = t.reloadValue
		if !t.controlFlag {
			t.reloadFlag = false
		}
		return
	}

	if t.linearCounter > 0 {
		t.linearCounter--
	}
	if !t.controlFlag {
		t.reloadFlag = false
	}
}

// ClockHalfFrame updates the length counter.
func (t *Triangle) ClockHalfFrame() {
	if !t.controlFlag && t.lengthCounter > 0 {
		t.lengthCounter--
	}
}

// SetEnabled enables or disables the channel from $4015 write.
func (t *Triangle) SetEnabled(enabled bool) {
	t.enabled = enabled
	if !enabled {
		t.lengthCounter = 0
	}
}

// Output returns the current sample amplitude (0 to 15) to send to the mixer.
func (t *Triangle) Output() byte {
	if !t.enabled || t.lengthCounter == 0 || t.linearCounter == 0 {
		return 0
	}
	if t.rawPeriod < triangleUltrasonicMutePeriod {
		return triangleNeutralDCOffset
	}
	return triangleTable[t.step]
}

// Reset silences the triangle channel and clears linear counter and reload states.
func (t *Triangle) Reset() {
	t.enabled = false
	t.controlFlag = false
	t.reloadFlag = false
	t.reloadValue = 0
	t.linearCounter = 0
	t.lengthCounter = 0
	t.rawPeriod = 0
	t.timerDivider = 0
	t.step = 0
}
