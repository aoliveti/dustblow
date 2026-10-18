// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge

import (
	"encoding/binary"
	"fmt"
	"io"
)

type i2cState byte

const (
	i2cStateIdle i2cState = iota
	i2cStateDevAddr
	i2cStateWordAddr
	i2cStateWriteData
	i2cStateReadData
	i2cStateAck
)

// I2CEEPROM implements the serial I²C state machine for 24C01 (128 bytes) and 24C02 (256 bytes) EEPROMs.
type I2CEEPROM struct {
	cartridge *Cartridge
	capacity  int // 128 or 256

	prevSCL bool
	prevSDA bool
	sdaOut  bool

	state     i2cState
	nextState i2cState
	bitCount  int
	shiftReg  byte
	wordAddr  byte
	isRead    bool
}

// NewI2CEEPROM constructs a new I²C serial EEPROM with the specified capacity (128 or 256 bytes).
func NewI2CEEPROM(cart *Cartridge, capacity int) *I2CEEPROM {
	if len(cart.SRAM) < capacity {
		sram := make([]byte, capacity)
		copy(sram, cart.SRAM)
		cart.SRAM = sram
	}

	return &I2CEEPROM{
		cartridge: cart,
		capacity:  capacity,
		prevSCL:   true,
		prevSDA:   true,
		sdaOut:    true,
		state:     i2cStateIdle,
	}
}

// ReadSDA returns the current state of the Serial Data line output by the EEPROM.
func (e *I2CEEPROM) ReadSDA() bool {
	return e.sdaOut
}

// WriteLines updates SCL and SDA from the mapper control register (e.g. $800D).
// bit 0 = SCL, bit 1 = SDA.
func (e *I2CEEPROM) WriteLines(scl, sda bool) {
	// Detect START condition: SCL is HIGH, SDA transitions HIGH -> LOW
	if scl && e.prevSCL && e.prevSDA && !sda {
		e.state = i2cStateDevAddr
		e.bitCount = 0
		e.shiftReg = 0
		e.sdaOut = true
		e.prevSCL = scl
		e.prevSDA = sda
		return
	}

	// Detect STOP condition: SCL is HIGH, SDA transitions LOW -> HIGH
	if scl && e.prevSCL && !e.prevSDA && sda {
		e.state = i2cStateIdle
		e.sdaOut = true
		e.prevSCL = scl
		e.prevSDA = sda
		return
	}

	// Handle SCL clock edges
	if !e.prevSCL && scl {
		// Rising edge of SCL: receiver samples SDA
		e.onSCLRising(sda)
	}
	if e.prevSCL && !scl {
		// Falling edge of SCL: transmitter updates SDA
		e.onSCLFalling()
	}

	e.prevSCL = scl
	e.prevSDA = sda
}

func (e *I2CEEPROM) onSCLRising(sda bool) {
	switch e.state {
	case i2cStateDevAddr, i2cStateWordAddr, i2cStateWriteData:
		bit := byte(0)
		if sda {
			bit = 1
		}
		e.shiftReg = (e.shiftReg << 1) | bit
		e.bitCount++

		if e.bitCount == 8 {
			e.processReceivedByte()
		}

	case i2cStateReadData:
		e.bitCount++
		if e.bitCount == 8 {
			// Expect master ACK next
			e.state = i2cStateAck
			e.nextState = i2cStateReadData
			e.wordAddr = byte(((int(e.wordAddr) + 1) % e.capacity) & 0xFF)
		}

	case i2cStateAck:
		// Master clocked the ACK cycle; advance to nextState
		e.state = e.nextState
		e.bitCount = 0
	}
}

func (e *I2CEEPROM) onSCLFalling() {
	switch e.state {
	case i2cStateAck:
		// Device drives ACK (pulls SDA low)
		e.sdaOut = false

	case i2cStateReadData:
		// Device shifts out current bit of the addressed byte (MSB first)
		currByte := byte(0)
		if int(e.wordAddr) < len(e.cartridge.SRAM) {
			currByte = e.cartridge.SRAM[e.wordAddr]
		}
		bitShift := 7 - (e.bitCount % 8)
		e.sdaOut = ((currByte >> bitShift) & 1) != 0

	default:
		// Release SDA to high
		e.sdaOut = true
	}
}

func (e *I2CEEPROM) processReceivedByte() {
	switch e.state {
	case i2cStateDevAddr:
		// Device address received.
		// Standard I2C: 1010 xxx R/W.
		e.isRead = (e.shiftReg & 1) != 0
		e.state = i2cStateAck

		e.nextState = i2cStateWordAddr
		if e.isRead {
			e.nextState = i2cStateReadData
		}

	case i2cStateWordAddr:
		// Word address received.
		e.wordAddr = byte((int(e.shiftReg) % e.capacity) & 0xFF)
		e.state = i2cStateAck
		e.nextState = i2cStateWriteData

	case i2cStateWriteData:
		// Write byte to SRAM.
		if int(e.wordAddr) < len(e.cartridge.SRAM) {
			e.cartridge.SRAM[e.wordAddr] = e.shiftReg
			e.cartridge.MarkSRAMDirty()
		}
		e.wordAddr = byte(((int(e.wordAddr) + 1) % e.capacity) & 0xFF)
		e.state = i2cStateAck
		e.nextState = i2cStateWriteData
	}
}

// SaveState serializes the I²C EEPROM FSM state.
func (e *I2CEEPROM) SaveState(w io.Writer) error {
	state := struct {
		PrevSCL   bool
		PrevSDA   bool
		SDAOut    bool
		State     byte
		NextState byte
		BitCount  int32
		ShiftReg  byte
		WordAddr  byte
		IsRead    bool
	}{
		PrevSCL:   e.prevSCL,
		PrevSDA:   e.prevSDA,
		SDAOut:    e.sdaOut,
		State:     byte(e.state),
		NextState: byte(e.nextState),
		BitCount:  int32(e.bitCount & 0x7FFFFFFF),
		ShiftReg:  e.shiftReg,
		WordAddr:  e.wordAddr,
		IsRead:    e.isRead,
	}

	if err := binary.Write(w, binary.LittleEndian, state); err != nil {
		return fmt.Errorf("write i2c eeprom state: %w", err)
	}
	return nil
}

// LoadState restores the I²C EEPROM FSM state.
func (e *I2CEEPROM) LoadState(r io.Reader) error {
	var state struct {
		PrevSCL   bool
		PrevSDA   bool
		SDAOut    bool
		State     byte
		NextState byte
		BitCount  int32
		ShiftReg  byte
		WordAddr  byte
		IsRead    bool
	}

	if err := binary.Read(r, binary.LittleEndian, &state); err != nil {
		return fmt.Errorf("read i2c eeprom state: %w", err)
	}

	e.prevSCL = state.PrevSCL
	e.prevSDA = state.PrevSDA
	e.sdaOut = state.SDAOut
	e.state = i2cState(state.State)
	e.nextState = i2cState(state.NextState)
	e.bitCount = int(state.BitCount)
	e.shiftReg = state.ShiftReg
	e.wordAddr = state.WordAddr
	e.isRead = state.IsRead
	return nil
}
