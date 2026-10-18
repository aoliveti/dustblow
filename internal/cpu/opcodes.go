// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpu

type instruction struct {
	name       string
	exec       func(c *CPU, addr uint16, pageCrossed bool) int
	mode       AddressingMode
	cycles     byte
	pageCycles byte
	unofficial bool
	isWrite    bool
	isRMW      bool
}

// Info contains metadata for an opcode used by tracers and disassemblers.
// Struct field layout is optimized for memory alignment.
type Info struct {
	Name       string
	Mode       AddressingMode
	Bytes      byte
	Cycles     byte
	Unofficial bool
}

var opcodeTable [256]instruction

// Lookup returns the metadata for a given 6502 opcode byte.
func Lookup(code byte) Info {
	inst := opcodeTable[code]
	var size byte
	switch inst.mode {
	case ModeImplied, ModeAccumulator:
		size = 1
	case ModeImmediate, ModeZeroPage, ModeZeroPageX, ModeZeroPageY, ModeRelative, ModeIndirectX, ModeIndirectY:
		size = 2
	case ModeAbsolute, ModeAbsoluteX, ModeAbsoluteY, ModeIndirect:
		size = 3
	}
	return Info{
		Name:       inst.name,
		Mode:       inst.mode,
		Bytes:      size,
		Cycles:     inst.cycles,
		Unofficial: inst.unofficial,
	}
}

func init() {
	// Initialize default NOP for all 256 entries to prevent nil exec panics.
	for i := range opcodeTable {
		opcodeTable[i] = instruction{
			name:       "NOP",
			exec:       func(_ *CPU, _ uint16, _ bool) int { return 0 },
			mode:       ModeImplied,
			cycles:     2,
			pageCycles: 0,
			unofficial: true,
			isWrite:    false,
			isRMW:      false,
		}
	}

	registerOfficialOpcodes()
	registerUndocumentedOpcodes()

	for i := range opcodeTable {
		switch opcodeTable[i].name {
		case "STA", "STX", "STY", "SAX", "SHA", "SHX", "SHY", "SHS", "AHX", "TAS":
			opcodeTable[i].isWrite = true
		case "ASL", "LSR", "ROL", "ROR", "INC", "DEC", "DCP", "ISB", "SLO", "RLA", "SRE", "RRA":
			if opcodeTable[i].mode != ModeAccumulator {
				opcodeTable[i].isRMW = true
			}
		}
	}
}

func def(code byte, name string, exec func(*CPU, uint16, bool) int, mode AddressingMode, cycles, pageCycles byte) {
	opcodeTable[code] = instruction{
		name:       name,
		exec:       exec,
		mode:       mode,
		cycles:     cycles,
		pageCycles: pageCycles,
		unofficial: false,
	}
}

func defUnofficial(code byte, name string, exec func(*CPU, uint16, bool) int, mode AddressingMode, cycles, pageCycles byte) {
	opcodeTable[code] = instruction{
		name:       name,
		exec:       exec,
		mode:       mode,
		cycles:     cycles,
		pageCycles: pageCycles,
		unofficial: true,
	}
}

func registerOfficialOpcodes() {
	// ADC
	def(0x69, "ADC", (*CPU).opADC, ModeImmediate, 2, 0)
	def(0x65, "ADC", (*CPU).opADC, ModeZeroPage, 3, 0)
	def(0x75, "ADC", (*CPU).opADC, ModeZeroPageX, 4, 0)
	def(0x6D, "ADC", (*CPU).opADC, ModeAbsolute, 4, 0)
	def(0x7D, "ADC", (*CPU).opADC, ModeAbsoluteX, 4, 1)
	def(0x79, "ADC", (*CPU).opADC, ModeAbsoluteY, 4, 1)
	def(0x61, "ADC", (*CPU).opADC, ModeIndirectX, 6, 0)
	def(0x71, "ADC", (*CPU).opADC, ModeIndirectY, 5, 1)

	// AND
	def(0x29, "AND", (*CPU).opAND, ModeImmediate, 2, 0)
	def(0x25, "AND", (*CPU).opAND, ModeZeroPage, 3, 0)
	def(0x35, "AND", (*CPU).opAND, ModeZeroPageX, 4, 0)
	def(0x2D, "AND", (*CPU).opAND, ModeAbsolute, 4, 0)
	def(0x3D, "AND", (*CPU).opAND, ModeAbsoluteX, 4, 1)
	def(0x39, "AND", (*CPU).opAND, ModeAbsoluteY, 4, 1)
	def(0x21, "AND", (*CPU).opAND, ModeIndirectX, 6, 0)
	def(0x31, "AND", (*CPU).opAND, ModeIndirectY, 5, 1)

	// ASL
	def(0x0A, "ASL", (*CPU).opASLAcc, ModeAccumulator, 2, 0)
	def(0x06, "ASL", (*CPU).opASLMem, ModeZeroPage, 5, 0)
	def(0x16, "ASL", (*CPU).opASLMem, ModeZeroPageX, 6, 0)
	def(0x0E, "ASL", (*CPU).opASLMem, ModeAbsolute, 6, 0)
	def(0x1E, "ASL", (*CPU).opASLMem, ModeAbsoluteX, 7, 0)

	// Branch Instructions
	def(0x90, "BCC", (*CPU).opBCC, ModeRelative, 2, 0)
	def(0xB0, "BCS", (*CPU).opBCS, ModeRelative, 2, 0)
	def(0xF0, "BEQ", (*CPU).opBEQ, ModeRelative, 2, 0)
	def(0x30, "BMI", (*CPU).opBMI, ModeRelative, 2, 0)
	def(0xD0, "BNE", (*CPU).opBNE, ModeRelative, 2, 0)
	def(0x10, "BPL", (*CPU).opBPL, ModeRelative, 2, 0)
	def(0x50, "BVC", (*CPU).opBVC, ModeRelative, 2, 0)
	def(0x70, "BVS", (*CPU).opBVS, ModeRelative, 2, 0)

	// BIT
	def(0x24, "BIT", (*CPU).opBIT, ModeZeroPage, 3, 0)
	def(0x2C, "BIT", (*CPU).opBIT, ModeAbsolute, 4, 0)

	// BRK
	def(0x00, "BRK", (*CPU).opBRK, ModeImplied, 7, 0)

	// Clear / Set Flags
	def(0x18, "CLC", (*CPU).opCLC, ModeImplied, 2, 0)
	def(0xD8, "CLD", (*CPU).opCLD, ModeImplied, 2, 0)
	def(0x58, "CLI", (*CPU).opCLI, ModeImplied, 2, 0)
	def(0xB8, "CLV", (*CPU).opCLV, ModeImplied, 2, 0)
	def(0x38, "SEC", (*CPU).opSEC, ModeImplied, 2, 0)
	def(0xF8, "SED", (*CPU).opSED, ModeImplied, 2, 0)
	def(0x78, "SEI", (*CPU).opSEI, ModeImplied, 2, 0)

	// CMP, CPX, CPY
	def(0xC9, "CMP", (*CPU).opCMP, ModeImmediate, 2, 0)
	def(0xC5, "CMP", (*CPU).opCMP, ModeZeroPage, 3, 0)
	def(0xD5, "CMP", (*CPU).opCMP, ModeZeroPageX, 4, 0)
	def(0xCD, "CMP", (*CPU).opCMP, ModeAbsolute, 4, 0)
	def(0xDD, "CMP", (*CPU).opCMP, ModeAbsoluteX, 4, 1)
	def(0xD9, "CMP", (*CPU).opCMP, ModeAbsoluteY, 4, 1)
	def(0xC1, "CMP", (*CPU).opCMP, ModeIndirectX, 6, 0)
	def(0xD1, "CMP", (*CPU).opCMP, ModeIndirectY, 5, 1)

	def(0xE0, "CPX", (*CPU).opCPX, ModeImmediate, 2, 0)
	def(0xE4, "CPX", (*CPU).opCPX, ModeZeroPage, 3, 0)
	def(0xEC, "CPX", (*CPU).opCPX, ModeAbsolute, 4, 0)

	def(0xC0, "CPY", (*CPU).opCPY, ModeImmediate, 2, 0)
	def(0xC4, "CPY", (*CPU).opCPY, ModeZeroPage, 3, 0)
	def(0xCC, "CPY", (*CPU).opCPY, ModeAbsolute, 4, 0)

	// DEC, DEX, DEY
	def(0xC6, "DEC", (*CPU).opDEC, ModeZeroPage, 5, 0)
	def(0xD6, "DEC", (*CPU).opDEC, ModeZeroPageX, 6, 0)
	def(0xCE, "DEC", (*CPU).opDEC, ModeAbsolute, 6, 0)
	def(0xDE, "DEC", (*CPU).opDEC, ModeAbsoluteX, 7, 0)
	def(0xCA, "DEX", (*CPU).opDEX, ModeImplied, 2, 0)
	def(0x88, "DEY", (*CPU).opDEY, ModeImplied, 2, 0)

	// EOR
	def(0x49, "EOR", (*CPU).opEOR, ModeImmediate, 2, 0)
	def(0x45, "EOR", (*CPU).opEOR, ModeZeroPage, 3, 0)
	def(0x55, "EOR", (*CPU).opEOR, ModeZeroPageX, 4, 0)
	def(0x4D, "EOR", (*CPU).opEOR, ModeAbsolute, 4, 0)
	def(0x5D, "EOR", (*CPU).opEOR, ModeAbsoluteX, 4, 1)
	def(0x59, "EOR", (*CPU).opEOR, ModeAbsoluteY, 4, 1)
	def(0x41, "EOR", (*CPU).opEOR, ModeIndirectX, 6, 0)
	def(0x51, "EOR", (*CPU).opEOR, ModeIndirectY, 5, 1)

	// INC, INX, INY
	def(0xE6, "INC", (*CPU).opINC, ModeZeroPage, 5, 0)
	def(0xF6, "INC", (*CPU).opINC, ModeZeroPageX, 6, 0)
	def(0xEE, "INC", (*CPU).opINC, ModeAbsolute, 6, 0)
	def(0xFE, "INC", (*CPU).opINC, ModeAbsoluteX, 7, 0)
	def(0xE8, "INX", (*CPU).opINX, ModeImplied, 2, 0)
	def(0xC8, "INY", (*CPU).opINY, ModeImplied, 2, 0)

	// JMP, JSR, RTS, RTI
	def(0x4C, "JMP", (*CPU).opJMP, ModeAbsolute, 3, 0)
	def(0x6C, "JMP", (*CPU).opJMP, ModeIndirect, 5, 0)
	def(0x20, "JSR", (*CPU).opJSR, ModeAbsolute, 6, 0)
	def(0x60, "RTS", (*CPU).opRTS, ModeImplied, 6, 0)
	def(0x40, "RTI", (*CPU).opRTI, ModeImplied, 6, 0)

	// LDA
	def(0xA9, "LDA", (*CPU).opLDA, ModeImmediate, 2, 0)
	def(0xA5, "LDA", (*CPU).opLDA, ModeZeroPage, 3, 0)
	def(0xB5, "LDA", (*CPU).opLDA, ModeZeroPageX, 4, 0)
	def(0xAD, "LDA", (*CPU).opLDA, ModeAbsolute, 4, 0)
	def(0xBD, "LDA", (*CPU).opLDA, ModeAbsoluteX, 4, 1)
	def(0xB9, "LDA", (*CPU).opLDA, ModeAbsoluteY, 4, 1)
	def(0xA1, "LDA", (*CPU).opLDA, ModeIndirectX, 6, 0)
	def(0xB1, "LDA", (*CPU).opLDA, ModeIndirectY, 5, 1)

	// LDX
	def(0xA2, "LDX", (*CPU).opLDX, ModeImmediate, 2, 0)
	def(0xA6, "LDX", (*CPU).opLDX, ModeZeroPage, 3, 0)
	def(0xB6, "LDX", (*CPU).opLDX, ModeZeroPageY, 4, 0)
	def(0xAE, "LDX", (*CPU).opLDX, ModeAbsolute, 4, 0)
	def(0xBE, "LDX", (*CPU).opLDX, ModeAbsoluteY, 4, 1)

	// LDY
	def(0xA0, "LDY", (*CPU).opLDY, ModeImmediate, 2, 0)
	def(0xA4, "LDY", (*CPU).opLDY, ModeZeroPage, 3, 0)
	def(0xB4, "LDY", (*CPU).opLDY, ModeZeroPageX, 4, 0)
	def(0xAC, "LDY", (*CPU).opLDY, ModeAbsolute, 4, 0)
	def(0xBC, "LDY", (*CPU).opLDY, ModeAbsoluteX, 4, 1)

	// LSR
	def(0x4A, "LSR", (*CPU).opLSRAcc, ModeAccumulator, 2, 0)
	def(0x46, "LSR", (*CPU).opLSRMem, ModeZeroPage, 5, 0)
	def(0x56, "LSR", (*CPU).opLSRMem, ModeZeroPageX, 6, 0)
	def(0x4E, "LSR", (*CPU).opLSRMem, ModeAbsolute, 6, 0)
	def(0x5E, "LSR", (*CPU).opLSRMem, ModeAbsoluteX, 7, 0)

	// NOP
	def(0xEA, "NOP", (*CPU).opNOP, ModeImplied, 2, 0)

	// ORA
	def(0x09, "ORA", (*CPU).opORA, ModeImmediate, 2, 0)
	def(0x05, "ORA", (*CPU).opORA, ModeZeroPage, 3, 0)
	def(0x15, "ORA", (*CPU).opORA, ModeZeroPageX, 4, 0)
	def(0x0D, "ORA", (*CPU).opORA, ModeAbsolute, 4, 0)
	def(0x1D, "ORA", (*CPU).opORA, ModeAbsoluteX, 4, 1)
	def(0x19, "ORA", (*CPU).opORA, ModeAbsoluteY, 4, 1)
	def(0x01, "ORA", (*CPU).opORA, ModeIndirectX, 6, 0)
	def(0x11, "ORA", (*CPU).opORA, ModeIndirectY, 5, 1)

	// PHA, PHP, PLA, PLP
	def(0x48, "PHA", (*CPU).opPHA, ModeImplied, 3, 0)
	def(0x08, "PHP", (*CPU).opPHP, ModeImplied, 3, 0)
	def(0x68, "PLA", (*CPU).opPLA, ModeImplied, 4, 0)
	def(0x28, "PLP", (*CPU).opPLP, ModeImplied, 4, 0)

	// ROL
	def(0x2A, "ROL", (*CPU).opROLAcc, ModeAccumulator, 2, 0)
	def(0x26, "ROL", (*CPU).opROLMem, ModeZeroPage, 5, 0)
	def(0x36, "ROL", (*CPU).opROLMem, ModeZeroPageX, 6, 0)
	def(0x2E, "ROL", (*CPU).opROLMem, ModeAbsolute, 6, 0)
	def(0x3E, "ROL", (*CPU).opROLMem, ModeAbsoluteX, 7, 0)

	// ROR
	def(0x6A, "ROR", (*CPU).opRORAcc, ModeAccumulator, 2, 0)
	def(0x66, "ROR", (*CPU).opRORMem, ModeZeroPage, 5, 0)
	def(0x76, "ROR", (*CPU).opRORMem, ModeZeroPageX, 6, 0)
	def(0x6E, "ROR", (*CPU).opRORMem, ModeAbsolute, 6, 0)
	def(0x7E, "ROR", (*CPU).opRORMem, ModeAbsoluteX, 7, 0)

	// SBC
	def(0xE9, "SBC", (*CPU).opSBC, ModeImmediate, 2, 0)
	def(0xE5, "SBC", (*CPU).opSBC, ModeZeroPage, 3, 0)
	def(0xF5, "SBC", (*CPU).opSBC, ModeZeroPageX, 4, 0)
	def(0xED, "SBC", (*CPU).opSBC, ModeAbsolute, 4, 0)
	def(0xFD, "SBC", (*CPU).opSBC, ModeAbsoluteX, 4, 1)
	def(0xF9, "SBC", (*CPU).opSBC, ModeAbsoluteY, 4, 1)
	def(0xE1, "SBC", (*CPU).opSBC, ModeIndirectX, 6, 0)
	def(0xF1, "SBC", (*CPU).opSBC, ModeIndirectY, 5, 1)

	// STA
	def(0x85, "STA", (*CPU).opSTA, ModeZeroPage, 3, 0)
	def(0x95, "STA", (*CPU).opSTA, ModeZeroPageX, 4, 0)
	def(0x8D, "STA", (*CPU).opSTA, ModeAbsolute, 4, 0)
	def(0x9D, "STA", (*CPU).opSTA, ModeAbsoluteX, 5, 0)
	def(0x99, "STA", (*CPU).opSTA, ModeAbsoluteY, 5, 0)
	def(0x81, "STA", (*CPU).opSTA, ModeIndirectX, 6, 0)
	def(0x91, "STA", (*CPU).opSTA, ModeIndirectY, 6, 0)

	// STX, STY
	def(0x86, "STX", (*CPU).opSTX, ModeZeroPage, 3, 0)
	def(0x96, "STX", (*CPU).opSTX, ModeZeroPageY, 4, 0)
	def(0x8E, "STX", (*CPU).opSTX, ModeAbsolute, 4, 0)

	def(0x84, "STY", (*CPU).opSTY, ModeZeroPage, 3, 0)
	def(0x94, "STY", (*CPU).opSTY, ModeZeroPageX, 4, 0)
	def(0x8C, "STY", (*CPU).opSTY, ModeAbsolute, 4, 0)

	// Transfer Instructions
	def(0xAA, "TAX", (*CPU).opTAX, ModeImplied, 2, 0)
	def(0xA8, "TAY", (*CPU).opTAY, ModeImplied, 2, 0)
	def(0xBA, "TSX", (*CPU).opTSX, ModeImplied, 2, 0)
	def(0x8A, "TXA", (*CPU).opTXA, ModeImplied, 2, 0)
	def(0x9A, "TXS", (*CPU).opTXS, ModeImplied, 2, 0)
	def(0x98, "TYA", (*CPU).opTYA, ModeImplied, 2, 0)
}

func registerUndocumentedOpcodes() {
	// LAX
	defUnofficial(0xA7, "LAX", (*CPU).opLAX, ModeZeroPage, 3, 0)
	defUnofficial(0xB7, "LAX", (*CPU).opLAX, ModeZeroPageY, 4, 0)
	defUnofficial(0xAF, "LAX", (*CPU).opLAX, ModeAbsolute, 4, 0)
	defUnofficial(0xBF, "LAX", (*CPU).opLAX, ModeAbsoluteY, 4, 1)
	defUnofficial(0xA3, "LAX", (*CPU).opLAX, ModeIndirectX, 6, 0)
	defUnofficial(0xB3, "LAX", (*CPU).opLAX, ModeIndirectY, 5, 1)

	// SAX
	defUnofficial(0x87, "SAX", (*CPU).opSAX, ModeZeroPage, 3, 0)
	defUnofficial(0x97, "SAX", (*CPU).opSAX, ModeZeroPageY, 4, 0)
	defUnofficial(0x8F, "SAX", (*CPU).opSAX, ModeAbsolute, 4, 0)
	defUnofficial(0x83, "SAX", (*CPU).opSAX, ModeIndirectX, 6, 0)

	// DCP
	defUnofficial(0xC7, "DCP", (*CPU).opDCP, ModeZeroPage, 5, 0)
	defUnofficial(0xD7, "DCP", (*CPU).opDCP, ModeZeroPageX, 6, 0)
	defUnofficial(0xCF, "DCP", (*CPU).opDCP, ModeAbsolute, 6, 0)
	defUnofficial(0xDF, "DCP", (*CPU).opDCP, ModeAbsoluteX, 7, 0)
	defUnofficial(0xDB, "DCP", (*CPU).opDCP, ModeAbsoluteY, 7, 0)
	defUnofficial(0xC3, "DCP", (*CPU).opDCP, ModeIndirectX, 8, 0)
	defUnofficial(0xD3, "DCP", (*CPU).opDCP, ModeIndirectY, 8, 0)

	// ISB (ISC)
	defUnofficial(0xE7, "ISB", (*CPU).opISC, ModeZeroPage, 5, 0)
	defUnofficial(0xF7, "ISB", (*CPU).opISC, ModeZeroPageX, 6, 0)
	defUnofficial(0xEF, "ISB", (*CPU).opISC, ModeAbsolute, 6, 0)
	defUnofficial(0xFF, "ISB", (*CPU).opISC, ModeAbsoluteX, 7, 0)
	defUnofficial(0xFB, "ISB", (*CPU).opISC, ModeAbsoluteY, 7, 0)
	defUnofficial(0xE3, "ISB", (*CPU).opISC, ModeIndirectX, 8, 0)
	defUnofficial(0xF3, "ISB", (*CPU).opISC, ModeIndirectY, 8, 0)

	// SLO
	defUnofficial(0x07, "SLO", (*CPU).opSLO, ModeZeroPage, 5, 0)
	defUnofficial(0x17, "SLO", (*CPU).opSLO, ModeZeroPageX, 6, 0)
	defUnofficial(0x0F, "SLO", (*CPU).opSLO, ModeAbsolute, 6, 0)
	defUnofficial(0x1F, "SLO", (*CPU).opSLO, ModeAbsoluteX, 7, 0)
	defUnofficial(0x1B, "SLO", (*CPU).opSLO, ModeAbsoluteY, 7, 0)
	defUnofficial(0x03, "SLO", (*CPU).opSLO, ModeIndirectX, 8, 0)
	defUnofficial(0x13, "SLO", (*CPU).opSLO, ModeIndirectY, 8, 0)

	// RLA
	defUnofficial(0x27, "RLA", (*CPU).opRLA, ModeZeroPage, 5, 0)
	defUnofficial(0x37, "RLA", (*CPU).opRLA, ModeZeroPageX, 6, 0)
	defUnofficial(0x2F, "RLA", (*CPU).opRLA, ModeAbsolute, 6, 0)
	defUnofficial(0x3F, "RLA", (*CPU).opRLA, ModeAbsoluteX, 7, 0)
	defUnofficial(0x3B, "RLA", (*CPU).opRLA, ModeAbsoluteY, 7, 0)
	defUnofficial(0x23, "RLA", (*CPU).opRLA, ModeIndirectX, 8, 0)
	defUnofficial(0x33, "RLA", (*CPU).opRLA, ModeIndirectY, 8, 0)

	// SRE
	defUnofficial(0x47, "SRE", (*CPU).opSRE, ModeZeroPage, 5, 0)
	defUnofficial(0x57, "SRE", (*CPU).opSRE, ModeZeroPageX, 6, 0)
	defUnofficial(0x4F, "SRE", (*CPU).opSRE, ModeAbsolute, 6, 0)
	defUnofficial(0x5F, "SRE", (*CPU).opSRE, ModeAbsoluteX, 7, 0)
	defUnofficial(0x5B, "SRE", (*CPU).opSRE, ModeAbsoluteY, 7, 0)
	defUnofficial(0x43, "SRE", (*CPU).opSRE, ModeIndirectX, 8, 0)
	defUnofficial(0x53, "SRE", (*CPU).opSRE, ModeIndirectY, 8, 0)

	// RRA
	defUnofficial(0x67, "RRA", (*CPU).opRRA, ModeZeroPage, 5, 0)
	defUnofficial(0x77, "RRA", (*CPU).opRRA, ModeZeroPageX, 6, 0)
	defUnofficial(0x6F, "RRA", (*CPU).opRRA, ModeAbsolute, 6, 0)
	defUnofficial(0x7F, "RRA", (*CPU).opRRA, ModeAbsoluteX, 7, 0)
	defUnofficial(0x7B, "RRA", (*CPU).opRRA, ModeAbsoluteY, 7, 0)
	defUnofficial(0x63, "RRA", (*CPU).opRRA, ModeIndirectX, 8, 0)
	defUnofficial(0x73, "RRA", (*CPU).opRRA, ModeIndirectY, 8, 0)

	// Duplicate SBC ($EB)
	defUnofficial(0xEB, "SBC", (*CPU).opSBC, ModeImmediate, 2, 0)

	// Unofficial Immediates: ANC, ALR, ARR, XAA, LAX (LXA), AXS
	defUnofficial(0x0B, "ANC", (*CPU).opANC, ModeImmediate, 2, 0)
	defUnofficial(0x2B, "ANC", (*CPU).opANC, ModeImmediate, 2, 0)
	defUnofficial(0x4B, "ALR", (*CPU).opALR, ModeImmediate, 2, 0)
	defUnofficial(0x6B, "ARR", (*CPU).opARR, ModeImmediate, 2, 0)
	defUnofficial(0x8B, "XAA", (*CPU).opXAA, ModeImmediate, 2, 0)
	defUnofficial(0xAB, "LAX", (*CPU).opLXA, ModeImmediate, 2, 0)
	defUnofficial(0xCB, "AXS", (*CPU).opAXS, ModeImmediate, 2, 0)

	// Unofficial SH* group & LAS
	defUnofficial(0x93, "AHX", (*CPU).opSHAIndY, ModeIndirectY, 6, 0)
	defUnofficial(0x9F, "AHX", (*CPU).opSHAAbsY, ModeAbsoluteY, 5, 0)
	defUnofficial(0x9B, "TAS", (*CPU).opSHS, ModeAbsoluteY, 5, 0)
	defUnofficial(0x9C, "SHY", (*CPU).opSHY, ModeAbsoluteX, 5, 0)
	defUnofficial(0x9E, "SHX", (*CPU).opSHX, ModeAbsoluteY, 5, 0)
	defUnofficial(0xBB, "LAS", (*CPU).opLAS, ModeAbsoluteY, 4, 1)

	// Undocumented NOPs (IGN/SKB)
	defUnofficial(0x1A, "NOP", (*CPU).opNOP, ModeImplied, 2, 0)
	defUnofficial(0x3A, "NOP", (*CPU).opNOP, ModeImplied, 2, 0)
	defUnofficial(0x5A, "NOP", (*CPU).opNOP, ModeImplied, 2, 0)
	defUnofficial(0x7A, "NOP", (*CPU).opNOP, ModeImplied, 2, 0)
	defUnofficial(0xDA, "NOP", (*CPU).opNOP, ModeImplied, 2, 0)
	defUnofficial(0xFA, "NOP", (*CPU).opNOP, ModeImplied, 2, 0)

	defUnofficial(0x04, "NOP", (*CPU).opNOPRead, ModeZeroPage, 3, 0)
	defUnofficial(0x44, "NOP", (*CPU).opNOPRead, ModeZeroPage, 3, 0)
	defUnofficial(0x64, "NOP", (*CPU).opNOPRead, ModeZeroPage, 3, 0)
	defUnofficial(0x14, "NOP", (*CPU).opNOPRead, ModeZeroPageX, 4, 0)
	defUnofficial(0x34, "NOP", (*CPU).opNOPRead, ModeZeroPageX, 4, 0)
	defUnofficial(0x54, "NOP", (*CPU).opNOPRead, ModeZeroPageX, 4, 0)
	defUnofficial(0x74, "NOP", (*CPU).opNOPRead, ModeZeroPageX, 4, 0)
	defUnofficial(0xD4, "NOP", (*CPU).opNOPRead, ModeZeroPageX, 4, 0)
	defUnofficial(0xF4, "NOP", (*CPU).opNOPRead, ModeZeroPageX, 4, 0)

	defUnofficial(0x0C, "NOP", (*CPU).opNOPRead, ModeAbsolute, 4, 0)
	defUnofficial(0x1C, "NOP", (*CPU).opNOPRead, ModeAbsoluteX, 4, 1)
	defUnofficial(0x3C, "NOP", (*CPU).opNOPRead, ModeAbsoluteX, 4, 1)
	defUnofficial(0x5C, "NOP", (*CPU).opNOPRead, ModeAbsoluteX, 4, 1)
	defUnofficial(0x7C, "NOP", (*CPU).opNOPRead, ModeAbsoluteX, 4, 1)
	defUnofficial(0xDC, "NOP", (*CPU).opNOPRead, ModeAbsoluteX, 4, 1)
	defUnofficial(0xFC, "NOP", (*CPU).opNOPRead, ModeAbsoluteX, 4, 1)

	defUnofficial(0x80, "NOP", (*CPU).opNOP, ModeImmediate, 2, 0)
	defUnofficial(0x82, "NOP", (*CPU).opNOP, ModeImmediate, 2, 0)
	defUnofficial(0x89, "NOP", (*CPU).opNOP, ModeImmediate, 2, 0)
	defUnofficial(0xC2, "NOP", (*CPU).opNOP, ModeImmediate, 2, 0)
	defUnofficial(0xE2, "NOP", (*CPU).opNOP, ModeImmediate, 2, 0)
}

// Opcode adapter methods for CPU dispatch.

func (c *CPU) opADC(addr uint16, _ bool) int { c.adc(c.bus.Read(addr)); return 0 }
func (c *CPU) opAND(addr uint16, _ bool) int { c.and(addr); return 0 }
func (c *CPU) opASLAcc(_ uint16, _ bool) int { c.aslAcc(); return 0 }
func (c *CPU) opASLMem(addr uint16, _ bool) int { c.aslMem(addr); return 0 }

func (c *CPU) opBCC(addr uint16, pageCrossed bool) int { return c.branch(!c.GetFlag(FlagCarry), addr, pageCrossed) }
func (c *CPU) opBCS(addr uint16, pageCrossed bool) int { return c.branch(c.GetFlag(FlagCarry), addr, pageCrossed) }
func (c *CPU) opBEQ(addr uint16, pageCrossed bool) int { return c.branch(c.GetFlag(FlagZero), addr, pageCrossed) }
func (c *CPU) opBMI(addr uint16, pageCrossed bool) int { return c.branch(c.GetFlag(FlagNegative), addr, pageCrossed) }
func (c *CPU) opBNE(addr uint16, pageCrossed bool) int { return c.branch(!c.GetFlag(FlagZero), addr, pageCrossed) }
func (c *CPU) opBPL(addr uint16, pageCrossed bool) int { return c.branch(!c.GetFlag(FlagNegative), addr, pageCrossed) }
func (c *CPU) opBVC(addr uint16, pageCrossed bool) int { return c.branch(!c.GetFlag(FlagOverflow), addr, pageCrossed) }
func (c *CPU) opBVS(addr uint16, pageCrossed bool) int { return c.branch(c.GetFlag(FlagOverflow), addr, pageCrossed) }

func (c *CPU) opBIT(addr uint16, _ bool) int { c.bit(addr); return 0 }
func (c *CPU) opBRK(_ uint16, _ bool) int   { c.brk(); return 0 }

func (c *CPU) opCLC(_ uint16, _ bool) int { c.SetFlag(FlagCarry, false); return 0 }
func (c *CPU) opCLD(_ uint16, _ bool) int { c.SetFlag(FlagDecimal, false); return 0 }
func (c *CPU) opCLI(_ uint16, _ bool) int { c.SetFlag(FlagInterrupt, false); return 0 }
func (c *CPU) opCLV(_ uint16, _ bool) int { c.SetFlag(FlagOverflow, false); return 0 }
func (c *CPU) opSEC(_ uint16, _ bool) int { c.SetFlag(FlagCarry, true); return 0 }
func (c *CPU) opSED(_ uint16, _ bool) int { c.SetFlag(FlagDecimal, true); return 0 }
func (c *CPU) opSEI(_ uint16, _ bool) int { c.SetFlag(FlagInterrupt, true); return 0 }

func (c *CPU) opCMP(addr uint16, _ bool) int { c.cmp(c.A, addr); return 0 }
func (c *CPU) opCPX(addr uint16, _ bool) int { c.cmp(c.X, addr); return 0 }
func (c *CPU) opCPY(addr uint16, _ bool) int { c.cmp(c.Y, addr); return 0 }

func (c *CPU) opDEC(addr uint16, _ bool) int { c.dec(addr); return 0 }
func (c *CPU) opDEX(_ uint16, _ bool) int   { c.dex(); return 0 }
func (c *CPU) opDEY(_ uint16, _ bool) int   { c.dey(); return 0 }

func (c *CPU) opEOR(addr uint16, _ bool) int { c.eor(addr); return 0 }
func (c *CPU) opINC(addr uint16, _ bool) int { c.inc(addr); return 0 }
func (c *CPU) opINX(_ uint16, _ bool) int   { c.inx(); return 0 }
func (c *CPU) opINY(_ uint16, _ bool) int   { c.iny(); return 0 }

func (c *CPU) opJMP(addr uint16, _ bool) int { c.jmp(addr); return 0 }
func (c *CPU) opJSR(addr uint16, _ bool) int { c.jsr(addr); return 0 }
func (c *CPU) opRTS(_ uint16, _ bool) int   { c.rts(); return 0 }
func (c *CPU) opRTI(_ uint16, _ bool) int   { c.rti(); return 0 }

func (c *CPU) opLDA(addr uint16, _ bool) int { c.lda(addr); return 0 }
func (c *CPU) opLDX(addr uint16, _ bool) int { c.ldx(addr); return 0 }
func (c *CPU) opLDY(addr uint16, _ bool) int { c.ldy(addr); return 0 }

func (c *CPU) opLSRAcc(_ uint16, _ bool) int    { c.lsrAcc(); return 0 }
func (c *CPU) opLSRMem(addr uint16, _ bool) int { c.lsrMem(addr); return 0 }
func (c *CPU) opNOP(_ uint16, _ bool) int       { return 0 }

func (c *CPU) opORA(addr uint16, _ bool) int { c.ora(addr); return 0 }
func (c *CPU) opPHA(_ uint16, _ bool) int   { c.pha(); return 0 }
func (c *CPU) opPHP(_ uint16, _ bool) int   { c.php(); return 0 }
func (c *CPU) opPLA(_ uint16, _ bool) int   { c.pla(); return 0 }
func (c *CPU) opPLP(_ uint16, _ bool) int   { c.plp(); return 0 }

func (c *CPU) opROLAcc(_ uint16, _ bool) int    { c.rolAcc(); return 0 }
func (c *CPU) opROLMem(addr uint16, _ bool) int { c.rolMem(addr); return 0 }
func (c *CPU) opRORAcc(_ uint16, _ bool) int    { c.rorAcc(); return 0 }
func (c *CPU) opRORMem(addr uint16, _ bool) int { c.rorMem(addr); return 0 }

func (c *CPU) opSBC(addr uint16, _ bool) int { c.sbc(c.bus.Read(addr)); return 0 }
func (c *CPU) opSTA(addr uint16, _ bool) int { c.sta(addr); return 0 }
func (c *CPU) opSTX(addr uint16, _ bool) int { c.stx(addr); return 0 }
func (c *CPU) opSTY(addr uint16, _ bool) int { c.sty(addr); return 0 }

func (c *CPU) opTAX(_ uint16, _ bool) int { c.tax(); return 0 }
func (c *CPU) opTAY(_ uint16, _ bool) int { c.tay(); return 0 }
func (c *CPU) opTSX(_ uint16, _ bool) int { c.tsx(); return 0 }
func (c *CPU) opTXA(_ uint16, _ bool) int { c.txa(); return 0 }
func (c *CPU) opTXS(_ uint16, _ bool) int { c.txs(); return 0 }
func (c *CPU) opTYA(_ uint16, _ bool) int { c.tya(); return 0 }

func (c *CPU) opLAX(addr uint16, _ bool) int { c.lax(addr); return 0 }
func (c *CPU) opSAX(addr uint16, _ bool) int { c.sax(addr); return 0 }
func (c *CPU) opDCP(addr uint16, _ bool) int { c.dcp(addr); return 0 }
func (c *CPU) opISC(addr uint16, _ bool) int { c.isc(addr); return 0 }
func (c *CPU) opSLO(addr uint16, _ bool) int { c.slo(addr); return 0 }
func (c *CPU) opRLA(addr uint16, _ bool) int { c.rla(addr); return 0 }
func (c *CPU) opSRE(addr uint16, _ bool) int { c.sre(addr); return 0 }
func (c *CPU) opRRA(addr uint16, _ bool) int { c.rra(addr); return 0 }

func (c *CPU) opNOPRead(addr uint16, _ bool) int { c.bus.Read(addr); return 0 }

func (c *CPU) opANC(addr uint16, _ bool) int { c.anc(c.bus.Read(addr)); return 0 }
func (c *CPU) opALR(addr uint16, _ bool) int { c.alr(c.bus.Read(addr)); return 0 }
func (c *CPU) opARR(addr uint16, _ bool) int { c.arr(c.bus.Read(addr)); return 0 }
func (c *CPU) opXAA(addr uint16, _ bool) int { c.xaa(c.bus.Read(addr)); return 0 }
func (c *CPU) opLXA(addr uint16, _ bool) int { c.lxa(c.bus.Read(addr)); return 0 }
func (c *CPU) opAXS(addr uint16, _ bool) int { c.axs(c.bus.Read(addr)); return 0 }
func (c *CPU) opLAS(addr uint16, _ bool) int { c.las(c.bus.Read(addr)); return 0 }

func (c *CPU) opSHAIndY(addr uint16, crossed bool) int { c.sha(addr, crossed, c.Y); return 0 }
func (c *CPU) opSHAAbsY(addr uint16, crossed bool) int { c.sha(addr, crossed, c.Y); return 0 }
func (c *CPU) opSHS(addr uint16, crossed bool) int     { c.shs(addr, crossed); return 0 }
func (c *CPU) opSHY(addr uint16, crossed bool) int     { c.shy(addr, crossed); return 0 }
func (c *CPU) opSHX(addr uint16, crossed bool) int     { c.shx(addr, crossed); return 0 }

