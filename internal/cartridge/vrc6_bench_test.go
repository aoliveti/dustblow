// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"testing"

	"github.com/dustblow/dustblow/internal/cartridge"
)

const (
	benchVRC6PRGSize = 256 * 1024
	benchVRC6CHRSize = 256 * 1024
	benchVRC6SRAM    = 8 * 1024
	benchVRC6Mapper  = 24
)

func BenchmarkMapperVRC6_ReadPRG(b *testing.B) {
	mapper := createTestVRC6CartridgeBench(b)

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		_ = mapper.ReadPRG(0x8000 + uint16(i&0x7FFF))
	}
}

func BenchmarkMapperVRC6_ReadCHR(b *testing.B) {
	mapper := createTestVRC6CartridgeBench(b)

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		_ = mapper.ReadCHR(uint16(i & 0x1FFF))
	}
}

func BenchmarkMapperVRC6_WritePRG(b *testing.B) {
	mapper := createTestVRC6CartridgeBench(b)

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		mapper.WritePRG(0x8000+uint16(i&0x7FFF), byte(i))
	}
}

func BenchmarkMapperVRC6_StepCPU(b *testing.B) {
	mapper := createTestVRC6CartridgeBench(b)
	steppable, ok := mapper.(cartridge.CPUSteppable)
	if !ok {
		b.Fatalf("mapper is not CPUSteppable: %T", mapper)
	}

	// Enable IRQ in scanline mode
	mapper.WritePRG(0xF001, 0x02)

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		steppable.StepCPU(i%4 + 1)
	}
}

func BenchmarkMapperVRC6_StepAudio(b *testing.B) {
	audio := cartridge.NewVRC6Audio()
	audio.WriteRegister(0x9000, 0x7F)
	audio.WriteRegister(0x9001, 10)
	audio.WriteRegister(0x9002, 0x80)

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		audio.StepAudio(i%4 + 1)
		_ = audio.AudioOutput()
	}
}

func createTestVRC6CartridgeBench(b *testing.B) cartridge.Mapper {
	b.Helper()

	prg := make([]byte, benchVRC6PRGSize)
	chr := make([]byte, benchVRC6CHRSize)

	cart := &cartridge.Cartridge{
		PRG:      prg,
		CHR:      chr,
		SRAM:     make([]byte, benchVRC6SRAM),
		MapperID: benchVRC6Mapper,
	}

	return cartridge.NewMapperVRC6(cart, benchVRC6Mapper)
}
