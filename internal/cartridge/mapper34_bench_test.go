// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"testing"
)

func BenchmarkMapper34ReadPRG(b *testing.B) {
	_, mapper := createTestBNROMCartridge()

	b.ResetTimer()
	b.ReportAllocs()

	var dummy byte
	for b.Loop() {
		dummy += mapper.ReadPRG(addressBNROMPRGBase)
		dummy += mapper.ReadPRG(addressBNROMPRGEnd)
	}
	_ = dummy
}

func BenchmarkMapper34WritePRGBankSwitch(b *testing.B) {
	_, mapper := createTestBNROMCartridge()

	b.ResetTimer()
	b.ReportAllocs()

	val := byte(0)
	for b.Loop() {
		mapper.WritePRG(addressBNROMPRGBase, val)
		val++
	}
}

func BenchmarkMapper34NINAWriteRegisters(b *testing.B) {
	mapper := createTestNINACartridge()

	b.ResetTimer()
	b.ReportAllocs()

	val := byte(0)
	for b.Loop() {
		mapper.WritePRG(addressNINARegPRG, val)
		mapper.WritePRG(addressNINARegCHR0, val)
		mapper.WritePRG(addressNINARegCHR1, val)
		val++
	}
}

func BenchmarkMapper34ReadCHR(b *testing.B) {
	mapper := createTestNINACartridge()

	b.ResetTimer()
	b.ReportAllocs()

	var dummy byte
	for b.Loop() {
		dummy += mapper.ReadCHR(addressCHRWindow0)
		dummy += mapper.ReadCHR(addressCHRWindow1)
	}
	_ = dummy
}
