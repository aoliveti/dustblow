// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"image"

	"github.com/dustblow/dustblow/internal/view"
)

const (
	bytesPerPixelRGBA = 4
	alphaChannelIndex = 3
	opaqueAlphaByte   = 0xFF
)

// ImagePixelReader abstracts reading pixel data from an image source such as *ebiten.Image.
type ImagePixelReader interface {
	ReadPixels(pixels []byte)
	SubImage(r image.Rectangle) image.Image
	Bounds() image.Rectangle
}

// ExtractViewportImage crops the active game display viewport from screen and copies its pixels into an image.RGBA.
// Returns nil if screen is nil or viewport dimensions are invalid.
func ExtractViewportImage(screen ImagePixelReader, vp view.Viewport) *image.RGBA {
	if screen == nil || vp.Width <= 0 || vp.Height <= 0 {
		return nil
	}

	bounds := screen.Bounds()
	x0 := max(vp.X, bounds.Min.X)
	y0 := max(vp.Y, bounds.Min.Y)
	x1 := min(vp.X+vp.Width, bounds.Max.X)
	y1 := min(vp.Y+vp.Height, bounds.Max.Y)

	w := x1 - x0
	h := y1 - y0
	if w <= 0 || h <= 0 {
		return nil
	}

	gameRect := image.Rect(x0, y0, x1, y1)
	sub, ok := screen.SubImage(gameRect).(ImagePixelReader)
	if !ok {
		return nil
	}

	pixelBuf := make([]byte, bytesPerPixelRGBA*w*h)
	sub.ReadPixels(pixelBuf)

	// NES video output has no transparency. Clamping alpha to 0xFF guarantees that
	// image.RGBA.Opaque() reports true, allowing Go's png.Encoder to emit pure 24-bit
	// RGB (Color Type 2, no alpha channel). This avoids macOS Finder blank white canvas
	// artifacts and allows immediate QuickLook thumbnail rendering.
	for i := alphaChannelIndex; i < len(pixelBuf); i += bytesPerPixelRGBA {
		pixelBuf[i] = opaqueAlphaByte
	}

	return &image.RGBA{
		Pix:    pixelBuf,
		Stride: bytesPerPixelRGBA * w,
		Rect:   image.Rect(0, 0, w, h),
	}
}
