// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package view

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateViewport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		winW         int
		winH         int
		aspectW      int
		aspectH      int
		expectedView Viewport
	}{
		{
			name:         "Exact 4:3 Window",
			winW:         960,
			winH:         720,
			aspectW:      4,
			aspectH:      3,
			expectedView: Viewport{X: 0, Y: 0, Width: 960, Height: 720},
		},
		{
			name:         "Widescreen 16:9 Window (Pillarbox)",
			winW:         1920,
			winH:         1080,
			aspectW:      4,
			aspectH:      3,
			expectedView: Viewport{X: 240, Y: 0, Width: 1440, Height: 1080},
		},
		{
			name:         "Tall Vertical Window (Letterbox)",
			winW:         720,
			winH:         1280,
			aspectW:      4,
			aspectH:      3,
			expectedView: Viewport{X: 0, Y: 370, Width: 720, Height: 540},
		},
		{
			name:         "Square Pixels 16:15 Aspect Ratio",
			winW:         768,
			winH:         720,
			aspectW:      16,
			aspectH:      15,
			expectedView: Viewport{X: 0, Y: 0, Width: 768, Height: 720},
		},
		{
			name:         "Zero or Negative Dimensions",
			winW:         0,
			winH:         720,
			aspectW:      4,
			aspectH:      3,
			expectedView: Viewport{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			vp := CalculateViewport(tt.winW, tt.winH, tt.aspectW, tt.aspectH)
			assert.Equal(t, tt.expectedView, vp)
		})
	}
}
