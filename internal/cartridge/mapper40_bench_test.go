// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"testing"

	"github.com/aoliveti/dustblow/internal/cartridge"
)

func BenchmarkMapper40ReadPRG(b *testing.B) {
	cart := &cartridge.Cartridge{
		PRG: make([]byte, testMapper40PRGBanks8*testMMC3PRGUnit8KB),
		CHR: make([]byte, testMapper40CHRBanks1*testCHRUnit8KB),
	}
	mapper := cartridge.NewMapper40(cart)

	b.ResetTimer()
	b.ReportAllocs()

	var dummy byte
	for b.Loop() {
		dummy += mapper.ReadPRG(addrM40Window6000)
		dummy += mapper.ReadPRG(addrM40WindowC000)
	}
	_ = dummy
}

func BenchmarkMapper40WritePRG(b *testing.B) {
	cart := &cartridge.Cartridge{
		PRG: make([]byte, testMapper40PRGBanks8*testMMC3PRGUnit8KB),
		CHR: make([]byte, testMapper40CHRBanks1*testCHRUnit8KB),
	}
	mapper := cartridge.NewMapper40(cart)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		mapper.WritePRG(addrM40RegSelectBank, 2)
	}
}

func BenchmarkMapper40StepCPU(b *testing.B) {
	cart := &cartridge.Cartridge{
		PRG: make([]byte, testMapper40PRGBanks8*testMMC3PRGUnit8KB),
		CHR: make([]byte, testMapper40CHRBanks1*testCHRUnit8KB),
	}
	mapper := cartridge.NewMapper40(cart)
	mapper.WritePRG(addrM40RegEnableIRQ, 0)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		mapper.StepCPU(2)
	}
}
