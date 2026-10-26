// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"testing"

	"github.com/aoliveti/dustblow/internal/cartridge"
)

func BenchmarkMapper0ReadPRG(b *testing.B) {
	cart := &cartridge.Cartridge{
		PRG: make([]byte, testPRGUnit32KB),
		CHR: make([]byte, testCHRUnit8KB),
	}
	mapper := cartridge.NewMapper0(cart)

	b.ResetTimer()
	b.ReportAllocs()

	var dummy byte
	for b.Loop() {
		// Alternate between lower and upper banks in the hot loop.
		dummy += mapper.ReadPRG(testAddressPRGBase)
		dummy += mapper.ReadPRG(testAddressPRGMirror)
	}
	_ = dummy
}

func BenchmarkMapper0ReadCHR(b *testing.B) {
	cart := &cartridge.Cartridge{
		PRG: make([]byte, testPRGUnit16KB),
		CHR: make([]byte, testCHRUnit8KB),
	}
	mapper := cartridge.NewMapper0(cart)

	b.ResetTimer()
	b.ReportAllocs()

	var dummy byte
	for b.Loop() {
		dummy += mapper.ReadCHR(testAddressCHRBase)
		dummy += mapper.ReadCHR(testAddressCHRAM)
	}
	_ = dummy
}

func BenchmarkMapper0WriteCHR(b *testing.B) {
	cart := &cartridge.Cartridge{
		PRG: make([]byte, testPRGUnit16KB),
		CHR: make([]byte, testCHRUnit8KB),
	}
	mapper := cartridge.NewMapper0(cart)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		mapper.WriteCHR(testAddressCHRAM, sampleTestWriteByte)
	}
}
