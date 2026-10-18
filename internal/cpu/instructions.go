// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpu

// 6502 Instructions Implementation conforming to the Nesdev Instruction Reference.

func (c *CPU) lda(addr uint16) {
	c.A = c.bus.Read(addr)
	c.SetZN(c.A)
}

func (c *CPU) ldx(addr uint16) {
	c.X = c.bus.Read(addr)
	c.SetZN(c.X)
}

func (c *CPU) ldy(addr uint16) {
	c.Y = c.bus.Read(addr)
	c.SetZN(c.Y)
}

func (c *CPU) sta(addr uint16) {
	c.bus.Write(addr, c.A)
}

func (c *CPU) stx(addr uint16) {
	c.bus.Write(addr, c.X)
}

func (c *CPU) sty(addr uint16) {
	c.bus.Write(addr, c.Y)
}

func (c *CPU) tax() {
	c.X = c.A
	c.SetZN(c.X)
}

func (c *CPU) tay() {
	c.Y = c.A
	c.SetZN(c.Y)
}

func (c *CPU) txa() {
	c.A = c.X
	c.SetZN(c.A)
}

func (c *CPU) tya() {
	c.A = c.Y
	c.SetZN(c.A)
}

func (c *CPU) tsx() {
	c.X = c.SP
	c.SetZN(c.X)
}

func (c *CPU) txs() {
	c.SP = c.X
}

func (c *CPU) pha() {
	c.Push(c.A)
}

func (c *CPU) php() {
	c.Push(c.Status | FlagBreak | FlagUnused)
}

func (c *CPU) pla() {
	_ = c.bus.Read(StackBaseAddress + uint16(c.SP))
	c.A = c.Pop()
	c.SetZN(c.A)
}

func (c *CPU) plp() {
	_ = c.bus.Read(StackBaseAddress + uint16(c.SP))
	c.Status = (c.Pop() &^ FlagBreak) | FlagUnused
}

func (c *CPU) and(addr uint16) {
	c.A &= c.bus.Read(addr)
	c.SetZN(c.A)
}

func (c *CPU) eor(addr uint16) {
	c.A ^= c.bus.Read(addr)
	c.SetZN(c.A)
}

func (c *CPU) ora(addr uint16) {
	c.A |= c.bus.Read(addr)
	c.SetZN(c.A)
}

func (c *CPU) bit(addr uint16) {
	val := c.bus.Read(addr)
	c.SetFlag(FlagZero, (c.A&val) == 0)
	c.SetFlag(FlagNegative, (val&FlagNegative) != 0)
	c.SetFlag(FlagOverflow, (val&FlagOverflow) != 0)
}

func (c *CPU) adc(val byte) {
	carry := byte(0)
	if c.GetFlag(FlagCarry) {
		carry = 1
	}

	sum := uint16(c.A) + uint16(val) + uint16(carry)
	c.SetFlag(FlagCarry, sum > 0xFF)

	// Overflow occurs when the sign of both inputs is the same, but different from the result.
	overflow := ^(c.A ^ val) & (c.A ^ byte(sum&0x00FF)) & MaskSignBit
	c.SetFlag(FlagOverflow, overflow != 0)

	c.A = byte(sum & 0x00FF)
	c.SetZN(c.A)
}

func (c *CPU) sbc(val byte) {
	// SBC is identical to ADC with the one's complement of the operand (^val).
	c.adc(^val)
}

func (c *CPU) cmp(reg byte, addr uint16) {
	val := c.bus.Read(addr)
	c.SetFlag(FlagCarry, reg >= val)
	c.SetZN(reg - val)
}

func (c *CPU) inc(addr uint16) {
	val := c.bus.Read(addr)
	c.bus.Write(addr, val)
	val++
	c.bus.Write(addr, val)
	c.SetZN(val)
}

func (c *CPU) inx() {
	c.X++
	c.SetZN(c.X)
}

func (c *CPU) iny() {
	c.Y++
	c.SetZN(c.Y)
}

func (c *CPU) dec(addr uint16) {
	val := c.bus.Read(addr)
	c.bus.Write(addr, val)
	val--
	c.bus.Write(addr, val)
	c.SetZN(val)
}

func (c *CPU) dex() {
	c.X--
	c.SetZN(c.X)
}

func (c *CPU) dey() {
	c.Y--
	c.SetZN(c.Y)
}

func (c *CPU) aslAcc() {
	c.SetFlag(FlagCarry, (c.A&MaskSignBit) != 0)
	c.A <<= 1
	c.SetZN(c.A)
}

func (c *CPU) aslMem(addr uint16) {
	val := c.bus.Read(addr)
	c.bus.Write(addr, val)
	c.SetFlag(FlagCarry, (val&MaskSignBit) != 0)
	val <<= 1
	c.bus.Write(addr, val)
	c.SetZN(val)
}

func (c *CPU) lsrAcc() {
	c.SetFlag(FlagCarry, (c.A&0x01) != 0)
	c.A >>= 1
	c.SetZN(c.A)
}

func (c *CPU) lsrMem(addr uint16) {
	val := c.bus.Read(addr)
	c.bus.Write(addr, val)
	c.SetFlag(FlagCarry, (val&0x01) != 0)
	val >>= 1
	c.bus.Write(addr, val)
	c.SetZN(val)
}

func (c *CPU) rolAcc() {
	carry := byte(0)
	if c.GetFlag(FlagCarry) {
		carry = 1
	}
	c.SetFlag(FlagCarry, (c.A&MaskSignBit) != 0)
	c.A = (c.A << 1) | carry
	c.SetZN(c.A)
}

func (c *CPU) rolMem(addr uint16) {
	val := c.bus.Read(addr)
	c.bus.Write(addr, val)
	carry := byte(0)
	if c.GetFlag(FlagCarry) {
		carry = 1
	}
	c.SetFlag(FlagCarry, (val&MaskSignBit) != 0)
	val = (val << 1) | carry
	c.bus.Write(addr, val)
	c.SetZN(val)
}

func (c *CPU) rorAcc() {
	carry := byte(0)
	if c.GetFlag(FlagCarry) {
		carry = MaskSignBit
	}
	c.SetFlag(FlagCarry, (c.A&0x01) != 0)
	c.A = (c.A >> 1) | carry
	c.SetZN(c.A)
}

func (c *CPU) rorMem(addr uint16) {
	val := c.bus.Read(addr)
	c.bus.Write(addr, val)
	carry := byte(0)
	if c.GetFlag(FlagCarry) {
		carry = MaskSignBit
	}
	c.SetFlag(FlagCarry, (val&0x01) != 0)
	val = (val >> 1) | carry
	c.bus.Write(addr, val)
	c.SetZN(val)
}

func (c *CPU) jmp(addr uint16) {
	c.PC = addr
}

func (c *CPU) jsr(addr uint16) {
	// JSR pushes the return address minus 1 (address of the 3rd byte of JSR instruction).
	c.PushWord(c.PC - 1)
	c.PC = addr
}

func (c *CPU) rts() {
	_ = c.bus.Read(StackBaseAddress + uint16(c.SP))
	low := uint16(c.Pop())
	high := uint16(c.Pop())
	addr := (high << 8) | low
	_ = c.bus.Read(addr)
	c.PC = addr + 1
}

func (c *CPU) rti() {
	_ = c.bus.Read(StackBaseAddress + uint16(c.SP))
	c.Status = (c.Pop() &^ FlagBreak) | FlagUnused
	c.PC = c.PopWord()
}

func (c *CPU) branch(cond bool, addr uint16, pageCrossed bool) int {
	if !cond {
		return 0
	}
	cycles := 1
	_ = c.bus.Read(c.PC)
	if pageCrossed {
		cycles++
		dummyAddr := (c.PC & maskPageBoundary) | (addr & maskLowByte)
		_ = c.bus.Read(dummyAddr)
	}
	c.PC = addr
	return cycles
}

func (c *CPU) brk() {
	c.PC++
	c.PushWord(c.PC)
	c.Push(c.Status | FlagBreak | FlagUnused)
	c.SetFlag(FlagInterrupt, true)
	c.PC = c.readWord(VectorIRQ)
}

// Undocumented / Illegal Opcode Implementations for nestest compliance.

func (c *CPU) lax(addr uint16) {
	c.A = c.bus.Read(addr)
	c.X = c.A
	c.SetZN(c.A)
}

func (c *CPU) sax(addr uint16) {
	c.bus.Write(addr, c.A&c.X)
}

func (c *CPU) dcp(addr uint16) {
	val := c.bus.Read(addr)
	c.bus.Write(addr, val)
	val--
	c.bus.Write(addr, val)
	c.SetFlag(FlagCarry, c.A >= val)
	c.SetZN(c.A - val)
}

func (c *CPU) isc(addr uint16) {
	val := c.bus.Read(addr)
	c.bus.Write(addr, val)
	val++
	c.bus.Write(addr, val)
	c.sbc(val)
}

func (c *CPU) slo(addr uint16) {
	val := c.bus.Read(addr)
	c.bus.Write(addr, val)
	c.SetFlag(FlagCarry, (val&MaskSignBit) != 0)
	val <<= 1
	c.bus.Write(addr, val)
	c.A |= val
	c.SetZN(c.A)
}

func (c *CPU) rla(addr uint16) {
	val := c.bus.Read(addr)
	c.bus.Write(addr, val)
	carry := byte(0)
	if c.GetFlag(FlagCarry) {
		carry = 1
	}
	c.SetFlag(FlagCarry, (val&MaskSignBit) != 0)
	val = (val << 1) | carry
	c.bus.Write(addr, val)
	c.A &= val
	c.SetZN(c.A)
}

func (c *CPU) sre(addr uint16) {
	val := c.bus.Read(addr)
	c.bus.Write(addr, val)
	c.SetFlag(FlagCarry, (val&0x01) != 0)
	val >>= 1
	c.bus.Write(addr, val)
	c.A ^= val
	c.SetZN(c.A)
}

func (c *CPU) rra(addr uint16) {
	val := c.bus.Read(addr)
	c.bus.Write(addr, val)
	carry := byte(0)
	if c.GetFlag(FlagCarry) {
		carry = MaskSignBit
	}
	c.SetFlag(FlagCarry, (val&0x01) != 0)
	val = (val >> 1) | carry
	c.bus.Write(addr, val)
	c.adc(val)
}

func (c *CPU) anc(val byte) {
	c.A &= val
	c.SetZN(c.A)
	c.SetFlag(FlagCarry, (c.A&FlagNegative) != 0)
}

func (c *CPU) alr(val byte) {
	v := c.A & val
	c.SetFlag(FlagCarry, (v&FlagCarry) != 0)
	c.A = v >> 1
	c.SetZN(c.A)
}

func (c *CPU) arr(val byte) {
	v := c.A & val
	carryIn := byte(0)
	if c.GetFlag(FlagCarry) {
		carryIn = MaskSignBit
	}
	c.A = (v >> 1) | carryIn
	c.SetZN(c.A)
	bit6 := (c.A & 0x40) != 0
	bit5 := (c.A & 0x20) != 0
	c.SetFlag(FlagCarry, bit6)
	c.SetFlag(FlagOverflow, bit6 != bit5)
}

const unoffMagic byte = 0xEE

func (c *CPU) xaa(val byte) {
	c.A = (c.A | unoffMagic) & c.X & val
	c.SetZN(c.A)
}

func (c *CPU) lxa(val byte) {
	res := (c.A | unoffMagic) & val
	c.A = res
	c.X = res
	c.SetZN(res)
}

func (c *CPU) axs(val byte) {
	diff := int(c.A&c.X) - int(val)
	c.X = byte(diff & 0xFF)
	c.SetZN(c.X)
	c.SetFlag(FlagCarry, diff >= 0)
}

func (c *CPU) las(val byte) {
	res := val & c.SP
	c.A = res
	c.X = res
	c.SP = res
	c.SetZN(res)
}

func (c *CPU) sha(addr uint16, pageCrossed bool, indexReg byte) {
	base := addr - uint16(indexReg)
	h := byte(base>>8) + 1
	val := c.A & c.X & h
	target := addr
	if pageCrossed {
		target = (addr & 0x00FF) | (uint16(val) << 8)
	}
	c.bus.Write(target, val)
}

func (c *CPU) shx(addr uint16, pageCrossed bool) {
	base := addr - uint16(c.Y)
	h := byte(base>>8) + 1
	val := c.X & h
	target := addr
	if pageCrossed {
		target = (addr & 0x00FF) | (uint16(val) << 8)
	}
	c.bus.Write(target, val)
}

func (c *CPU) shy(addr uint16, pageCrossed bool) {
	base := addr - uint16(c.X)
	h := byte(base>>8) + 1
	val := c.Y & h
	target := addr
	if pageCrossed {
		target = (addr & 0x00FF) | (uint16(val) << 8)
	}
	c.bus.Write(target, val)
}

func (c *CPU) shs(addr uint16, pageCrossed bool) {
	c.SP = c.A & c.X
	base := addr - uint16(c.Y)
	h := byte(base>>8) + 1
	val := c.SP & h
	target := addr
	if pageCrossed {
		target = (addr & 0x00FF) | (uint16(val) << 8)
	}
	c.bus.Write(target, val)
}

