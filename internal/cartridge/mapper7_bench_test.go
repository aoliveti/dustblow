// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"testing"
)

func BenchmarkMapper7ReadPRG(b *testing.B) {
	cart, mapper := createTestAxROMCartridge(testAxROMPRGBanks8)

	b.ResetTimer()
	b.ReportAllocs()

	var dummy byte
	for b.Loop() {
		dummy += mapper.ReadPRG(addressPRGWindowStart)
		dummy += mapper.ReadPRG(addressPRGWindowMid)
		dummy += mapper.ReadPRG(addressPRGWindowEnd)
	}
	_ = dummy
	_ = cart
}

func BenchmarkMapper7WritePRGBankSwitch(b *testing.B) {
	_, mapper := createTestAxROMCartridge(testAxROMPRGBanks8)

	b.ResetTimer()
	b.ReportAllocs()

	val := byte(0)
	for b.Loop() {
		mapper.WritePRG(addressPRGWindowStart, val)
		val++
	}
}

func BenchmarkMapper7ReadCHR(b *testing.B) {
	_, mapper := createTestAxROMCartridge(testAxROMPRGBanks8)

	b.ResetTimer()
	b.ReportAllocs()

	var dummy byte
	for b.Loop() {
		dummy += mapper.ReadCHR(addressCHRRAMWindow)
	}
	_ = dummy
}
