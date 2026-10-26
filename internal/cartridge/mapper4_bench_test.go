// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"testing"

	"github.com/aoliveti/dustblow/internal/cartridge"
)

func BenchmarkMapper4ReadPRG(b *testing.B) {
	cart := &cartridge.Cartridge{
		PRG: make([]byte, testMMC3PRGBanks16*testMMC3PRGUnit8KB),
		CHR: make([]byte, testMMC3CHRBanks64*testMMC3CHRUnit1KB),
	}
	mapper := cartridge.NewMapper4(cart)

	b.ResetTimer()
	b.ReportAllocs()

	var dummy byte
	for b.Loop() {
		dummy += mapper.ReadPRG(addrMMC3Window8000)
		dummy += mapper.ReadPRG(addrMMC3WindowC000)
	}
	_ = dummy
}

func BenchmarkMapper4WritePRG(b *testing.B) {
	cart := &cartridge.Cartridge{
		PRG: make([]byte, testMMC3PRGBanks16*testMMC3PRGUnit8KB),
		CHR: make([]byte, testMMC3CHRBanks64*testMMC3CHRUnit1KB),
	}
	mapper := cartridge.NewMapper4(cart)

	b.ResetTimer()
	b.ReportAllocs()

	var bank byte
	for b.Loop() {
		mapper.WritePRG(addrMMC3BankData, bank)
		bank++
	}
}

func BenchmarkMapper4ReadCHR(b *testing.B) {
	cart := &cartridge.Cartridge{
		PRG: make([]byte, testMMC3PRGBanks16*testMMC3PRGUnit8KB),
		CHR: make([]byte, testMMC3CHRBanks64*testMMC3CHRUnit1KB),
	}
	mapper := cartridge.NewMapper4(cart)

	b.ResetTimer()
	b.ReportAllocs()

	var dummy byte
	for b.Loop() {
		dummy += mapper.ReadCHR(addrMMC3CHR0000)
		dummy += mapper.ReadCHR(addrMMC3CHR1000)
	}
	_ = dummy
}

func BenchmarkMapper4WriteCHR(b *testing.B) {
	cart := &cartridge.Cartridge{
		PRG: make([]byte, testMMC3PRGBanks16*testMMC3PRGUnit8KB),
		CHR: make([]byte, testMMC3CHRBanks64*testMMC3CHRUnit1KB),
	}
	mapper := cartridge.NewMapper4(cart)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		mapper.WriteCHR(addrMMC3CHR0000, 0x42)
	}
}

func BenchmarkMapper4ClockScanline(b *testing.B) {
	cart := &cartridge.Cartridge{
		PRG: make([]byte, testMMC3PRGBanks16*testMMC3PRGUnit8KB),
		CHR: make([]byte, testMMC3CHRBanks64*testMMC3CHRUnit1KB),
	}
	mapper := cartridge.NewMapper4(cart)
	mapper.WritePRG(addrMMC3IRQLatch, 100)
	mapper.WritePRG(addrMMC3IRQReload, 0)
	mapper.WritePRG(addrMMC3IRQEnable, 0)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		mapper.ClockScanline(0)
	}
}
