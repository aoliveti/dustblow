// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"testing"

	"github.com/aoliveti/dustblow/internal/cartridge"
)

func BenchmarkMMC5AudioStep(b *testing.B) {
	audio := cartridge.NewMMC5Audio()
	audio.WriteRegister(0x5015, 0x03)
	audio.WriteRegister(0x5000, 0xBF)
	audio.WriteRegister(0x5002, 0x20)
	audio.WriteRegister(0x5003, 0x08)
	audio.WriteRegister(0x5004, 0xBF)
	audio.WriteRegister(0x5006, 0x30)
	audio.WriteRegister(0x5007, 0x08)

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		audio.StepAudio(2)
		_ = audio.AudioOutput()
	}
}
