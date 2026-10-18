// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cartridge implements NES cartridge loading, iNES header parsing, and memory mappers.
//
// iNES 1.0 File Format Specification:
//
//	+------------------+ 0x0000
//	| Header (16 B)    | 16-byte metadata header
//	+------------------+ 0x0010
//	| Trainer (512 B)  | Optional trainer data for older dumps (if flag set)
//	+------------------+
//	| PRG-ROM Data     | N * 16 KB (Program code for 6502 CPU)
//	+------------------+
//	| CHR-ROM Data     | N * 8 KB (Tile patterns for 2C02 PPU, if present)
//	+------------------+ End of File
//
// 16-Byte Header Layout:
//
//	Bytes 0-3:  Identification string "NES\x1A" (0x4E 0x45 0x53 0x1A)
//	Byte 4:     PRG-ROM size in 16 KB units (16384 bytes each)
//	Byte 5:     CHR-ROM size in 8 KB units (8192 bytes each; 0 indicates 8 KB CHR-RAM)
//	Byte 6:     Hardware Configuration (Control Byte 1):
//	              Bit 0: Mirroring (0: Horizontal / vertical arrangement, 1: Vertical / horizontal arrangement)
//	              Bit 1: Battery-backed PRG-RAM ($6000-$7FFF) present
//	              Bit 2: 512-byte Trainer present at $7000-$71FF before PRG-ROM
//	              Bit 3: Four-screen VRAM layout
//	              Bits 4-7: Lower 4 bits of Mapper ID (bits 0-3)
//	Byte 7:     Extended Flags (Control Byte 2):
//	              Bit 0: VS Unisystem arcade system
//	              Bit 1: PlayChoice-10 arcade system
//	              Bits 2-3: NES 2.0 format identifier (if bits 2-3 == 2)
//	              Bits 4-7: Upper 4 bits of Mapper ID (bits 4-7)
//	Bytes 8-15: Padding / Reserved (typically zeros in iNES 1.0 format)
package cartridge

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

const (
	headerSize  = 16
	prgUnitSize = 16 * 1024 // 16 KB PRG-ROM unit
	chrUnitSize = 8 * 1024  // 8 KB CHR-ROM unit
	chrRAMSize  = 8 * 1024  // 8 KB default CHR-RAM size
	trainerSize = 512       // 512 bytes trainer (if present)

	// Header byte indices.
	headerIndexPRGUnits  = 4
	headerIndexCHRUnits  = 5
	headerIndexHWConfig  = 6
	headerIndexExtFlags  = 7
	headerIndexSubmapper = 8

	// Hardware configuration bitmasks (Byte 6).
	flagMirrorVertical    = 0x01
	flagBatteryPresent    = 0x02
	flagTrainerPresent    = 0x04
	flagFourScreenMirror  = 0x08
	flagMapperLowerNibble = 0xF0

	// Extended flags bitmasks (Byte 7).
	flagMapperUpperNibble = 0xF0
	flagNES20Mask         = 0x0C
	flagNES20Magic        = 0x08

	// Bit shifts.
	mapperNibbleShift = 4
	submapperShift    = 4

	// Supported Mapper IDs.
	mapperNROM    = 0
	mapperMMC1    = 1
	mapperUxROM   = 2
	mapperCNROM   = 3
	mapperMMC3    = 4
	mapperMMC5    = 5
	mapperAxROM   = 7
	mapperMMC2    = 9
	mapperMMC4      = 10
	mapperBandai16  = 16
	mapperNamco163  = 19
	mapperVRC4a4c   = 21
	mapperVRC2a     = 22
	mapperVRC2b4e   = 23
	mapperVRC6a     = 24
	mapperVRC2c4b   = 25
	mapperVRC6b     = 26
	mapperBNROM     = 34
	mapperSMB2J     = 40
	mapperGxROM     = 66
	mapperFME7      = 69
	mapperCamerica  = 71
	mapperBandai153 = 153
	mapperBandai159 = 159
)

var (
	// inesMagic is the 4-byte identification header "NES\x1A".
	inesMagic = [4]byte{'N', 'E', 'S', 0x1A}

	// ErrInvalidHeader is returned when the ROM header is too short or lacks the iNES magic bytes.
	ErrInvalidHeader = errors.New("cartridge: invalid ines header")
	// ErrUnsupportedMapper is returned when the ROM requires an unimplemented mapper.
	ErrUnsupportedMapper = errors.New("cartridge: unsupported mapper")
	// ErrTruncatedROM is returned when the ROM payload is smaller than declared in the header.
	ErrTruncatedROM = errors.New("cartridge: truncated rom payload")
)

// New parses an iNES format ROM from an io.Reader stream and returns an initialized Cartridge.
func New(r io.Reader) (*Cartridge, error) {
	var header [headerSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, ErrInvalidHeader
	}

	if [4]byte(header[:4]) != inesMagic {
		return nil, ErrInvalidHeader
	}

	prgSize := int(header[headerIndexPRGUnits]) * prgUnitSize
	chrSize := int(header[headerIndexCHRUnits]) * chrUnitSize
	hwConfig := header[headerIndexHWConfig]
	extFlags := header[headerIndexExtFlags]

	mapperID := (extFlags & flagMapperUpperNibble) | ((hwConfig & flagMapperLowerNibble) >> mapperNibbleShift)
	mirroring := parseMirroring(hwConfig)
	hasBattery := (hwConfig & flagBatteryPresent) != 0
	hasTrainer := (hwConfig & flagTrainerPresent) != 0

	if hasTrainer {
		if _, err := io.CopyN(io.Discard, r, trainerSize); err != nil {
			return nil, ErrTruncatedROM
		}
	}

	prg := make([]byte, prgSize)
	if _, err := io.ReadFull(r, prg); err != nil {
		return nil, ErrTruncatedROM
	}

	chr := make([]byte, chrRAMSize)
	if chrSize > 0 {
		chr = make([]byte, chrSize)
		if _, err := io.ReadFull(r, chr); err != nil {
			return nil, ErrTruncatedROM
		}
	}

	hasher := sha256.New()
	hasher.Write(prg)
	hasher.Write(chr)

	var submapper byte
	if (extFlags & flagNES20Mask) == flagNES20Magic {
		submapper = header[headerIndexSubmapper] >> submapperShift
	}

	cart := &Cartridge{
		PRG:        prg,
		CHR:        chr,
		IsCHRRAM:   chrSize == 0,
		SRAM:       make([]byte, SRAMSize),
		MapperID:   mapperID,
		Submapper:  submapper,
		Mirroring:  mirroring,
		HasBattery: hasBattery,
		Hash:       hex.EncodeToString(hasher.Sum(nil)),
	}

	var mapper Mapper
	switch mapperID {
	case mapperNROM:
		mapper = NewMapper0(cart)
	case mapperMMC1:
		mapper = NewMapper1(cart)
	case mapperUxROM:
		mapper = NewMapper2(cart)
	case mapperCNROM:
		mapper = NewMapper3(cart)
	case mapperMMC3:
		mapper = NewMapper4(cart)
	case mapperMMC5:
		mapper = NewMapper5(cart)
	case mapperAxROM:
		mapper = NewMapper7(cart)
	case mapperMMC2:
		mapper = NewMapper9(cart)
	case mapperMMC4:
		mapper = NewMapper10(cart)
	case mapperBandai16, mapperBandai153, mapperBandai159:
		mapper = NewMapperBandai(cart, mapperID)
	case mapperNamco163:
		mapper = NewMapper19(cart)
	case mapperVRC4a4c, mapperVRC2a, mapperVRC2b4e, mapperVRC2c4b:
		mapper = NewMapperVRC(cart, mapperID, submapper)
	case mapperVRC6a, mapperVRC6b:
		mapper = NewMapperVRC6(cart, mapperID)
	case mapperBNROM:
		mapper = NewMapper34(cart)
	case mapperSMB2J:
		mapper = NewMapper40(cart)
	case mapperGxROM:
		mapper = NewMapper66(cart)
	case mapperFME7:
		mapper = NewMapper69(cart)
	case mapperCamerica:
		mapper = NewMapper71(cart)
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedMapper, mapperID)
	}

	cart.BindMapper(mapper)
	return cart, nil
}

func parseMirroring(hwConfig byte) Mirroring {
	if hwConfig&flagFourScreenMirror != 0 {
		return MirrorFourScreen
	}
	if hwConfig&flagMirrorVertical != 0 {
		return MirrorVertical
	}
	return MirrorHorizontal
}
