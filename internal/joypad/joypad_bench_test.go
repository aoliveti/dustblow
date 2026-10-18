// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package joypad_test

import (
	"testing"

	"github.com/dustblow/dustblow/internal/joypad"
)

func BenchmarkJoypad_Read(b *testing.B) {
	j := joypad.New()
	j.SetButton(joypad.ButtonA, true)
	j.Write(strobeEnablePulse)
	j.Write(strobeDisablePulse)

	b.ResetTimer()
	b.ReportAllocs()

	var dummy byte
	for b.Loop() {
		dummy += j.Read()
	}
	_ = dummy
}

func BenchmarkJoypad_Write(b *testing.B) {
	j := joypad.New()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		j.Write(strobeEnablePulse)
		j.Write(strobeDisablePulse)
	}
}

func BenchmarkJoypad_SetButton(b *testing.B) {
	j := joypad.New()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		j.SetButton(joypad.ButtonA, true)
		j.SetButton(joypad.ButtonA, false)
	}
}
