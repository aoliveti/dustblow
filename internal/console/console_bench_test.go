// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package console_test

import (
	"testing"

	"github.com/aoliveti/dustblow/internal/console"
	"github.com/aoliveti/dustblow/internal/joypad"
)

func BenchmarkConsole_Step(b *testing.B) {
	cart := newTestCartridge()
	c := console.New(cart)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = c.Step()
	}
}

func BenchmarkConsole_StepFrame(b *testing.B) {
	cart := newTestCartridge()
	c := console.New(cart)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = c.StepFrame()
	}
}

func BenchmarkConsole_SetButtons(b *testing.B) {
	cart := newTestCartridge()
	c := console.New(cart)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		c.SetButtons(joypad.Port1, byte(joypad.ButtonA|joypad.ButtonStart))
	}
}
