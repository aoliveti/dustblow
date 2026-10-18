// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package console_test

import (
	"testing"

	"github.com/dustblow/dustblow/internal/cartridge"
	"github.com/dustblow/dustblow/internal/console"
	"github.com/dustblow/dustblow/internal/cpu"
	"github.com/dustblow/dustblow/internal/joypad"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	prgRomSize32KB = 32768
	chrRomSize8KB  = 8192

	sampleStrobeHigh byte = 0x01
	sampleStrobeLow  byte = 0x00
	buttonPressed    byte = 0x01
	buttonReleased   byte = 0x00

	registerJoypad1Addr     uint16 = 0x4016
	registerJoypad2Addr     uint16 = 0x4017
	registerFrameCounterAddr uint16 = 0x4017
	resetVectorAddr         uint16 = 0xFFFC
	testProgramOrigin   uint16 = 0x8000
	expectedFramePixels        = 256 * 240 * 4
)

func newTestCartridge() *cartridge.Cartridge {
	prg := make([]byte, prgRomSize32KB)
	// Set Reset vector at $FFFC-$FFFD to point to $8000
	vectorOffset := resetVectorAddr - testProgramOrigin
	prg[vectorOffset] = byte(testProgramOrigin & 0xFF)
	prg[vectorOffset+1] = byte(testProgramOrigin >> 8)
	// Put a NOP (0xEA) at $8000
	prg[0] = 0xEA

	cart := &cartridge.Cartridge{
		PRG: prg,
		CHR: make([]byte, chrRomSize8KB),
	}
	cart.Mapper = cartridge.NewMapper0(cart)
	return cart
}

func TestConsole_Initialization(t *testing.T) {
	t.Parallel()

	cart := newTestCartridge()
	c := console.New(cart)
	require.NotNil(t, c)
	require.NotNil(t, c.CPU)
	require.NotNil(t, c.PPU)
	require.NotNil(t, c.Bus)

	assert.True(t, c.IsJoypadConnected(joypad.Port1))
	assert.True(t, c.IsJoypadConnected(joypad.Port2))
}

func TestConsole_InputAndHotPlugging(t *testing.T) {
	t.Parallel()

	cart := newTestCartridge()
	c := console.New(cart)

	// Set button via Console API
	c.SetButton(joypad.Port1, joypad.ButtonA, true)
	c.SetButtons(joypad.Port2, byte(joypad.ButtonB))

	// Strobe via memory write
	c.Bus.Write(registerJoypad1Addr, sampleStrobeHigh)
	c.Bus.Write(registerJoypad1Addr, sampleStrobeLow)

	assert.Equal(t, buttonPressed, c.Bus.Read(registerJoypad1Addr))
	assert.Equal(t, buttonReleased, c.Bus.Read(registerJoypad1Addr))

	assert.Equal(t, buttonReleased, c.Bus.Read(registerJoypad2Addr))
	assert.Equal(t, buttonPressed, c.Bus.Read(registerJoypad2Addr))

	// Disconnect Port 1
	c.DisconnectJoypad(joypad.Port1)
	assert.False(t, c.IsJoypadConnected(joypad.Port1))

	// Inactive port read returns open bus
	assert.Equal(t, byte(0xFF), c.Bus.Read(registerJoypad1Addr))

	// Setting buttons on disconnected port is a safe no-op
	c.SetButton(joypad.Port1, joypad.ButtonA, true)
	c.SetButtons(joypad.Port1, 0xFF)

	// Reconnect Port 1
	c.ConnectJoypad(joypad.Port1)
	assert.True(t, c.IsJoypadConnected(joypad.Port1))

	// Out of range port safety
	invalidPort := joypad.Port(99)
	assert.False(t, c.IsJoypadConnected(invalidPort))
	c.SetButton(invalidPort, joypad.ButtonA, true)
	c.SetButtons(invalidPort, 0xFF)
	c.ConnectJoypad(invalidPort)
	c.DisconnectJoypad(invalidPort)
}

func TestConsole_StepAndStepFrame(t *testing.T) {
	t.Parallel()

	cart := newTestCartridge()
	c := console.New(cart)

	cycles := c.Step()
	assert.Positive(t, cycles)

	fb := c.StepFrame()
	assert.Len(t, fb, expectedFramePixels)
	assert.Equal(t, uint64(1), c.PPU.Frame())

	c.Reset()
	assert.Equal(t, testProgramOrigin, c.CPU.State().PC)
}

func TestConsole_StepFrameOutput(t *testing.T) {
	t.Parallel()

	cart := newTestCartridge()
	c := console.New(cart)

	out := c.StepFrameOutput()
	assert.Len(t, out.Video, expectedFramePixels)
	assert.NotEmpty(t, out.Audio)
}

func TestConsole_MultiButtonCombinations(t *testing.T) {
	t.Parallel()

	cart := newTestCartridge()
	c := console.New(cart)

	// Simulate simultaneous Crouch-Slide combination: Button B + Down + Right
	slideCombo := byte(joypad.ButtonB | joypad.ButtonDown | joypad.ButtonRight)
	c.SetButtons(joypad.Port1, slideCombo)

	// Strobe joypad
	c.Bus.Write(registerJoypad1Addr, sampleStrobeHigh)
	c.Bus.Write(registerJoypad1Addr, sampleStrobeLow)

	// Serial reading sequence: A, B, Select, Start, Up, Down, Left, Right
	assert.Equal(t, buttonReleased, c.Bus.Read(registerJoypad1Addr)) // A
	assert.Equal(t, buttonPressed, c.Bus.Read(registerJoypad1Addr))  // B (Run)
	assert.Equal(t, buttonReleased, c.Bus.Read(registerJoypad1Addr)) // Select
	assert.Equal(t, buttonReleased, c.Bus.Read(registerJoypad1Addr)) // Start
	assert.Equal(t, buttonReleased, c.Bus.Read(registerJoypad1Addr)) // Up
	assert.Equal(t, buttonPressed, c.Bus.Read(registerJoypad1Addr))  // Down (Crouch)
	assert.Equal(t, buttonReleased, c.Bus.Read(registerJoypad1Addr)) // Left
	assert.Equal(t, buttonPressed, c.Bus.Read(registerJoypad1Addr))  // Right (Run Direction)
}

func TestConsole_RecordingToggle(t *testing.T) {
	t.Parallel()

	cart := newTestCartridge()
	c := console.New(cart)

	c.SetRecording(true)
	assert.True(t, c.APU.IsRecording())

	c.SetRecording(false)
	assert.False(t, c.APU.IsRecording())
}

func TestConsole_PauseToggle(t *testing.T) {
	t.Parallel()

	cart := newTestCartridge()
	c := console.New(cart)

	assert.False(t, c.IsPaused())

	// Toggle pause on
	isPaused := c.TogglePause()
	assert.True(t, isPaused)
	assert.True(t, c.IsPaused())

	// While paused, StepFrame should not advance PPU frames or CPU cycles
	initialFrame := c.PPU.Frame()
	initialCycles := c.CPU.Cycles
	c.StepFrame()
	assert.Equal(t, initialFrame, c.PPU.Frame())
	assert.Equal(t, initialCycles, c.CPU.Cycles)

	// Toggle pause off
	isPaused = c.TogglePause()
	assert.False(t, isPaused)
	assert.False(t, c.IsPaused())

	// Reset unpauses if paused
	c.TogglePause()
	assert.True(t, c.IsPaused())
	c.Reset()
	assert.False(t, c.IsPaused())
}

func TestConsole_Reset_CoordinatesSubsystems(t *testing.T) {
	t.Parallel()

	cart := newTestCartridge()
	c := console.New(cart)

	// Dirty PPU and APU state
	c.PPU.WriteRegister(0x2000, 0x80)
	c.PPU.WriteRegister(0x2001, 0x1E)
	c.APU.WriteRegister(0x4015, 0x01)
	c.APU.WriteRegister(0x4003, 0x08) // loads Pulse 1 length counter
	c.TogglePause()

	assert.True(t, c.IsPaused())
	assert.NotZero(t, c.PPU.Ctrl())
	assert.NotZero(t, c.PPU.Mask())
	assert.NotZero(t, c.APU.ReadRegister(0x4015))

	c.Reset()

	assert.False(t, c.IsPaused())
	assert.Zero(t, c.PPU.Ctrl())
	assert.Zero(t, c.PPU.Mask())
	assert.Zero(t, c.APU.ReadRegister(0x4015))
}

func TestConsole_DMCStallCyclesStep(t *testing.T) {
	t.Parallel()

	cart := newTestCartridge()
	c := console.New(cart)

	// Configure DMC sample at $C000
	c.APU.WriteRegister(0x4010, 0x0F) // Maximum rate
	c.APU.WriteRegister(0x4012, 0x00) // Sample address $C000
	c.APU.WriteRegister(0x4013, 0x00) // Sample length 1 byte

	// Enable DMC: triggers sample buffer load and 4 DMA stall cycles
	c.APU.WriteRegister(0x4015, 0x10)

	initialPPUCycle := c.PPU.State().Cycle
	stepCycles := c.Step()

	// Step must report at least 2 CPU cycles (for NOP) + 3 DMC stall cycles = 5 cycles
	assert.GreaterOrEqual(t, stepCycles, 5)
	// PPU must have advanced by stepCycles * 3
	assert.Equal(t, initialPPUCycle+stepCycles*3, c.PPU.State().Cycle)
}

func TestConsole_MaskedIRQ_DoesNotStepExtraCycles(t *testing.T) {
	t.Parallel()

	cart := newTestCartridge()
	c := console.New(cart)

	// Set CPU interrupt flag (I=1) to mask IRQ
	c.CPU.SetFlag(cpu.FlagInterrupt, true)

	// Trigger frame IRQ in APU by writing to $4017 with frame IRQ enabled (bit 6 = 0)
	c.APU.WriteRegister(registerFrameCounterAddr, 0x00)
	// Clock APU frame sequencer to trigger IRQ
	for range 30000 {
		c.APU.Step(1)
		if c.APU.IRQPending() {
			break
		}
	}
	require.True(t, c.APU.IRQPending(), "APU should have IRQ pending")

	// Since I=1, Step() must execute the 2-cycle NOP and NOT add 7 interrupt cycles
	initialPPUCycle := c.PPU.State().Cycle
	stepCycles := c.Step()

	assert.Equal(t, 2, stepCycles, "masked IRQ must not add interrupt cycles")
	assert.Equal(t, initialPPUCycle+6, c.PPU.State().Cycle, "PPU must only advance by 2*3=6 cycles")
}

