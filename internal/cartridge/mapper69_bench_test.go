// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"testing"

	"github.com/dustblow/dustblow/internal/cartridge"
)

func BenchmarkMapper69_ReadPRG(b *testing.B) {
	mapper := createTestFME7CartridgeBench(b)

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		_ = mapper.ReadPRG(0x8000 + uint16(i&0x7FFF))
	}
}

func BenchmarkMapper69_ReadCHR(b *testing.B) {
	mapper := createTestFME7CartridgeBench(b)

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		_ = mapper.ReadCHR(uint16(i & 0x1FFF))
	}
}

func BenchmarkMapper69_StepCPU(b *testing.B) {
	mapper := createTestFME7CartridgeBench(b)
	steppable := mapper.(cartridge.CPUSteppable)

	// Configure counter running and enabled
	mapper.WritePRG(0x8000, 0x0D)
	mapper.WritePRG(0xA000, 0x81)

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		steppable.StepCPU(i%4 + 1)
	}
}

func createTestFME7CartridgeBench(b *testing.B) cartridge.Mapper {
	b.Helper()

	prg := make([]byte, testFME7PRG256K)
	chr := make([]byte, testFME7CHR128K)

	cart := &cartridge.Cartridge{
		PRG:      prg,
		CHR:      chr,
		SRAM:     make([]byte, 8192),
		MapperID: 69,
	}

	mapper := cartridge.NewMapper69(cart)
	cart.Mapper = mapper
	return mapper
}
