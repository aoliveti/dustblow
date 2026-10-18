// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package apu

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPulse_DutyAndMuting(t *testing.T) {
	t.Parallel()

	p1 := NewPulse(false)
	p1.SetEnabled(true)

	// Duty 2 (50%): 0 1 1 1 1 0 0 0, constant volume 15
	p1.WriteControl(0x9F) // Duty 2, const vol, vol 15
	p1.WriteTimerLow(0x50)
	p1.WriteTimerHigh(0x08) // Length loaded, step=0

	assert.Equal(t, byte(0), p1.Output()) // step 0 is 0

	// Step timer through the 50% duty waveform
	p1.step = 1
	assert.Equal(t, byte(15), p1.Output()) // step 1 is 1

	// Test continuous muting for period < 8
	p1.rawPeriod = 7
	assert.Equal(t, byte(0), p1.Output()) // muted

	// Test sweep negate difference between Pulse 1 (1's complement) and Pulse 2 (2's complement)
	p1.rawPeriod = 100
	p1.sweep.shift = 1
	p1.sweep.negate = true
	// change = 50 -> for Pulse 1: 100 - 50 - 1 = 49
	assert.Equal(t, uint16(49), p1.sweep.TargetPeriod(100))

	p2 := NewPulse(true)
	p2.rawPeriod = 100
	p2.sweep.shift = 1
	p2.sweep.negate = true
	// change = 50 -> for Pulse 2: 100 - 50 = 50
	assert.Equal(t, uint16(50), p2.sweep.TargetPeriod(100))
}

func TestTriangle_LinearCounterAnd32Step(t *testing.T) {
	t.Parallel()

	tri := NewTriangle()
	tri.SetEnabled(true)

	// Write linear counter reload value 10
	tri.WriteControl(0x0A) // controlFlag=false, reload=10
	tri.WriteTimerLow(0x10)
	tri.WriteTimerHigh(0x08) // sets reloadFlag, length loaded

	// Quarter frame tick should reload linear counter to 10
	tri.ClockQuarterFrame()
	assert.Equal(t, byte(10), tri.linearCounter)
	assert.False(t, tri.reloadFlag)

	// Initial step should be 15
	assert.Equal(t, byte(15), tri.Output())

	// Step timer
	tri.StepTimer()
	// Sequence decrements: 15 -> 14
	tri.step = 1
	assert.Equal(t, byte(14), tri.Output())

	// Halting the channel leaves DAC output frozen at current value
	tri.linearCounter = 0
	lastVal := tri.Output()
	tri.StepTimer()
	assert.Equal(t, lastVal, tri.Output())
}

func TestNoise_LFSR(t *testing.T) {
	t.Parallel()

	noise := NewNoise()
	noise.SetEnabled(true)
	noise.WriteControl(0x1F)       // const vol 15
	noise.WriteLength(0x08)        // length loaded
	noise.WriteModeAndPeriod(0x00) // mode 0 (15-bit), period index 0

	initialLFSR := noise.shiftRegister
	assert.Equal(t, uint16(1), initialLFSR)

	// Advance timer to trigger LFSR shift
	noise.StepTimer()
	assert.NotEqual(t, initialLFSR, noise.shiftRegister)
}

func TestFrameCounter_4StepAnd5Step(t *testing.T) {
	t.Parallel()

	quarterCount := 0
	halfCount := 0

	fc := NewFrameCounter(
		func() { quarterCount++ },
		func() { halfCount++ },
	)

	// Mode 0 (4-step)
	fc.WriteControl(0x00, false)
	assert.False(t, fc.IRQPending())

	// Step through step 1 (cycle 7457 + reset delay)
	fc.Step(cycleStep1 + resetDelayPutCycle)
	assert.Equal(t, 1, quarterCount)
	assert.Equal(t, 0, halfCount)

	// Step through step 4 (cycle 29829)
	fc.Step(cycleStep4 - cycleStep1)
	assert.Equal(t, 4, quarterCount)
	assert.Equal(t, 2, halfCount)
	assert.True(t, fc.IRQPending())

	// Clear IRQ
	fc.ClearIRQ()
	assert.False(t, fc.IRQPending())

	// Mode 1 (5-step)
	quarterCount = 0
	halfCount = 0
	fc.WriteControl(0x80, false)
	assert.Equal(t, 1, quarterCount)
	assert.Equal(t, 1, halfCount)
	fc.Step(resetDelayPutCycle)

	// 5-step mode should never set IRQ
	fc.Step(cycleStep5)
	assert.False(t, fc.IRQPending())
}

func TestMixer_LookupTables(t *testing.T) {
	t.Parallel()

	// Silence returns 0
	assert.InDelta(t, float32(0.0), MixChannels(0, 0, 0, 0, 0), 1e-6)

	// Pure pulse output
	pOut := MixChannels(15, 15, 0, 0, 0)
	assert.Greater(t, pOut, float32(0.2))

	// Pure triangle output
	tOut := MixChannels(0, 0, 15, 0, 0)
	assert.Greater(t, tOut, float32(0.05))
}

func TestAPU_StatusRegister(t *testing.T) {
	t.Parallel()

	a := New(nil)

	// Enable Pulse 1 and Pulse 2
	a.WriteRegister(0x4015, 0x03)
	// Load length counter for Pulse 1
	a.WriteRegister(0x4003, 0x08)

	status := a.ReadRegister(0x4015)
	assert.NotZero(t, status&0x01) // Pulse 1 length > 0
	assert.Zero(t, status&0x02)    // Pulse 2 length == 0

	// Step APU with recording enabled
	a.SetRecording(true)
	a.Step(100)
	samples := a.TakeFrameSamples()
	assert.NotEmpty(t, samples)

	// Step APU with recording disabled
	a.SetRecording(false)
	a.Step(100)
	assert.Nil(t, a.TakeFrameSamples())
}

func TestRingBuffer_ReadWrite(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer()
	// Prebuffering initially returns silence
	buf := make([]byte, 4)
	n, err := rb.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Zero(t, buf[0])

	// Fill buffer past prebuffer threshold
	for range (prebufferThreshold / 4) + 10 {
		rb.WriteSample(0.5)
	}

	// Now Read delivers actual samples
	n, err = rb.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 4, n)

	// Verify stereo output has matching L and R channels
	assert.Equal(t, buf[0], buf[2])
	assert.Equal(t, buf[1], buf[3])
	assert.NotZero(t, buf[0])
}

func TestRingBuffer_ClearAndOverflow(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer()
	// Fill buffer past capacity to force sample dropping (overflow)
	for range (rb.size / 4) + 100 {
		rb.WriteSample(0.8)
	}

	assert.Equal(t, rb.size, rb.count)

	// Even after massive overflow, Left channel (bytes 0,1) must strictly match Right channel (bytes 2,3)
	buf := make([]byte, 4)
	n, err := rb.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, buf[0], buf[2])
	assert.Equal(t, buf[1], buf[3])

	// Clear should reset buffer back to clean prebuffering state
	rb.Clear()
	assert.Equal(t, 0, rb.count)
	assert.Equal(t, 0, rb.head)
	assert.Equal(t, 0, rb.tail)
	assert.True(t, rb.prebuffering)
}

func TestDMC_DirectLoadAndDMA(t *testing.T) {
	t.Parallel()

	memory := make(map[uint16]byte)
	memory[0xC000] = 0xAA // 10101010 bit pattern
	memory[0xC001] = 0x55

	dmc := NewDMC(func(addr uint16) byte {
		return memory[addr]
	})

	// Test direct 7-bit DAC load ($4011)
	dmc.WriteDirectLoad(64)
	assert.Equal(t, byte(64), dmc.Output())

	// Configure sample address $C000 ($4012) and length 2 bytes ($4013)
	dmc.WriteSampleAddress(0) // 0xC000 + (0 << 6) = 0xC000
	dmc.WriteSampleLength(0)  // (0 << 4) + 1 = 1 byte
	assert.Equal(t, uint16(0xC000), dmc.sampleAddress)
	assert.Equal(t, uint16(1), dmc.sampleLength)

	// Configure control ($4010): IRQ enabled, no loop, rate 15 (fastest, 54 cycles)
	dmc.WriteControl(0x8F)
	assert.True(t, dmc.irqEnabled)
	assert.False(t, dmc.loopFlag)
	assert.Equal(t, byte(15), dmc.rateIndex)

	// Enable DMC from $4015
	dmc.SetEnabled(true)
	assert.True(t, dmc.enabled)
	assert.True(t, dmc.HasBuffer())

	// Step timer through the sample bytes:
	// First 8 ticks empty the initial shift register and load the sample byte into buffer,
	// subsequent ticks modulate output level according to sample bits.
	modulated := false
	for range 600 {
		dmc.StepTimer()
		if dmc.Output() != 64 {
			modulated = true
		}
	}

	// Output level should have modulated
	assert.True(t, modulated)

	// Step further to complete the sample and trigger IRQ
	for range 600 {
		dmc.StepTimer()
	}
	assert.True(t, dmc.irqPending)

	// Test disabling DMC clears bytes and pending IRQ
	dmc.SetEnabled(false)
	assert.Equal(t, uint16(0), dmc.bytesRemaining)
	assert.False(t, dmc.irqPending)
}

func TestDMC_LoopPlayback(t *testing.T) {
	t.Parallel()

	memory := map[uint16]byte{0xC000: 0xFF}
	dmc := NewDMC(func(addr uint16) byte { return memory[addr] })

	dmc.WriteSampleAddress(0)
	dmc.WriteSampleLength(0) // 1 byte
	dmc.WriteControl(0x40)   // loop enabled, IRQ disabled
	dmc.SetEnabled(true)

	// Run enough cycles to complete the 1 byte sample
	for range 600 {
		dmc.StepTimer()
	}

	// Because loop is enabled, bytesRemaining was reloaded instead of staying 0
	assert.Positive(t, dmc.bytesRemaining)
	assert.False(t, dmc.irqPending)
}

func TestPulse_SweepAndEnvelope(t *testing.T) {
	t.Parallel()

	p := NewPulse(false)
	p.SetEnabled(true)

	// Test envelope decaying in non-loop mode
	p.WriteControl(0x00) // Duty 0, const volume false, volume/period 0
	p.WriteTimerHigh(0x08)
	assert.True(t, p.envelope.startFlag)

	p.ClockQuarterFrame()
	assert.Equal(t, byte(15), p.envelope.decayLevel)
	assert.False(t, p.envelope.startFlag)

	// Decay through to 0
	for range 15 {
		p.ClockQuarterFrame()
	}
	assert.Equal(t, byte(0), p.envelope.decayLevel)

	// Test sweep unit clock and reload
	p.WriteSweep(0x88) // enabled, period 0, negate, shift 0
	assert.True(t, p.sweep.enabled)
	assert.True(t, p.sweep.reloadFlag)

	p.rawPeriod = 500
	p.ClockHalfFrame()
	assert.False(t, p.sweep.reloadFlag)
}

func TestTriangle_UltrasonicMuting(t *testing.T) {
	t.Parallel()

	tri := NewTriangle()
	tri.SetEnabled(true)
	tri.WriteControl(0x7F) // reload 127
	tri.WriteTimerHigh(0x08)
	tri.ClockQuarterFrame()

	// Ultrasonic period (rawPeriod < 2) must inhibit sequencer stepping
	tri.rawPeriod = 1
	initialStep := tri.step
	tri.StepTimer()
	assert.Equal(t, initialStep, tri.step)

	// ClockHalfFrame decrements length counter when controlFlag is false
	tri.controlFlag = false
	tri.lengthCounter = 10
	tri.ClockHalfFrame()
	assert.Equal(t, byte(9), tri.lengthCounter)
}

func TestNoise_ShortMode(t *testing.T) {
	t.Parallel()

	noise := NewNoise()
	noise.SetEnabled(true)
	noise.WriteControl(0x15)
	noise.WriteLength(0x08)
	// Mode 1 (93-bit short metallic mode)
	noise.WriteModeAndPeriod(0x80)
	assert.True(t, noise.modeFlag)

	noise.StepTimer()
	assert.Positive(t, noise.shiftRegister)

	// ClockHalfFrame decrements length counter
	noise.lengthHalt = false
	noise.lengthCounter = 5
	noise.ClockHalfFrame()
	assert.Equal(t, byte(4), noise.lengthCounter)
}

func TestFilterChain_Filters(t *testing.T) {
	t.Parallel()

	sampleRate := 44100.0
	fc := NewFilterChain(sampleRate)

	// Step zero input yields zero
	assert.InDelta(t, float32(0), fc.Step(0), 1e-6)

	// Step step function input through filter chain
	filtered := fc.Step(1.0)
	assert.NotZero(t, filtered)
}

func TestAPU_FullRegisterMappingAndIRQ(t *testing.T) {
	t.Parallel()

	a := New(func(_ uint16) byte { return 0 })

	// Write across all register blocks
	a.WriteRegister(registerPulse1Control, 0x9F)
	a.WriteRegister(registerPulse1Sweep, 0x88)
	a.WriteRegister(registerPulse1TimerLow, 0x20)
	a.WriteRegister(registerPulse1TimerHigh, 0x08)

	a.WriteRegister(registerPulse2Control, 0x9F)
	a.WriteRegister(registerPulse2Sweep, 0x88)
	a.WriteRegister(registerPulse2TimerLow, 0x20)
	a.WriteRegister(registerPulse2TimerHigh, 0x08)

	a.WriteRegister(registerTriangleControl, 0x85)
	a.WriteRegister(registerTriangleTimerLow, 0x10)
	a.WriteRegister(registerTriangleTimerHigh, 0x08)

	a.WriteRegister(registerNoiseControl, 0x1F)
	a.WriteRegister(registerNoisePeriod, 0x05)
	a.WriteRegister(registerNoiseLength, 0x08)

	a.WriteRegister(registerDMCControl, 0x8F)
	a.WriteRegister(registerDMCDirectLoad, 0x40)
	a.WriteRegister(registerDMCSampleAddress, 0x02)
	a.WriteRegister(registerDMCSampleLength, 0x04)

	a.WriteRegister(registerStatus, 0x1F) // Enable all 5 channels
	a.WriteRegister(registerFrameCounter, 0x00) // 4-step mode

	// AudioStream returns non-nil reader
	assert.NotNil(t, a.AudioStream())

	// Flush audio clears buffer
	a.FlushAudio()

	// Recording toggle
	a.SetRecording(true)
	assert.True(t, a.IsRecording())
	a.SetRecording(false)
	assert.False(t, a.IsRecording())

	// IRQ pending verification
	assert.False(t, a.IRQPending())
	a.FrameCounter.irqLine = true
	assert.True(t, a.IRQPending())
	a.FrameCounter.irqLine = false

	a.DMC.irqPending = true
	assert.True(t, a.IRQPending())

	// Read status schedules clearing frame IRQ on next get cycle
	a.FrameCounter.irqFlag = true
	status := a.ReadRegister(registerStatus)
	assert.NotZero(t, status&statusFrameIRQ)
	a.Step(2)
	assert.False(t, a.FrameCounter.IRQPending())

	// Unknown register read returns 0
	assert.Zero(t, a.ReadRegister(0x4000))
}

func TestAPU_StepAdvancesFrameClocks(t *testing.T) {
	t.Parallel()

	a := New(nil)
	a.WriteRegister(registerPulse1Control, 0x00) // Envelope decay mode (const volume false, period 0)
	a.WriteRegister(registerPulse1TimerHigh, 0x08)
	a.WriteRegister(registerTriangleControl, 0x0A)
	a.WriteRegister(registerTriangleTimerHigh, 0x08)
	a.WriteRegister(registerNoiseControl, 0x15)
	a.WriteRegister(registerNoiseLength, 0x08)

	// Step enough CPU cycles to cross quarter and half frame marks
	a.Step(cycleStep2 + 100)

	// The envelope should have decayed from 15
	assert.Less(t, a.Pulse1.envelope.decayLevel, byte(15))
}

func TestTriangle_UltrasonicPopSuppression(t *testing.T) {
	t.Parallel()

	tri := NewTriangle()
	tri.SetEnabled(true)
	tri.linearCounter = 5
	tri.lengthCounter = 5
	tri.step = 0 // Table output at step 0 is 15

	tests := []struct {
		name           string
		period         uint16
		enabled        bool
		linearCounter  byte
		lengthCounter  byte
		expectedOutput byte
	}{
		{
			name:           "normal audible period produces waveform value",
			period:         100,
			enabled:        true,
			linearCounter:  5,
			lengthCounter:  5,
			expectedOutput: 15,
		},
		{
			name:           "ultrasonic period 1 outputs neutral DC bias",
			period:         1,
			enabled:        true,
			linearCounter:  5,
			lengthCounter:  5,
			expectedOutput: triangleNeutralDCOffset,
		},
		{
			name:           "ultrasonic period 0 outputs neutral DC bias",
			period:         0,
			enabled:        true,
			linearCounter:  5,
			lengthCounter:  5,
			expectedOutput: triangleNeutralDCOffset,
		},
		{
			name:           "disabled channel outputs zero",
			period:         1,
			enabled:        false,
			linearCounter:  5,
			lengthCounter:  5,
			expectedOutput: 0,
		},
		{
			name:           "zero linear counter outputs zero",
			period:         1,
			enabled:        true,
			linearCounter:  0,
			lengthCounter:  5,
			expectedOutput: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			localTri := NewTriangle()
			localTri.SetEnabled(tt.enabled)
			localTri.rawPeriod = tt.period
			localTri.linearCounter = tt.linearCounter
			localTri.lengthCounter = tt.lengthCounter
			assert.Equal(t, tt.expectedOutput, localTri.Output())
		})
	}
}

func TestDMC_DMAStallCycles(t *testing.T) {
	t.Parallel()

	sampleData := []byte{0x55, 0xAA}
	readMem := func(addr uint16) byte {
		offset := int(addr - 0xC000)
		if offset >= 0 && offset < len(sampleData) {
			return sampleData[offset]
		}
		return 0
	}

	apuInstance := New(readMem)
	require.NotNil(t, apuInstance)

	// Configure DMC sample at $C000, length 1 byte
	apuInstance.WriteRegister(registerDMCControl, 0x0F)       // fastest rate
	apuInstance.WriteRegister(registerDMCSampleAddress, 0x00) // $C000
	apuInstance.WriteRegister(registerDMCSampleLength, 0x00)  // 1 byte (0*16 + 1 = 1)

	// Initially zero stall cycles
	assert.Zero(t, apuInstance.TakeStallCycles())

	// Enable DMC via $4015
	apuInstance.WriteRegister(registerStatus, enableDMCMask)

	// fetchSample should have added 3 DMA stall cycles for fetch on GET cycle
	stalls := apuInstance.TakeStallCycles()
	assert.Equal(t, dmcDMAStallCyclesGet, stalls)

	// Subsequent query returns 0 until next DMA
	assert.Zero(t, apuInstance.TakeStallCycles())

	// Test DMA fetch landing on a PUT cycle (4 stall cycles)
	apuInstance.Reset()
	apuInstance.Step(1) // align parity so that next cycle is PUT cycle
	apuInstance.WriteRegister(registerDMCControl, 0x0F)
	apuInstance.WriteRegister(registerDMCSampleAddress, 0x00)
	apuInstance.WriteRegister(registerDMCSampleLength, 0x00)
	apuInstance.TakeStallCycles()
	apuInstance.WriteRegister(registerStatus, enableDMCMask)
	assert.Equal(t, dmcDMAStallCyclesPut, apuInstance.TakeStallCycles())
}

func TestAPU_Reset_SilencesAllChannels(t *testing.T) {
	t.Parallel()

	apu := New(func(uint16) byte { return 0 })
	require.NotNil(t, apu)

	// Enable Pulse 1, Pulse 2, Triangle, Noise via $4015
	apu.WriteRegister(registerStatus, enablePulse1Mask|enablePulse2Mask|enableTriangleMask|enableNoiseMask)

	// Configure and start Triangle playing continuous note
	apu.WriteRegister(registerTriangleControl, triangleControlFlagMask|0x7F) // Halt/loop flag set, reload = 127
	apu.WriteRegister(registerTriangleTimerLow, 0x50)
	apu.WriteRegister(registerTriangleTimerHigh, 0x01) // period = 0x150, length counter loaded

	// Configure Pulse 1
	apu.WriteRegister(registerPulse1Control, pulseLengthHaltMask|pulseConstVolumeMask|0x0F) // max volume, loop
	apu.WriteRegister(registerPulse1TimerLow, 0x80)
	apu.WriteRegister(registerPulse1TimerHigh, 0x01)

	// Step APU past quarter/half-frame ticks so channels are active and outputting audio
	apu.Step(15000)
	assert.NotZero(t, apu.Triangle.Output())
	assert.NotZero(t, apu.ReadRegister(registerStatus))

	// Trigger Reset
	apu.Reset()

	// Verify all channels are immediately silenced and disabled
	assert.Zero(t, apu.ReadRegister(registerStatus))
	assert.Zero(t, apu.Triangle.Output())
	assert.Zero(t, apu.Pulse1.Output())
	assert.Zero(t, apu.Pulse2.Output())
	assert.Zero(t, apu.Noise.Output())
	assert.Zero(t, apu.DMC.Output())
	assert.False(t, apu.IRQPending())
}
