// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend_test

import (
	"testing"

	"github.com/dustblow/dustblow/internal/frontend"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToast_TransientCountdown(t *testing.T) {
	t.Parallel()

	toast := frontend.NewToast()
	require.NotNil(t, toast)

	toast.Show("State saved", frontend.IconCheckmark)
	assert.False(t, toast.IsSticky())

	// Advance 50 frames
	for range 50 {
		toast.Update()
	}
	assert.False(t, toast.IsSticky())

	// Advance beyond duration (108 frames)
	for range 100 {
		toast.Update()
	}
	assert.False(t, toast.IsSticky())
}

func TestToast_StickyPersistent(t *testing.T) {
	t.Parallel()

	toast := frontend.NewToast()
	require.NotNil(t, toast)

	toast.ShowSticky("Paused", frontend.IconPause)
	assert.True(t, toast.IsSticky())

	// Advance 300 frames: sticky toast should not expire
	for range 300 {
		toast.Update()
	}
	assert.True(t, toast.IsSticky())

	// Showing a transient toast overrides sticky
	toast.Show("Resumed", frontend.IconDisplay)
	assert.False(t, toast.IsSticky())

	// Advance 200 frames: transient toast expires
	for range 200 {
		toast.Update()
	}
	assert.False(t, toast.IsSticky())
}

func TestToast_Dismiss(t *testing.T) {
	t.Parallel()

	toast := frontend.NewToast()
	require.NotNil(t, toast)

	toast.ShowSticky("Paused", frontend.IconPause)
	assert.True(t, toast.IsSticky())

	toast.Dismiss()
	assert.False(t, toast.IsSticky())
}

func TestToast_NilGuards(t *testing.T) {
	t.Parallel()

	var toast *frontend.Toast
	assert.False(t, toast.IsSticky())
	toast.Show("Test", frontend.IconCheckmark)
	toast.ShowSticky("Test", frontend.IconPause)
	toast.Dismiss()
	toast.Update()
	toast.Draw(nil)
}

func TestToast_DrawWithoutPanic(t *testing.T) {
	t.Parallel()

	toast := frontend.NewToast()
	require.NotNil(t, toast)

	screen := ebiten.NewImage(256, 240)

	// Draw when empty: no-op
	toast.Draw(screen)

	// Draw transient toast
	toast.Show("Video Mode", frontend.IconDisplay)
	toast.Draw(screen)

	// Draw sticky pause toast
	toast.ShowSticky("Paused", frontend.IconPause)
	for range 20 {
		toast.Update()
		toast.Draw(screen)
	}

	// Verify all icon types render cleanly without panic
	allIcons := []frontend.ToastIcon{
		frontend.IconCheckmark,
		frontend.IconDownload,
		frontend.IconPause,
		frontend.IconCamera,
		frontend.IconGamepad,
		frontend.IconMute,
		frontend.IconUnmute,
		frontend.IconFastForward,
		frontend.IconReset,
		frontend.IconError,
		frontend.IconWarning,
		frontend.IconSlot,
		frontend.IconDisplay,
	}

	for _, icon := range allIcons {
		toast.Show("Icon Test", icon)
		toast.Draw(screen)
	}
}
