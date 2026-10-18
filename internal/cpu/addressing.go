// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpu

// 6502 Addressing Mode Technical Specification:
//
//	1. Implied:          No operand or implied register (e.g. NOP, TAX).
//	2. Accumulator:      Operates directly on the accumulator register A (e.g. ASL A).
//	3. Immediate:        Operand is a constant byte stored in the byte following the opcode (#$10).
//	4. Zero Page:        Operand address is in the first 256 bytes of memory ($0000-$00FF).
//	5. Zero Page, X:     Zero page address indexed by X register with 8-bit wrap-around: ($zp + X) & $FF.
//	6. Zero Page, Y:     Zero page address indexed by Y register with 8-bit wrap-around: ($zp + Y) & $FF.
//	7. Absolute:         Full 16-bit address operand specified in little-endian format (low byte, high byte).
//	8. Absolute, X:      16-bit address indexed with register X. Crosses page boundary if (base & $FF00) != (eff & $FF00).
//	9. Absolute, Y:      16-bit address indexed with register Y. Crosses page boundary if (base & $FF00) != (eff & $FF00).
//	10. Relative:        8-bit signed branch displacement (-128 to +127) added to PC upon branch.
//	11. Indirect:        16-bit pointer to target address (used by JMP). Replicates 6502 $xxFF page boundary bug.
//	12. Indexed Ind (X): Pre-indexed: 8-bit ZP pointer offset by X with wrap-around to fetch 16-bit address ($zp + X).
//	13. Ind Indexed (Y): Post-indexed: Fetches 16-bit address from ZP pointer, then adds Y. Detects page boundary crossing.

// AddressingMode represents one of the 13 addressing modes supported by the 6502.
type AddressingMode byte

// AddressingMode constants represent each of the 13 supported addressing modes.
const (
	ModeImplied AddressingMode = iota
	ModeAccumulator
	ModeImmediate
	ModeZeroPage
	ModeZeroPageX
	ModeZeroPageY
	ModeAbsolute
	ModeAbsoluteX
	ModeAbsoluteY
	ModeRelative
	ModeIndirect
	ModeIndirectX
	ModeIndirectY
)

const (
	// MaskPageBoundary is used to detect crossing between 256-byte pages ($xx00).
	maskPageBoundary uint16 = 0xFF00
	// MaskZeroPageWrap is used to force 8-bit address wrap-around within Zero Page ($0000-$00FF).
	maskZeroPageWrap byte = 0xFF
	// maskLowByte isolates the low 8 bits of a 16-bit address ($00FF).
	maskLowByte uint16 = 0x00FF
	// LowBytePageWrapTarget is the $xxFF boundary condition triggering the 6502 JMP ($xxFF) bug.
	lowBytePageWrapTarget uint16 = 0x00FF
)

// resolveAddress calculates the effective memory address and reports if a 256-byte page boundary was crossed.
func (c *CPU) resolveAddress(mode AddressingMode, isWriteOrRMW bool) (addr uint16, pageCrossed bool) {
	switch mode {
	case ModeImmediate:
		addr = c.PC
		c.PC++
		return addr, false

	case ModeZeroPage:
		addr = uint16(c.bus.Read(c.PC))
		c.PC++
		return addr, false

	case ModeZeroPageX:
		zp := c.bus.Read(c.PC)
		c.PC++
		addr = uint16((zp + c.X) & maskZeroPageWrap)
		return addr, false

	case ModeZeroPageY:
		zp := c.bus.Read(c.PC)
		c.PC++
		addr = uint16((zp + c.Y) & maskZeroPageWrap)
		return addr, false

	case ModeAbsolute:
		addr = c.readWord(c.PC)
		c.PC += 2
		return addr, false

	case ModeAbsoluteX:
		base := c.readWord(c.PC)
		c.PC += 2
		addr = base + uint16(c.X)
		pageCrossed = (base & maskPageBoundary) != (addr & maskPageBoundary)
		if isWriteOrRMW || pageCrossed {
			dummyAddr := (base & maskPageBoundary) | (addr & maskLowByte)
			_ = c.bus.Read(dummyAddr)
		}
		return addr, pageCrossed

	case ModeAbsoluteY:
		base := c.readWord(c.PC)
		c.PC += 2
		addr = base + uint16(c.Y)
		pageCrossed = (base & maskPageBoundary) != (addr & maskPageBoundary)
		if isWriteOrRMW || pageCrossed {
			dummyAddr := (base & maskPageBoundary) | (addr & maskLowByte)
			_ = c.bus.Read(dummyAddr)
		}
		return addr, pageCrossed

	case ModeRelative:
		offset := c.bus.Read(c.PC)
		c.PC++
		addr = c.PC + uint16(offset)
		if offset >= MaskSignBit {
			addr -= 0x0100
		}
		pageCrossed = (c.PC & maskPageBoundary) != (addr & maskPageBoundary)
		return addr, pageCrossed

	case ModeIndirect:
		ptr := c.readWord(c.PC)
		c.PC += 2
		addr = c.readIndirectWord(ptr)
		return addr, false

	case ModeIndirectX:
		zp := c.bus.Read(c.PC)
		c.PC++
		ptr := (zp + c.X) & maskZeroPageWrap
		low := uint16(c.bus.Read(uint16(ptr)))
		high := uint16(c.bus.Read(uint16((ptr + 1) & maskZeroPageWrap)))
		addr = (high << 8) | low
		return addr, false

	case ModeIndirectY:
		zp := c.bus.Read(c.PC)
		c.PC++
		low := uint16(c.bus.Read(uint16(zp)))
		high := uint16(c.bus.Read(uint16((zp + 1) & maskZeroPageWrap)))
		base := (high << 8) | low
		addr = base + uint16(c.Y)
		pageCrossed = (base & maskPageBoundary) != (addr & maskPageBoundary)
		if isWriteOrRMW || pageCrossed {
			dummyAddr := (base & maskPageBoundary) | (addr & maskLowByte)
			_ = c.bus.Read(dummyAddr)
		}
		return addr, pageCrossed

	case ModeImplied, ModeAccumulator:
		_ = c.bus.Read(c.PC)
		return 0, false

	default:
		return 0, false
	}
}

// readWord reads a 16-bit little-endian word from memory at the specified address.
func (c *CPU) readWord(addr uint16) uint16 {
	low := uint16(c.bus.Read(addr))
	high := uint16(c.bus.Read(addr + 1))
	return (high << 8) | low
}

// readIndirectWord reads a 16-bit address from a pointer, replicating the 6502 hardware bug
// where JMP ($xxFF) fetches the high byte from $xx00 instead of $xx00 + $0100.
func (c *CPU) readIndirectWord(ptr uint16) uint16 {
	low := uint16(c.bus.Read(ptr))
	if (ptr & lowBytePageWrapTarget) == lowBytePageWrapTarget {
		high := uint16(c.bus.Read(ptr & maskPageBoundary))
		return (high << 8) | low
	}

	high := uint16(c.bus.Read(ptr + 1))
	return (high << 8) | low
}
