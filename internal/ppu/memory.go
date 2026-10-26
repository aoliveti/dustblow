// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ppu

import "github.com/aoliveti/dustblow/internal/cartridge"

const (
	// VRAMSize is the physical capacity of the console's internal video RAM (2048 bytes).
	VRAMSize = 2048
	// PaletteSize is the size of the internal palette RAM (32 bytes).
	PaletteSize = 32
	// OAMSize is the capacity of Object Attribute Memory (256 bytes, 64 sprites * 4 bytes).
	OAMSize = 256
	// oamRowBytes is the size of an 8-byte row involved in OAM corruption.
	oamRowBytes = 8
	// oamMaxRows is the number of 8-byte rows in 256-byte OAM.
	oamMaxRows = 32
	// defaultOAMCorruptionRow is the fallback corrupted row index.
	defaultOAMCorruptionRow = 4

	// Memory map boundary constants ($0000-$3FFF).
	patternTable0Base uint16 = 0x0000
	patternTable1Base uint16 = 0x1000
	patternTableEnd   uint16 = 0x1FFF
	nametableBase     uint16 = 0x2000
	nametableEnd      uint16 = 0x3EFF
	paletteBase       uint16 = 0x3F00

	// Nametable geometry constants.
	nametableSize       uint16 = 0x0400 // 1024 bytes per nametable
	nametableOffsetMask uint16 = 0x03FF // 10-bit offset within a nametable
	nametableWrapMask   uint16 = 0x0FFF // 12-bit range within $2000-$2FFF
	paletteOffsetMask   uint16 = 0x001F // 5-bit index within 32-byte palette

	// Palette mirroring targets ($3F10, $3F14, $3F18, $3F1C mirror $3F00, $3F04, $3F08, $3F0C).
	paletteMirrorSprite0 = 0x10
	paletteMirrorSprite1 = 0x14
	paletteMirrorSprite2 = 0x18
	paletteMirrorSprite3 = 0x1C
	paletteMirrorOffset  = 0x10
)

// mirrorPaletteAddress maps palette RAM addresses ($3F00-$3FFF) into a 0-31 byte index,
// applying the hardware mirrors for sprite backdrop indices.
func mirrorPaletteAddress(addr uint16) uint16 {
	idx := addr & paletteOffsetMask
	switch idx {
	case paletteMirrorSprite0, paletteMirrorSprite1, paletteMirrorSprite2, paletteMirrorSprite3:
		return idx - paletteMirrorOffset
	default:
		return idx
	}
}

// mirrorNametableAddress translates an address in $2000-$3EFF to an offset in internal 2KB VRAM
// based on the cartridge's physical mirroring layout.
func mirrorNametableAddress(addr uint16, mirroring cartridge.Mirroring) uint16 {
	normalized := (addr - nametableBase) & nametableWrapMask
	table := normalized / nametableSize
	offset := normalized & nametableOffsetMask

	switch mirroring {
	case cartridge.MirrorSingle0:
		// All nametables map to physical page 0.
		table = 0
	case cartridge.MirrorSingle1:
		// All nametables map to physical page 1.
		table = 1
	case cartridge.MirrorHorizontal:
		// Nametables 0 & 1 map to physical page 0; Nametables 2 & 3 map to physical page 1.
		table /= 2
	case cartridge.MirrorVertical:
		// Nametables 0 & 2 map to physical page 0; Nametables 1 & 3 map to physical page 1.
		table %= 2
	case cartridge.MirrorFourScreen:
		// 4-screen mirroring uses external RAM; wrap safely within 2KB if unmapped.
		table %= 2
	}

	return (table * nametableSize) + offset
}

// ReadVRAM reads a byte from the 14-bit PPU address space ($0000-$3FFF).
func (p *PPU) ReadVRAM(addr uint16) byte {
	addr &= maskVRAM14Bit

	switch {
	case addr <= patternTableEnd:
		return p.cartridge.ReadCHR(addr)
	case addr <= nametableEnd:
		if val, ok := p.cartridge.ReadNametable(addr, p.vram[:]); ok {
			return val
		}
		mirroring := cartridge.MirrorHorizontal
		if p.cartridge != nil {
			mirroring = p.cartridge.Mirroring
		}
		return p.vram[mirrorNametableAddress(addr, mirroring)]
	default:
		return p.palette[mirrorPaletteAddress(addr)]
	}
}

// WriteVRAM writes a byte to the 14-bit PPU address space ($0000-$3FFF).
func (p *PPU) WriteVRAM(addr uint16, val byte) {
	addr &= maskVRAM14Bit

	switch {
	case addr <= patternTableEnd:
		p.cartridge.WriteCHR(addr, val)
	case addr <= nametableEnd:
		if p.cartridge.WriteNametable(addr, val, p.vram[:]) {
			return
		}
		mirroring := cartridge.MirrorHorizontal
		if p.cartridge != nil {
			mirroring = p.cartridge.Mirroring
		}
		p.vram[mirrorNametableAddress(addr, mirroring)] = val
	default:
		p.palette[mirrorPaletteAddress(addr)] = val
	}
}

// PeekVRAM reads a byte passively without side effects from the 14-bit PPU address space.
func (p *PPU) PeekVRAM(addr uint16) byte {
	addr &= maskVRAM14Bit
	return p.ReadVRAM(addr)
}

// WriteOAM writes a byte to the internal 256-byte OAM memory at the given address.
func (p *PPU) WriteOAM(addr, val byte) {
	p.oam[addr] = val
}

// ReadOAM reads a byte from the internal 256-byte OAM memory at the given address.
func (p *PPU) ReadOAM(addr byte) byte {
	return p.oam[addr]
}

// OAMAddr returns the current OAM address pointer.
func (p *PPU) OAMAddr() byte {
	return p.oamAddr
}
