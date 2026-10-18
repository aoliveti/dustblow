// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"testing"

	"github.com/dustblow/dustblow/internal/cartridge"
)

func BenchmarkSunsoft5BAudio_StepAudio(b *testing.B) {
	audio := cartridge.NewSunsoft5BAudio()
	audio.SelectRegister(regToneALow)
	audio.WriteData(0x04)
	audio.SelectRegister(regToneAHigh)
	audio.WriteData(0x00)
	audio.SelectRegister(regMixer)
	audio.WriteData(mixerEnableAllTones)
	audio.SelectRegister(regVolA)
	audio.WriteData(maxVolumeFixed)

	b.ResetTimer()
	b.ReportAllocs()

	const testCPUStepCycles = 114 // Approximate CPU cycles per scanline

	for b.Loop() {
		audio.StepAudio(testCPUStepCycles)
	}
}

func BenchmarkSunsoft5BAudio_AudioOutput(b *testing.B) {
	audio := cartridge.NewSunsoft5BAudio()
	audio.SelectRegister(regToneALow)
	audio.WriteData(0x04)
	audio.SelectRegister(regMixer)
	audio.WriteData(mixerEnableAllTones)
	audio.SelectRegister(regVolA)
	audio.WriteData(maxVolumeFixed)

	b.ResetTimer()
	b.ReportAllocs()

	var sink float32
	for b.Loop() {
		sink += audio.AudioOutput()
	}
	_ = sink
}
