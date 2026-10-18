// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package view

import "math"

// Viewport defines the destination rectangle for the game display within the host window.
type Viewport struct {
	X      int
	Y      int
	Width  int
	Height int
}

// CalculateViewport computes the largest centered rectangle fitting within (winWidth, winHeight)
// that preserves the target aspect ratio (aspectWidth : aspectHeight).
func CalculateViewport(winWidth, winHeight, aspectWidth, aspectHeight int) Viewport {
	if winWidth <= 0 || winHeight <= 0 || aspectWidth <= 0 || aspectHeight <= 0 {
		return Viewport{}
	}

	targetAspect := float64(aspectWidth) / float64(aspectHeight)
	windowAspect := float64(winWidth) / float64(winHeight)

	if windowAspect > targetAspect {
		// Window is wider than target aspect: pillarbox (black bars on left/right)
		height := winHeight
		width := int(math.Round(float64(winHeight) * targetAspect))
		x := (winWidth - width) / 2
		return Viewport{
			X:      x,
			Y:      0,
			Width:  width,
			Height: height,
		}
	}

	// Window is taller than target aspect: letterbox (black bars on top/bottom)
	width := winWidth
	height := int(math.Round(float64(winWidth) / targetAspect))
	y := (winHeight - height) / 2
	return Viewport{
		X:      0,
		Y:      y,
		Width:  width,
		Height: height,
	}
}
