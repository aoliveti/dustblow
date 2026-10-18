// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ppu_test

import (
	"testing"

	"github.com/dustblow/dustblow/internal/cartridge"
	"github.com/dustblow/dustblow/internal/ppu"
)

const (
	benchRegPPUCTRL   uint16 = 0x2000
	benchRegPPUMASK   uint16 = 0x2001
	benchRegPPUSTATUS uint16 = 0x2002

	benchCtrlNMIEnable byte = 0x90

	benchPPUCyclesPerCPU   = 3
	benchTotalScanlines    = 262
	benchCyclesPerScanline = 341
	benchTotalFrameCycles  = benchTotalScanlines * benchCyclesPerScanline
)

func BenchmarkPPU_ReadRegister(b *testing.B) {
	p := ppu.New(nil)
	b.ResetTimer()

	for b.Loop() {
		_ = p.ReadRegister(benchRegPPUSTATUS)
	}
}

func BenchmarkPPU_WriteRegister(b *testing.B) {
	p := ppu.New(nil)
	b.ResetTimer()

	for b.Loop() {
		p.WriteRegister(benchRegPPUCTRL, benchCtrlNMIEnable)
	}
}

func BenchmarkPPU_StepCycle(b *testing.B) {
	p := ppu.New(nil)
	b.ResetTimer()

	for b.Loop() {
		p.Step(benchPPUCyclesPerCPU)
	}
}

func BenchmarkPPU_RenderFrame(b *testing.B) {
	cart := newTestCartridge(cartridge.MirrorHorizontal)
	p := ppu.New(cart)
	p.WriteRegister(benchRegPPUMASK, ppu.FlagMaskShowBG|ppu.FlagMaskShowLeftBG)
	b.ResetTimer()

	for b.Loop() {
		p.Step(benchTotalFrameCycles)
	}
}
