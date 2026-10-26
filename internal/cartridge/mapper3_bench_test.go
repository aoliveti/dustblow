// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"testing"

	"github.com/aoliveti/dustblow/internal/cartridge"
)

func BenchmarkMapper3ReadPRG(b *testing.B) {
	cart := &cartridge.Cartridge{
		PRG: make([]byte, testPRGUnit32KB),
		CHR: make([]byte, testCNROMCHRBanks4*testCHRUnit8KB),
	}
	mapper := cartridge.NewMapper3(cart)

	b.ResetTimer()
	b.ReportAllocs()

	var dummy byte
	for b.Loop() {
		dummy += mapper.ReadPRG(addressPRGStart)
		dummy += mapper.ReadPRG(addressPRGMirror)
	}
	_ = dummy
}

func BenchmarkMapper3WritePRG(b *testing.B) {
	cart := &cartridge.Cartridge{
		PRG: make([]byte, testPRGUnit32KB),
		CHR: make([]byte, testCNROMCHRBanks4*testCHRUnit8KB),
	}
	mapper := cartridge.NewMapper3(cart)

	b.ResetTimer()
	b.ReportAllocs()

	var bank byte
	for b.Loop() {
		mapper.WritePRG(addressPRGStart, bank)
		bank++
	}
}

func BenchmarkMapper3ReadCHR(b *testing.B) {
	cart := &cartridge.Cartridge{
		PRG: make([]byte, testPRGUnit32KB),
		CHR: make([]byte, testCNROMCHRBanks4*testCHRUnit8KB),
	}
	mapper := cartridge.NewMapper3(cart)

	b.ResetTimer()
	b.ReportAllocs()

	var dummy byte
	for b.Loop() {
		dummy += mapper.ReadCHR(addressCHRWindow)
		dummy += mapper.ReadCHR(addressCHREnd)
	}
	_ = dummy
}

func BenchmarkMapper3WriteCHR(b *testing.B) {
	cart := &cartridge.Cartridge{
		PRG: make([]byte, testPRGUnit32KB),
		CHR: make([]byte, testCNROMCHRBanks4*testCHRUnit8KB),
	}
	mapper := cartridge.NewMapper3(cart)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		mapper.WriteCHR(addressCHRWindow, sampleTilePattern)
	}
}
