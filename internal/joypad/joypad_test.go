// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package joypad_test

import (
	"sync"
	"testing"

	"github.com/dustblow/dustblow/internal/joypad"
	"github.com/stretchr/testify/assert"
)

const (
	strobeEnablePulse  byte = 0x01
	strobeDisablePulse byte = 0x00

	outputButtonPressed  byte = 0x01
	outputButtonReleased byte = 0x00
	outputOpenBusPullUp  byte = 0x01

	totalStandardButtons int = 8
	concurrentTestCycles int = 500
)

func strobeJoypad(j *joypad.Joypad) {
	j.Write(strobeEnablePulse)
	j.Write(strobeDisablePulse)
}

func TestJoypad_SerialReadingSequence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		activeButtons byte
		expectedBits  [totalStandardButtons]byte
	}{
		{
			name:          "No buttons pressed",
			activeButtons: 0x00,
			expectedBits: [totalStandardButtons]byte{
				outputButtonReleased, outputButtonReleased, outputButtonReleased, outputButtonReleased,
				outputButtonReleased, outputButtonReleased, outputButtonReleased, outputButtonReleased,
			},
		},
		{
			name:          "Button A only",
			activeButtons: byte(joypad.ButtonA),
			expectedBits: [totalStandardButtons]byte{
				outputButtonPressed, outputButtonReleased, outputButtonReleased, outputButtonReleased,
				outputButtonReleased, outputButtonReleased, outputButtonReleased, outputButtonReleased,
			},
		},
		{
			name:          "Button B and Start pressed",
			activeButtons: byte(joypad.ButtonB | joypad.ButtonStart),
			expectedBits: [totalStandardButtons]byte{
				outputButtonReleased, outputButtonPressed, outputButtonReleased, outputButtonPressed,
				outputButtonReleased, outputButtonReleased, outputButtonReleased, outputButtonReleased,
			},
		},
		{
			name:          "All buttons pressed except opposing directions",
			activeButtons: byte(joypad.ButtonA | joypad.ButtonB | joypad.ButtonSelect | joypad.ButtonStart | joypad.ButtonUp | joypad.ButtonLeft),
			expectedBits: [totalStandardButtons]byte{
				outputButtonPressed, outputButtonPressed, outputButtonPressed, outputButtonPressed,
				outputButtonPressed, outputButtonReleased, outputButtonPressed, outputButtonReleased,
			},
		},
		{
			name:          "Slide combo: Button B, Down, and Right",
			activeButtons: byte(joypad.ButtonB | joypad.ButtonDown | joypad.ButtonRight),
			expectedBits: [totalStandardButtons]byte{
				outputButtonReleased, outputButtonPressed, outputButtonReleased, outputButtonReleased,
				outputButtonReleased, outputButtonPressed, outputButtonReleased, outputButtonPressed,
			},
		},
		{
			name:          "Run-jump combo: Button A, Button B, and Right",
			activeButtons: byte(joypad.ButtonA | joypad.ButtonB | joypad.ButtonRight),
			expectedBits: [totalStandardButtons]byte{
				outputButtonPressed, outputButtonPressed, outputButtonReleased, outputButtonReleased,
				outputButtonReleased, outputButtonReleased, outputButtonReleased, outputButtonPressed,
			},
		},
		{
			name:          "Duck run-jump combo: Button A, Button B, Down, and Right",
			activeButtons: byte(joypad.ButtonA | joypad.ButtonB | joypad.ButtonDown | joypad.ButtonRight),
			expectedBits: [totalStandardButtons]byte{
				outputButtonPressed, outputButtonPressed, outputButtonReleased, outputButtonReleased,
				outputButtonReleased, outputButtonPressed, outputButtonReleased, outputButtonPressed,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			j := joypad.New()
			j.SetButtons(tt.activeButtons)
			strobeJoypad(j)

			for i := range totalStandardButtons {
				actual := j.Read()
				assert.Equal(t, tt.expectedBits[i], actual)
			}

			// Reads beyond the 8 standard buttons must return 1 (pull-up resistor behavior)
			for range 4 {
				assert.Equal(t, outputOpenBusPullUp, j.Read())
			}
		})
	}
}

func TestJoypad_ContinuousStrobe(t *testing.T) {
	t.Parallel()

	j := joypad.New()
	j.Write(strobeEnablePulse) // Strobe held high

	j.SetButton(joypad.ButtonA, true)
	assert.Equal(t, outputButtonPressed, j.Peek())
	assert.Equal(t, outputButtonPressed, j.Read())
	assert.Equal(t, outputButtonPressed, j.Read()) // Does not advance while strobe is high

	j.SetButton(joypad.ButtonA, false)
	assert.Equal(t, outputButtonReleased, j.Peek())
	assert.Equal(t, outputButtonReleased, j.Read())
}

func TestJoypad_PeekDoesNotAdvanceShiftRegister(t *testing.T) {
	t.Parallel()

	j := joypad.New()
	j.SetButton(joypad.ButtonA, true)
	strobeJoypad(j)

	assert.Equal(t, outputButtonPressed, j.Peek())
	assert.Equal(t, outputButtonPressed, j.Peek())
	assert.Equal(t, outputButtonPressed, j.Read())
	assert.Equal(t, outputButtonReleased, j.Peek())
}

func TestJoypad_ButtonsMethod(t *testing.T) {
	t.Parallel()

	j := joypad.New()
	j.SetButton(joypad.ButtonA, true)
	j.SetButton(joypad.ButtonStart, true)

	assert.Equal(t, byte(joypad.ButtonA|joypad.ButtonStart), j.Buttons())
}

func TestJoypad_SanitizeOpposingDirections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    byte
		expected byte
	}{
		{
			name:     "Left and Right pressed cancels Right",
			input:    byte(joypad.ButtonLeft | joypad.ButtonRight),
			expected: byte(joypad.ButtonLeft),
		},
		{
			name:     "Up and Down pressed cancels Down",
			input:    byte(joypad.ButtonUp | joypad.ButtonDown),
			expected: byte(joypad.ButtonUp),
		},
		{
			name:     "Diagonal Up-Right is preserved",
			input:    byte(joypad.ButtonUp | joypad.ButtonRight),
			expected: byte(joypad.ButtonUp | joypad.ButtonRight),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, joypad.SanitizeOpposingDirections(tt.input))
		})
	}
}

func TestJoypad_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	j := joypad.New()
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := range concurrentTestCycles {
			j.SetButton(joypad.ButtonA, i%2 == 0)
			j.SetButton(joypad.ButtonB, i%2 != 0)
		}
	}()

	go func() {
		defer wg.Done()
		for range concurrentTestCycles {
			strobeJoypad(j)
			for range totalStandardButtons {
				_ = j.Read()
			}
		}
	}()

	wg.Wait()
}

func TestJoypad_NilReceiverSafety(t *testing.T) {
	t.Parallel()

	var j *joypad.Joypad
	j.SetButton(joypad.ButtonA, true)
	j.SetButtons(0xFF)
	assert.Zero(t, j.Buttons())
	j.Write(0x01)
	assert.Equal(t, outputButtonReleased, j.Read())
	assert.Equal(t, outputButtonReleased, j.Peek())
}

func TestJoypad_PortConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, joypad.Port1, joypad.Port(0))
	assert.Equal(t, joypad.Port2, joypad.Port(1))
	assert.Equal(t, 2, joypad.PortCount)
}
