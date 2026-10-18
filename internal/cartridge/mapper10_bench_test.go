// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"testing"
)

func BenchmarkMapper10ReadPRG(b *testing.B) {
	_, mapper := createTestMMC4Cartridge()

	b.ResetTimer()
	b.ReportAllocs()

	var dummy byte
	for b.Loop() {
		dummy += mapper.ReadPRG(addressPRGWindow16KSwitchable)
		dummy += mapper.ReadPRG(addressPRGWindow16KFixedLast)
	}
	_ = dummy
}

func BenchmarkMapper10WritePRG(b *testing.B) {
	_, mapper := createTestMMC4Cartridge()

	b.ResetTimer()
	b.ReportAllocs()

	val := byte(0)
	for b.Loop() {
		mapper.WritePRG(addressRegPRGBank, val)
		val++
	}
}

func BenchmarkMapper10ReadCHRLatchSwitching(b *testing.B) {
	_, mapper := createTestMMC4Cartridge()

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
