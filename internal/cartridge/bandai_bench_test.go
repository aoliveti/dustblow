// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"testing"
)

func BenchmarkBandai_ReadPRG(b *testing.B) {
	cart := createTestCartridgeBandai(16)
	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_ = cart.ReadPRG(0x8000)
	}
}

func BenchmarkBandai_ReadCHR(b *testing.B) {
	cart := createTestCartridgeBandai(16)
	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_ = cart.ReadCHR(0x0400)
	}
}

func BenchmarkBandai_StepCPU(b *testing.B) {
	cart := createTestCartridgeBandai(16)
	cart.WritePRG(0x800B, 0xFF)
	cart.WritePRG(0x800C, 0xFF)
	cart.WritePRG(0x800A, 1) // enable IRQ
	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		cart.StepCPU(1)
	}
}
