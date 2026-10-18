// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bus_test

import (
	"testing"

	"github.com/dustblow/dustblow/internal/bus"
)

func BenchmarkBus_ReadRAM(b *testing.B) {
	sysBus := bus.New(nil)
	b.ResetTimer()
	b.ReportAllocs()

	var dummy byte
	for b.Loop() {
		dummy += sysBus.Read(testAddressRAMZeroPage)
		dummy += sysBus.Read(testAddressRAMMirror4KB)
	}
	_ = dummy
}

func BenchmarkBus_WriteRAM(b *testing.B) {
	sysBus := bus.New(nil)
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		sysBus.Write(testAddressRAMZeroPage, sampleRAMPayload)
	}
}

func BenchmarkBus_ReadPRG(b *testing.B) {
	cart := newTestCartridge()
	sysBus := bus.New(cart)
	b.ResetTimer()
	b.ReportAllocs()

	var dummy byte
	for b.Loop() {
		dummy += sysBus.Read(testAddressPRGBase)
	}
	_ = dummy
}

func BenchmarkBus_Peek(b *testing.B) {
	cart := newTestCartridge()
	sysBus := bus.New(cart)
	b.ResetTimer()
	b.ReportAllocs()

	var dummy byte
	for b.Loop() {
		dummy += sysBus.Peek(testAddressPRGBase)
		dummy += sysBus.Peek(testAddressRAMZeroPage)
	}
	_ = dummy
}
