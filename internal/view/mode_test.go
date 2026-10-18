// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package view

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMode_Next(t *testing.T) {
	t.Parallel()

	tests := []struct {
		current  Mode
		expected Mode
	}{
		{ModeSharpBilinear, ModeCRT},
		{ModeCRT, ModePixelPerfect},
		{ModePixelPerfect, ModeSharpBilinear},
	}

	for _, tt := range tests {
		t.Run(tt.current.String(), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.current.Next())
		})
	}
}

func TestMode_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mode     Mode
		expected string
	}{
		{ModeSharpBilinear, "Sharp Bilinear (4:3)"},
		{ModeCRT, "CRT Scanlines (4:3)"},
		{ModePixelPerfect, "Pixel Perfect (1:1)"},
		{Mode(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.mode.String())
		})
	}
}
