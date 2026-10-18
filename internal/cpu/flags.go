// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpu

// 6502 Status Register (P) Bitmask Definitions:
//
//	7   6   5   4   3   2   1   0
//	N   V   U   B   D   I   Z   C
//	|   |   |   |   |   |   |   +-- (C) Carry Flag: Set if last operation resulted in carry/overflow or no-borrow.
//	|   |   |   |   |   |   +------ (Z) Zero Flag: Set if last operation resulted in a zero value.
//	|   |   |   |   |   +---------- (I) Interrupt Disable: Set to ignore maskable IRQ interrupts.
//	|   |   |   |   +-------------- (D) Decimal Mode: Decimal BCD mode flag (disabled in NES Ricoh 2A03 ALU).
//	|   |   |   +------------------ (B) Break Command: Set when pushed to stack by BRK or PHP instructions.
//	|   |   +---------------------- (U) Unused / Reserved: Always 1 when pushed to the stack.
//	|   +-------------------------- (V) Overflow Flag: Set if signed arithmetic produced a two's-complement overflow.
//	+------------------------------ (N) Negative Flag: Set if bit 7 of the result is 1 (negative signed value).
const (
	// FlagCarry is bit 0 of the status register.
	FlagCarry byte = 1 << 0
	// FlagZero is bit 1 of the status register.
	FlagZero byte = 1 << 1
	// FlagInterrupt is bit 2 of the status register.
	FlagInterrupt byte = 1 << 2
	// FlagDecimal is bit 3 of the status register.
	FlagDecimal byte = 1 << 3
	// FlagBreak is bit 4 of the status register.
	FlagBreak byte = 1 << 4
	// FlagUnused is bit 5 of the status register (always set to 1 in stack pushes).
	FlagUnused byte = 1 << 5
	// FlagOverflow is bit 6 of the status register.
	FlagOverflow byte = 1 << 6
	// FlagNegative is bit 7 of the status register.
	FlagNegative byte = 1 << 7

	// MaskSignBit is the mask to test the negative sign bit (bit 7).
	MaskSignBit byte = 0x80
)

// SetFlag sets or clears a specific flag bit in the CPU status register.
func (c *CPU) SetFlag(flag byte, set bool) {
	if set {
		c.Status |= flag
		return
	}
	c.Status &= ^flag
}

// GetFlag returns true if the specified status flag bit is set.
func (c *CPU) GetFlag(flag byte) bool {
	return (c.Status & flag) != 0
}

// SetZN updates the Zero (Z) and Negative (N) flags based on the provided 8-bit result value.
func (c *CPU) SetZN(val byte) {
	c.SetFlag(FlagZero, val == 0)
	c.SetFlag(FlagNegative, (val&MaskSignBit) != 0)
}
