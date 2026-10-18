// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package console

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/dustblow/dustblow/internal/cartridge"
	"github.com/dustblow/dustblow/internal/cpu"
	"github.com/dustblow/dustblow/internal/ppu"
)

const (
	snapshotMagic   = "DUST"
	snapshotVersion = uint32(1)
)

var (
	// ErrInvalidSnapshot is returned when the snapshot header or payload is corrupted.
	ErrInvalidSnapshot = errors.New("console: invalid snapshot data")
	// ErrUnsupportedVersion is returned when snapshot version is newer than supported.
	ErrUnsupportedVersion = errors.New("console: unsupported snapshot version")
)

// SaveState serializes the entire console hardware state into a compact binary snapshot.
func (c *Console) SaveState() ([]byte, error) {
	if c == nil || c.CPU == nil || c.PPU == nil || c.Bus == nil {
		return nil, ErrInvalidSnapshot
	}

	buf := new(bytes.Buffer)
	buf.WriteString(snapshotMagic)
	if err := binary.Write(buf, binary.LittleEndian, snapshotVersion); err != nil {
		return nil, fmt.Errorf("write header: %w", err)
	}

	if err := c.saveCPU(buf); err != nil {
		return nil, err
	}

	ram := c.Bus.RAM()
	buf.Write(ram[:])

	if err := c.savePPU(buf); err != nil {
		return nil, err
	}

	if err := c.saveCartridge(buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (c *Console) saveCPU(buf *bytes.Buffer) error {
	cpuState := c.CPU.State()
	if err := binary.Write(buf, binary.LittleEndian, cpuState.Cycles); err != nil {
		return fmt.Errorf("write cpu cycles: %w", err)
	}
	if err := binary.Write(buf, binary.LittleEndian, cpuState.PC); err != nil {
		return fmt.Errorf("write cpu pc: %w", err)
	}
	cpuRegs := [5]byte{cpuState.A, cpuState.X, cpuState.Y, cpuState.SP, cpuState.Status}
	buf.Write(cpuRegs[:])
	return nil
}

func (c *Console) savePPU(buf *bytes.Buffer) error {
	ps := c.PPU.State()
	buf.Write(ps.VRAM[:])
	buf.Write(ps.OAM[:])
	buf.Write(ps.Palette[:])

	if err := binary.Write(buf, binary.LittleEndian, ps.Frame); err != nil {
		return fmt.Errorf("write ppu frame: %w", err)
	}
	if err := binary.Write(buf, binary.LittleEndian, safeInt32(ps.Scanline)); err != nil {
		return fmt.Errorf("write ppu scanline: %w", err)
	}
	if err := binary.Write(buf, binary.LittleEndian, safeInt32(ps.Cycle)); err != nil {
		return fmt.Errorf("write ppu cycle: %w", err)
	}
	if err := binary.Write(buf, binary.LittleEndian, ps.V); err != nil {
		return fmt.Errorf("write ppu v: %w", err)
	}
	if err := binary.Write(buf, binary.LittleEndian, ps.T); err != nil {
		return fmt.Errorf("write ppu t: %w", err)
	}
	if err := binary.Write(buf, binary.LittleEndian, ps.BGShiftPatternLow); err != nil {
		return fmt.Errorf("write ppu bg pattern low: %w", err)
	}
	if err := binary.Write(buf, binary.LittleEndian, ps.BGShiftPatternHigh); err != nil {
		return fmt.Errorf("write ppu bg pattern high: %w", err)
	}
	if err := binary.Write(buf, binary.LittleEndian, ps.BGShiftAttrLow); err != nil {
		return fmt.Errorf("write ppu bg attr low: %w", err)
	}
	if err := binary.Write(buf, binary.LittleEndian, ps.BGShiftAttrHigh); err != nil {
		return fmt.Errorf("write ppu bg attr high: %w", err)
	}

	ppuRegs := [6]byte{ps.Ctrl, ps.Mask, ps.Status, ps.OAMAddr, ps.ReadBuffer, ps.X}
	buf.Write(ppuRegs[:])

	var boolFlags byte
	if ps.W {
		boolFlags |= 0x01
	}
	if ps.NMICurrent {
		boolFlags |= 0x02
	}
	if ps.NMIPrevious {
		boolFlags |= 0x04
	}
	buf.WriteByte(boolFlags)
	return nil
}

func (c *Console) saveCartridge(buf *bytes.Buffer) error {
	if c.Cartridge == nil {
		buf.WriteByte(0)
		_ = binary.Write(buf, binary.LittleEndian, uint32(0))
		_ = binary.Write(buf, binary.LittleEndian, uint32(0))
		_ = binary.Write(buf, binary.LittleEndian, uint32(0))
		return nil
	}

	buf.WriteByte(byte(c.Cartridge.Mirroring))

	sramLen := safeUint32(len(c.Cartridge.SRAM))
	if err := binary.Write(buf, binary.LittleEndian, sramLen); err != nil {
		return fmt.Errorf("write sram len: %w", err)
	}
	if sramLen > 0 {
		buf.Write(c.Cartridge.SRAM)
	}

	chrLen := safeUint32(len(c.Cartridge.CHR))
	if err := binary.Write(buf, binary.LittleEndian, chrLen); err != nil {
		return fmt.Errorf("write chr len: %w", err)
	}
	if chrLen > 0 {
		buf.Write(c.Cartridge.CHR)
	}

	var mapperBuf bytes.Buffer
	if err := c.Cartridge.SaveState(&mapperBuf); err != nil {
		return fmt.Errorf("write mapper state: %w", err)
	}
	mapperLen := safeUint32(mapperBuf.Len())
	if err := binary.Write(buf, binary.LittleEndian, mapperLen); err != nil {
		return fmt.Errorf("write mapper len: %w", err)
	}
	if mapperLen > 0 {
		buf.Write(mapperBuf.Bytes())
	}
	return nil
}

// LoadState restores the entire console hardware state from a binary snapshot.
func (c *Console) LoadState(data []byte) error {
	if c == nil || c.CPU == nil || c.PPU == nil || c.Bus == nil {
		return ErrInvalidSnapshot
	}

	r := bytes.NewReader(data)
	if err := loadHeader(r); err != nil {
		return err
	}
	if err := c.loadCPU(r); err != nil {
		return err
	}
	if err := c.loadRAM(r); err != nil {
		return err
	}
	if err := c.loadPPU(r); err != nil {
		return err
	}
	return c.loadCartridge(r)
}

func loadHeader(r io.Reader) error {
	var magic [4]byte
	if err := binary.Read(r, binary.LittleEndian, &magic); err != nil {
		return ErrInvalidSnapshot
	}
	if string(magic[:]) != snapshotMagic {
		return fmt.Errorf("%w: unrecognized magic %q", ErrInvalidSnapshot, string(magic[:]))
	}

	var version uint32
	if err := binary.Read(r, binary.LittleEndian, &version); err != nil {
		return ErrInvalidSnapshot
	}
	if version != snapshotVersion {
		return fmt.Errorf("%w: got %d, want %d", ErrUnsupportedVersion, version, snapshotVersion)
	}
	return nil
}

func (c *Console) loadCPU(r io.Reader) error {
	var cycles uint64
	var pc uint16
	if err := binary.Read(r, binary.LittleEndian, &cycles); err != nil {
		return fmt.Errorf("%w: read cpu cycles: %w", ErrInvalidSnapshot, err)
	}
	if err := binary.Read(r, binary.LittleEndian, &pc); err != nil {
		return fmt.Errorf("%w: read cpu pc: %w", ErrInvalidSnapshot, err)
	}

	var cpuRegs [5]byte
	if err := binary.Read(r, binary.LittleEndian, &cpuRegs); err != nil {
		return fmt.Errorf("%w: read cpu regs: %w", ErrInvalidSnapshot, err)
	}

	c.CPU.RestoreState(cpu.State{
		Cycles: cycles,
		PC:     pc,
		A:      cpuRegs[0],
		X:      cpuRegs[1],
		Y:      cpuRegs[2],
		SP:     cpuRegs[3],
		Status: cpuRegs[4],
	})
	return nil
}

func (c *Console) loadRAM(r io.Reader) error {
	var ram [2048]byte
	if err := binary.Read(r, binary.LittleEndian, &ram); err != nil {
		return fmt.Errorf("%w: read ram: %w", ErrInvalidSnapshot, err)
	}
	c.Bus.SetRAM(ram)
	return nil
}

func (c *Console) loadPPU(r io.Reader) error {
	var vram [2048]byte
	var oam [256]byte
	var palette [32]byte
	if err := binary.Read(r, binary.LittleEndian, &vram); err != nil {
		return fmt.Errorf("%w: read vram: %w", ErrInvalidSnapshot, err)
	}
	if err := binary.Read(r, binary.LittleEndian, &oam); err != nil {
		return fmt.Errorf("%w: read oam: %w", ErrInvalidSnapshot, err)
	}
	if err := binary.Read(r, binary.LittleEndian, &palette); err != nil {
		return fmt.Errorf("%w: read palette: %w", ErrInvalidSnapshot, err)
	}

	var frame uint64
	var scanline, cycle int32
	var v, t, bgLow, bgHigh, attrLow, attrHigh uint16
	if err := binary.Read(r, binary.LittleEndian, &frame); err != nil {
		return fmt.Errorf("%w: read ppu frame: %w", ErrInvalidSnapshot, err)
	}
	if err := binary.Read(r, binary.LittleEndian, &scanline); err != nil {
		return fmt.Errorf("%w: read ppu scanline: %w", ErrInvalidSnapshot, err)
	}
	if err := binary.Read(r, binary.LittleEndian, &cycle); err != nil {
		return fmt.Errorf("%w: read ppu cycle: %w", ErrInvalidSnapshot, err)
	}
	if err := binary.Read(r, binary.LittleEndian, &v); err != nil {
		return fmt.Errorf("%w: read ppu v: %w", ErrInvalidSnapshot, err)
	}
	if err := binary.Read(r, binary.LittleEndian, &t); err != nil {
		return fmt.Errorf("%w: read ppu t: %w", ErrInvalidSnapshot, err)
	}
	if err := binary.Read(r, binary.LittleEndian, &bgLow); err != nil {
		return fmt.Errorf("%w: read ppu bg low: %w", ErrInvalidSnapshot, err)
	}
	if err := binary.Read(r, binary.LittleEndian, &bgHigh); err != nil {
		return fmt.Errorf("%w: read ppu bg high: %w", ErrInvalidSnapshot, err)
	}
	if err := binary.Read(r, binary.LittleEndian, &attrLow); err != nil {
		return fmt.Errorf("%w: read ppu attr low: %w", ErrInvalidSnapshot, err)
	}
	if err := binary.Read(r, binary.LittleEndian, &attrHigh); err != nil {
		return fmt.Errorf("%w: read ppu attr high: %w", ErrInvalidSnapshot, err)
	}

	var ppuRegs [6]byte
	if err := binary.Read(r, binary.LittleEndian, &ppuRegs); err != nil {
		return fmt.Errorf("%w: read ppu regs: %w", ErrInvalidSnapshot, err)
	}

	var boolFlags byte
	if err := binary.Read(r, binary.LittleEndian, &boolFlags); err != nil {
		return fmt.Errorf("%w: read bool flags: %w", ErrInvalidSnapshot, err)
	}

	c.PPU.RestoreState(ppu.State{
		VRAM:               vram,
		OAM:                oam,
		Palette:            palette,
		Frame:              frame,
		Scanline:           int(scanline),
		Cycle:              int(cycle),
		V:                  v,
		T:                  t,
		BGShiftPatternLow:  bgLow,
		BGShiftPatternHigh: bgHigh,
		BGShiftAttrLow:     attrLow,
		BGShiftAttrHigh:    attrHigh,
		Ctrl:               ppuRegs[0],
		Mask:               ppuRegs[1],
		Status:             ppuRegs[2],
		OAMAddr:            ppuRegs[3],
		ReadBuffer:         ppuRegs[4],
		X:                  ppuRegs[5],
		W:                  (boolFlags & 0x01) != 0,
		NMICurrent:         (boolFlags & 0x02) != 0,
		NMIPrevious:        (boolFlags & 0x04) != 0,
	})
	return nil
}

func (c *Console) loadCartridge(r io.Reader) error {
	var mirroring byte
	if err := binary.Read(r, binary.LittleEndian, &mirroring); err != nil {
		return fmt.Errorf("%w: read mirroring: %w", ErrInvalidSnapshot, err)
	}
	if c.Cartridge != nil {
		c.Cartridge.Mirroring = cartridge.Mirroring(mirroring)
	}

	var sramLen uint32
	if err := binary.Read(r, binary.LittleEndian, &sramLen); err != nil {
		return fmt.Errorf("%w: read sram len: %w", ErrInvalidSnapshot, err)
	}
	if sramLen > 0 {
		sramData := make([]byte, sramLen)
		if err := binary.Read(r, binary.LittleEndian, sramData); err != nil {
			return fmt.Errorf("%w: read sram data: %w", ErrInvalidSnapshot, err)
		}
		if c.Cartridge != nil {
			c.Cartridge.SRAM = sramData
		}
	}

	var chrLen uint32
	if err := binary.Read(r, binary.LittleEndian, &chrLen); err != nil {
		return fmt.Errorf("%w: read chr len: %w", ErrInvalidSnapshot, err)
	}
	if chrLen > 0 {
		chrData := make([]byte, chrLen)
		if err := binary.Read(r, binary.LittleEndian, chrData); err != nil {
			return fmt.Errorf("%w: read chr data: %w", ErrInvalidSnapshot, err)
		}
		if c.Cartridge != nil && len(c.Cartridge.CHR) == int(chrLen) {
			copy(c.Cartridge.CHR, chrData)
		}
	}

	var mapperLen uint32
	if err := binary.Read(r, binary.LittleEndian, &mapperLen); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil
		}
		return fmt.Errorf("%w: read mapper len: %w", ErrInvalidSnapshot, err)
	}
	if mapperLen == 0 {
		return nil
	}
	mapperData := make([]byte, mapperLen)
	if err := binary.Read(r, binary.LittleEndian, mapperData); err != nil {
		return fmt.Errorf("%w: read mapper data: %w", ErrInvalidSnapshot, err)
	}
	if c.Cartridge != nil {
		if err := c.Cartridge.LoadState(bytes.NewReader(mapperData)); err != nil {
			return fmt.Errorf("%w: restore mapper state: %w", ErrInvalidSnapshot, err)
		}
	}
	return nil
}

func safeInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}

func safeUint32(v int) uint32 {
	if v < 0 {
		return 0
	}
	if v > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(v)
}
