// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend_test

import (
	"testing"

	"github.com/aoliveti/dustblow/internal/frontend"
	"github.com/aoliveti/dustblow/internal/view"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testDisplayWidth  = 960
	testDisplayHeight = 720
)

func TestPipeline_ModeSwitching(t *testing.T) {
	t.Parallel()

	p, err := frontend.NewPipeline()
	require.NoError(t, err)

	assert.Equal(t, view.ModeSharpBilinear, p.Mode())

	next := p.CycleMode()
	assert.Equal(t, view.ModeCRT, next)
	assert.Equal(t, view.ModeCRT, p.Mode())

	next = p.CycleMode()
	assert.Equal(t, view.ModePixelPerfect, next)
	assert.Equal(t, view.ModePixelPerfect, p.Mode())

	next = p.CycleMode()
	assert.Equal(t, view.ModeSharpBilinear, next)

	p.SetMode(view.ModeCRT)
	assert.Equal(t, view.ModeCRT, p.Mode())
}

func TestPipeline_RenderGameViewportGeometry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		mode             view.Mode
		winW             int
		winH             int
		expectedViewport view.Viewport
	}{
		{
			name:             "4:3 Window with Overscan",
			mode:             view.ModeSharpBilinear,
			winW:             testDisplayWidth,
			winH:             testDisplayHeight,
			expectedViewport: view.Viewport{X: 0, Y: 0, Width: 960, Height: 720},
		},
		{
			name:             "Widescreen 16:9 Window with Overscan (Pillarbox)",
			mode:             view.ModeCRT,
			winW:             1920,
			winH:             1080,
			expectedViewport: view.Viewport{X: 240, Y: 0, Width: 1440, Height: 1080},
		},
		{
			name:             "Pixel Perfect with Overscan (15:14 Aspect Ratio)",
			mode:             view.ModePixelPerfect,
			winW:             1500,
			winH:             1400,
			expectedViewport: view.Viewport{X: 0, Y: 0, Width: 1500, Height: 1400},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, err := frontend.NewPipeline()
			require.NoError(t, err)

			p.SetMode(tt.mode)

			screen := ebiten.NewImage(tt.winW, tt.winH)
			defer screen.Deallocate()

			vp := p.RenderGame(screen, nil, 0)
			assert.Equal(t, tt.expectedViewport, vp)
		})
	}
}

func TestPipeline_RenderNilScreen(t *testing.T) {
	t.Parallel()

	p, err := frontend.NewPipeline()
	require.NoError(t, err)

	dummyBuffer := make([]byte, 256*240*4)
	vp := p.RenderGame(nil, dummyBuffer, 1)
	assert.Equal(t, view.Viewport{}, vp)
}

func TestPipeline_InvalidModeIgnored(t *testing.T) {
	t.Parallel()

	p, err := frontend.NewPipeline()
	require.NoError(t, err)

	p.SetMode(view.ModeSharpBilinear)
	p.SetMode(view.Mode(-1))
	assert.Equal(t, view.ModeSharpBilinear, p.Mode())

	p.SetMode(view.Mode(99))
	assert.Equal(t, view.ModeSharpBilinear, p.Mode())
}
