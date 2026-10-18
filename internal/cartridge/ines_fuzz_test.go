// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cartridge_test

import (
	"bytes"
	"testing"

	"github.com/dustblow/dustblow/internal/cartridge"
)

func FuzzNew(f *testing.F) {
	// Seed 1: Empty input.
	f.Add([]byte{})

	// Seed 2: Too short header.
	f.Add([]byte{'N', 'E', 'S'})

	// Seed 3: Valid NROM-128 minimum ROM.
	nrom128 := makeHeader(1, 1, 0, 0)
	nrom128 = append(nrom128, make([]byte, testPRGUnit16KB+testCHRUnit8KB)...)
	f.Add(nrom128)

	// Seed 4: Valid NROM-256 minimum ROM.
	nrom256 := makeHeader(2, 1, 1, 0)
	nrom256 = append(nrom256, make([]byte, testPRGUnit32KB+testCHRUnit8KB)...)
	f.Add(nrom256)

	// Seed 5: ROM with Trainer flag and data.
	withTrainer := makeHeader(1, 1, testFlagTrainerPresent, 0)
	withTrainer = append(withTrainer, make([]byte, 512+testPRGUnit16KB+testCHRUnit8KB)...)
	f.Add(withTrainer)

	// Seed 6: Unsupported mapper ID.
	unsupportedMapper := makeHeader(1, 1, testUnsupportedMapperNibble, 0)
	unsupportedMapper = append(unsupportedMapper, make([]byte, testPRGUnit16KB+testCHRUnit8KB)...)
	f.Add(unsupportedMapper)

	f.Fuzz(func(t *testing.T, data []byte) {
		// Invariant: New must never panic on arbitrary inputs.
		// It must return either a valid Cartridge or a non-nil error.
		cart, err := cartridge.New(bytes.NewReader(data))
		if err != nil {
			if cart != nil {
				t.Fatalf("expected nil cartridge when error is non-nil, got: %v", cart)
			}
			return
		}

		if cart == nil {
			t.Fatal("expected non-nil cartridge when error is nil")
		}

		// Basic invariant checks on successfully parsed cartridge.
		_ = cart.ReadPRG(testAddressPRGBase)
		_ = cart.ReadPRG(testAddressPRGMirror)
		_ = cart.ReadCHR(testAddressCHRBase)
	})
}
