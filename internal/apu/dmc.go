// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package apu

// dmcRateTable stores the NTSC CPU cycle periods for the 16 DMC playback rates.

var dmcRateTable = [16]uint16{
	428, 380, 340, 320, 286, 254, 226, 214, 190, 160, 142, 128, 106, 84, 72, 54,
}

const (
	dmcSampleAddressBase uint16 = 0xC000
	dmcAddressWrap       uint16 = 0x8000
	dmcOutputDelta       byte   = 2
	dmcMaxOutputLevel    byte   = 125
	dmcMinOutputLevel    byte   = 2

	dmcIRQEnableMask  byte = 0x80
	dmcLoopMask       byte = 0x40
	dmcRateMask       byte = 0x0F
	dmcDirectLoadMask byte = 0x7F
	dmcBitDepth       byte = 8

	dmcDMAStallCyclesGet = 3
	dmcDMAStallCyclesPut = 4
)

// DMC plays 1-bit delta PCM samples and direct 7-bit DAC levels.
type DMC struct {
	enabled        bool
	irqEnabled     bool
	irqPending     bool
	loopFlag       bool
	rateIndex      byte
	timerDivider   uint16
	outputLevel    byte // 7-bit DAC level (0 to 127)
	sampleAddress  uint16
	sampleLength   uint16
	currentAddress uint16
	bytesRemaining uint16
	sampleBuffer   byte
	hasBuffer      bool
	shiftRegister  byte
	bitsRemaining  byte
	isSilenced     bool
	stallCycles    int
	readMemory     func(addr uint16) byte
	isGetCycle     func() bool
}

// SetGetCycleHook sets the callback to determine if current CPU cycle is an APU get cycle.
func (d *DMC) SetGetCycleHook(hook func() bool) {
	d.isGetCycle = hook
}

// NewDMC creates an initialized DMC.
func NewDMC(readMemory func(addr uint16) byte) *DMC {
	return &DMC{
		readMemory:    readMemory,
		bitsRemaining: dmcBitDepth,
		isSilenced:    true,
	}
}

// WriteControl handles writes to $4010.
func (d *DMC) WriteControl(val byte) {
	d.irqEnabled = (val & dmcIRQEnableMask) != 0
	d.loopFlag = (val & dmcLoopMask) != 0
	d.rateIndex = val & dmcRateMask

	if !d.irqEnabled {
		d.irqPending = false
	}
}

// WriteDirectLoad handles writes to $4011 (direct 7-bit DAC write).
func (d *DMC) WriteDirectLoad(val byte) {
	d.outputLevel = val & dmcDirectLoadMask
}

// WriteSampleAddress handles writes to $4012.
func (d *DMC) WriteSampleAddress(val byte) {
	d.sampleAddress = dmcSampleAddressBase + (uint16(val) << 6)
}

// WriteSampleLength handles writes to $4013.
func (d *DMC) WriteSampleLength(val byte) {
	d.sampleLength = (uint16(val) << 4) + 1
}

// restartSample reloads the sample address and length pointers.
func (d *DMC) restartSample() {
	d.currentAddress = d.sampleAddress
	d.bytesRemaining = d.sampleLength
}

func (d *DMC) calcDMAStallCycles() int {
	if d.isGetCycle != nil && d.isGetCycle() {
		return dmcDMAStallCyclesGet
	}
	return dmcDMAStallCyclesPut
}

// fetchSample attempts to fill the sample buffer from memory if empty.
func (d *DMC) fetchSample() {
	if d.bytesRemaining == 0 || d.hasBuffer {
		return
	}
	if d.readMemory != nil {
		stall := d.calcDMAStallCycles()
		d.sampleBuffer = d.readMemory(d.currentAddress)
		d.stallCycles += stall
	}

	d.hasBuffer = true
	d.currentAddress++
	if d.currentAddress == 0 {
		d.currentAddress = dmcAddressWrap
	}
	d.bytesRemaining--
	if d.bytesRemaining > 0 {
		return
	}
	if d.loopFlag {
		d.restartSample()
		return
	}
	if d.irqEnabled {
		d.irqPending = true
	}
}

func (d *DMC) updateOutputLevel() {
	bit := d.shiftRegister & 0x01
	d.shiftRegister >>= 1

	if bit != 0 {
		if d.outputLevel <= dmcMaxOutputLevel {
			d.outputLevel += dmcOutputDelta
		}
		return
	}

	if d.outputLevel >= dmcMinOutputLevel {
		d.outputLevel -= dmcOutputDelta
	}
}

func (d *DMC) reloadShiftRegister() {
	d.bitsRemaining = dmcBitDepth
	if !d.hasBuffer {
		d.isSilenced = true
		return
	}
	d.isSilenced = false
	d.shiftRegister = d.sampleBuffer
	d.hasBuffer = false
	if d.enabled {
		d.fetchSample()
	}
}

// StepTimer is clocked on EVERY CPU cycle.
func (d *DMC) StepTimer() {
	if d.enabled {
		d.fetchSample()
	}

	if d.timerDivider > 0 {
		d.timerDivider--
		return
	}

	d.timerDivider = dmcRateTable[d.rateIndex] - 1
	if !d.isSilenced {
		d.updateOutputLevel()
	}

	d.bitsRemaining--
	if d.bitsRemaining == 0 {
		d.reloadShiftRegister()
	}
}

// SetEnabled enables or disables DMC sample playback from $4015.
func (d *DMC) SetEnabled(enabled bool) {
	d.irqPending = false
	if !enabled {
		d.enabled = false
		d.bytesRemaining = 0
		return
	}
	if d.bytesRemaining > 0 && d.enabled {
		return
	}
	d.enabled = true
	if d.bytesRemaining == 0 {
		d.restartSample()
	}
	d.fetchSample()
}

// Output returns the current 7-bit DAC amplitude (0 to 127).
func (d *DMC) Output() byte {
	return d.outputLevel
}

// IRQPending reports whether the DMC channel is currently requesting an interrupt.
func (d *DMC) IRQPending() bool {
	return d.irqPending
}

// HasBuffer returns whether the sample buffer is full.
func (d *DMC) HasBuffer() bool {
	return d.hasBuffer
}

// TakeStallCycles returns and resets accumulated CPU stall cycles caused by DMC DMA fetches.
func (d *DMC) TakeStallCycles() int {
	stalls := d.stallCycles
	d.stallCycles = 0
	return stalls
}

// Reset silences the DMC channel, clears pending IRQs, and empties sample buffers.
func (d *DMC) Reset() {
	d.enabled = false
	d.irqEnabled = false
	d.irqPending = false
	d.loopFlag = false
	d.rateIndex = 0
	d.timerDivider = 0
	d.outputLevel = 0
	d.sampleAddress = 0
	d.sampleLength = 0
	d.currentAddress = 0
	d.bytesRemaining = 0
	d.sampleBuffer = 0
	d.hasBuffer = false
	d.shiftRegister = 0
	d.bitsRemaining = dmcBitDepth
	d.isSilenced = true
	d.stallCycles = 0
}
