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
	n163ChannelCount     = 8
	n163RAMSize          = 128
	n163CyclesPerChannel = 15
	n163SampleBias       = 8
	n163MaxVolume        = 15
	// n163MixingScale balances the 8-channel N163 output against the 2A03 APU channels.
	n163MixingScale float32 = 0.003
)

// Namco163Audio implements the 8-channel wavetable sound expansion of the Namco 163 ASIC.
type Namco163Audio struct {
	ram           [n163RAMSize]byte
	cycleDivider  int
	currChannel   int
	currentOutput float32
	soundDisabled bool
}

// NewNamco163Audio constructs a new Namco 163 audio synthesizer.
func NewNamco163Audio() *Namco163Audio {
	return &Namco163Audio{
		currChannel: 7,
	}
}

// SetSoundDisabled configures whether sound synthesis output is muted (controlled by $E000 bit 6).
func (a *Namco163Audio) SetSoundDisabled(disabled bool) {
	a.soundDisabled = disabled
}

// ReadRAM reads a byte from the internal 128-byte sound RAM.
func (a *Namco163Audio) ReadRAM(addr byte) byte {
	return a.ram[addr&0x7F]
}

// WriteRAM writes a byte to the internal 128-byte sound RAM.
func (a *Namco163Audio) WriteRAM(addr byte, val byte) {
	a.ram[addr&0x7F] = val
}

// StepAudio advances the sound generator by the given number of CPU cycles.
// The Namco 163 uses time-division multiplexing, updating one channel every 15 CPU cycles.
func (a *Namco163Audio) StepAudio(cycles int) {
	if a.soundDisabled {
		a.currentOutput = 0
		return
	}

	a.cycleDivider += cycles
	for a.cycleDivider >= n163CyclesPerChannel {
		a.cycleDivider -= n163CyclesPerChannel
		a.stepOneChannel()
	}
}

func (a *Namco163Audio) stepOneChannel() {
	// Register $7F bits 4..6 determine number of active channels minus 1 (0..7 -> 1..8 channels).
	activeChannels := int((a.ram[0x7F]>>4)&0x07) + 1
	minChannel := n163ChannelCount - activeChannels

	// Move to the next active channel in round-robin sequence (7 down to minChannel, then wrap).
	a.currChannel--
	if a.currChannel < minChannel || a.currChannel > 7 {
		a.currChannel = 7
	}

	base := a.currChannel * 8

	// Channel register layout in RAM:
	// base+0: Low frequency (bits 0..7)
	// base+1: Low phase (bits 0..7)
	// base+2: Mid frequency (bits 8..15)
	// base+3: Mid phase (bits 8..15)
	// base+4: High frequency (bits 16..17) | Wave Length (bits 2..7)
	// base+5: High phase (bits 16..23)
	// base+6: Wave Address
	// base+7: Volume (bits 0..3)
	freq := uint32(a.ram[base]) | (uint32(a.ram[base+2]) << 8) | ((uint32(a.ram[base+4]) & 0x03) << 16)
	phase := uint32(a.ram[base+1]) | (uint32(a.ram[base+3]) << 8) | (uint32(a.ram[base+5]) << 16)

	// Update phase accumulator (24-bit)
	phase = (phase + freq) & 0x00FFFFFF

	// Write updated phase back to internal RAM as real hardware does
	a.ram[base+1] = byte(phase & 0xFF)
	a.ram[base+3] = byte((phase >> 8) & 0xFF)
	a.ram[base+5] = byte((phase >> 16) & 0xFF)

	// Waveform length: (64 - (b >> 2)) * 4 samples (4, 8, ..., 256)
	waveLenSamples := (64 - uint32(a.ram[base+4]>>2)) * 4
	if waveLenSamples == 0 {
		waveLenSamples = 256
	}

	waveAddress := uint32(a.ram[base+6])
	sampleOffset := (phase >> 16) % waveLenSamples
	sampleIndex := (waveAddress*2 + sampleOffset) & 0xFF

	// Each RAM byte contains two 4-bit samples: even sample is low nibble, odd is high nibble
	ramByte := a.ram[(sampleIndex/2)&0x7F]
	sample := (ramByte >> 4) & 0x0F
	if (sampleIndex & 1) == 0 {
		sample = ramByte & 0x0F
	}

	vol := a.ram[base+7] & 0x0F
	signedSample := int(sample) - n163SampleBias
	chanOut := float32(signedSample * int(vol))

	a.currentOutput = chanOut * n163MixingScale
}

// AudioOutput returns the currently latched TDM DAC sample from the Namco 163.
func (a *Namco163Audio) AudioOutput() float32 {
	if a.soundDisabled {
		return 0
	}
	return a.currentOutput
}

// SaveState serializes the Namco 163 audio synthesizer state.
func (a *Namco163Audio) SaveState(w io.Writer) error {
	state := struct {
		RAM           [n163RAMSize]byte
		CycleDivider  int32
		CurrChannel   int32
		CurrentOutput float32
		SoundDisabled bool
	}{
		RAM:           a.ram,
		CycleDivider:  int32(a.cycleDivider & 0x7FFFFFFF),
		CurrChannel:   int32(a.currChannel & 0x7FFFFFFF),
		CurrentOutput: a.currentOutput,
		SoundDisabled: a.soundDisabled,
	}

	if err := binary.Write(w, binary.LittleEndian, state); err != nil {
		return fmt.Errorf("write namco163 audio state: %w", err)
	}
	return nil
}

// LoadState restores the Namco 163 audio synthesizer state.
func (a *Namco163Audio) LoadState(r io.Reader) error {
	var state struct {
		RAM           [n163RAMSize]byte
		CycleDivider  int32
		CurrChannel   int32
		CurrentOutput float32
		SoundDisabled bool
	}

	if err := binary.Read(r, binary.LittleEndian, &state); err != nil {
		return fmt.Errorf("read namco163 audio state: %w", err)
	}

	a.ram = state.RAM
	a.cycleDivider = int(state.CycleDivider)
	a.currChannel = int(state.CurrChannel)
	a.currentOutput = state.CurrentOutput
	a.soundDisabled = state.SoundDisabled
	return nil
}

// Reset silences wavetable audio synthesis on console reset.
func (a *Namco163Audio) Reset() {
	a.cycleDivider = 0
	a.currChannel = 7
	a.currentOutput = 0
	a.soundDisabled = true
}
