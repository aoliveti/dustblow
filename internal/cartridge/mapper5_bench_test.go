// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"testing"

	"github.com/aoliveti/dustblow/internal/cartridge"
)

func BenchmarkMapper5_ReadPRG(b *testing.B) {
	prg := make([]byte, testMMC5PRG128K)
	cart := &cartridge.Cartridge{
		PRG:      prg,
		CHR:      make([]byte, testMMC5CHR128K),
		MapperID: 5,
	}
	mapper := cartridge.NewMapper5(cart)

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		_ = mapper.ReadPRG(0x8000 + uint16(i&0x7FFF))
	}
}

func BenchmarkMapper5_ReadCHR(b *testing.B) {
	cart := &cartridge.Cartridge{
		PRG:      make([]byte, testMMC5PRG128K),
		CHR:      make([]byte, testMMC5CHR128K),
		MapperID: 5,
	}
	mapper := cartridge.NewMapper5(cart)

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		_ = mapper.ReadCHR(uint16(i & 0x1FFF))
	}
}

func BenchmarkMapper5_ReadNametable(b *testing.B) {
	cart := &cartridge.Cartridge{
		PRG:      make([]byte, testMMC5PRG128K),
		CHR:      make([]byte, testMMC5CHR128K),
		MapperID: 5,
	}
	mapper := cartridge.NewMapper5(cart)
	vram := make([]byte, 2048)

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		_, _ = mapper.ReadNametable(0x2000+uint16(i&0x0FFF), vram)
	}
}
