// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpu_test

import (
	"testing"

	"github.com/aoliveti/dustblow/internal/cpu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAddressProgram uint16 = 0x8000
	testAddressData    uint16 = 0x0200
	testAddressPointer uint16 = 0x0040
	testAddressTarget  uint16 = 0x0300

	samplePositiveValue  byte = 0x42
	sampleAlternateValue byte = 0x24
	sampleNegativeValue  byte = 0x80
	testDataZero         byte = 0x00
	testDataFF           byte = 0xFF

	testCyclesBaseBranch     int = 2
	testCyclesBranchTaken    int = 3
	testCyclesBranchPageSpan int = 4
)

type mockMemory struct {
	data [65536]byte
}

func (m *mockMemory) Read(addr uint16) byte {
	return m.data[addr]
}

func (m *mockMemory) Write(addr uint16, data byte) {
	m.data[addr] = data
}

func newTestCPU() (*cpu.CPU, *mockMemory) {
	mem := &mockMemory{}
	c := cpu.New(mem)
	return c, mem
}

func TestCPU_Reset(t *testing.T) {
	t.Parallel()

	c, mem := newTestCPU()
	mem.data[cpu.VectorReset] = 0x34
	mem.data[cpu.VectorReset+1] = 0x12

	c.Reset()

	assert.Equal(t, uint16(0x1234), c.PC)
	assert.Equal(t, cpu.InitialStackPointer, c.SP)
	assert.Equal(t, cpu.InitialStatusFlags, c.Status)
	assert.Equal(t, uint64(cpu.InterruptCycles), c.Cycles)
}

func TestCPU_State(t *testing.T) {
	t.Parallel()

	c, _ := newTestCPU()
	c.PC = testAddressProgram
	c.A = samplePositiveValue
	c.X = sampleAlternateValue
	c.Y = sampleNegativeValue
	c.SP = 0xFD
	c.Status = 0x24
	c.Cycles = 42

	// Verify State snapshot
	s := c.State()
	assert.Equal(t, testAddressProgram, s.PC)
	assert.Equal(t, samplePositiveValue, s.A)
	assert.Equal(t, sampleAlternateValue, s.X)
	assert.Equal(t, sampleNegativeValue, s.Y)
	assert.Equal(t, byte(0xFD), s.SP)
	assert.Equal(t, byte(0x24), s.Status)
	assert.Equal(t, uint64(42), s.Cycles)
}

func TestCPU_LoadAndStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(c *cpu.CPU, mem *mockMemory)
		verify    func(t *testing.T, c *cpu.CPU, mem *mockMemory, cycles int)
		expCycles int
	}{
		{
			name: "LDA Immediate Positive",
			setup: func(c *cpu.CPU, mem *mockMemory) {
				c.PC = testAddressProgram
				mem.data[testAddressProgram] = 0xA9 // LDA #$42
				mem.data[testAddressProgram+1] = samplePositiveValue
			},
			verify: func(t *testing.T, c *cpu.CPU, _ *mockMemory, cycles int) {
				t.Helper()
				assert.Equal(t, 2, cycles)
				assert.Equal(t, samplePositiveValue, c.A)
				assert.False(t, c.GetFlag(cpu.FlagZero))
				assert.False(t, c.GetFlag(cpu.FlagNegative))
			},
		},
		{
			name: "LDA Immediate Zero sets Z flag",
			setup: func(c *cpu.CPU, mem *mockMemory) {
				c.PC = testAddressProgram
				mem.data[testAddressProgram] = 0xA9 // LDA #$00
				mem.data[testAddressProgram+1] = testDataZero
			},
			verify: func(t *testing.T, c *cpu.CPU, _ *mockMemory, cycles int) {
				t.Helper()
				assert.Equal(t, 2, cycles)
				assert.Equal(t, testDataZero, c.A)
				assert.True(t, c.GetFlag(cpu.FlagZero))
				assert.False(t, c.GetFlag(cpu.FlagNegative))
			},
		},
		{
			name: "LDA Immediate Negative sets N flag",
			setup: func(c *cpu.CPU, mem *mockMemory) {
				c.PC = testAddressProgram
				mem.data[testAddressProgram] = 0xA9 // LDA #$80
				mem.data[testAddressProgram+1] = sampleNegativeValue
			},
			verify: func(t *testing.T, c *cpu.CPU, _ *mockMemory, cycles int) {
				t.Helper()
				assert.Equal(t, 2, cycles)
				assert.Equal(t, sampleNegativeValue, c.A)
				assert.False(t, c.GetFlag(cpu.FlagZero))
				assert.True(t, c.GetFlag(cpu.FlagNegative))
			},
		},
		{
			name: "LDX and STX Absolute",
			setup: func(c *cpu.CPU, mem *mockMemory) {
				c.PC = testAddressProgram
				mem.data[testAddressProgram] = 0xA2 // LDX #$42
				mem.data[testAddressProgram+1] = samplePositiveValue
				mem.data[testAddressProgram+2] = 0x8E // STX $0200
				mem.data[testAddressProgram+3] = byte(testAddressData & 0xFF)
				mem.data[testAddressProgram+4] = byte(testAddressData >> 8)
			},
			verify: func(t *testing.T, c *cpu.CPU, mem *mockMemory, _ int) {
				t.Helper()
				c.Step() // LDX
				assert.Equal(t, samplePositiveValue, c.X)
				c.Step() // STX
				assert.Equal(t, samplePositiveValue, mem.data[testAddressData])
			},
		},
		{
			name: "LDY and STY ZeroPage",
			setup: func(c *cpu.CPU, mem *mockMemory) {
				c.PC = testAddressProgram
				mem.data[testAddressProgram] = 0xA0 // LDY #$24
				mem.data[testAddressProgram+1] = sampleAlternateValue
				mem.data[testAddressProgram+2] = 0x84 // STY $40
				mem.data[testAddressProgram+3] = byte(testAddressPointer)
			},
			verify: func(t *testing.T, c *cpu.CPU, mem *mockMemory, _ int) {
				t.Helper()
				c.Step() // LDY
				assert.Equal(t, sampleAlternateValue, c.Y)
				c.Step() // STY
				assert.Equal(t, sampleAlternateValue, mem.data[testAddressPointer])
			},
		},
		{
			name: "STA Absolute",
			setup: func(c *cpu.CPU, mem *mockMemory) {
				c.PC = testAddressProgram
				c.A = samplePositiveValue
				mem.data[testAddressProgram] = 0x8D // STA $0200
				mem.data[testAddressProgram+1] = byte(testAddressData & 0xFF)
				mem.data[testAddressProgram+2] = byte(testAddressData >> 8)
			},
			verify: func(t *testing.T, _ *cpu.CPU, mem *mockMemory, cycles int) {
				t.Helper()
				assert.Equal(t, 4, cycles)
				assert.Equal(t, samplePositiveValue, mem.data[testAddressData])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, mem := newTestCPU()
			tt.setup(c, mem)
			cycles := c.Step()
			tt.verify(t, c, mem, cycles)
		})
	}
}

func TestCPU_RegisterTransfers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		setup  func(c *cpu.CPU)
		opcode byte
		verify func(t *testing.T, c *cpu.CPU)
	}{
		{
			name:   "TAX",
			setup:  func(c *cpu.CPU) { c.A = samplePositiveValue },
			opcode: 0xAA,
			verify: func(t *testing.T, c *cpu.CPU) {
				t.Helper()
				assert.Equal(t, samplePositiveValue, c.X)
				assert.False(t, c.GetFlag(cpu.FlagZero))
			},
		},
		{
			name:   "TAY",
			setup:  func(c *cpu.CPU) { c.A = sampleAlternateValue },
			opcode: 0xA8,
			verify: func(t *testing.T, c *cpu.CPU) {
				t.Helper()
				assert.Equal(t, sampleAlternateValue, c.Y)
				assert.False(t, c.GetFlag(cpu.FlagZero))
			},
		},
		{
			name:   "TXA",
			setup:  func(c *cpu.CPU) { c.X = sampleNegativeValue },
			opcode: 0x8A,
			verify: func(t *testing.T, c *cpu.CPU) {
				t.Helper()
				assert.Equal(t, sampleNegativeValue, c.A)
				assert.True(t, c.GetFlag(cpu.FlagNegative))
			},
		},
		{
			name:   "TYA",
			setup:  func(c *cpu.CPU) { c.Y = testDataZero },
			opcode: 0x98,
			verify: func(t *testing.T, c *cpu.CPU) {
				t.Helper()
				assert.Equal(t, testDataZero, c.A)
				assert.True(t, c.GetFlag(cpu.FlagZero))
			},
		},
		{
			name:   "TSX",
			setup:  func(c *cpu.CPU) { c.SP = 0xFD },
			opcode: 0xBA,
			verify: func(t *testing.T, c *cpu.CPU) {
				t.Helper()
				assert.Equal(t, byte(0xFD), c.X)
				assert.True(t, c.GetFlag(cpu.FlagNegative))
			},
		},
		{
			name:   "TXS",
			setup:  func(c *cpu.CPU) { c.X = 0xAA },
			opcode: 0x9A,
			verify: func(t *testing.T, c *cpu.CPU) {
				t.Helper()
				assert.Equal(t, byte(0xAA), c.SP)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, mem := newTestCPU()
			c.PC = testAddressProgram
			tt.setup(c)
			mem.data[testAddressProgram] = tt.opcode
			c.Step()
			tt.verify(t, c)
		})
	}
}

func TestCPU_Arithmetic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		isADC     bool
		initA     byte
		operand   byte
		initCarry bool
		expectedA byte
		carry     bool
		overflow  bool
		zero      bool
		neg       bool
	}{
		{
			name:      "ADC simple addition without carry",
			isADC:     true,
			initA:     10,
			operand:   20,
			initCarry: false,
			expectedA: 30,
			carry:     false,
			overflow:  false,
		},
		{
			name:      "ADC addition with carry in",
			isADC:     true,
			initA:     10,
			operand:   20,
			initCarry: true,
			expectedA: 31,
			carry:     false,
			overflow:  false,
		},
		{
			name:      "ADC unsigned carry out",
			isADC:     true,
			initA:     255,
			operand:   1,
			initCarry: false,
			expectedA: 0,
			carry:     true,
			zero:      true,
		},
		{
			name:      "ADC signed positive overflow",
			isADC:     true,
			initA:     127,
			operand:   1,
			initCarry: false,
			expectedA: 128,
			carry:     false,
			overflow:  true,
			neg:       true,
		},
		{
			name:      "ADC signed negative overflow",
			isADC:     true,
			initA:     0x80,
			operand:   0x80,
			initCarry: false,
			expectedA: 0x00,
			carry:     true,
			overflow:  true,
			zero:      true,
		},
		{
			name:      "SBC simple subtraction no borrow",
			isADC:     false,
			initA:     50,
			operand:   20,
			initCarry: true,
			expectedA: 30,
			carry:     true,
			overflow:  false,
		},
		{
			name:      "SBC subtraction with borrow",
			isADC:     false,
			initA:     50,
			operand:   20,
			initCarry: false,
			expectedA: 29,
			carry:     true,
			overflow:  false,
		},
		{
			name:      "SBC subtraction with borrow out",
			isADC:     false,
			initA:     20,
			operand:   50,
			initCarry: true,
			expectedA: 226,
			carry:     false,
			overflow:  false,
			neg:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, mem := newTestCPU()
			c.PC = testAddressProgram
			c.A = tt.initA
			c.SetFlag(cpu.FlagCarry, tt.initCarry)

			opcode := byte(0xE9) // SBC immediate
			if tt.isADC {
				opcode = 0x69 // ADC immediate
			}
			mem.data[testAddressProgram] = opcode
			mem.data[testAddressProgram+1] = tt.operand

			c.Step()

			assert.Equal(t, tt.expectedA, c.A)
			assert.Equal(t, tt.carry, c.GetFlag(cpu.FlagCarry))
			assert.Equal(t, tt.overflow, c.GetFlag(cpu.FlagOverflow))
			assert.Equal(t, tt.zero, c.GetFlag(cpu.FlagZero))
			assert.Equal(t, tt.neg, c.GetFlag(cpu.FlagNegative))
		})
	}
}

func TestCPU_Logic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		initA     byte
		operand   byte
		opcode    byte
		expectedA byte
		zero      bool
		neg       bool
	}{
		{
			name:      "AND immediate",
			initA:     0b11110000,
			operand:   0b10101010,
			opcode:    0x29, // AND #val
			expectedA: 0b10100000,
			zero:      false,
			neg:       true,
		},
		{
			name:      "ORA immediate",
			initA:     0b11000000,
			operand:   0b00001111,
			opcode:    0x09, // ORA #val
			expectedA: 0b11001111,
			zero:      false,
			neg:       true,
		},
		{
			name:      "EOR immediate",
			initA:     0b11111111,
			operand:   0b11111111,
			opcode:    0x49, // EOR #val
			expectedA: 0x00,
			zero:      true,
			neg:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, mem := newTestCPU()
			c.PC = testAddressProgram
			c.A = tt.initA
			mem.data[testAddressProgram] = tt.opcode
			mem.data[testAddressProgram+1] = tt.operand

			c.Step()

			assert.Equal(t, tt.expectedA, c.A)
			assert.Equal(t, tt.zero, c.GetFlag(cpu.FlagZero))
			assert.Equal(t, tt.neg, c.GetFlag(cpu.FlagNegative))
		})
	}
}

func TestCPU_BIT(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		acc      byte
		memVal   byte
		expZero  bool
		expNeg   bool
		expOVerf bool
	}{
		{
			name:     "BIT matches no bits, N and V flags set from memory bits 7 and 6",
			acc:      0x01,
			memVal:   0xC0, // bit 7 and bit 6 are 1
			expZero:  true, // (0x01 & 0xC0) == 0
			expNeg:   true, // bit 7
			expOVerf: true, // bit 6
		},
		{
			name:     "BIT matches bits, N and V clear",
			acc:      0x0F,
			memVal:   0x03,
			expZero:  false, // (0x0F & 0x03) != 0
			expNeg:   false,
			expOVerf: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, mem := newTestCPU()
			c.PC = testAddressProgram
			c.A = tt.acc
			mem.data[testAddressProgram] = 0x24 // BIT $40
			mem.data[testAddressProgram+1] = byte(testAddressPointer)
			mem.data[testAddressPointer] = tt.memVal

			c.Step()

			assert.Equal(t, tt.expZero, c.GetFlag(cpu.FlagZero))
			assert.Equal(t, tt.expNeg, c.GetFlag(cpu.FlagNegative))
			assert.Equal(t, tt.expOVerf, c.GetFlag(cpu.FlagOverflow))
		})
	}
}

func TestCPU_Comparisons(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		opcode   byte
		initVal  byte
		operand  byte
		expCarry bool
		expZero  bool
		expNeg   bool
	}{
		{name: "CMP greater", opcode: 0xC9, initVal: 0x50, operand: 0x20, expCarry: true, expZero: false, expNeg: false},
		{name: "CMP equal", opcode: 0xC9, initVal: 0x50, operand: 0x50, expCarry: true, expZero: true, expNeg: false},
		{name: "CMP less", opcode: 0xC9, initVal: 0x20, operand: 0x50, expCarry: false, expZero: false, expNeg: true},
		{name: "CPX equal", opcode: 0xE0, initVal: 0x10, operand: 0x10, expCarry: true, expZero: true, expNeg: false},
		{name: "CPY less", opcode: 0xC0, initVal: 0x05, operand: 0x10, expCarry: false, expZero: false, expNeg: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, mem := newTestCPU()
			c.PC = testAddressProgram
			switch tt.opcode {
			case 0xC9:
				c.A = tt.initVal
			case 0xE0:
				c.X = tt.initVal
			case 0xC0:
				c.Y = tt.initVal
			}

			mem.data[testAddressProgram] = tt.opcode
			mem.data[testAddressProgram+1] = tt.operand

			c.Step()

			assert.Equal(t, tt.expCarry, c.GetFlag(cpu.FlagCarry))
			assert.Equal(t, tt.expZero, c.GetFlag(cpu.FlagZero))
			assert.Equal(t, tt.expNeg, c.GetFlag(cpu.FlagNegative))
		})
	}
}

func TestCPU_IncrementsAndDecrements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(c *cpu.CPU, mem *mockMemory)
		verify  func(t *testing.T, c *cpu.CPU, mem *mockMemory)
		opcodes []byte
	}{
		{
			name: "INX wraps 0xFF to 0x00",
			setup: func(c *cpu.CPU, _ *mockMemory) {
				c.X = testDataFF
			},
			opcodes: []byte{0xE8}, // INX
			verify: func(t *testing.T, c *cpu.CPU, _ *mockMemory) {
				t.Helper()
				assert.Equal(t, testDataZero, c.X)
				assert.True(t, c.GetFlag(cpu.FlagZero))
			},
		},
		{
			name: "DEX wraps 0x00 to 0xFF",
			setup: func(c *cpu.CPU, _ *mockMemory) {
				c.X = testDataZero
			},
			opcodes: []byte{0xCA}, // DEX
			verify: func(t *testing.T, c *cpu.CPU, _ *mockMemory) {
				t.Helper()
				assert.Equal(t, testDataFF, c.X)
				assert.True(t, c.GetFlag(cpu.FlagNegative))
			},
		},
		{
			name: "INC and DEC ZeroPage",
			setup: func(_ *cpu.CPU, mem *mockMemory) {
				mem.data[testAddressPointer] = 0x09
			},
			opcodes: []byte{
				0xE6, byte(testAddressPointer), // INC $40
				0xC6, byte(testAddressPointer), // DEC $40
			},
			verify: func(t *testing.T, _ *cpu.CPU, mem *mockMemory) {
				t.Helper()
				assert.Equal(t, byte(0x09), mem.data[testAddressPointer])
			},
		},
		{
			name: "INY and DEY",
			setup: func(c *cpu.CPU, _ *mockMemory) {
				c.Y = 0x01
			},
			opcodes: []byte{
				0xC8, // INY -> 2
				0x88, // DEY -> 1
			},
			verify: func(t *testing.T, c *cpu.CPU, _ *mockMemory) {
				t.Helper()
				assert.Equal(t, byte(0x01), c.Y)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, mem := newTestCPU()
			c.PC = testAddressProgram
			tt.setup(c, mem)
			for i, op := range tt.opcodes {
				mem.data[testAddressProgram+uint16(i)] = op
			}

			for range tt.opcodes {
				c.Step()
			}
			tt.verify(t, c, mem)
		})
	}
}

func TestCPU_ShiftsAndRotates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(c *cpu.CPU, mem *mockMemory)
		verify    func(t *testing.T, c *cpu.CPU, mem *mockMemory)
		initCarry bool
		opcodes   []byte
	}{
		{
			name: "ASL Accumulator sets carry on bit 7",
			setup: func(c *cpu.CPU, _ *mockMemory) {
				c.A = 0b10000001
			},
			opcodes: []byte{0x0A}, // ASL A
			verify: func(t *testing.T, c *cpu.CPU, _ *mockMemory) {
				t.Helper()
				assert.Equal(t, byte(0b00000010), c.A)
				assert.True(t, c.GetFlag(cpu.FlagCarry))
			},
		},
		{
			name: "LSR Accumulator sets carry on bit 0",
			setup: func(c *cpu.CPU, _ *mockMemory) {
				c.A = 0b00000011
			},
			opcodes: []byte{0x4A}, // LSR A
			verify: func(t *testing.T, c *cpu.CPU, _ *mockMemory) {
				t.Helper()
				assert.Equal(t, byte(0b00000001), c.A)
				assert.True(t, c.GetFlag(cpu.FlagCarry))
			},
		},
		{
			name:      "ROL Accumulator with carry in",
			initCarry: true,
			setup: func(c *cpu.CPU, _ *mockMemory) {
				c.A = 0b10000000
			},
			opcodes: []byte{0x2A}, // ROL A
			verify: func(t *testing.T, c *cpu.CPU, _ *mockMemory) {
				t.Helper()
				assert.Equal(t, byte(0b00000001), c.A)
				assert.True(t, c.GetFlag(cpu.FlagCarry))
			},
		},
		{
			name:      "ROR Accumulator with carry in",
			initCarry: true,
			setup: func(c *cpu.CPU, _ *mockMemory) {
				c.A = 0b00000001
			},
			opcodes: []byte{0x6A}, // ROR A
			verify: func(t *testing.T, c *cpu.CPU, _ *mockMemory) {
				t.Helper()
				assert.Equal(t, byte(0b10000000), c.A)
				assert.True(t, c.GetFlag(cpu.FlagCarry))
			},
		},
		{
			name: "ASL and LSR Memory",
			setup: func(_ *cpu.CPU, mem *mockMemory) {
				mem.data[testAddressPointer] = 0x40
			},
			opcodes: []byte{
				0x06, byte(testAddressPointer), // ASL $40 -> 0x80
				0x46, byte(testAddressPointer), // LSR $40 -> 0x40
			},
			verify: func(t *testing.T, _ *cpu.CPU, mem *mockMemory) {
				t.Helper()
				assert.Equal(t, byte(0x40), mem.data[testAddressPointer])
			},
		},
		{
			name:      "ROL and ROR Memory",
			initCarry: true,
			setup: func(_ *cpu.CPU, mem *mockMemory) {
				mem.data[testAddressPointer] = 0x01
			},
			opcodes: []byte{
				0x26, byte(testAddressPointer), // ROL $40 -> 0x03, C=0
				0x66, byte(testAddressPointer), // ROR $40 -> 0x01, C=1
			},
			verify: func(t *testing.T, c *cpu.CPU, mem *mockMemory) {
				t.Helper()
				assert.Equal(t, byte(0x01), mem.data[testAddressPointer])
				assert.True(t, c.GetFlag(cpu.FlagCarry))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, mem := newTestCPU()
			c.PC = testAddressProgram
			c.SetFlag(cpu.FlagCarry, tt.initCarry)
			tt.setup(c, mem)
			for i, op := range tt.opcodes {
				mem.data[testAddressProgram+uint16(i)] = op
			}

			for range tt.opcodes {
				c.Step()
			}
			tt.verify(t, c, mem)
		})
	}
}

func TestCPU_Branches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		opcode     byte
		setFlag    func(c *cpu.CPU)
		shouldTake bool
	}{
		{name: "BEQ taken when Z=1", opcode: 0xF0, setFlag: func(c *cpu.CPU) { c.SetFlag(cpu.FlagZero, true) }, shouldTake: true},
		{name: "BEQ not taken when Z=0", opcode: 0xF0, setFlag: func(c *cpu.CPU) { c.SetFlag(cpu.FlagZero, false) }, shouldTake: false},
		{name: "BNE taken when Z=0", opcode: 0xD0, setFlag: func(c *cpu.CPU) { c.SetFlag(cpu.FlagZero, false) }, shouldTake: true},
		{name: "BCS taken when C=1", opcode: 0xB0, setFlag: func(c *cpu.CPU) { c.SetFlag(cpu.FlagCarry, true) }, shouldTake: true},
		{name: "BCC taken when C=0", opcode: 0x90, setFlag: func(c *cpu.CPU) { c.SetFlag(cpu.FlagCarry, false) }, shouldTake: true},
		{name: "BMI taken when N=1", opcode: 0x30, setFlag: func(c *cpu.CPU) { c.SetFlag(cpu.FlagNegative, true) }, shouldTake: true},
		{name: "BPL taken when N=0", opcode: 0x10, setFlag: func(c *cpu.CPU) { c.SetFlag(cpu.FlagNegative, false) }, shouldTake: true},
		{name: "BVS taken when V=1", opcode: 0x70, setFlag: func(c *cpu.CPU) { c.SetFlag(cpu.FlagOverflow, true) }, shouldTake: true},
		{name: "BVC taken when V=0", opcode: 0x50, setFlag: func(c *cpu.CPU) { c.SetFlag(cpu.FlagOverflow, false) }, shouldTake: true},
		{name: "BVC not taken when V=1", opcode: 0x50, setFlag: func(c *cpu.CPU) { c.SetFlag(cpu.FlagOverflow, true) }, shouldTake: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, mem := newTestCPU()
			c.PC = testAddressProgram
			tt.setFlag(c)

			mem.data[testAddressProgram] = tt.opcode
			mem.data[testAddressProgram+1] = 0x06 // branch +6 bytes

			cycles := c.Step()

			if tt.shouldTake {
				assert.Equal(t, testCyclesBranchTaken, cycles)
				assert.Equal(t, testAddressProgram+2+6, c.PC)
				return
			}
			assert.Equal(t, testCyclesBaseBranch, cycles)
			assert.Equal(t, testAddressProgram+2, c.PC)
		})
	}
}

func TestCPU_BranchPageCrossing(t *testing.T) {
	t.Parallel()

	c, mem := newTestCPU()
	c.PC = 0x80FD
	c.SetFlag(cpu.FlagZero, false) // branch taken

	mem.data[0x80FD] = 0xD0 // BNE +$04
	mem.data[0x80FE] = 0x04

	cycles := c.Step()
	assert.Equal(t, testCyclesBranchPageSpan, cycles)
	assert.Equal(t, uint16(0x8103), c.PC)
}

func TestCPU_StatusFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		opcode   byte
		flag     byte
		expected bool
	}{
		{name: "SEC sets carry", opcode: 0x38, flag: cpu.FlagCarry, expected: true},
		{name: "CLC clears carry", opcode: 0x18, flag: cpu.FlagCarry, expected: false},
		{name: "SED sets decimal", opcode: 0xF8, flag: cpu.FlagDecimal, expected: true},
		{name: "CLD clears decimal", opcode: 0xD8, flag: cpu.FlagDecimal, expected: false},
		{name: "SEI sets interrupt", opcode: 0x78, flag: cpu.FlagInterrupt, expected: true},
		{name: "CLI clears interrupt", opcode: 0x58, flag: cpu.FlagInterrupt, expected: false},
		{name: "CLV clears overflow", opcode: 0xB8, flag: cpu.FlagOverflow, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, mem := newTestCPU()
			c.PC = testAddressProgram
			if !tt.expected {
				c.SetFlag(tt.flag, true) // pre-set flag to verify clear
			}

			mem.data[testAddressProgram] = tt.opcode
			c.Step()

			assert.Equal(t, tt.expected, c.GetFlag(tt.flag))
		})
	}
}

func TestCPU_Stack(t *testing.T) {
	t.Parallel()

	t.Run("PHA and PLA", func(t *testing.T) {
		t.Parallel()
		c, mem := newTestCPU()
		c.PC = testAddressProgram
		c.SP = 0xFF
		c.A = samplePositiveValue

		mem.data[testAddressProgram] = 0x48   // PHA
		mem.data[testAddressProgram+1] = 0xA9 // LDA #$00
		mem.data[testAddressProgram+2] = testDataZero
		mem.data[testAddressProgram+3] = 0x68 // PLA

		c.Step() // PHA
		assert.Equal(t, byte(0xFE), c.SP)
		assert.Equal(t, samplePositiveValue, mem.data[cpu.StackBaseAddress+0xFF])

		c.Step() // LDA #$00
		assert.Equal(t, testDataZero, c.A)

		c.Step() // PLA
		assert.Equal(t, byte(0xFF), c.SP)
		assert.Equal(t, samplePositiveValue, c.A)
	})

	t.Run("PHP sets B and U flags in stack and PLP restores", func(t *testing.T) {
		t.Parallel()
		c, mem := newTestCPU()
		c.PC = testAddressProgram
		c.SP = 0xFF
		c.Status = cpu.FlagCarry // C=1, all other flags 0

		mem.data[testAddressProgram] = 0x08   // PHP
		mem.data[testAddressProgram+1] = 0x38 // SEC
		mem.data[testAddressProgram+2] = 0x28 // PLP

		c.Step() // PHP
		pushed := mem.data[cpu.StackBaseAddress+0xFF]
		// PHP must always push with Break (bit 4) and Unused (bit 5) set
		assert.Equal(t, cpu.FlagBreak|cpu.FlagUnused|cpu.FlagCarry, pushed)

		// Modify status before pull
		c.Status = 0x00

		c.Step() // SEC
		c.Step() // PLP
		assert.True(t, c.GetFlag(cpu.FlagCarry))
		assert.True(t, c.GetFlag(cpu.FlagUnused))
	})
}

func TestCPU_JumpsAndSubroutines(t *testing.T) {
	t.Parallel()

	t.Run("NOP does nothing and takes 2 cycles", func(t *testing.T) {
		t.Parallel()
		c, mem := newTestCPU()
		c.PC = testAddressProgram
		mem.data[testAddressProgram] = 0xEA // NOP

		cycles := c.Step()
		assert.Equal(t, 2, cycles)
		assert.Equal(t, testAddressProgram+1, c.PC)
	})

	t.Run("JMP Absolute", func(t *testing.T) {
		t.Parallel()
		c, mem := newTestCPU()
		c.PC = testAddressProgram

		mem.data[testAddressProgram] = 0x4C // JMP $0300
		mem.data[testAddressProgram+1] = byte(testAddressTarget & 0xFF)
		mem.data[testAddressProgram+2] = byte(testAddressTarget >> 8)

		c.Step()
		assert.Equal(t, testAddressTarget, c.PC)
	})

	t.Run("JMP Indirect Hardware Bug", func(t *testing.T) {
		t.Parallel()
		c, mem := newTestCPU()
		c.PC = testAddressProgram

		// JMP ($10FF): 6502 bug fetches high byte from $1000 instead of $1100
		mem.data[testAddressProgram] = 0x6C
		mem.data[testAddressProgram+1] = 0xFF
		mem.data[testAddressProgram+2] = 0x10

		mem.data[0x10FF] = 0x42
		mem.data[0x1000] = 0x33
		mem.data[0x1100] = 0x99

		c.Step()
		assert.Equal(t, uint16(0x3342), c.PC)
	})

	t.Run("JSR and RTS", func(t *testing.T) {
		t.Parallel()
		c, mem := newTestCPU()
		c.PC = testAddressProgram
		c.SP = 0xFF

		mem.data[testAddressProgram] = 0x20 // JSR $0300
		mem.data[testAddressProgram+1] = byte(testAddressTarget & 0xFF)
		mem.data[testAddressProgram+2] = byte(testAddressTarget >> 8)
		mem.data[testAddressProgram+3] = 0xEA // NOP (return address target)
		mem.data[testAddressTarget] = 0x60    // RTS

		cyclesJSR := c.Step()
		assert.Equal(t, 6, cyclesJSR)
		assert.Equal(t, testAddressTarget, c.PC)

		cyclesRTS := c.Step()
		assert.Equal(t, 6, cyclesRTS)
		assert.Equal(t, testAddressProgram+3, c.PC)
	})
}

func TestCPU_Interrupts(t *testing.T) {
	t.Parallel()

	t.Run("NMI Interrupt", func(t *testing.T) {
		t.Parallel()
		c, mem := newTestCPU()
		c.PC = testAddressProgram
		mem.data[cpu.VectorNMI] = 0x00
		mem.data[cpu.VectorNMI+1] = 0x90

		c.TriggerNMI()

		assert.Equal(t, uint16(0x9000), c.PC)
		require.True(t, c.GetFlag(cpu.FlagInterrupt))

		pushedStatus := c.Pop()
		assert.Equal(t, cpu.FlagUnused, pushedStatus&(cpu.FlagBreak|cpu.FlagUnused))
	})

	t.Run("IRQ Masked and Triggered", func(t *testing.T) {
		t.Parallel()
		c, mem := newTestCPU()
		c.PC = testAddressProgram
		mem.data[cpu.VectorIRQ] = 0x00
		mem.data[cpu.VectorIRQ+1] = 0x85

		// When I=1, IRQ should be ignored
		c.SetFlag(cpu.FlagInterrupt, true)
		c.TriggerIRQ()
		assert.Equal(t, testAddressProgram, c.PC)

		// When I=0, IRQ should trigger
		c.SetFlag(cpu.FlagInterrupt, false)
		c.TriggerIRQ()
		assert.Equal(t, uint16(0x8500), c.PC)
	})

	t.Run("BRK and RTI", func(t *testing.T) {
		t.Parallel()
		c, mem := newTestCPU()
		c.PC = testAddressProgram
		mem.data[testAddressProgram] = 0x00 // BRK
		mem.data[cpu.VectorIRQ] = byte(testAddressTarget & 0xFF)
		mem.data[cpu.VectorIRQ+1] = byte(testAddressTarget >> 8)
		mem.data[testAddressTarget] = 0x40 // RTI

		c.Step() // BRK
		assert.Equal(t, testAddressTarget, c.PC)
		assert.True(t, c.GetFlag(cpu.FlagInterrupt))

		c.Step() // RTI
		assert.Equal(t, testAddressProgram+2, c.PC)
	})
}

func TestCPU_ComplexAddressingModes(t *testing.T) {
	t.Parallel()

	t.Run("Indexed Indirect (X)", func(t *testing.T) {
		t.Parallel()
		c, mem := newTestCPU()
		c.PC = testAddressProgram
		c.X = 0x04

		// LDA ($20,X) -> pointer at $24 ($20 + 4)
		mem.data[testAddressProgram] = 0xA1
		mem.data[testAddressProgram+1] = 0x20
		mem.data[0x0024] = 0x00
		mem.data[0x0025] = 0x03 // pointer points to $0300
		mem.data[testAddressTarget] = samplePositiveValue

		c.Step()
		assert.Equal(t, samplePositiveValue, c.A)
	})

	t.Run("Indirect Indexed (Y) with and without Page Crossing", func(t *testing.T) {
		t.Parallel()
		c, mem := newTestCPU()
		c.PC = testAddressProgram
		c.Y = 0x10

		// Pointer at $40 points to $0200 -> effective $0210 (no page cross)
		mem.data[0x0040] = 0x00
		mem.data[0x0041] = 0x02
		mem.data[0x0210] = sampleAlternateValue

		mem.data[testAddressProgram] = 0xB1 // LDA ($40),Y
		mem.data[testAddressProgram+1] = 0x40

		cycles := c.Step()
		assert.Equal(t, 5, cycles) // 5 cycles without page cross
		assert.Equal(t, sampleAlternateValue, c.A)

		// Now set Y = 0xFF to cross page ($0200 + 0xFF = $02FF no, from $02F0 + 0x20 -> $0310)
		c.Y = 0xFF
		mem.data[0x0040] = 0x05
		mem.data[0x0041] = 0x02 // points to $0205 + 0xFF = $0304 (page crossed from $02 to $03)
		mem.data[0x0304] = samplePositiveValue

		mem.data[c.PC] = 0xB1
		mem.data[c.PC+1] = 0x40

		cyclesCross := c.Step()
		assert.Equal(t, 6, cyclesCross) // 5 + 1 page cross = 6 cycles
		assert.Equal(t, samplePositiveValue, c.A)
	})
}

func TestCPU_UndocumentedOpcodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		setup  func(c *cpu.CPU, mem *mockMemory)
		verify func(t *testing.T, c *cpu.CPU, mem *mockMemory)
		opcode byte
	}{
		{
			name:   "LAX loads A and X simultaneously",
			opcode: 0xA7, // *LAX $40
			setup: func(_ *cpu.CPU, mem *mockMemory) {
				mem.data[testAddressPointer] = samplePositiveValue
			},
			verify: func(t *testing.T, c *cpu.CPU, _ *mockMemory) {
				t.Helper()
				assert.Equal(t, samplePositiveValue, c.A)
				assert.Equal(t, samplePositiveValue, c.X)
			},
		},
		{
			name:   "SAX stores A AND X to memory",
			opcode: 0x87, // *SAX $40
			setup: func(c *cpu.CPU, _ *mockMemory) {
				c.A = 0b11110000
				c.X = 0b10101010
			},
			verify: func(t *testing.T, _ *cpu.CPU, mem *mockMemory) {
				t.Helper()
				assert.Equal(t, byte(0b10100000), mem.data[testAddressPointer])
			},
		},
		{
			name:   "DCP decrements memory and compares with A",
			opcode: 0xC7, // *DCP $40
			setup: func(c *cpu.CPU, mem *mockMemory) {
				c.A = 0x05
				mem.data[testAddressPointer] = 0x06 // 0x06 - 1 = 0x05, CMP with 0x05 -> Z=1, C=1
			},
			verify: func(t *testing.T, c *cpu.CPU, mem *mockMemory) {
				t.Helper()
				assert.Equal(t, byte(0x05), mem.data[testAddressPointer])
				assert.True(t, c.GetFlag(cpu.FlagZero))
				assert.True(t, c.GetFlag(cpu.FlagCarry))
			},
		},
		{
			name:   "ISC increments memory and subtracts from A with SBC",
			opcode: 0xE7, // *ISC $40
			setup: func(c *cpu.CPU, mem *mockMemory) {
				c.A = 10
				c.SetFlag(cpu.FlagCarry, true)
				mem.data[testAddressPointer] = 2 // 2 + 1 = 3, SBC: 10 - 3 = 7
			},
			verify: func(t *testing.T, c *cpu.CPU, mem *mockMemory) {
				t.Helper()
				assert.Equal(t, byte(3), mem.data[testAddressPointer])
				assert.Equal(t, byte(7), c.A)
			},
		},
		{
			name:   "SLO shifts left memory and ORs with A",
			opcode: 0x07, // *SLO $40
			setup: func(c *cpu.CPU, mem *mockMemory) {
				c.A = 0x01
				mem.data[testAddressPointer] = 0x02 // 0x02 << 1 = 0x04, A |= 0x04 -> 0x05
			},
			verify: func(t *testing.T, c *cpu.CPU, mem *mockMemory) {
				t.Helper()
				assert.Equal(t, byte(0x04), mem.data[testAddressPointer])
				assert.Equal(t, byte(0x05), c.A)
			},
		},
		{
			name:   "RLA rotates left memory and ANDs with A",
			opcode: 0x27, // *RLA $40
			setup: func(c *cpu.CPU, mem *mockMemory) {
				c.A = 0xFF
				c.SetFlag(cpu.FlagCarry, true)
				mem.data[testAddressPointer] = 0x01 // (0x01 << 1) | 1 = 0x03, A &= 0x03 -> 0x03
			},
			verify: func(t *testing.T, c *cpu.CPU, mem *mockMemory) {
				t.Helper()
				assert.Equal(t, byte(0x03), mem.data[testAddressPointer])
				assert.Equal(t, byte(0x03), c.A)
			},
		},
		{
			name:   "SRE shifts right memory and XORs with A",
			opcode: 0x47, // *SRE $40
			setup: func(c *cpu.CPU, mem *mockMemory) {
				c.A = 0xFF
				mem.data[testAddressPointer] = 0x04 // 0x04 >> 1 = 0x02, A ^= 0x02 -> 0xFD
			},
			verify: func(t *testing.T, c *cpu.CPU, mem *mockMemory) {
				t.Helper()
				assert.Equal(t, byte(0x02), mem.data[testAddressPointer])
				assert.Equal(t, byte(0xFD), c.A)
			},
		},
		{
			name:   "RRA rotates right memory and ADCs with A",
			opcode: 0x67, // *RRA $40
			setup: func(c *cpu.CPU, mem *mockMemory) {
				c.A = 10
				c.SetFlag(cpu.FlagCarry, true)
				mem.data[testAddressPointer] = 0x00 // (0 >> 1) | 0x80 = 0x80, Carry becomes 0
				// ADC: 10 + 0x80 + 0 = 0x8A = 138
			},
			verify: func(t *testing.T, c *cpu.CPU, mem *mockMemory) {
				t.Helper()
				assert.Equal(t, byte(0x80), mem.data[testAddressPointer])
				assert.Equal(t, byte(138), c.A)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, mem := newTestCPU()
			c.PC = testAddressProgram
			tt.setup(c, mem)

			mem.data[testAddressProgram] = tt.opcode
			mem.data[testAddressProgram+1] = byte(testAddressPointer)

			c.Step()
			tt.verify(t, c, mem)
		})
	}
}

func TestCPU_UnofficialImmediatesAndSH(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		setup  func(c *cpu.CPU, mem *mockMemory)
		code   []byte
		verify func(t *testing.T, c *cpu.CPU, mem *mockMemory)
	}{
		{
			name: "ANC sets carry to negative flag",
			code: []byte{0x0B, 0xF0}, // *ANC #$F0
			setup: func(c *cpu.CPU, _ *mockMemory) {
				c.A = 0x95
			},
			verify: func(t *testing.T, c *cpu.CPU, _ *mockMemory) {
				t.Helper()
				assert.Equal(t, byte(0x90), c.A)
				assert.True(t, c.GetFlag(cpu.FlagCarry))
				assert.True(t, c.GetFlag(cpu.FlagNegative))
			},
		},
		{
			name: "ALR ANDs and shifts right into carry",
			code: []byte{0x4B, 0xF0}, // *ALR #$F0
			setup: func(c *cpu.CPU, _ *mockMemory) {
				c.A = 0x81
			},
			verify: func(t *testing.T, c *cpu.CPU, _ *mockMemory) {
				t.Helper()
				assert.Equal(t, byte(0x40), c.A)
				assert.False(t, c.GetFlag(cpu.FlagCarry))
			},
		},
		{
			name: "ARR ANDs and rotates right with complex flags",
			code: []byte{0x6B, 0xF2}, // *ARR #$F2
			setup: func(c *cpu.CPU, _ *mockMemory) {
				c.A = 0xEE
			},
			verify: func(t *testing.T, c *cpu.CPU, _ *mockMemory) {
				t.Helper()
				assert.Equal(t, byte(0x71), c.A)
				assert.True(t, c.GetFlag(cpu.FlagCarry))
			},
		},
		{
			name: "AXS subtracts immediate from A and X",
			code: []byte{0xCB, 0x10}, // *AXS #$10
			setup: func(c *cpu.CPU, _ *mockMemory) {
				c.A = 0xF0
				c.X = 0xB0
			},
			verify: func(t *testing.T, c *cpu.CPU, _ *mockMemory) {
				t.Helper()
				assert.Equal(t, byte(0xA0), c.X)
				assert.True(t, c.GetFlag(cpu.FlagCarry))
			},
		},
		{
			name: "LAS sets A, X, SP from memory AND SP",
			code: []byte{0xBB, 0x40, 0x00}, // *LAS $0040, Y
			setup: func(c *cpu.CPU, mem *mockMemory) {
				c.SP = 0x7F
				c.Y = 0
				mem.data[0x0040] = 0xC5
			},
			verify: func(t *testing.T, c *cpu.CPU, _ *mockMemory) {
				t.Helper()
				assert.Equal(t, byte(0x45), c.A)
				assert.Equal(t, byte(0x45), c.X)
				assert.Equal(t, byte(0x45), c.SP)
			},
		},
		{
			name: "NOP absolute reads target address",
			code: []byte{0x0C, 0x40, 0x00}, // *NOP $0040
			setup: func(_ *cpu.CPU, mem *mockMemory) {
				mem.data[0x0040] = 0x5A
			},
			verify: func(t *testing.T, _ *cpu.CPU, mem *mockMemory) {
				t.Helper()
				assert.Equal(t, byte(0x5A), mem.data[0x0040])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, mem := newTestCPU()
			c.PC = testAddressProgram
			tt.setup(c, mem)
			copy(mem.data[testAddressProgram:], tt.code)
			c.Step()
			tt.verify(t, c, mem)
		})
	}
}
