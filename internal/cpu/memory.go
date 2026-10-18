// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cpu implements the Ricoh 2A03 (MOS 6502) CPU core used in the NES.
//
// 6502 CPU Memory Map & Hardware Vectors:
//
//	+------------------+ $FFFF
//	| IRQ / BRK Vector | ($FFFE-$FFFF: 16-bit little-endian vector)
//	+------------------+ $FFFD
//	| RESET Vector     | ($FFFC-$FFFD: 16-bit little-endian vector)
//	+------------------+ $FFFB
//	| NMI Vector       | ($FFFA-$FFFB: 16-bit little-endian vector)
//	+------------------+ $8000
//	| Cartridge PRG    | (PRG-ROM / PRG-RAM space)
//	+------------------+ $6000
//	| Expansion / SRAM | (Cartridge save RAM)
//	+------------------+ $4020
//	| APU & I/O Regs   | ($4000-$401F: Sound channels, Joypads, OAM DMA)
//	+------------------+ $2000
//	| PPU Registers    | ($2000-$2007, mirrored every 8 bytes up to $3FFF)
//	+------------------+ $0200
//	| Internal RAM     | ($0200-$07FF: General RAM, mirrored up to $1FFF)
//	+------------------+ $0100
//	| Stack Page       | ($0100-$01FF: 256-byte descending stack pointed by SP)
//	+------------------+ $0000
//	| Zero Page        | ($0000-$00FF: Fast access 256-byte direct memory)
//	+------------------+
package cpu

const (
	// VectorNMI is the memory address of the 16-bit Non-Maskable Interrupt vector.
	VectorNMI uint16 = 0xFFFA
	// VectorReset is the memory address of the 16-bit Reset vector.
	VectorReset uint16 = 0xFFFC
	// VectorIRQ is the memory address of the 16-bit Interrupt Request / BRK vector.
	VectorIRQ uint16 = 0xFFFE

	// StackBaseAddress is the starting address of Page 1 in CPU memory ($0100-$01FF).
	StackBaseAddress uint16 = 0x0100

	// InitialStackPointer is the default stack pointer value ($FD) after CPU reset on the NES.
	InitialStackPointer byte = 0xFD

	// InitialStatusFlags is the default status register value ($24, IRQ disabled and unused bit set).
	InitialStatusFlags byte = 0x24

	// InterruptCycles represents the number of clock cycles consumed by NMI, IRQ, or BRK interrupt entry.
	InterruptCycles int = 7
)

// Memory defines the abstraction for CPU memory bus read and write operations.
type Memory interface {
	Read(addr uint16) byte
	Write(addr uint16, data byte)
}
