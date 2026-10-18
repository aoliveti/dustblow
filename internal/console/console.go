// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package console coordinates the NES system subsystems (CPU, PPU, Bus, Cartridge, and Joypads),
// managing master clock synchronization, interrupt propagation, and user input routing.
package console

import (
	"github.com/dustblow/dustblow/internal/apu"


	"github.com/dustblow/dustblow/internal/bus"
	"github.com/dustblow/dustblow/internal/cartridge"
	"github.com/dustblow/dustblow/internal/cpu"
	"github.com/dustblow/dustblow/internal/joypad"
	"github.com/dustblow/dustblow/internal/ppu"
)

const (
	// PPUCyclesPerCPUCycle is the master clock divider ratio between PPU and CPU on NTSC systems (3:1).
	PPUCyclesPerCPUCycle = 3

	opcodeBRK byte = 0x00
	opcodeJSR byte = 0x20
	opcodePLP byte = 0x28
	opcodeCLI byte = 0x58
	opcodeSEI byte = 0x78

	opcodeBranchMask             byte = 0x1F
	opcodeBranchPattern          byte = 0x10
	branchCyclesTakenNoPageCross      = 3

	ppuWriteDotOffset = 1
	ppuReadDotOffset  = 2
)

// Console represents a fully integrated NES hardware system.
type Console struct {
	CPU              *cpu.CPU
	PPU              *ppu.PPU
	APU              *apu.APU
	Bus              *bus.Bus
	Cartridge        *cartridge.Cartridge
	joypads          [joypad.PortCount]*joypad.Joypad
	steppedCycles    int
	steppedPPUCycles int
	dmaStallCycles   int
	nmiDelay         bool
	nmiPending       bool
	irqPending       bool
	irqDelay         bool
	paused           bool
}

// New creates and initializes a complete NES Console loaded with the given cartridge.
func New(cart *cartridge.Cartridge) *Console {
	b := bus.New(cart)
	p := b.PPU()
	a := apu.New(b.ReadDMC)
	if cart != nil && cart.ExpansionAudio() != nil {
		a.AttachExpansionAudio(cart.ExpansionAudio())
	}
	b.AttachAPU(a)
	c := &Console{
		CPU:       cpu.New(b),
		PPU:       p,
		APU:       a,
		Bus:       b,
		Cartridge: cart,
		joypads:   [joypad.PortCount]*joypad.Joypad{joypad.New(), joypad.New()},
	}

	b.SetIOAccessHook(c.handleIOAccess)

	b.AttachController(joypad.Port1, c.joypads[joypad.Port1])
	b.AttachController(joypad.Port2, c.joypads[joypad.Port2])
	c.CPU.Reset()

	return c
}

func (c *Console) handleIOAccess(_ uint16, isWrite bool) {
	instCycles := c.CPU.CurrentInstCycles()
	if instCycles <= 1 {
		return
	}
	targetCPU := instCycles - 1
	if targetCPU > c.steppedCycles {
		c.stepComponentsToCycle(targetCPU)
	}
	targetPPU := targetCPU*PPUCyclesPerCPUCycle + ppuReadDotOffset
	if isWrite {
		targetPPU = targetCPU*PPUCyclesPerCPUCycle + ppuWriteDotOffset
	}
	if targetPPU > c.steppedPPUCycles {
		ppuToStep := targetPPU - c.steppedPPUCycles
		c.PPU.Step(ppuToStep)
		c.steppedPPUCycles = targetPPU
	}
}

func (c *Console) stepComponentsToCycle(targetCPU int) {
	cyclesToStep := targetCPU - c.steppedCycles
	if c.APU != nil {
		c.APU.Step(cyclesToStep)
		c.dmaStallCycles += c.APU.TakeStallCycles()
	}
	if c.Cartridge != nil {
		c.Cartridge.StepCPU(cyclesToStep)
	}
	c.steppedCycles = targetCPU
}


// SetButtons updates the active button bitmask for the controller connected to the specified port.
// If no controller is connected to the port, this is a safe no-op.
func (c *Console) SetButtons(port joypad.Port, mask byte) {
	if c == nil {
		return
	}
	if int(port) < len(c.joypads) && c.joypads[port] != nil {
		c.joypads[port].SetButtons(mask)
	}
}

// SetButton updates the pressed state of a single button on the controller connected to the specified port.
// If no controller is connected to the port, this is a safe no-op.
func (c *Console) SetButton(port joypad.Port, btn joypad.Button, pressed bool) {
	if c == nil {
		return
	}
	if int(port) < len(c.joypads) && c.joypads[port] != nil {
		c.joypads[port].SetButton(btn, pressed)
	}
}

// ConnectJoypad connects a new standard joypad to the specified port.
func (c *Console) ConnectJoypad(port joypad.Port) {
	if int(port) < len(c.joypads) {
		pad := joypad.New()
		c.joypads[port] = pad
		c.Bus.AttachController(port, pad)
	}
}

// DisconnectJoypad disconnects the controller from the specified port, leaving it open-bus.
func (c *Console) DisconnectJoypad(port joypad.Port) {
	if int(port) < len(c.joypads) {
		c.joypads[port] = nil
		c.Bus.DetachController(port)
	}
}

// IsJoypadConnected reports whether a controller is currently plugged into the specified port.
func (c *Console) IsJoypadConnected(port joypad.Port) bool {
	if int(port) < len(c.joypads) {
		return c.joypads[port] != nil
	}
	return false
}

// FrameOutput bundles a rendered 256x240 video frame with the synchronized audio samples generated in that frame.
type FrameOutput struct {
	Video []byte
	Audio []float32
}

const (
	brkInterruptHijackLimit = 4
	irqInterruptHijackLimit = 4
)

// Step executes a single CPU instruction, advances the PPU and APU by the corresponding
// clock cycles, and handles NMI and IRQ interrupt delivery.
// Returns the number of CPU cycles consumed.
func (c *Console) Step() int {
	hadDelayedNMI := c.nmiDelay
	c.nmiDelay = false

	wasI := (c.CPU.Status & cpu.FlagInterrupt) != 0
	c.irqDelay = false

	c.steppedCycles = 0
	c.steppedPPUCycles = 0

	opcode := c.Bus.Peek(c.CPU.PC)
	wasIRQPending := c.isIRQPending()
	cpuCycles := c.CPU.Step()
	stallCycles, isOAMDMA := c.consumeStallCycles(cpuCycles)

	remainingCycles := cpuCycles - c.steppedCycles + stallCycles
	if c.Cartridge != nil {
		c.Cartridge.StepCPU(remainingCycles)
	}

	nmiCycle := c.advancePPU(cpuCycles, stallCycles)

	c.stepAPUStalls(stallCycles, isOAMDMA)

	c.steppedCycles = 0
	c.steppedPPUCycles = 0

	totalCycles := cpuCycles + stallCycles

	if opcode == opcodeBRK {
		if hadDelayedNMI || (c.nmiPending && nmiCycle > 0 && nmiCycle <= brkInterruptHijackLimit) {
			c.redirectVectorToNMI()
		}
		return totalCycles
	}

	isI := (c.CPU.Status & cpu.FlagInterrupt) != 0
	c.updateIRQDelay(opcode, cpuCycles, wasIRQPending, wasI, isI)
	latchedIRQ := wasIRQPending && !wasI
	forcedIRQ := opcode == opcodeSEI && !wasI && isI && wasIRQPending
	canTriggerIRQ := forcedIRQ || (!c.irqDelay && (latchedIRQ || c.isIRQPending()) && !c.CPU.GetFlag(cpu.FlagInterrupt))

	nmiActive := hadDelayedNMI || c.nmiPending || c.nmiDelay
	if canTriggerIRQ {
		if nmiActive {
			c.nmiPending = false
			c.nmiDelay = false
		}
		return c.handleIRQ(totalCycles, forcedIRQ, latchedIRQ, nmiActive)
	}

	if hadDelayedNMI || c.nmiPending {
		c.nmiPending = false
		c.nmiDelay = false
		c.CPU.TriggerNMI()
		c.stepComponents(cpu.InterruptCycles)
		return totalCycles + cpu.InterruptCycles
	}

	return totalCycles
}

func (c *Console) stepAPUStalls(stallCycles int, isOAMDMA bool) {
	if stallCycles <= 0 || c.APU == nil {
		return
	}
	c.APU.Step(stallCycles)
	if isOAMDMA {
		c.APU.SyncDMA()
	}
}

func (c *Console) updateIRQDelay(opcode byte, cpuCycles int, wasIRQPending, wasI, isI bool) {
	if (opcode == opcodeCLI || opcode == opcodePLP) && !isI && wasI {
		c.irqDelay = true
	}
	isBranch := (opcode & opcodeBranchMask) == opcodeBranchPattern
	if isBranch && cpuCycles == branchCyclesTakenNoPageCross && !wasIRQPending && c.isIRQPending() {
		c.irqDelay = true
	}
	// The 6502 samples the IRQ line during phi2 of the cycle before the last cycle of an instruction.
	// If an IRQ is asserted on the final cycle of JSR (or during end-of-instruction stalls),
	// the interrupt was not polled in time and must be delayed until after the target instruction.
	if opcode == opcodeJSR && !wasIRQPending && c.isIRQPending() {
		c.irqDelay = true
	}
}

func (c *Console) consumeStallCycles(cpuCycles int) (int, bool) {
	stallCycles := c.dmaStallCycles
	c.dmaStallCycles = 0
	if c.APU != nil {
		rem := cpuCycles - c.steppedCycles
		if rem > 0 {
			c.APU.Step(rem)
			stallCycles += c.APU.TakeStallCycles()
		}
	}
	isOAMDMA := false
	if c.Bus != nil {
		oamStalls := c.Bus.TakeDMAStallCycles()
		if oamStalls > 0 {
			isOAMDMA = true
			stallCycles += oamStalls
		}
	}
	return stallCycles, isOAMDMA
}


func (c *Console) advancePPU(cpuCycles, stallCycles int) int {
	nmiCycle := 0
	startCPUCycle := c.steppedCycles
	for cycleIdx := startCPUCycle + 1; cycleIdx <= cpuCycles+stallCycles; cycleIdx++ {
		targetPPU := cycleIdx * PPUCyclesPerCPUCycle
		if targetPPU <= c.steppedPPUCycles {
			continue
		}
		c.PPU.Step(targetPPU - c.steppedPPUCycles)
		c.steppedPPUCycles = targetPPU
		if !c.PPU.PollNMI() {
			continue
		}
		if cycleIdx == cpuCycles && stallCycles == 0 {
			c.nmiDelay = true
			continue
		}
		c.nmiPending = true
		if nmiCycle == 0 {
			nmiCycle = cycleIdx
		}
	}
	return nmiCycle
}

func (c *Console) handleIRQ(totalCycles int, forced, latched, hadDelayedNMI bool) int {
	if !forced && c.irqDelay {
		return totalCycles
	}
	if !forced && !latched && !c.isIRQPending() {
		return totalCycles
	}
	if !forced && !c.CPU.TriggerIRQ() {
		return totalCycles
	}


	if forced {
		c.CPU.TriggerIRQForced()
	}

	irqHijacked := hadDelayedNMI
	for irqCycle := 1; irqCycle <= cpu.InterruptCycles; irqCycle++ {
		c.PPU.Step(PPUCyclesPerCPUCycle)
		if c.PPU.PollNMI() {
			if irqCycle <= irqInterruptHijackLimit {
				irqHijacked = true
				continue
			}
			c.nmiPending = true
		}
		if c.APU != nil {
			c.APU.Step(1)
		}
		if c.Cartridge != nil {
			c.Cartridge.StepCPU(1)
		}
	}

	if irqHijacked {
		c.redirectVectorToNMI()
	}

	return totalCycles + cpu.InterruptCycles
}

func (c *Console) isIRQPending() bool {
	return (c.APU != nil && c.APU.IRQPending()) || (c.Cartridge != nil && c.Cartridge.IRQPending())
}

// IsIRQPending returns whether an IRQ is pending.
func (c *Console) IsIRQPending() bool {
	return c.isIRQPending()
}

func (c *Console) redirectVectorToNMI() {
	low := uint16(c.Bus.Read(cpu.VectorNMI))
	high := uint16(c.Bus.Read(cpu.VectorNMI + 1))
	c.CPU.PC = (high << 8) | low
	c.nmiPending = false
	c.nmiDelay = false
}

// NMIPending returns whether an NMI is currently pending.
func (c *Console) NMIPending() bool {
	return c.nmiPending
}

// NMIDelay returns whether an NMI was delayed.
func (c *Console) NMIDelay() bool {
	return c.nmiDelay
}


func (c *Console) stepComponents(cycles int) {
	c.PPU.Step(cycles * PPUCyclesPerCPUCycle)
	if c.PPU.PollNMI() {
		c.nmiPending = true
	}
	if c.APU != nil {
		c.APU.Step(cycles)
	}
	if c.Cartridge != nil {
		c.Cartridge.StepCPU(cycles)
	}
}

// TogglePause inverts the paused state and returns the new state (true if paused, false if running).
func (c *Console) TogglePause() bool {
	c.paused = !c.paused
	return c.paused
}

// IsPaused reports whether system emulation is currently suspended.
func (c *Console) IsPaused() bool {
	return c.paused
}

// StepFrame runs CPU, PPU, and APU execution until the current video frame completes.
// If the console is paused, this is a safe no-op returning the existing frame buffer.
// Returns a direct reference to the rendered 256x240 RGB frame buffer.
func (c *Console) StepFrame() []byte {
	if c.paused {
		return c.PPU.FrameBuffer()
	}
	startFrame := c.PPU.Frame()
	for c.PPU.Frame() == startFrame {
		c.Step()
	}
	return c.PPU.FrameBuffer()
}

// StepFrameOutput advances emulation by exactly one video frame and returns the synchronized video and audio output.
// Automatically enables frame audio capture if not previously enabled.
// Ideal for recording, live streaming, or multi-threaded media sinks.
func (c *Console) StepFrameOutput() FrameOutput {
	if c.APU != nil && !c.APU.IsRecording() {
		c.APU.SetRecording(true)
	}
	fb := c.StepFrame()
	var audioSamples []float32
	if c.APU != nil {
		audioSamples = c.APU.TakeFrameSamples()
	}
	return FrameOutput{
		Video: fb,
		Audio: audioSamples,
	}
}

// SetRecording enables or disables frame audio sample capture for recording or streaming.
func (c *Console) SetRecording(enabled bool) {
	if c.APU != nil {
		c.APU.SetRecording(enabled)
	}
}

// FlushAudio flushes buffered audio samples from the APU, if present.
func (c *Console) FlushAudio() {
	if c != nil && c.APU != nil {
		c.APU.FlushAudio()
	}
}

// Reset re-initializes CPU, PPU, APU, and Cartridge to clean reset state and unpauses the system.
func (c *Console) Reset() {
	c.paused = false
	c.irqPending = false
	c.nmiPending = false
	c.nmiDelay = false
	c.irqDelay = false
	c.steppedCycles = 0
	c.steppedPPUCycles = 0
	c.dmaStallCycles = 0
	c.CPU.Reset()
	if c.PPU != nil {
		c.PPU.Reset()
	}
	if c.APU != nil {
		c.APU.Reset()
	}
	if c.Cartridge != nil {
		c.Cartridge.Reset()
	}
}
