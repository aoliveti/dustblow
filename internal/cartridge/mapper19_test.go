// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"bytes"
	"testing"

	"github.com/dustblow/dustblow/internal/cartridge"
)

func createTestCartridgeMapper19() *cartridge.Cartridge {
	// 64KB PRG (8 banks of 8KB), 16KB CHR (16 banks of 1KB)
	prg := make([]byte, 64*1024)
	for i := range prg {
		prg[i] = byte(i / 0x2000)
	}

	chr := make([]byte, 16*1024)
	for i := range chr {
		chr[i] = byte(i / 0x0400)
	}

	cart := &cartridge.Cartridge{
		PRG:       prg,
		CHR:       chr,
		MapperID:  19,
		Mirroring: cartridge.MirrorVertical,
	}

	cartridge.NewMapper19(cart)
	return cart
}

func TestMapper19_PRGBanking(t *testing.T) {
	t.Parallel()

	cart := createTestCartridgeMapper19()

	// Initial banks: $8000 -> bank 0, $A000 -> bank 1, $C000 -> bank 2, $E000 -> fixed bank 7
	if got := cart.ReadPRG(0x8000); got != 0 {
		t.Fatalf("expected PRG bank 0 at $8000, got %d", got)
	}
	if got := cart.ReadPRG(0xA000); got != 1 {
		t.Fatalf("expected PRG bank 1 at $A000, got %d", got)
	}
	if got := cart.ReadPRG(0xC000); got != 2 {
		t.Fatalf("expected PRG bank 2 at $C000, got %d", got)
	}
	if got := cart.ReadPRG(0xE000); got != 7 {
		t.Fatalf("expected PRG bank 7 at $E000, got %d", got)
	}

	// Switch banks: $8000 to bank 4, $A000 to bank 5, $C000 to bank 3
	cart.WritePRG(0xE000, 4)
	cart.WritePRG(0xE800, 5)
	cart.WritePRG(0xF000, 3)

	if got := cart.ReadPRG(0x8000); got != 4 {
		t.Fatalf("expected PRG bank 4 at $8000, got %d", got)
	}
	if got := cart.ReadPRG(0xA000); got != 5 {
		t.Fatalf("expected PRG bank 5 at $A000, got %d", got)
	}
	if got := cart.ReadPRG(0xC000); got != 3 {
		t.Fatalf("expected PRG bank 3 at $C000, got %d", got)
	}
	// $E000 remains fixed to 7
	if got := cart.ReadPRG(0xE000); got != 7 {
		t.Fatalf("expected PRG bank 7 at $E000, got %d", got)
	}
}

func TestMapper19_CHRandNametableBanking(t *testing.T) {
	t.Parallel()

	cart := createTestCartridgeMapper19()

	// Switch CHR slot 0 ($0000) to bank 3, slot 7 ($1C00) to bank 9
	cart.WritePRG(0x8000, 3)
	cart.WritePRG(0xB800, 9)

	if got := cart.ReadCHR(0x0000); got != 3 {
		t.Fatalf("expected CHR bank 3 at $0000, got %d", got)
	}
	if got := cart.ReadCHR(0x1C00); got != 9 {
		t.Fatalf("expected CHR bank 9 at $1C00, got %d", got)
	}

	// Nametables:
	vram := make([]byte, 2048)
	vram[0] = 0xAA    // CIRAM page 0
	vram[1024] = 0xBB // CIRAM page 1

	// Set NT slot 0 to CIRAM page 0 ($E0) and slot 1 to CIRAM page 1 ($E1)
	cart.WritePRG(0xC000, 0xE0)
	cart.WritePRG(0xC800, 0xE1)

	val0, ok0 := cart.ReadNametable(0x2000, vram)
	if !ok0 || val0 != 0xAA {
		t.Fatalf("expected CIRAM page 0 (0xAA), got %02X (ok=%v)", val0, ok0)
	}

	val1, ok1 := cart.ReadNametable(0x2400, vram)
	if !ok1 || val1 != 0xBB {
		t.Fatalf("expected CIRAM page 1 (0xBB), got %02X (ok=%v)", val1, ok1)
	}

	// Set NT slot 2 to CHR-ROM bank 5 (< $E0)
	cart.WritePRG(0xD000, 5)
	val2, ok2 := cart.ReadNametable(0x2800, vram)
	if !ok2 || val2 != 5 {
		t.Fatalf("expected CHR-ROM bank 5, got %02X (ok=%v)", val2, ok2)
	}
}

func TestMapper19_InternalRAMandIRQ(t *testing.T) {
	t.Parallel()

	cart := createTestCartridgeMapper19()

	// Write internal RAM via port $F800 and $4800 with auto-increment
	cart.WritePRG(0xF800, 0x10|0x80) // address $10, auto-increment on
	cart.WritePRG(0x4800, 0x42)      // write to $10
	cart.WritePRG(0x4800, 0x99)      // auto-increment to $11, write 0x99

	// Read back
	cart.WritePRG(0xF800, 0x10|0x80)
	if got := cart.ReadPRG(0x4800); got != 0x42 {
		t.Fatalf("expected RAM at $10 to be 0x42, got %02X", got)
	}
	if got := cart.ReadPRG(0x4800); got != 0x99 {
		t.Fatalf("expected RAM at $11 to be 0x99, got %02X", got)
	}

	// IRQ Counter:
	// Set IRQ counter to 0x7FF0 and enable counting (bit 7 of $5800)
	cart.WritePRG(0x5000, 0xF0)      // low 8 bits
	cart.WritePRG(0x5800, 0x7F|0x80) // high 7 bits 0x7F, enable bit = 1

	if cart.IRQPending() {
		t.Fatalf("IRQ should not be pending yet")
	}

	// Step 10 cycles -> counter becomes 0x7FFA
	cart.StepCPU(10)
	if cart.IRQPending() {
		t.Fatalf("IRQ should not be pending at 0x7FFA")
	}

	// Step 10 cycles -> hits 0x7FFF target and triggers IRQ
	cart.StepCPU(10)
	if !cart.IRQPending() {
		t.Fatalf("expected IRQ to be pending after hitting $7FFF")
	}

	// Acknowledging IRQ by writing to $5000
	cart.WritePRG(0x5000, 0x00)
	if cart.IRQPending() {
		t.Fatalf("expected IRQ pending to be cleared on $5000 write")
	}
}

func TestMapper19_SaveAndLoadState(t *testing.T) {
	t.Parallel()

	cart := createTestCartridgeMapper19()

	// Setup custom state
	cart.WritePRG(0xE000, 3)
	cart.WritePRG(0xE800, 4)
	cart.WritePRG(0xF000, 5)
	cart.WritePRG(0x8000, 6)
	cart.WritePRG(0xC000, 0xE1)
	cart.WritePRG(0xF800, 0x25|0x80)
	cart.WritePRG(0x4800, 0x77)
	cart.WritePRG(0x5000, 0x34)
	cart.WritePRG(0x5800, 0x12|0x80)

	var buf bytes.Buffer
	if err := cart.SaveState(&buf); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	// Create clean instance and alter its state
	cart2 := createTestCartridgeMapper19()
	cart2.WritePRG(0xE000, 0)
	cart2.WritePRG(0xE800, 0)
	cart2.WritePRG(0xF000, 0)

	// Restore state
	if err := cart2.LoadState(&buf); err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	// Verify restored banking and registers
	if got := cart2.ReadPRG(0x8000); got != 3 {
		t.Errorf("expected restored PRG bank 3 at $8000, got %d", got)
	}
	if got := cart2.ReadPRG(0xA000); got != 4 {
		t.Errorf("expected restored PRG bank 4 at $A000, got %d", got)
	}
	if got := cart2.ReadPRG(0xC000); got != 5 {
		t.Errorf("expected restored PRG bank 5 at $C000, got %d", got)
	}
	if got := cart2.ReadCHR(0x0000); got != 6 {
		t.Errorf("expected restored CHR bank 6 at $0000, got %d", got)
	}

	// Verify RAM
	cart2.WritePRG(0xF800, 0x25)
	if got := cart2.ReadPRG(0x4800); got != 0x77 {
		t.Errorf("expected restored internal RAM byte 0x77, got %02X", got)
	}

	// Verify IRQ counter
	low := cart2.ReadPRG(0x5000)
	high := cart2.ReadPRG(0x5800)
	if low != 0x34 || (high&0x7F) != 0x12 || (high&0x80) == 0 {
		t.Errorf("expected restored IRQ counter 0x1234 enabled, got low=%02X high=%02X", low, high)
	}
}
