// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpu_test

import (
	"testing"

	"github.com/aoliveti/dustblow/internal/cpu"
)

func BenchmarkCPUStep(b *testing.B) {
	mem := &mockMemory{}
	c := cpu.New(mem)

	// Fill memory with a sequence of NOPs, INX, INY, and JMP loop.
	// $8000: NOP
	// $8001: INX
	// $8002: INY
	// $8003: JMP $8000
	mem.data[0x8000] = 0xEA // NOP
	mem.data[0x8001] = 0xE8 // INX
	mem.data[0x8002] = 0xC8 // INY
	mem.data[0x8003] = 0x4C // JMP $8000
	mem.data[0x8004] = 0x00
	mem.data[0x8005] = 0x80

	c.PC = 0x8000
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		c.Step()
	}
}
