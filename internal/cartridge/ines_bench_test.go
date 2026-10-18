// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"bytes"
	"testing"

	"github.com/dustblow/dustblow/internal/cartridge"
)

func BenchmarkNewNROM128(b *testing.B) {
	rom := makeHeader(1, 1, testFlagHorizontalMirror, 0)
	rom = append(rom, make([]byte, testPRGUnit16KB+testCHRUnit8KB)...)

	reader := bytes.NewReader(rom)
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, _ = reader.Seek(0, 0)
		cart, err := cartridge.New(reader)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		_ = cart
	}
}

func BenchmarkNewNROM256(b *testing.B) {
	rom := makeHeader(2, 1, testFlagVerticalMirror, 0)
	rom = append(rom, make([]byte, testPRGUnit32KB+testCHRUnit8KB)...)

	reader := bytes.NewReader(rom)
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, _ = reader.Seek(0, 0)
		cart, err := cartridge.New(reader)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		_ = cart
	}
}
