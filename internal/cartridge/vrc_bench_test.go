// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"testing"

	"github.com/dustblow/dustblow/internal/cartridge"
)

const (
	benchVRCPRGSize = 256 * 1024
	benchVRCCHRSize = 256 * 1024
	benchVRCSRAM    = 8 * 1024
	benchVRCMapper  = 23
)

func BenchmarkMapperVRC_ReadPRG(b *testing.B) {
	mapper := createTestVRCCartridgeBench(b)

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		_ = mapper.ReadPRG(0x8000 + uint16(i&0x7FFF))
	}
}

func BenchmarkMapperVRC_ReadCHR(b *testing.B) {
	mapper := createTestVRCCartridgeBench(b)

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		_ = mapper.ReadCHR(uint16(i & 0x1FFF))
	}
}

func BenchmarkMapperVRC_WritePRG(b *testing.B) {
	mapper := createTestVRCCartridgeBench(b)

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		mapper.WritePRG(0x8000+uint16(i&0x3FFF), byte(i))
	}
}

func BenchmarkMapperVRC_StepCPU(b *testing.B) {
	mapper := createTestVRCCartridgeBench(b)
	steppable, ok := mapper.(cartridge.CPUSteppable)
	if !ok {
		b.Fatalf("mapper is not CPUSteppable: %T", mapper)
	}

	// Enable IRQ in scanline mode
	mapper.WritePRG(0xF002, 0x02)

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		steppable.StepCPU(i%4 + 1)
	}
}

func createTestVRCCartridgeBench(b *testing.B) cartridge.Mapper {
	b.Helper()

	prg := make([]byte, benchVRCPRGSize)
	chr := make([]byte, benchVRCCHRSize)

	cart := &cartridge.Cartridge{
		PRG:       prg,
		CHR:       chr,
		SRAM:      make([]byte, benchVRCSRAM),
		MapperID:  benchVRCMapper,
		Submapper: 1,
	}

	return cartridge.NewMapperVRC(cart, benchVRCMapper, 1)
}
