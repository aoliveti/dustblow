// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package joypad implements standard NES controller shift register emulation ($4016 and $4017).
package joypad

import "sync/atomic"

// Button represents a bit flag for an individual NES controller button.
type Button byte

const (
	// ButtonA is the A action button.
	ButtonA Button = 1 << iota
	// ButtonB is the B action button.
	ButtonB
	// ButtonSelect is the Select button.
	ButtonSelect
	// ButtonStart is the Start button.
	ButtonStart
	// ButtonUp is the D-Pad Up direction.
	ButtonUp
	// ButtonDown is the D-Pad Down direction.
	ButtonDown
	// ButtonLeft is the D-Pad Left direction.
	ButtonLeft
	// ButtonRight is the D-Pad Right direction.
	ButtonRight
)

// Port represents a physical controller port on the NES console.
type Port byte

const (
	// Port1 is the primary controller port mapped to $4016.
	Port1 Port = iota
	// Port2 is the secondary controller port mapped to $4017.
	Port2
	// PortCount is the total number of standard controller ports (2).
	PortCount = 2
)

const (
	// StrobeMask isolates the strobe latch bit written to register $4016.
	StrobeMask byte = 0x01
	// ButtonReadMask isolates the serial button output bit returned on D0.
	ButtonReadMask byte = 0x01
	// ShiftInPullUpBit is shifted into bit 7 on each read cycle.
	// Original NES hardware uses a 4021 CMOS shift register with its serial input
	// tied to VCC (logic 1); after 8 reads, all subsequent reads yield 1.
	ShiftInPullUpBit byte = 0x80
	// ButtonStorageMask limits atomic button storage to a valid 8-bit unsigned range.
	ButtonStorageMask uint32 = 0xFF
)

// Joypad emulates a standard NES controller with an 8-bit parallel-in serial-out shift register.
type Joypad struct {
	buttons       atomic.Uint32
	shiftRegister byte
	strobe        bool
}

// New creates and initializes a standard NES Joypad.
func New() *Joypad {
	return &Joypad{}
}

// SetButton updates the pressed state of a single button.
// It is safe for concurrent use by the host input system, and safely ignores nil receivers.
func (j *Joypad) SetButton(btn Button, pressed bool) {
	if j == nil {
		return
	}
	for {
		old := j.buttons.Load()
		next := (old &^ uint32(btn)) & ButtonStorageMask
		if pressed {
			next = (old | uint32(btn)) & ButtonStorageMask
		}
		if j.buttons.CompareAndSwap(old, next) {
			break
		}
	}
}

// SetButtons atomically sets the entire 8-button bitmask.
// Automatically sanitizes physically impossible opposing D-Pad directions (Left+Right or Up+Down).
// It is safe for concurrent use and safely ignores nil receivers.
func (j *Joypad) SetButtons(mask byte) {
	if j == nil {
		return
	}
	sanitized := SanitizeOpposingDirections(mask)
	j.buttons.Store(uint32(sanitized) & ButtonStorageMask)
	if j.strobe {
		j.shiftRegister = sanitized
	}
}

// Buttons returns the current active button mask, or 0 if nil.
func (j *Joypad) Buttons() byte {
	if j == nil {
		return 0
	}
	return byte(j.buttons.Load() & ButtonStorageMask)
}

// Write handles the strobe latch signal written by the CPU to $4016.
// When strobe is high (1), the shift register continuously latches the current button states.
// When strobe transitions to low (0), the latched states are frozen and ready for serial reading.
func (j *Joypad) Write(data byte) {
	if j == nil {
		return
	}
	newStrobe := (data & StrobeMask) != 0
	if j.strobe || newStrobe {
		j.shiftRegister = byte(j.buttons.Load() & ButtonStorageMask)
	}
	j.strobe = newStrobe
}

// Read returns the next serial button bit (0 = released, 1 = pressed).
// After 8 button states have been read, subsequent reads continuously return 1 due to the pull-up resistor.
func (j *Joypad) Read() byte {
	if j == nil {
		return ShiftInPullUpBit & ButtonReadMask
	}
	if j.strobe {
		return byte(j.buttons.Load()&ButtonStorageMask) & ButtonReadMask
	}

	val := j.shiftRegister & ButtonReadMask
	j.shiftRegister = (j.shiftRegister >> 1) | ShiftInPullUpBit
	return val
}

// Peek inspects the current button bit without advancing the shift register.
// Useful for tracers, disassemblers, and debugging tools.
func (j *Joypad) Peek() byte {
	if j == nil {
		return ShiftInPullUpBit & ButtonReadMask
	}
	if j.strobe {
		return byte(j.buttons.Load()&ButtonStorageMask) & ButtonReadMask
	}
	return j.shiftRegister & ButtonReadMask
}

// SanitizeOpposingDirections removes physically impossible simultaneous D-pad inputs
// (Left+Right and Up+Down) that could cause glitchy behavior or crashes in original NES games.
func SanitizeOpposingDirections(mask byte) byte {
	if (mask&byte(ButtonLeft)) != 0 && (mask&byte(ButtonRight)) != 0 {
		mask &^= byte(ButtonRight)
	}
	if (mask&byte(ButtonUp)) != 0 && (mask&byte(ButtonDown)) != 0 {
		mask &^= byte(ButtonDown)
	}
	return mask
}
