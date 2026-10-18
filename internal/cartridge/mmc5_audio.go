// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	// Hardware register memory addresses ($5000-$5015)
	regMMC5Pulse1Control    uint16 = 0x5000
	regMMC5Pulse1PeriodLow  uint16 = 0x5002
	regMMC5Pulse1PeriodHigh uint16 = 0x5003
	regMMC5Pulse2Control    uint16 = 0x5004
	regMMC5Pulse2PeriodLow  uint16 = 0x5006
	regMMC5Pulse2PeriodHigh uint16 = 0x5007
	regMMC5PCMModeIRQ       uint16 = 0x5010
	regMMC5PCMRawData       uint16 = 0x5011
	regMMC5AudioStatus      uint16 = 0x5015

	// mmc5FrameDividerPeriod is CPU cycles per 240 Hz frame sequencer tick (1789773 / 240 ≈ 7457).
	mmc5FrameDividerPeriod = 7457

	// Audio mixing constants
	mmc5PulseTableSize = 31
	mmc5PCMScale       = float32(0.2267 / 127.0) // Scaled so DAC 127 matches 2A03 DMC max amplitude
)

var mmc5DutyTable = [4][8]byte{
	{0, 1, 0, 0, 0, 0, 0, 0}, // 12.5%
	{0, 1, 1, 0, 0, 0, 0, 0}, // 25.0%
	{0, 1, 1, 1, 1, 0, 0, 0}, // 50.0%
	{1, 0, 0, 1, 1, 1, 1, 1}, // 75.0%
}

var mmc5LengthTable = [32]byte{
	10, 254, 20, 2, 40, 4, 80, 6, 160, 8, 60, 10, 14, 12, 26, 14,
	12, 16, 24, 18, 48, 20, 96, 22, 192, 24, 72, 26, 16, 28, 32, 30,
}

var mmc5PulseTable [mmc5PulseTableSize]float32

func init() {
	// Pre-calculate MMC5 pulse non-linear lookup table matching APU pulse weights
	for i := 1; i < mmc5PulseTableSize; i++ {
		mmc5PulseTable[i] = 95.88 / ((8128.0 / float32(i)) + 100.0)
	}
}

type mmc5Envelope struct {
	startFlag   bool
	loopFlag    bool
	constVolume bool
	volume      byte
	divider     byte
	decayLevel  byte
}

func (e *mmc5Envelope) Clock() {
	if e.startFlag {
		e.startFlag = false
		e.decayLevel = 15
		e.divider = e.volume
		return
	}

	if e.divider > 0 {
		e.divider--
		return
	}

	e.divider = e.volume
	if e.decayLevel > 0 {
		e.decayLevel--
		return
	}
	if e.loopFlag {
		e.decayLevel = 15
	}
}

func (e *mmc5Envelope) Output() byte {
	if e.constVolume {
		return e.volume
	}
	return e.decayLevel
}

type mmc5Pulse struct {
	envelope      mmc5Envelope
	rawPeriod     uint16
	timerDivider  uint16
	duty          byte
	step          byte
	lengthCounter byte
	enabled       bool
	lengthHalt    bool
}

func (p *mmc5Pulse) WriteControl(val byte) {
	p.duty = (val >> 6) & 0x03
	p.lengthHalt = (val & 0x20) != 0
	p.envelope.loopFlag = p.lengthHalt
	p.envelope.constVolume = (val & 0x10) != 0
	p.envelope.volume = val & 0x0F
}

func (p *mmc5Pulse) WritePeriodLow(val byte) {
	p.rawPeriod = (p.rawPeriod & 0x0700) | uint16(val)
}

func (p *mmc5Pulse) WritePeriodHigh(val byte) {
	p.rawPeriod = (p.rawPeriod & 0x00FF) | (uint16(val&0x07) << 8)
	if p.enabled {
		p.lengthCounter = mmc5LengthTable[(val>>3)&0x1F]
	}
	p.step = 0
	p.envelope.startFlag = true
}

func (p *mmc5Pulse) StepTimer() {
	if p.timerDivider == 0 {
		p.timerDivider = p.rawPeriod
		p.step = (p.step + 1) & 0x07
		return
	}
	p.timerDivider--
}

func (p *mmc5Pulse) Clock240Hz() {
	p.envelope.Clock()
	if !p.lengthHalt && p.lengthCounter > 0 {
		p.lengthCounter--
	}
}

func (p *mmc5Pulse) SetEnabled(enabled bool) {
	p.enabled = enabled
	if !enabled {
		p.lengthCounter = 0
	}
}

func (p *mmc5Pulse) Output() byte {
	if !p.enabled || p.lengthCounter == 0 {
		return 0
	}
	if mmc5DutyTable[p.duty][p.step] == 0 {
		return 0
	}
	return p.envelope.Output()
}

func (p *mmc5Pulse) HasLength() bool {
	return p.lengthCounter > 0
}

type mmc5PCM struct {
	dac        byte
	readMode   bool
	irqEnable  bool
	irqTrip    bool
}

func (pcm *mmc5PCM) WriteControl(val byte) {
	pcm.readMode = (val & 0x01) != 0
	pcm.irqEnable = (val & 0x80) != 0
}

func (pcm *mmc5PCM) ReadControl() byte {
	var val byte = 0x01 // MMC5A revision default bit 0
	if pcm.irqTrip && pcm.irqEnable {
		val |= 0x80
	}
	pcm.irqTrip = false // Reading acknowledges and clears IRQ
	return val
}

func (pcm *mmc5PCM) WriteData(val byte) {
	if pcm.readMode {
		return
	}
	if val == 0 {
		pcm.irqTrip = true
		return
	}
	pcm.dac = val
	pcm.irqTrip = false
}

func (pcm *mmc5PCM) ReadTrigger(val byte) {
	if !pcm.readMode {
		return
	}
	if val == 0 {
		pcm.irqTrip = true
		return
	}
	pcm.dac = val
	pcm.irqTrip = false
}

func (pcm *mmc5PCM) IRQPending() bool {
	return pcm.irqTrip && pcm.irqEnable
}

func (pcm *mmc5PCM) Output() float32 {
	return float32(pcm.dac) * mmc5PCMScale
}

// MMC5Audio coordinates the extra Pulse 1, Pulse 2, and 8-bit PCM audio channels on MMC5.
type MMC5Audio struct {
	pulse1       mmc5Pulse
	pulse2       mmc5Pulse
	pcm          mmc5PCM
	frameDivider int
	apuCycleOdd  bool
}

// NewMMC5Audio creates an initialized MMC5 audio synthesizer.
func NewMMC5Audio() *MMC5Audio {
	return &MMC5Audio{
		frameDivider: mmc5FrameDividerPeriod,
	}
}

// StepAudio advances the MMC5 audio timers and 240 Hz frame sequencer by cpuCycles.
func (a *MMC5Audio) StepAudio(cycles int) {
	for range cycles {
		a.frameDivider--
		if a.frameDivider <= 0 {
			a.frameDivider += mmc5FrameDividerPeriod
			a.pulse1.Clock240Hz()
			a.pulse2.Clock240Hz()
		}

		a.apuCycleOdd = !a.apuCycleOdd
		if a.apuCycleOdd {
			a.pulse1.StepTimer()
			a.pulse2.StepTimer()
		}
	}
}

// AudioOutput computes the instantaneous expansion audio sample amplitude.
func (a *MMC5Audio) AudioOutput() float32 {
	pSum := a.pulse1.Output() + a.pulse2.Output()
	return mmc5PulseTable[pSum] + a.pcm.Output()
}

// ReadRegister intercepts CPU reads to audio registers ($5010 and $5015).
func (a *MMC5Audio) ReadRegister(addr uint16) (byte, bool) {
	switch addr {
	case regMMC5PCMModeIRQ:
		return a.pcm.ReadControl(), true

	case regMMC5AudioStatus:
		var st byte
		if a.pulse1.HasLength() {
			st |= 0x01
		}
		if a.pulse2.HasLength() {
			st |= 0x02
		}
		return st, true

	default:
		return 0, false
	}
}

// WriteRegister intercepts CPU writes to MMC5 audio registers ($5000-$5007, $5010, $5011, $5015).
func (a *MMC5Audio) WriteRegister(addr uint16, val byte) bool {
	switch addr {
	case regMMC5Pulse1Control:
		a.pulse1.WriteControl(val)
		return true

	case regMMC5Pulse1PeriodLow:
		a.pulse1.WritePeriodLow(val)
		return true

	case regMMC5Pulse1PeriodHigh:
		a.pulse1.WritePeriodHigh(val)
		return true

	case regMMC5Pulse2Control:
		a.pulse2.WriteControl(val)
		return true

	case regMMC5Pulse2PeriodLow:
		a.pulse2.WritePeriodLow(val)
		return true

	case regMMC5Pulse2PeriodHigh:
		a.pulse2.WritePeriodHigh(val)
		return true

	case regMMC5PCMModeIRQ:
		a.pcm.WriteControl(val)
		return true

	case regMMC5PCMRawData:
		a.pcm.WriteData(val)
		return true

	case regMMC5AudioStatus:
		a.pulse1.SetEnabled((val & 0x01) != 0)
		a.pulse2.SetEnabled((val & 0x02) != 0)
		return true

	default:
		return false
	}
}

// ReadPRGTrigger notifies the PCM channel of a CPU read in range $8000-$BFFF for read-mode streaming.
func (a *MMC5Audio) ReadPRGTrigger(addr uint16, val byte) {
	if addr >= 0x8000 && addr <= 0xBFFF {
		a.pcm.ReadTrigger(val)
	}
}

// IRQPending reports whether the PCM channel is asserting an interrupt line.
func (a *MMC5Audio) IRQPending() bool {
	return a.pcm.IRQPending()
}

// SaveState serializes the MMC5 audio synthesizer state.
func (a *MMC5Audio) SaveState(w io.Writer) error {
	state := struct {
		FrameDivider int32
		APUCycleOdd  bool
		// Pulse1
		P1StartFlag    bool
		P1LoopFlag     bool
		P1ConstVolume  bool
		P1Volume       byte
		P1Divider      byte
		P1DecayLevel   byte
		P1RawPeriod    uint16
		P1TimerDivider uint16
		P1Duty         byte
		P1Step         byte
		P1Length       byte
		P1Enabled      bool
		P1LengthHalt   bool
		// Pulse2
		P2StartFlag    bool
		P2LoopFlag     bool
		P2ConstVolume  bool
		P2Volume       byte
		P2Divider      byte
		P2DecayLevel   byte
		P2RawPeriod    uint16
		P2TimerDivider uint16
		P2Duty         byte
		P2Step         byte
		P2Length       byte
		P2Enabled      bool
		P2LengthHalt   bool
		// PCM
		PCMDAC       byte
		PCMReadMode  bool
		PCMIRQEnable bool
		PCMIRQTrip   bool
	}{
		FrameDivider:   int32(a.frameDivider & 0x7FFFFFFF),
		APUCycleOdd:    a.apuCycleOdd,
		P1StartFlag:    a.pulse1.envelope.startFlag,
		P1LoopFlag:     a.pulse1.envelope.loopFlag,
		P1ConstVolume:  a.pulse1.envelope.constVolume,
		P1Volume:       a.pulse1.envelope.volume,
		P1Divider:      a.pulse1.envelope.divider,
		P1DecayLevel:   a.pulse1.envelope.decayLevel,
		P1RawPeriod:    a.pulse1.rawPeriod,
		P1TimerDivider: a.pulse1.timerDivider,
		P1Duty:         a.pulse1.duty,
		P1Step:         a.pulse1.step,
		P1Length:       a.pulse1.lengthCounter,
		P1Enabled:      a.pulse1.enabled,
		P1LengthHalt:   a.pulse1.lengthHalt,
		P2StartFlag:    a.pulse2.envelope.startFlag,
		P2LoopFlag:     a.pulse2.envelope.loopFlag,
		P2ConstVolume:  a.pulse2.envelope.constVolume,
		P2Volume:       a.pulse2.envelope.volume,
		P2Divider:      a.pulse2.envelope.divider,
		P2DecayLevel:   a.pulse2.envelope.decayLevel,
		P2RawPeriod:    a.pulse2.rawPeriod,
		P2TimerDivider: a.pulse2.timerDivider,
		P2Duty:         a.pulse2.duty,
		P2Step:         a.pulse2.step,
		P2Length:       a.pulse2.lengthCounter,
		P2Enabled:      a.pulse2.enabled,
		P2LengthHalt:   a.pulse2.lengthHalt,
		PCMDAC:         a.pcm.dac,
		PCMReadMode:    a.pcm.readMode,
		PCMIRQEnable:   a.pcm.irqEnable,
		PCMIRQTrip:     a.pcm.irqTrip,
	}

	if err := binary.Write(w, binary.LittleEndian, state); err != nil {
		return fmt.Errorf("write mmc5 audio state: %w", err)
	}
	return nil
}

// LoadState restores the MMC5 audio synthesizer state.
func (a *MMC5Audio) LoadState(r io.Reader) error {
	var state struct {
		FrameDivider int32
		APUCycleOdd  bool
		// Pulse1
		P1StartFlag    bool
		P1LoopFlag     bool
		P1ConstVolume  bool
		P1Volume       byte
		P1Divider      byte
		P1DecayLevel   byte
		P1RawPeriod    uint16
		P1TimerDivider uint16
		P1Duty         byte
		P1Step         byte
		P1Length       byte
		P1Enabled      bool
		P1LengthHalt   bool
		// Pulse2
		P2StartFlag    bool
		P2LoopFlag     bool
		P2ConstVolume  bool
		P2Volume       byte
		P2Divider      byte
		P2DecayLevel   byte
		P2RawPeriod    uint16
		P2TimerDivider uint16
		P2Duty         byte
		P2Step         byte
		P2Length       byte
		P2Enabled      bool
		P2LengthHalt   bool
		// PCM
		PCMDAC       byte
		PCMReadMode  bool
		PCMIRQEnable bool
		PCMIRQTrip   bool
	}

	if err := binary.Read(r, binary.LittleEndian, &state); err != nil {
		return fmt.Errorf("read mmc5 audio state: %w", err)
	}

	a.frameDivider = int(state.FrameDivider)
	a.apuCycleOdd = state.APUCycleOdd

	a.pulse1.envelope.startFlag = state.P1StartFlag
	a.pulse1.envelope.loopFlag = state.P1LoopFlag
	a.pulse1.envelope.constVolume = state.P1ConstVolume
	a.pulse1.envelope.volume = state.P1Volume
	a.pulse1.envelope.divider = state.P1Divider
	a.pulse1.envelope.decayLevel = state.P1DecayLevel
	a.pulse1.rawPeriod = state.P1RawPeriod
	a.pulse1.timerDivider = state.P1TimerDivider
	a.pulse1.duty = state.P1Duty
	a.pulse1.step = state.P1Step
	a.pulse1.lengthCounter = state.P1Length
	a.pulse1.enabled = state.P1Enabled
	a.pulse1.lengthHalt = state.P1LengthHalt

	a.pulse2.envelope.startFlag = state.P2StartFlag
	a.pulse2.envelope.loopFlag = state.P2LoopFlag
	a.pulse2.envelope.constVolume = state.P2ConstVolume
	a.pulse2.envelope.volume = state.P2Volume
	a.pulse2.envelope.divider = state.P2Divider
	a.pulse2.envelope.decayLevel = state.P2DecayLevel
	a.pulse2.rawPeriod = state.P2RawPeriod
	a.pulse2.timerDivider = state.P2TimerDivider
	a.pulse2.duty = state.P2Duty
	a.pulse2.step = state.P2Step
	a.pulse2.lengthCounter = state.P2Length
	a.pulse2.enabled = state.P2Enabled
	a.pulse2.lengthHalt = state.P2LengthHalt

	a.pcm.dac = state.PCMDAC
	a.pcm.readMode = state.PCMReadMode
	a.pcm.irqEnable = state.PCMIRQEnable
	a.pcm.irqTrip = state.PCMIRQTrip

	return nil
}

// Reset silences MMC5 expansion pulse channels and resets PCM on console reset.
func (a *MMC5Audio) Reset() {
	a.pulse1 = mmc5Pulse{}
	a.pulse2 = mmc5Pulse{}
	a.pcm = mmc5PCM{}
}
