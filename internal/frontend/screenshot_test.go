// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend_test

import (
	"image"
	"image/color"
	"testing"

	"github.com/dustblow/dustblow/internal/frontend"
	"github.com/dustblow/dustblow/internal/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPixelImage struct {
	rgba *image.RGBA
}

func newMockPixelImage(w, h int, col color.RGBA) *mockPixelImage {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, col)
		}
	}
	return &mockPixelImage{rgba: img}
}

func (m *mockPixelImage) ReadPixels(pixels []byte) {
	copy(pixels, m.rgba.Pix)
}

func (m *mockPixelImage) SubImage(r image.Rectangle) image.Image {
	sub := m.rgba.SubImage(r).(*image.RGBA)
	// Create a contiguous slice matching sub-image bounds
	w := r.Dx()
	h := r.Dy()
	contiguous := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			contiguous.Set(x, y, sub.At(r.Min.X+x, r.Min.Y+y))
		}
	}
	return &mockPixelImage{rgba: contiguous}
}

func (m *mockPixelImage) Bounds() image.Rectangle {
	return m.rgba.Bounds()
}

func (m *mockPixelImage) ColorModel() color.Model {
	return m.rgba.ColorModel()
}

func (m *mockPixelImage) At(x, y int) color.Color {
	return m.rgba.At(x, y)
}

func TestExtractViewportImage_NilAndInvalidGuards(t *testing.T) {
	t.Parallel()

	assert.Nil(t, frontend.ExtractViewportImage(nil, view.Viewport{Width: 100, Height: 100}))

	screen := newMockPixelImage(100, 100, color.RGBA{R: 255, A: 255})
	assert.Nil(t, frontend.ExtractViewportImage(screen, view.Viewport{Width: 0, Height: 100}))
	assert.Nil(t, frontend.ExtractViewportImage(screen, view.Viewport{Width: 100, Height: -5}))
	assert.Nil(t, frontend.ExtractViewportImage(screen, view.Viewport{X: 200, Y: 200, Width: 100, Height: 100}))
}

func TestExtractViewportImage_ValidCrop(t *testing.T) {
	t.Parallel()

	const (
		screenWidth  = 300
		screenHeight = 200
		vpX          = 50
		vpY          = 20
		vpWidth      = 200
		vpHeight     = 160
	)

	fillColor := color.RGBA{R: 128, G: 64, B: 32, A: 255}
	screen := newMockPixelImage(screenWidth, screenHeight, fillColor)

	vp := view.Viewport{
		X:      vpX,
		Y:      vpY,
		Width:  vpWidth,
		Height: vpHeight,
	}

	img := frontend.ExtractViewportImage(screen, vp)
	require.NotNil(t, img)

	assert.Equal(t, vpWidth, img.Bounds().Dx())
	assert.Equal(t, vpHeight, img.Bounds().Dy())

	// Verify sample pixel values match the filled color
	r, g, b, a := img.At(0, 0).RGBA()
	assert.InDelta(t, uint32(128)<<8, r, 0x0100)
	assert.InDelta(t, uint32(64)<<8, g, 0x0100)
	assert.InDelta(t, uint32(32)<<8, b, 0x0100)
	assert.InDelta(t, uint32(255)<<8, a, 0x0100)
	assert.True(t, img.Opaque())
}

func TestExtractViewportImage_NormalizesAlphaToOpaque(t *testing.T) {
	t.Parallel()

	// Create screen with partially transparent pixels (e.g. unrendered border pixels)
	transparentColor := color.RGBA{R: 50, G: 50, B: 50, A: 0}
	screen := newMockPixelImage(100, 100, transparentColor)

	vp := view.Viewport{
		X:      10,
		Y:      10,
		Width:  80,
		Height: 80,
	}

	img := frontend.ExtractViewportImage(screen, vp)
	require.NotNil(t, img)
	assert.True(t, img.Opaque())

	_, _, _, a := img.At(0, 0).RGBA()
	assert.InDelta(t, uint32(255)<<8, a, 0x0100)
}
