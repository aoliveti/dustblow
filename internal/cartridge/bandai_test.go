// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"bytes"
	"testing"

	"github.com/aoliveti/dustblow/internal/cartridge"
)

func createTestCartridgeBandai(mapperID byte) *cartridge.Cartridge {
	// 128KB PRG (8 banks of 16KB), 16KB CHR (16 banks of 1KB)
	prg := make([]byte, 128*1024)
	for i := range prg {
		prg[i] = byte(i / 0x4000)
	}

	chr := make([]byte, 16*1024)
	for i := range chr {
		chr[i] = byte(i / 0x0400)
	}

	cart := &cartridge.Cartridge{
		PRG:       prg,
		CHR:       chr,
		MapperID:  mapperID,
		Mirroring: cartridge.MirrorVertical,
	}

	cartridge.NewMapperBandai(cart, mapperID)
	return cart
}

func TestBandai_PRGAndCHRandMirroring(t *testing.T) {
	t.Parallel()

	cart := createTestCartridgeBandai(16)

	// Initial banks: $8000 -> bank 0, $C000 -> fixed bank 7
	if got := cart.ReadPRG(0x8000); got != 0 {
		t.Fatalf("expected PRG bank 0 at $8000, got %d", got)
	}
	if got := cart.ReadPRG(0xC000); got != 7 {
		t.Fatalf("expected PRG bank 7 at $C000, got %d", got)
	}

	// Switch PRG bank to 5 ($8008)
	cart.WritePRG(0x8008, 5)
	if got := cart.ReadPRG(0x8000); got != 5 {
		t.Fatalf("expected PRG bank 5 at $8000, got %d", got)
	}
	if got := cart.ReadPRG(0xC000); got != 7 {
		t.Fatalf("expected fixed PRG bank 7 at $C000, got %d", got)
	}

	// Switch CHR bank 0 to 4 ($8000), CHR bank 7 to 9 ($8007)
	cart.WritePRG(0x8000, 4)
	cart.WritePRG(0x8007, 9)

	if got := cart.ReadCHR(0x0000); got != 4 {
		t.Fatalf("expected CHR bank 4 at $0000, got %d", got)
	}
	if got := cart.ReadCHR(0x1C00); got != 9 {
		t.Fatalf("expected CHR bank 9 at $1C00, got %d", got)
	}

	// Test dynamic mirroring ($8009)
	cart.WritePRG(0x8009, 1) // Horizontal
	if cart.Mirroring != cartridge.MirrorHorizontal {
		t.Fatalf("expected MirrorHorizontal, got %v", cart.Mirroring)
	}

	cart.WritePRG(0x8009, 2) // Single 0
	if cart.Mirroring != cartridge.MirrorSingle0 {
		t.Fatalf("expected MirrorSingle0, got %v", cart.Mirroring)
	}
}

func TestBandai_IRQCounter(t *testing.T) {
	t.Parallel()

	cart := createTestCartridgeBandai(16)

	// Set latch to 10 ($800B low = 10, $800C high = 0)
	cart.WritePRG(0x800B, 10)
	cart.WritePRG(0x800C, 0)

	// Enable IRQ ($800A = 1) -> loads latch into counter
	cart.WritePRG(0x800A, 1)

	if cart.IRQPending() {
		t.Fatalf("IRQ should not be pending yet")
	}

	// Step 5 cycles -> counter is 5
	cart.StepCPU(5)
	if cart.IRQPending() {
		t.Fatalf("IRQ should not be pending after 5 cycles")
	}

	// Step 5 cycles -> counter reaches 0 -> triggers IRQ
	cart.StepCPU(5)
	if !cart.IRQPending() {
		t.Fatalf("expected IRQ to be pending after hitting 0")
	}

	// Disable/acknowledge IRQ
	cart.WritePRG(0x800A, 0)
	if cart.IRQPending() {
		t.Fatalf("expected IRQ to be cleared after write to $800A")
	}
}

// Helper to send a bit on I2C bus via $800D:
// bit 0 = SCL, bit 1 = SDA.
func sendI2CBit(cart *cartridge.Cartridge, bit byte) {
	// SCL low, set SDA
	cart.WritePRG(0x800D, bit<<1)
	// SCL high, keep SDA
	cart.WritePRG(0x800D, (bit<<1)|1)
	// SCL low
	cart.WritePRG(0x800D, bit<<1)
}

func sendI2CByte(cart *cartridge.Cartridge, b byte) {
	for i := 7; i >= 0; i-- {
		bit := (b >> i) & 1
		sendI2CBit(cart, bit)
	}
	// 9th clock pulse for ACK (master reads ACK, so SDA released high/1)
	cart.WritePRG(0x800D, 2) // SDA high, SCL low
	cart.WritePRG(0x800D, 3) // SDA high, SCL high
	cart.WritePRG(0x800D, 2) // SDA high, SCL low
}

func sendI2CStart(cart *cartridge.Cartridge) {
	cart.WritePRG(0x800D, 3) // SCL high, SDA high
	cart.WritePRG(0x800D, 1) // SCL high, SDA low (START)
	cart.WritePRG(0x800D, 0) // SCL low, SDA low
}

func sendI2CStop(cart *cartridge.Cartridge) {
	cart.WritePRG(0x800D, 0) // SCL low, SDA low
	cart.WritePRG(0x800D, 1) // SCL high, SDA low
	cart.WritePRG(0x800D, 3) // SCL high, SDA high (STOP)
}

func TestBandai_I2CEEPROM(t *testing.T) {
	t.Parallel()

	cart := createTestCartridgeBandai(159) // 24C01 (128 bytes)

	// Write byte 0x5A to address 0x14
	sendI2CStart(cart)
	sendI2CByte(cart, 0xA0) // Device address, Write
	sendI2CByte(cart, 0x14) // Word address
	sendI2CByte(cart, 0x5A) // Data byte
	sendI2CStop(cart)

	// Verify byte was written to cartridge SRAM at index 0x14
	if cart.SRAM[0x14] != 0x5A {
		t.Fatalf("expected SRAM[0x14] to be 0x5A, got %02X", cart.SRAM[0x14])
	}

	// Read byte back via I2C
	sendI2CStart(cart)
	sendI2CByte(cart, 0xA0) // Set address
	sendI2CByte(cart, 0x14)
	sendI2CStart(cart)      // Repeated start
	sendI2CByte(cart, 0xA1) // Device address, Read

	// Shift out 8 bits from $6000
	var readByte byte
	for range 8 {
		// SCL low -> device updates SDA
		cart.WritePRG(0x800D, 0)
		// SCL high -> master reads SDA from $6000
		cart.WritePRG(0x800D, 1)
		bit := cart.ReadPRG(0x6000) & 1
		readByte = (readByte << 1) | bit
	}
	sendI2CStop(cart)

	if readByte != 0x5A {
		t.Fatalf("expected read byte 0x5A, got %02X", readByte)
	}
}

func TestBandai_SaveAndLoadState(t *testing.T) {
	t.Parallel()

	cart := createTestCartridgeBandai(159)

	// Set custom state
	cart.WritePRG(0x8008, 3)
	cart.WritePRG(0x8000, 6)
	cart.WritePRG(0x8009, 1) // MirrorHorizontal
	cart.WritePRG(0x800B, 42)
	cart.WritePRG(0x800C, 1)
	cart.WritePRG(0x800A, 1) // IRQ enabled

	var buf bytes.Buffer
	if err := cart.SaveState(&buf); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	// Alter state
	cart2 := createTestCartridgeBandai(159)
	cart2.WritePRG(0x8008, 0)
	cart2.WritePRG(0x8000, 0)

	// Restore state
	if err := cart2.LoadState(&buf); err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	if got := cart2.ReadPRG(0x8000); got != 3 {
		t.Errorf("expected restored PRG bank 3, got %d", got)
	}
	if got := cart2.ReadCHR(0x0000); got != 6 {
		t.Errorf("expected restored CHR bank 6, got %d", got)
	}
	if cart2.Mirroring != cartridge.MirrorHorizontal {
		t.Errorf("expected restored MirrorHorizontal, got %v", cart2.Mirroring)
	}
}
