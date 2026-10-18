// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge

import (
	"bytes"
	"testing"
)

func createTestVRCCartridge(prgBanks8K int, chrBanks1K int, mapperID byte, submapper byte) *Cartridge {
	prg := make([]byte, prgBanks8K*vrcPRGBankSize8K)
	for b := range prgBanks8K {
		for i := range vrcPRGBankSize8K {
			prg[b*vrcPRGBankSize8K+i] = byte(b)
		}
	}

	chr := make([]byte, chrBanks1K*vrcCHRBankSize1K)
	for b := range chrBanks1K {
		for i := range vrcCHRBankSize1K {
			chr[b*vrcCHRBankSize1K+i] = byte(b)
		}
	}

	cart := &Cartridge{
		PRG:       prg,
		CHR:       chr,
		SRAM:      make([]byte, SRAMSize),
		MapperID:  mapperID,
		Submapper: submapper,
		Mirroring: MirrorVertical,
	}
	cart.Mapper = NewMapperVRC(cart, mapperID, submapper)
	return cart
}

func TestMapperVRC_PRGBanking(t *testing.T) {
	t.Parallel()

	numBanks := 16
	cart := createTestVRCCartridge(numBanks, 16, mapperVRC2b4e, 1) // VRC4f: A0=A0, A1=A1
	m, ok := cart.Mapper.(*MapperVRC)
	if !ok {
		t.Fatalf("expected *MapperVRC, got %T", cart.Mapper)
	}

	// In swapMode 0 (default):
	// $8000: prgBank0 (default 0)
	// $A000: prgBank1 (default 0)
	// $C000: fixed to numBanks - 2 (14)
	// $E000: fixed to numBanks - 1 (15)
	if got := m.ReadPRG(0x8000); got != 0 {
		t.Errorf("initial $8000 got %d, want 0", got)
	}
	if got := m.ReadPRG(0xC000); got != 14 {
		t.Errorf("initial $C000 got %d, want 14", got)
	}
	if got := m.ReadPRG(0xE000); got != 15 {
		t.Errorf("initial $E000 got %d, want 15", got)
	}

	// Select bank 3 for $8000
	m.WritePRG(0x8000, 3)
	if got := m.ReadPRG(0x8000); got != 3 {
		t.Errorf("$8000 after write got %d, want 3", got)
	}

	// Select bank 4 for $A000
	m.WritePRG(0xA000, 4)
	if got := m.ReadPRG(0xA000); got != 4 {
		t.Errorf("$A000 after write got %d, want 4", got)
	}

	// Switch swapMode to 1: $9002 (A0=0, A1=1 on VRC4f -> offset 2)
	// data = 0x02 (bit 1 = swap mode 1)
	m.WritePRG(0x9002, 0x02)
	// Now:
	// $8000 should be fixed to numBanks - 2 (14)
	// $C000 should be switchable bank0 (3)
	if got := m.ReadPRG(0x8000); got != 14 {
		t.Errorf("swapMode 1: $8000 got %d, want 14", got)
	}
	if got := m.ReadPRG(0xC000); got != 3 {
		t.Errorf("swapMode 1: $C000 got %d, want 3", got)
	}
}

func TestMapperVRC_CHRBanking(t *testing.T) {
	t.Parallel()

	numCHRBanks := 32
	cart := createTestVRCCartridge(8, numCHRBanks, mapperVRC2b4e, 1) // VRC4f: A0=A0, A1=A1
	m := cart.Mapper.(*MapperVRC)

	// CHR slot 0: low nibble at $B000 (reg 0), high nibble at $B001 (reg 1)
	// Set bank 19 (0x13): low = 3, high = 1
	m.WritePRG(0xB000, 3)
	m.WritePRG(0xB001, 1)
	if got := m.ReadCHR(0x0000); got != 19 {
		t.Errorf("CHR slot 0 got %d, want 19", got)
	}

	// CHR slot 1: low nibble at $B002 (reg 2), high nibble at $B003 (reg 3)
	// Set bank 25 (0x19): low = 9, high = 1
	m.WritePRG(0xB002, 9)
	m.WritePRG(0xB003, 1)
	if got := m.ReadCHR(0x0400); got != 25 {
		t.Errorf("CHR slot 1 got %d, want 25", got)
	}
}

func TestMapperVRC_VRC2a_CHRShift(t *testing.T) {
	t.Parallel()

	// Mapper 22 (VRC2a): A0=A1, A1=A0.
	// Low nibble reg 0 ($B000), high nibble reg 1 ($B002).
	// CHR bank value is shifted right by 1.
	cart := createTestVRCCartridge(8, 32, mapperVRC2a, 0)
	m := cart.Mapper.(*MapperVRC)

	// Set combined value = 14 (0x0E): low = 0x0E, high = 0
	// 14 >> 1 = 7
	m.WritePRG(0xB000, 0x0E)
	m.WritePRG(0xB002, 0x00)
	if got := m.ReadCHR(0x0000); got != 7 {
		t.Errorf("VRC2a CHR slot 0 got %d, want 7", got)
	}
}

func TestMapperVRC_Mirroring(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mapperID  byte
		data      byte
		wantMode  Mirroring
	}{
		{name: "VRC4 Vertical", mapperID: mapperVRC4a4c, data: 0, wantMode: MirrorVertical},
		{name: "VRC4 Horizontal", mapperID: mapperVRC4a4c, data: 1, wantMode: MirrorHorizontal},
		{name: "VRC4 Single0", mapperID: mapperVRC4a4c, data: 2, wantMode: MirrorSingle0},
		{name: "VRC4 Single1", mapperID: mapperVRC4a4c, data: 3, wantMode: MirrorSingle1},
		{name: "VRC2 Vertical", mapperID: mapperVRC2a, data: 0, wantMode: MirrorVertical},
		{name: "VRC2 Horizontal", mapperID: mapperVRC2a, data: 1, wantMode: MirrorHorizontal},
		{name: "VRC2 Wai Wai World 0xFF safeguard", mapperID: mapperVRC2b4e, data: 0xFF, wantMode: MirrorHorizontal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cart := createTestVRCCartridge(8, 8, tt.mapperID, 0)
			cart.Mapper.WritePRG(0x9000, tt.data)
			if cart.Mirroring != tt.wantMode {
				t.Errorf("got mirroring %v, want %v", cart.Mirroring, tt.wantMode)
			}
		})
	}
}

func TestMapperVRC_IRQ(t *testing.T) {
	t.Parallel()

	// VRC4f: A0=A0, A1=A1.
	// $F000 (reg 0): latch low
	// $F001 (reg 1): latch high
	// $F002 (reg 2): control
	// $F003 (reg 3): acknowledge
	cart := createTestVRCCartridge(8, 8, mapperVRC2b4e, 1)
	m := cart.Mapper.(*MapperVRC)

	// Set latch to 0xFE: low = 0x0E, high = 0x0F
	m.WritePRG(0xF000, 0x0E)
	m.WritePRG(0xF001, 0x0F)

	// Enable IRQ in cycle mode (control data = 0x06: bit 1=enable, bit 2=cycle mode)
	m.WritePRG(0xF002, 0x06)

	if m.IRQPending() {
		t.Fatal("IRQ should not be pending yet")
	}

	// 1 cycle: 0xFE -> 0xFF
	m.StepCPU(1)
	if m.IRQPending() {
		t.Fatal("IRQ should not be pending after 1 cycle")
	}

	// 1 cycle: 0xFF -> trip IRQ and reload latch (0xFE)
	m.StepCPU(1)
	if !m.IRQPending() {
		t.Fatal("IRQ should be pending after counter trips")
	}

	// Acknowledge IRQ ($F003)
	m.WritePRG(0xF003, 0)
	if m.IRQPending() {
		t.Fatal("IRQ should be cleared after acknowledge")
	}
}

func TestMapperVRC_ScanlineIRQ(t *testing.T) {
	t.Parallel()

	cart := createTestVRCCartridge(8, 8, mapperVRC2b4e, 1)
	m := cart.Mapper.(*MapperVRC)

	// Latch = 0xFF (will trip after 1 clock)
	m.WritePRG(0xF000, 0x0F)
	m.WritePRG(0xF001, 0x0F)

	// Enable IRQ in scanline mode (control data = 0x02: bit 1=enable, bit 2=0 for scanline mode)
	m.WritePRG(0xF002, 0x02)

	// Prescaler starts at 341 and subtracts 3 every cycle.
	// 113 cycles * 3 = 339 (prescaler = 2, no clock)
	m.StepCPU(113)
	if m.IRQPending() {
		t.Fatal("IRQ asserted prematurely before 1 scanline")
	}

	// 1 more cycle (total 114 cycles): prescaler drops to <= 0, clocks counter, counter was 0xFF -> trips IRQ!
	m.StepCPU(1)
	if !m.IRQPending() {
		t.Fatal("IRQ should be asserted after 114 CPU cycles in scanline mode")
	}
}

func TestMapperVRC_MicrowireAndWRAM(t *testing.T) {
	t.Parallel()

	// Test VRC2 open bus & latch behavior
	cartVRC2 := createTestVRCCartridge(8, 8, mapperVRC2a, 0)
	cartVRC2.HasBattery = false
	mVRC2 := cartVRC2.Mapper.(*MapperVRC)

	// Write bit 1 to latch
	mVRC2.WritePRG(0x6000, 0x01)
	// Reading from $6100 should return ($61 & 0xFE) | 1 = 0x61
	if got := mVRC2.ReadPRG(0x6100); got != 0x61 {
		t.Errorf("VRC2 latch read got 0x%02X, want 0x61", got)
	}

	// Write bit 0 to latch
	mVRC2.WritePRG(0x6000, 0x00)
	if got := mVRC2.ReadPRG(0x6100); got != 0x60 {
		t.Errorf("VRC2 latch read got 0x%02X, want 0x60", got)
	}

	// Test VRC4 WRAM enable / disable
	cartVRC4 := createTestVRCCartridge(8, 8, mapperVRC2b4e, 1)
	mVRC4 := cartVRC4.Mapper.(*MapperVRC)

	// Initially WRAM is disabled; writes to $6000 are ignored
	mVRC4.WritePRG(0x6000, 0x42)
	// Enable WRAM: $9002 with bit 0 = 1
	mVRC4.WritePRG(0x9002, 0x01)
	mVRC4.WritePRG(0x6000, 0x42)
	if got := mVRC4.ReadPRG(0x6000); got != 0x42 {
		t.Errorf("VRC4 WRAM read got 0x%02X, want 0x42", got)
	}
}

func TestMapperVRC_VariantDecoding(t *testing.T) {
	t.Parallel()

	variants := []struct {
		name      string
		mapperID  byte
		submapper byte
		reg0Addr  uint16
		reg1Addr  uint16
		reg2Addr  uint16
		reg3Addr  uint16
	}{
		{
			name:      "VRC4a (Mapper 21 sub 1)",
			mapperID:  mapperVRC4a4c,
			submapper: 1,
			reg0Addr:  0x8000,
			reg1Addr:  0x8002,
			reg2Addr:  0x8004,
			reg3Addr:  0x8006,
		},
		{
			name:      "VRC4c (Mapper 21 sub 2)",
			mapperID:  mapperVRC4a4c,
			submapper: 2,
			reg0Addr:  0x8000,
			reg1Addr:  0x8040,
			reg2Addr:  0x8080,
			reg3Addr:  0x80C0,
		},
		{
			name:      "VRC4e (Mapper 23 sub 2)",
			mapperID:  mapperVRC2b4e,
			submapper: 2,
			reg0Addr:  0x8000,
			reg1Addr:  0x8004,
			reg2Addr:  0x8008,
			reg3Addr:  0x800C,
		},
		{
			name:      "VRC4b (Mapper 25 sub 1)",
			mapperID:  mapperVRC2c4b,
			submapper: 1,
			reg0Addr:  0x8000,
			reg1Addr:  0x8002,
			reg2Addr:  0x8001,
			reg3Addr:  0x8003,
		},
		{
			name:      "VRC4d (Mapper 25 sub 2)",
			mapperID:  mapperVRC2c4b,
			submapper: 2,
			reg0Addr:  0x8000,
			reg1Addr:  0x8008,
			reg2Addr:  0x8004,
			reg3Addr:  0x800C,
		},
	}

	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			t.Parallel()
			cart := createTestVRCCartridge(8, 8, v.mapperID, v.submapper)
			m := cart.Mapper.(*MapperVRC)

			if r := m.decodeReg(v.reg0Addr); r != 0 {
				t.Errorf("reg0 decoded as %d, want 0", r)
			}
			if r := m.decodeReg(v.reg1Addr); r != 1 {
				t.Errorf("reg1 decoded as %d, want 1", r)
			}
			if r := m.decodeReg(v.reg2Addr); r != 2 {
				t.Errorf("reg2 decoded as %d, want 2", r)
			}
			if r := m.decodeReg(v.reg3Addr); r != 3 {
				t.Errorf("reg3 decoded as %d, want 3", r)
			}
		})
	}
}

func TestMapperVRC_SaveAndLoadState(t *testing.T) {
	t.Parallel()

	cart1 := createTestVRCCartridge(16, 16, mapperVRC2b4e, 1) // VRC4f
	m1 := cart1.Mapper.(*MapperVRC)

	// Set PRG bank 3 at $8000
	m1.WritePRG(0x8000, 3)
	// Set mirroring horizontal (write 1 to $9000)
	m1.WritePRG(0x9000, 1)
	// Set CHR bank 5 at slot 0 ($B000 low, $B001 high)
	m1.WritePRG(0xB000, 5)
	m1.WritePRG(0xB001, 0)
	// Set IRQ latch to 200, enable cycle mode
	m1.WritePRG(0xF000, 0xC8) // low nibble 8
	m1.WritePRG(0xF001, 0x0C) // high nibble C -> 0xC8 = 200
	m1.WritePRG(0xF002, 0x06) // cycle mode + enable

	// Advance 10 cycles
	m1.StepCPU(10)

	var buf bytes.Buffer
	if err := m1.SaveState(&buf); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	cart2 := createTestVRCCartridge(16, 16, mapperVRC2b4e, 1)
	m2 := cart2.Mapper.(*MapperVRC)

	if err := m2.LoadState(&buf); err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if got := m2.ReadPRG(0x8000); got != 3 {
		t.Errorf("ReadPRG($8000) = %d, want 3", got)
	}
	if cart2.Mirroring != MirrorHorizontal {
		t.Errorf("cart2.Mirroring = %v, want MirrorHorizontal", cart2.Mirroring)
	}
	if got := m2.ReadCHR(0x0000); got != 5 {
		t.Errorf("ReadCHR($0000) = %d, want 5", got)
	}
	if m2.irqCounter != 210 {
		t.Errorf("irqCounter = %d, want 210", m2.irqCounter)
	}
	if !m2.irqEnabled {
		t.Errorf("expected irqEnabled to be true")
	}
	if !m2.irqModeCycle {
		t.Errorf("expected irqModeCycle to be true")
	}
}
