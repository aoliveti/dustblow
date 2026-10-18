// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge

import (
	"bytes"
	"testing"
)

func createTestVRC6Cartridge(prgBanks8K int, chrBanks1K int, mapperID byte) *Cartridge {
	prg := make([]byte, prgBanks8K*vrc6PRGBank8KSize)
	for b := range prgBanks8K {
		for i := range vrc6PRGBank8KSize {
			prg[b*vrc6PRGBank8KSize+i] = byte(b)
		}
	}

	chr := make([]byte, chrBanks1K*vrc6CHRBank1KSize)
	for b := range chrBanks1K {
		for i := range vrc6CHRBank1KSize {
			chr[b*vrc6CHRBank1KSize+i] = byte(b)
		}
	}

	cart := &Cartridge{
		PRG:       prg,
		CHR:       chr,
		SRAM:      make([]byte, SRAMSize),
		MapperID:  mapperID,
		Mirroring: MirrorVertical,
	}
	cart.Mapper = NewMapperVRC6(cart, mapperID)
	return cart
}

func TestMapperVRC6_PRGBanking(t *testing.T) {
	t.Parallel()

	// 16 8KB banks = 8 16KB banks
	num8KBanks := 16
	cart := createTestVRC6Cartridge(num8KBanks, 16, mapperVRC6a)
	m := cart.Mapper.(*MapperVRC6)

	// Initially:
	// $8000 (16KB window): bank 0 (contains 8KB banks 0 and 1)
	// $C000 (8KB window): bank 0
	// $E000 (8KB fixed): last bank (15)
	if got := m.ReadPRG(0x8000); got != 0 {
		t.Errorf("initial $8000 got %d, want 0", got)
	}
	if got := m.ReadPRG(0xA000); got != 1 {
		t.Errorf("initial $A000 got %d, want 1", got)
	}
	if got := m.ReadPRG(0xC000); got != 0 {
		t.Errorf("initial $C000 got %d, want 0", got)
	}
	if got := m.ReadPRG(0xE000); got != 15 {
		t.Errorf("initial $E000 got %d, want 15", got)
	}

	// Select 16KB PRG bank 2 at $8000 (corresponds to 8KB banks 4 and 5)
	m.WritePRG(0x8000, 2)
	if got := m.ReadPRG(0x8000); got != 4 {
		t.Errorf("$8000 after write got %d, want 4", got)
	}
	if got := m.ReadPRG(0xA000); got != 5 {
		t.Errorf("$A000 after write got %d, want 5", got)
	}

	// Select 8KB PRG bank 7 at $C000
	m.WritePRG(0xC000, 7)
	if got := m.ReadPRG(0xC000); got != 7 {
		t.Errorf("$C000 after write got %d, want 7", got)
	}
}

func TestMapperVRC6_CHRBanking(t *testing.T) {
	t.Parallel()

	cart := createTestVRC6Cartridge(8, 32, mapperVRC6a)
	m := cart.Mapper.(*MapperVRC6)

	// Write CHR slots 0..3 via $D000..$D003
	m.WritePRG(0xD000, 10)
	m.WritePRG(0xD001, 11)
	m.WritePRG(0xD002, 12)
	m.WritePRG(0xD003, 13)

	// Write CHR slots 4..7 via $E000..$E003
	m.WritePRG(0xE000, 14)
	m.WritePRG(0xE001, 15)
	m.WritePRG(0xE002, 16)
	m.WritePRG(0xE003, 17)

	for slot := range 8 {
		addr := uint16(slot * 0x0400)
		want := byte(10 + slot)
		if got := m.ReadCHR(addr); got != want {
			t.Errorf("CHR slot %d at addr 0x%04X got %d, want %d", slot, addr, got, want)
		}
	}
}

func TestMapperVRC6_MirroringAndRAM(t *testing.T) {
	t.Parallel()

	cart := createTestVRC6Cartridge(8, 8, mapperVRC6a)
	m := cart.Mapper.(*MapperVRC6)

	mirrorTests := []struct {
		data     byte
		wantMode Mirroring
	}{
		{data: 0x20, wantMode: MirrorVertical},   // bits 3:2 = 00
		{data: 0x24, wantMode: MirrorHorizontal}, // bits 3:2 = 01
		{data: 0x28, wantMode: MirrorSingle0},    // bits 3:2 = 10
		{data: 0x2C, wantMode: MirrorSingle1},    // bits 3:2 = 11
	}

	for _, tt := range mirrorTests {
		m.WritePRG(0xB003, tt.data)
		if cart.Mirroring != tt.wantMode {
			t.Errorf("write 0x%02X got mirroring %v, want %v", tt.data, cart.Mirroring, tt.wantMode)
		}
	}

	// RAM enable: bit 7 = 1
	m.WritePRG(0xB003, 0xA0)
	m.WritePRG(0x6000, 0x42)
	if got := m.ReadPRG(0x6000); got != 0x42 {
		t.Errorf("RAM read after write got 0x%02X, want 0x42", got)
	}
}

func TestMapperVRC6_IRQ(t *testing.T) {
	t.Parallel()

	cart := createTestVRC6Cartridge(8, 8, mapperVRC6a)
	m := cart.Mapper.(*MapperVRC6)

	// Latch = 0xFE
	m.WritePRG(0xF000, 0xFE)
	// Enable IRQ in cycle mode (bit 1=1, bit 2=1 -> 0x06)
	m.WritePRG(0xF001, 0x06)

	if m.IRQPending() {
		t.Fatal("IRQ asserted prematurely")
	}

	// 1 cycle: 0xFE -> 0xFF
	m.StepCPU(1)
	if m.IRQPending() {
		t.Fatal("IRQ asserted before counter overflow")
	}

	// 1 cycle: 0xFF -> trip IRQ and reload 0xFE
	m.StepCPU(1)
	if !m.IRQPending() {
		t.Fatal("IRQ should be asserted")
	}

	// Acknowledge IRQ ($F002)
	m.WritePRG(0xF002, 0)
	if m.IRQPending() {
		t.Fatal("IRQ should be cleared after acknowledge")
	}
}

func TestMapperVRC6_VariantDecoding(t *testing.T) {
	t.Parallel()

	// Mapper 24 (VRC6a): $x001 is reg 1, $x002 is reg 2
	cart24 := createTestVRC6Cartridge(8, 8, mapperVRC6a)
	m24 := cart24.Mapper.(*MapperVRC6)
	if r := m24.decodeReg(0xD001); r != 1 {
		t.Errorf("Mapper 24 reg 1 decoded as %d", r)
	}
	if r := m24.decodeReg(0xD002); r != 2 {
		t.Errorf("Mapper 24 reg 2 decoded as %d", r)
	}

	// Mapper 26 (VRC6b): $x001 is reg 2, $x002 is reg 1
	cart26 := createTestVRC6Cartridge(8, 8, mapperVRC6b)
	m26 := cart26.Mapper.(*MapperVRC6)
	if r := m26.decodeReg(0xD001); r != 2 {
		t.Errorf("Mapper 26 reg 1 decoded as %d, want 2", r)
	}
	if r := m26.decodeReg(0xD002); r != 1 {
		t.Errorf("Mapper 26 reg 2 decoded as %d, want 1", r)
	}
}

func TestMapperVRC6_Audio(t *testing.T) {
	t.Parallel()

	audio := NewVRC6Audio()

	// Configure Pulse 1: volume 15, duty 7 (50%), period 10, enable
	audio.WriteRegister(0x9000, 0x7F) // Duty 7, volume 15
	audio.WriteRegister(0x9001, 10)
	audio.WriteRegister(0x9002, 0x80) // Enable

	// Configure Sawtooth: rate 2, period 5, enable
	audio.WriteRegister(0xB000, 2)
	audio.WriteRegister(0xB001, 5)
	audio.WriteRegister(0xB002, 0x80) // Enable

	audio.StepAudio(100)
	sample := audio.AudioOutput()
	if sample <= 0 {
		t.Errorf("expected positive audio output, got %f", sample)
	}
}

func TestMapperVRC6_SaveAndLoadState(t *testing.T) {
	t.Parallel()

	num8KBanks := 16
	cart1 := createTestVRC6Cartridge(num8KBanks, 16, mapperVRC6a)
	m1 := cart1.Mapper.(*MapperVRC6)

	// Set 16KB PRG bank 2 at $8000
	m1.WritePRG(0x8000, 2)
	// Set 8KB PRG bank 3 at $C000
	m1.WritePRG(0xC000, 3)
	// Set mirroring horizontal (mode 1: 1 << 2 = 0x04)
	m1.WritePRG(0xB003, 0x04)
	// Set CHR bank 4 at slot 0
	m1.WritePRG(0xD000, 4)
	// Set IRQ latch to 150, enable cycle mode
	m1.WritePRG(0xF000, 150)
	m1.WritePRG(0xF001, 0x06) // cycle mode + enable
	m1.StepCPU(10)

	// Configure pulse1 audio
	m1.audio.WriteRegister(0x9000, 0x7F)
	m1.audio.WriteRegister(0x9001, 10)
	m1.audio.WriteRegister(0x9002, 0x80)
	m1.audio.StepAudio(50)

	var buf bytes.Buffer
	if err := m1.SaveState(&buf); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	cart2 := createTestVRC6Cartridge(num8KBanks, 16, mapperVRC6a)
	m2 := cart2.Mapper.(*MapperVRC6)

	if err := m2.LoadState(&buf); err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if got := m2.ReadPRG(0x8000); got != 4 {
		t.Errorf("ReadPRG($8000) = %d, want 4", got)
	}
	if got := m2.ReadPRG(0xC000); got != 3 {
		t.Errorf("ReadPRG($C000) = %d, want 3", got)
	}
	if cart2.Mirroring != MirrorHorizontal {
		t.Errorf("cart2.Mirroring = %v, want MirrorHorizontal", cart2.Mirroring)
	}
	if got := m2.ReadCHR(0x0000); got != 4 {
		t.Errorf("ReadCHR($0000) = %d, want 4", got)
	}
	if m2.irqCounter != 160 {
		t.Errorf("irqCounter = %d, want 160", m2.irqCounter)
	}
	if m2.audio.pulse1.volume != 15 {
		t.Errorf("pulse1 volume = %d, want 15", m2.audio.pulse1.volume)
	}
}
