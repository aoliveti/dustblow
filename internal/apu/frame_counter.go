// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package apu

const (
	cycleStep1         = 7457
	cycleStep2         = 14913
	cycleStep3         = 22371
	cycleIRQAssert4Step = 29828
	cycleStep4         = 29829
	cycleReset4Step     = 29830
	cycleStep5         = 37281
	cycleReset5Step     = 37282

	frameCounterMode5StepMask  byte = 0x80
	frameCounterIRQInhibitMask byte = 0x40

	resetDelayPutCycle    = 4
	resetDelayGetCycle    = 5
	clearIRQDelayPutCycle = 1
	clearIRQDelayGetCycle = 2
)

// FrameCounter generates low-frequency clocks (240 Hz / 192 Hz) and frame interrupts.
type FrameCounter struct {
	mode5Step        bool
	pendingMode5Step bool
	pendingReset     bool
	irqInhibit       bool
	irqFlag          bool
	irqLine          bool
	resetDelay       int
	clearIRQDelay    int
	cycleCount       uint32
	onQuarter        func()
	onHalf           func()
}

// NewFrameCounter creates a FrameCounter connected to quarter and half-frame callbacks.
func NewFrameCounter(onQuarter, onHalf func()) *FrameCounter {
	return &FrameCounter{
		onQuarter: onQuarter,
		onHalf:    onHalf,
	}
}

// WriteControl handles writes to $4017 taking into account APU cycle parity.
func (fc *FrameCounter) WriteControl(val byte, isGetCycle bool) {
	fc.pendingMode5Step = (val & frameCounterMode5StepMask) != 0
	fc.irqInhibit = (val & frameCounterIRQInhibitMask) != 0
	if fc.irqInhibit {
		fc.irqFlag = false
		fc.irqLine = false
		fc.clearIRQDelay = 0
	}

	// Writing bit 7 immediately clocks quarter and half frame units
	if fc.pendingMode5Step {
		fc.triggerHalf()
	}

	fc.pendingReset = true
	fc.resetDelay = resetDelayPutCycle
	if isGetCycle {
		fc.resetDelay = resetDelayGetCycle
	}
}

// ReadStatus returns the current frame IRQ flag state and schedules flag clearing on the next get cycle.
func (fc *FrameCounter) ReadStatus(isGetCycle bool) bool {
	flag := fc.irqFlag
	fc.clearIRQDelay = clearIRQDelayPutCycle
	if isGetCycle {
		fc.clearIRQDelay = clearIRQDelayGetCycle
	}
	return flag
}

func (fc *FrameCounter) triggerQuarter() {
	if fc.onQuarter != nil {
		fc.onQuarter()
	}
}

func (fc *FrameCounter) triggerHalf() {
	fc.triggerQuarter()
	if fc.onHalf != nil {
		fc.onHalf()
	}
}

func (fc *FrameCounter) step4Step() {
	switch fc.cycleCount {
	case cycleStep1, cycleStep3:
		fc.triggerQuarter()
	case cycleStep2:
		fc.triggerHalf()
	case cycleIRQAssert4Step:
		fc.irqFlag = true
	case cycleStep4:
		fc.triggerHalf()
		fc.irqFlag = true
	case cycleReset4Step:
		fc.irqFlag = !fc.irqInhibit
		fc.irqLine = !fc.irqInhibit
		fc.cycleCount = 0
	}
}

func (fc *FrameCounter) step5Step() {
	switch fc.cycleCount {
	case cycleStep1, cycleStep3:
		fc.triggerQuarter()
	case cycleStep2, cycleStep5:
		fc.triggerHalf()
	case cycleReset5Step:
		fc.cycleCount = 0
	}
}

// Step advances the frame counter by cpuCycles.
func (fc *FrameCounter) Step(cpuCycles uint32) {
	for range cpuCycles {
		if fc.clearIRQDelay > 0 {
			fc.clearIRQDelay--
			if fc.clearIRQDelay == 0 {
				fc.irqFlag = false
				fc.irqLine = false
			}
		}

		if fc.pendingReset {
			fc.resetDelay--
			if fc.resetDelay == 0 {
				fc.pendingReset = false
				fc.mode5Step = fc.pendingMode5Step
				fc.cycleCount = 0
			}
		}

		fc.cycleCount++
		if fc.mode5Step {
			fc.step5Step()
			continue
		}
		fc.step4Step()
	}
}

// ClearIRQ clears the frame interrupt flag immediately.
func (fc *FrameCounter) ClearIRQ() {
	fc.irqFlag = false
	fc.irqLine = false
	fc.clearIRQDelay = 0
}

// CycleCount returns the current cycle count of the frame counter.
func (fc *FrameCounter) CycleCount() uint32 {
	return fc.cycleCount
}

// IRQPending returns true if a frame interrupt is asserted to the CPU.
func (fc *FrameCounter) IRQPending() bool {
	return !fc.irqInhibit && fc.irqLine
}

// Reset restores the frame counter to initial power-on/reset state.
func (fc *FrameCounter) Reset() {
	fc.mode5Step = false
	fc.pendingMode5Step = false
	fc.pendingReset = false
	fc.irqInhibit = false
	fc.irqFlag = false
	fc.irqLine = false
	fc.resetDelay = 0
	fc.clearIRQDelay = 0
	fc.cycleCount = 0
}
