// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend_test

import (
	"testing"

	"github.com/dustblow/dustblow/internal/cartridge"
	"github.com/dustblow/dustblow/internal/console"
	"github.com/dustblow/dustblow/internal/frontend"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testPRGSize = 32768
	testCHRSize = 8192
)

func newDummyCartridge() *cartridge.Cartridge {
	prg := make([]byte, testPRGSize)
	cart := &cartridge.Cartridge{
		PRG: prg,
		CHR: make([]byte, testCHRSize),
	}
	cart.Mapper = cartridge.NewMapper0(cart)
	return cart
}

func TestGame_PauseAndReset(t *testing.T) {
	t.Parallel()

	c := console.New(newDummyCartridge())
	require.NotNil(t, c)

	g := frontend.NewGameWithRenderer(c, nil, "Dustblow Test")
	require.NotNil(t, g)

	assert.False(t, g.IsPaused())
	assert.False(t, c.IsPaused())
	assert.False(t, g.Toast().IsSticky())

	// 1. Toggle pause: game, console, and toast should reflect paused state
	g.TogglePause()
	assert.True(t, g.IsPaused())
	assert.True(t, c.IsPaused())
	assert.True(t, g.Toast().IsSticky())

	// 2. Reset while paused: must unpause game and console, and clear sticky toast
	g.Reset()
	assert.False(t, g.IsPaused())
	assert.False(t, c.IsPaused())
	assert.False(t, g.Toast().IsSticky())
}

func TestGame_ResetWhenNotPaused(t *testing.T) {
	t.Parallel()

	c := console.New(newDummyCartridge())
	require.NotNil(t, c)

	g := frontend.NewGameWithRenderer(c, nil, "Dustblow Test")
	require.NotNil(t, g)

	assert.False(t, g.IsPaused())
	assert.False(t, c.IsPaused())

	// Reset when already running remains running
	g.Reset()
	assert.False(t, g.IsPaused())
	assert.False(t, c.IsPaused())
	assert.False(t, g.Toast().IsSticky())
}

func TestGame_ToggleMute(t *testing.T) {
	t.Parallel()

	c := console.New(newDummyCartridge())
	require.NotNil(t, c)

	g := frontend.NewGameWithRenderer(c, nil, "Dustblow Test")
	require.NotNil(t, g)

	assert.False(t, g.IsMuted())

	// 1. Toggle mute on
	g.ToggleMute()
	assert.True(t, g.IsMuted())
	assert.Equal(t, "Muted", g.Toast().Message())
	assert.False(t, g.Toast().IsSticky())

	// 2. Toggle mute off
	g.ToggleMute()
	assert.False(t, g.IsMuted())
	assert.Equal(t, "Unmuted", g.Toast().Message())
	assert.False(t, g.Toast().IsSticky())
}

func TestGame_FastForward(t *testing.T) {
	t.Parallel()

	c := console.New(newDummyCartridge())
	require.NotNil(t, c)

	g := frontend.NewGameWithRenderer(c, nil, "Dustblow Test")
	require.NotNil(t, g)

	assert.False(t, g.IsFastForward())

	// 1. Enable fast-forward: shows sticky toast
	g.SetFastForward(true)
	assert.True(t, g.IsFastForward())
	assert.True(t, g.Toast().IsSticky())
	assert.Equal(t, "Fast-Forward (3x)", g.Toast().Message())

	// 2. Disable fast-forward: dismisses sticky toast
	g.SetFastForward(false)
	assert.False(t, g.IsFastForward())
	assert.False(t, g.Toast().IsSticky())

	// 3. Pausing while fast-forwarding cancels fast-forward and shows Paused sticky toast
	g.SetFastForward(true)
	assert.True(t, g.IsFastForward())
	g.TogglePause()
	assert.True(t, g.IsPaused())
	assert.False(t, g.IsFastForward())
	assert.Equal(t, "Paused", g.Toast().Message())

	// 4. Cannot enable fast-forward while paused
	g.SetFastForward(true)
	assert.False(t, g.IsFastForward())
}

func TestGame_NilGuards(t *testing.T) {
	t.Parallel()

	var g *frontend.Game
	assert.False(t, g.IsPaused())
	assert.False(t, g.IsMuted())
	assert.False(t, g.IsFastForward())
	assert.False(t, g.IsCursorHidden())
	assert.Nil(t, g.Toast())
	g.TogglePause()
	g.ToggleMute()
	g.SetFastForward(true)
	g.Reset()
	g.RequestScreenshot()
	g.Close()
}

func TestGame_RequestScreenshot(t *testing.T) {
	t.Parallel()

	c := console.New(newDummyCartridge())
	g := frontend.NewGameWithRenderer(c, nil, "Dustblow Test")
	require.NotNil(t, g)

	// Calling RequestScreenshot sets the flag without error
	g.RequestScreenshot()
}

func TestGame_Layout(t *testing.T) {
	t.Parallel()

	c := console.New(newDummyCartridge())
	g := frontend.NewGameWithRenderer(c, nil, "Dustblow Test")
	require.NotNil(t, g)

	tests := []struct {
		name          string
		outsideWidth  int
		outsideHeight int
	}{
		{
			name:          "Standard 4:3 Window",
			outsideWidth:  960,
			outsideHeight: 720,
		},
		{
			name:          "Fullscreen 16:10 Laptop",
			outsideWidth:  1470,
			outsideHeight: 956,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w, h := g.Layout(tt.outsideWidth, tt.outsideHeight)
			assert.GreaterOrEqual(t, w, tt.outsideWidth)
			assert.GreaterOrEqual(t, h, tt.outsideHeight)
		})
	}
}
