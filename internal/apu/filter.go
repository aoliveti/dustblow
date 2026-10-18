// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package apu

import "math"

// FirstOrderFilter implements a single-pole IIR filter (high-pass or low-pass).
type FirstOrderFilter struct {
	b0    float32
	b1    float32
	a1    float32
	prevX float32
	prevY float32
}

// NewHighPassFilter creates a first-order high-pass RC filter at cutoff frequency fc and sample rate fs.
func NewHighPassFilter(sampleRate, cutoff float64) FirstOrderFilter {
	c := float32(2.0 * math.Pi * cutoff / sampleRate)
	alpha := 1.0 / (1.0 + c)
	return FirstOrderFilter{
		b0: alpha,
		b1: -alpha,
		a1: alpha,
	}
}

// NewLowPassFilter creates a first-order low-pass RC filter at cutoff frequency fc and sample rate fs.
func NewLowPassFilter(sampleRate, cutoff float64) FirstOrderFilter {
	c := float32(2.0 * math.Pi * cutoff / sampleRate)
	alpha := c / (1.0 + c)
	return FirstOrderFilter{
		b0: alpha,
		b1: 0,
		a1: 1.0 - alpha,
	}
}

// Step applies the filter to input sample x and returns the filtered output.
func (f *FirstOrderFilter) Step(x float32) float32 {
	y := f.b0*x + f.b1*f.prevX + f.a1*f.prevY
	f.prevX = x
	f.prevY = y
	return y
}

// Reset clears any lingering filter state and historical samples.
func (f *FirstOrderFilter) Reset() {
	f.prevX = 0
	f.prevY = 0
}

const (
	cutoffHighPass1 float64 = 90.0
	cutoffHighPass2 float64 = 440.0
	cutoffLowPass   float64 = 14000.0
)

// FilterChain models the NES motherboard analog audio circuitry:
// Two high-pass filters (90 Hz, 440 Hz) and one low-pass filter (14 kHz).
type FilterChain struct {
	hp90  FirstOrderFilter
	hp440 FirstOrderFilter
	lp14k FirstOrderFilter
}

// NewFilterChain initializes the analog RC filter chain at the target sample rate.
func NewFilterChain(sampleRate float64) *FilterChain {
	return &FilterChain{
		hp90:  NewHighPassFilter(sampleRate, cutoffHighPass1),
		hp440: NewHighPassFilter(sampleRate, cutoffHighPass2),
		lp14k: NewLowPassFilter(sampleRate, cutoffLowPass),
	}
}

// Step processes a raw mixed sample through the analog circuit.
func (fc *FilterChain) Step(sample float32) float32 {
	s := fc.hp90.Step(sample)
	s = fc.hp440.Step(s)
	s = fc.lp14k.Step(s)
	return s
}

// Reset clears the filter memory across all stages in the analog chain.
func (fc *FilterChain) Reset() {
	fc.hp90.Reset()
	fc.hp440.Reset()
	fc.lp14k.Reset()
}
