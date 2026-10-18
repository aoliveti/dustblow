// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package joypad

// Action button mask for latching (A, B, Select, Start).
const (
	actionButtonMask = byte(ButtonA | ButtonB | ButtonSelect | ButtonStart)
	inputHoldFrames  = 2
)

// Latch maintains a minimum hold duration for action button taps
// to prevent single-frame USB packet drops or driver polling jitter from losing inputs.
type Latch struct {
	holdCounters [4]uint8 // A, B, Select, Start
}

// Process updates the hold counters with the newly polled raw mask and returns the stabilized button mask.
func (l *Latch) Process(rawMask byte) byte {
	stabilized := rawMask &^ actionButtonMask

	actionButtons := [...]Button{
		ButtonA,
		ButtonB,
		ButtonSelect,
		ButtonStart,
	}

	for i, btn := range actionButtons {
		btnMask := byte(btn)
		if (rawMask & btnMask) != 0 {
			l.holdCounters[i] = inputHoldFrames
			stabilized |= btnMask
			continue
		}

		if l.holdCounters[i] > 0 {
			l.holdCounters[i]--
			stabilized |= btnMask
		}
	}

	return stabilized
}
