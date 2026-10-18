// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpu

const opcodeJSR byte = 0x20

// State represents an immutable point-in-time snapshot of the CPU registers and cycle count.
// Field order is optimized for memory alignment (16 bytes total).
type State struct {
	Cycles uint64
	PC     uint16
	A      byte
	X      byte
	Y      byte
	SP     byte
	Status byte
}

type cycleCountingBus struct {
	realBus  Memory
	subCycle int
}

func (cb *cycleCountingBus) Read(addr uint16) byte {
	cb.subCycle++
	return cb.realBus.Read(addr)
}

func (cb *cycleCountingBus) Write(addr uint16, data byte) {
	cb.subCycle++
	cb.realBus.Write(addr, data)
}

// CPU represents the state and registers of the Ricoh 2A03 (MOS 6502) microprocessor.
// Field order is optimized for 8-byte boundary alignment.
type CPU struct {
	Cycles uint64
	bus    cycleCountingBus
	PC     uint16
	A      byte
	X      byte
	Y      byte
	SP     byte
	Status byte
}

// New creates and initializes a new 6502 CPU instance connected to the specified memory bus.
func New(bus Memory) *CPU {
	cpu := &CPU{
		bus:    cycleCountingBus{realBus: bus},
		SP:     InitialStackPointer,
		Status: InitialStatusFlags,
	}
	return cpu
}

// State returns an immutable value copy of the current CPU registers and cycle count.
func (c *CPU) State() State {
	return State{
		Cycles: c.Cycles,
		PC:     c.PC,
		A:      c.A,
		X:      c.X,
		Y:      c.Y,
		SP:     c.SP,
		Status: c.Status,
	}
}

// RestoreState restores the CPU registers and cycle count from a saved state snapshot.
func (c *CPU) RestoreState(s State) {
	c.Cycles = s.Cycles
	c.PC = s.PC
	c.A = s.A
	c.X = s.X
	c.Y = s.Y
	c.SP = s.SP
	c.Status = s.Status
}

// Reset triggers the CPU reset sequence, setting SP, Status, and loading PC from the reset vector ($FFFC).
func (c *CPU) Reset() {
	c.SP = InitialStackPointer
	c.Status = InitialStatusFlags
	c.PC = c.readWord(VectorReset)
	c.Cycles += uint64(InterruptCycles)
}

// CurrentInstCycles returns the cycle count of the instruction currently being executed.
func (c *CPU) CurrentInstCycles() int {
	return c.bus.subCycle
}

// Step fetches, decodes, and executes the next single instruction, returning the cycles consumed.
func (c *CPU) Step() int {
	c.bus.subCycle = 0

	opcode := c.bus.Read(c.PC)
	c.PC++

	inst := &opcodeTable[opcode]

	if opcode == opcodeJSR { // JSR: 6502 cycle order: fetch low, dummy stack read, push PCH, push PCL, fetch high
		c.execJSR()
		c.Cycles += uint64(inst.cycles)
		return int(inst.cycles)
	}

	addr, pageCrossed := c.resolveAddress(inst.mode, inst.isWrite || inst.isRMW)

	cycles := int(inst.cycles)
	if pageCrossed && inst.pageCycles > 0 {
		cycles += int(inst.pageCycles)
	}

	cycles += inst.exec(c, addr, pageCrossed)
	if cycles > 0 {
		c.Cycles += uint64(cycles)
	}
	return cycles
}

func (c *CPU) execJSR() {
	low := c.bus.Read(c.PC)
	c.PC++
	_ = c.bus.Read(StackBaseAddress + uint16(c.SP))
	c.Push(byte(c.PC >> 8))
	c.Push(byte(c.PC & 0xFF))
	high := c.bus.Read(c.PC)
	c.PC = (uint16(high) << 8) | uint16(low)
}

// Push writes a single 8-bit byte to the top of the stack and decrements SP.
func (c *CPU) Push(val byte) {
	c.bus.Write(StackBaseAddress+uint16(c.SP), val)
	c.SP--
}

// Pop increments SP and reads a single 8-bit byte from the top of the stack.
func (c *CPU) Pop() byte {
	c.SP++
	return c.bus.Read(StackBaseAddress + uint16(c.SP))
}

// PushWord writes a 16-bit word to the stack in little-endian order (high byte first, then low byte).
func (c *CPU) PushWord(val uint16) {
	c.Push(byte(val >> 8))
	c.Push(byte(val & 0xFF))
}

// PopWord pulls a 16-bit word from the stack in little-endian order.
func (c *CPU) PopWord() uint16 {
	low := uint16(c.Pop())
	high := uint16(c.Pop())
	return (high << 8) | low
}

// TriggerNMI triggers a Non-Maskable Interrupt ($FFFA), pushing PC and Status to the stack.
func (c *CPU) TriggerNMI() {
	c.PushWord(c.PC)
	status := (c.Status &^ FlagBreak) | FlagUnused
	c.Push(status)
	c.SetFlag(FlagInterrupt, true)
	c.PC = c.readWord(VectorNMI)
	c.Cycles += uint64(InterruptCycles)
}

// TriggerIRQ triggers a maskable interrupt ($FFFE) if the Interrupt Disable flag (I) is clear.
// It returns true if the interrupt was taken, or false if it was masked by FlagInterrupt.
func (c *CPU) TriggerIRQ() bool {
	if c.GetFlag(FlagInterrupt) {
		return false
	}

	c.TriggerIRQForced()
	return true
}

// TriggerIRQForced triggers an IRQ unconditionally without checking FlagInterrupt.
func (c *CPU) TriggerIRQForced() {
	c.PushWord(c.PC)
	status := (c.Status &^ FlagBreak) | FlagUnused
	c.Push(status)
	c.SetFlag(FlagInterrupt, true)
	c.PC = c.readWord(VectorIRQ)
	c.Cycles += uint64(InterruptCycles)
}
