// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"testing"

	"github.com/dustblow/dustblow/internal/cartridge"
)

func BenchmarkMapper71_ReadPRG(b *testing.B) {
	prg := make([]byte, testM71PRGSize128K)
	cart := &cartridge.Cartridge{PRG: prg}
	m := cartridge.NewMapper71(cart)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = m.ReadPRG(testM71AddrBank0)
		_ = m.ReadPRG(testM71AddrFixedBank)
	}
}

func BenchmarkMapper71_WritePRG(b *testing.B) {
	prg := make([]byte, testM71PRGSize128K)
	cart := &cartridge.Cartridge{PRG: prg}
	m := cartridge.NewMapper71(cart)

	b.ResetTimer()
	b.ReportAllocs()

	var counter byte
	for b.Loop() {
		m.WritePRG(testM71RegBankSwitch, counter&0x07)
		counter++
	}
}

func BenchmarkMapper71_ReadCHR(b *testing.B) {
	chr := make([]byte, testM71CHRSize8K)
	cart := &cartridge.Cartridge{CHR: chr}
	m := cartridge.NewMapper71(cart)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = m.ReadCHR(testM71CHRAddr)
	}
}
