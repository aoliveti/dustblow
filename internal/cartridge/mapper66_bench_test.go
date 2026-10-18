// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"testing"
)

func BenchmarkMapper66ReadPRG(b *testing.B) {
	_, mapper := createTestGxROMCartridge()

	b.ResetTimer()
	b.ReportAllocs()

	var dummy byte
	for b.Loop() {
		dummy += mapper.ReadPRG(addressGxROMPRGBase)
		dummy += mapper.ReadPRG(addressGxROMPRGEnd)
	}
	_ = dummy
}

func BenchmarkMapper66WritePRGBankSwitch(b *testing.B) {
	_, mapper := createTestGxROMCartridge()

	b.ResetTimer()
	b.ReportAllocs()

	val := byte(0)
	for b.Loop() {
		mapper.WritePRG(addressGxROMPRGBase, val)
		val++
	}
}

func BenchmarkMapper66ReadCHR(b *testing.B) {
	_, mapper := createTestGxROMCartridge()

	b.ResetTimer()
	b.ReportAllocs()

	var dummy byte
	for b.Loop() {
		dummy += mapper.ReadCHR(addressGxROMCHRBase)
		dummy += mapper.ReadCHR(addressGxROMCHREnd)
	}
	_ = dummy
}
