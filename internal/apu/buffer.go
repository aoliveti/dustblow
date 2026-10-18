// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package apu

import (
	"sync"
)

const (
	bytesPerSampleFrame = 4
	defaultBufferSize   = 16384 // ~92.8ms capacity (4096 stereo samples)
	prebufferThreshold  = 3528  // ~20.0ms cushion (882 stereo samples)
	masterGain          = float32(2.5)
	maxPCM16Amplitude   = float32(32767.0)
)

// RingBuffer is a thread-safe, non-allocating circular buffer converting float32 samples to stereo 16-bit PCM.
type RingBuffer struct {
	mu           sync.Mutex
	data         []byte
	head         int
	tail         int
	count        int
	size         int
	prebuffering bool
}

// NewRingBuffer creates a new ring buffer with default capacity.
func NewRingBuffer() *RingBuffer {
	return &RingBuffer{
		data:         make([]byte, defaultBufferSize),
		size:         defaultBufferSize,
		prebuffering: true,
	}
}

// WriteSample encodes a mono float32 sample into a 16-bit signed stereo frame (4 bytes) and stores it.
func (rb *RingBuffer) WriteSample(sample float32) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	amplified := sample * masterGain
	if amplified > 1.0 {
		amplified = 1.0
	}
	if amplified < -1.0 {
		amplified = -1.0
	}

	val := int16(amplified * maxPCM16Amplitude)
	b0 := byte(val & 0xFF)
	b1 := byte((val >> 8) & 0xFF)

	// Stereo frame: Left (2 bytes) + Right (2 bytes)
	frame := [bytesPerSampleFrame]byte{b0, b1, b0, b1}

	if rb.count+bytesPerSampleFrame > rb.size {
		// Buffer full: drop oldest complete stereo frame (4 bytes) to preserve 16-bit channel alignment
		rb.head = (rb.head + bytesPerSampleFrame) % rb.size
		rb.count -= bytesPerSampleFrame
	}

	for _, b := range frame {
		rb.data[rb.tail] = b
		rb.tail = (rb.tail + 1) % rb.size
		rb.count++
	}
}

// Read implements io.Reader to supply stereo 16-bit PCM to Ebitengine's audio player.
func (rb *RingBuffer) Read(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	// Accumulate cushion before initial playback to absorb OS scheduler jitter
	if rb.prebuffering {
		if rb.count < prebufferThreshold {
			for i := range p {
				p[i] = 0
			}
			return len(p), nil
		}
		rb.prebuffering = false
	}

	available := (rb.count / bytesPerSampleFrame) * bytesPerSampleFrame
	if available == 0 {
		rb.prebuffering = true
		for i := range p {
			p[i] = 0
		}
		return len(p), nil
	}

	limit := min((len(p)/bytesPerSampleFrame)*bytesPerSampleFrame, available)

	n := 0
	for n < limit {
		p[n] = rb.data[rb.head]
		rb.head = (rb.head + 1) % rb.size
		rb.count--
		n++
	}

	return n, nil
}

// Clear flushes all buffered audio data and resets the prebuffering gate.
func (rb *RingBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.head = 0
	rb.tail = 0
	rb.count = 0
	rb.prebuffering = true
}
