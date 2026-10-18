// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"testing"
)

func BenchmarkMapper19_ReadPRG(b *testing.B) {
	cart := createTestCartridgeMapper19()
	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_ = cart.ReadPRG(0x8000)
	}
}

func BenchmarkMapper19_ReadCHR(b *testing.B) {
	cart := createTestCartridgeMapper19()
	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_ = cart.ReadCHR(0x0400)
	}
}

func BenchmarkMapper19_StepCPU(b *testing.B) {
	cart := createTestCartridgeMapper19()
	cart.WritePRG(0x5800, 0x80) // enable IRQ
	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		cart.StepCPU(1)
	}
}

func BenchmarkMapper19_StepAudio(b *testing.B) {
	cart := createTestCartridgeMapper19()
	audio := cart.ExpansionAudio()
	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		audio.StepAudio(15)
	}
}
