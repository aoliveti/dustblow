// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package view implements video scaling, aspect ratio correction, and post-processing shaders.
package view

// Mode defines the active post-processing and scaling filter for the NES video stream.
type Mode int

const (
	// ModeSharpBilinear applies anti-aliased integer prescaling and sharp bilinear interpolation
	// in 4:3 aspect ratio, eliminating pixel shimmering and wobble during scrolling.
	ModeSharpBilinear Mode = 0

	// ModeCRT reproduces an authentic vintage CRT television display with horizontal scanlines,
	// subtle RGB aperture grille phosphor mask, and 4:3 aspect ratio.
	ModeCRT Mode = 1

	// ModePixelPerfect renders crisp square pixels (1:1 PAR) without aspect ratio stretching.
	ModePixelPerfect Mode = 2

	// ModeCount is the total number of selectable shader modes.
	ModeCount = 3
)

// String returns the user-friendly display name of the shader mode.
func (m Mode) String() string {
	switch m {
	case ModeSharpBilinear:
		return "Sharp Bilinear (4:3)"
	case ModeCRT:
		return "CRT Scanlines (4:3)"
	case ModePixelPerfect:
		return "Pixel Perfect (1:1)"
	default:
		return "Unknown"
	}
}

// Next returns the next sequential shader mode in the continuous cycle.
func (m Mode) Next() Mode {
	return (m + 1) % ModeCount
}
