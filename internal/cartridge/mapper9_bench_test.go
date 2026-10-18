// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"testing"
)

func BenchmarkMapper9ReadPRG(b *testing.B) {
	_, mapper := createTestMMC2Cartridge()

	b.ResetTimer()
	b.ReportAllocs()

	var dummy byte
	for b.Loop() {
		dummy += mapper.ReadPRG(addressPRGSwitchable)
		dummy += mapper.ReadPRG(addressPRGFixedPenultimateMinus1)
		dummy += mapper.ReadPRG(addressPRGFixedPenultimate)
		dummy += mapper.ReadPRG(addressPRGFixedLast)
	}
	_ = dummy
}

func BenchmarkMapper9WritePRG(b *testing.B) {
	_, mapper := createTestMMC2Cartridge()

	b.ResetTimer()
	b.ReportAllocs()

	val := byte(0)
	for b.Loop() {
		mapper.WritePRG(addressRegPRGBank, val)
		val++
	}
}

func BenchmarkMapper9ReadCHRLatchSwitching(b *testing.B) {
	_, mapper := createTestMMC2Cartridge()

	b.ResetTimer()
	b.ReportAllocs()

	var dummy byte
	for b.Loop() {
		dummy += mapper.ReadCHR(addressTrigger0FD)
		dummy += mapper.ReadCHR(addressTrigger0FE)
		dummy += mapper.ReadCHR(addressTrigger1FD)
		dummy += mapper.ReadCHR(addressTrigger1FE)
	}
	_ = dummy
}
