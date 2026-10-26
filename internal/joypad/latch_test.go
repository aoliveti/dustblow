// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package joypad_test

import (
	"testing"

	"github.com/aoliveti/dustblow/internal/joypad"
)

func TestLatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sequence  []byte
		expected  []byte
		checkName string
	}{
		{
			name: "SingleFrameTapHoldsForTwoFrames",
			sequence: []byte{
				byte(joypad.ButtonA), // Frame 0: pressed
				0,                    // Frame 1: released physically (or USB dropped)
				0,                    // Frame 2: still held by latch
				0,                    // Frame 3: latch expired
			},
			expected: []byte{
				byte(joypad.ButtonA), // Frame 0: active
				byte(joypad.ButtonA), // Frame 1: held active
				byte(joypad.ButtonA), // Frame 2: held active
				0,                    // Frame 3: released
			},
			checkName: "Button A retention",
		},
		{
			name: "DirectionalButtonsPassThroughWithoutLatch",
			sequence: []byte{
				byte(joypad.ButtonRight), // Frame 0: right
				byte(joypad.ButtonLeft),  // Frame 1: immediate switch to left
				0,                        // Frame 2: neutral
			},
			expected: []byte{
				byte(joypad.ButtonRight), // Frame 0: right
				byte(joypad.ButtonLeft),  // Frame 1: left (no lingering right)
				0,                        // Frame 2: neutral
			},
			checkName: "D-Pad immediate response",
		},
		{
			name: "HoldingButtonKeepsItActive",
			sequence: []byte{
				byte(joypad.ButtonB),
				byte(joypad.ButtonB),
				byte(joypad.ButtonB),
				0,
				0,
				0,
			},
			expected: []byte{
				byte(joypad.ButtonB),
				byte(joypad.ButtonB),
				byte(joypad.ButtonB),
				byte(joypad.ButtonB),
				byte(joypad.ButtonB),
				0,
			},
			checkName: "Continuous hold plus 2-frame extension",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var latch joypad.Latch
			for frame, input := range tt.sequence {
				got := latch.Process(input)
				want := tt.expected[frame]
				if got != want {
					t.Fatalf("frame %d: got 0x%02X, want 0x%02X (%s)", frame, got, want, tt.checkName)
				}
			}
		})
	}
}
