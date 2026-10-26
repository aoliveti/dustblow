// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend_test

import (
	"testing"

	"github.com/aoliveti/dustblow/internal/frontend"
	"github.com/stretchr/testify/assert"
)

const (
	testTimeoutTicks = 90
	testSampleX      = 100
	testSampleY      = 200
	testMovedX       = 150
	testMovedY       = 250
)

func TestCursorManager_FullscreenAutoHideAndWake(t *testing.T) {
	t.Parallel()

	cm := frontend.NewCursorManager(testSampleX, testSampleY)
	assert.False(t, cm.IsHidden())

	var hiddenStates []bool
	recordHidden := func(hidden bool) {
		hiddenStates = append(hiddenStates, hidden)
	}

	// 1. Advance in fullscreen without moving mouse for 89 ticks: cursor remains visible
	for range testTimeoutTicks - 1 {
		cm.Update(true, testSampleX, testSampleY, recordHidden)
	}
	assert.False(t, cm.IsHidden())
	assert.Empty(t, hiddenStates)

	// 2. Advance 90th tick: cursor auto-hides
	cm.Update(true, testSampleX, testSampleY, recordHidden)
	assert.True(t, cm.IsHidden())
	assert.Equal(t, []bool{true}, hiddenStates)

	// 3. Move mouse while hidden: cursor wakes immediately
	cm.Update(true, testMovedX, testMovedY, recordHidden)
	assert.False(t, cm.IsHidden())
	assert.Equal(t, []bool{true, false}, hiddenStates)

	// 4. Moving mouse continuously keeps cursor visible
	for i := range 10 {
		cm.Update(true, testMovedX+i, testMovedY+i, recordHidden)
	}
	assert.False(t, cm.IsHidden())

	// 5. Inactivity after move hides again
	for range testTimeoutTicks {
		cm.Update(true, testMovedX+9, testMovedY+9, recordHidden)
	}
	assert.True(t, cm.IsHidden())
	assert.Equal(t, []bool{true, false, true}, hiddenStates)

	// 6. Exiting fullscreen restores cursor
	cm.Update(false, testMovedX+9, testMovedY+9, recordHidden)
	assert.False(t, cm.IsHidden())
	assert.Equal(t, []bool{true, false, true, false}, hiddenStates)
}

func TestCursorManager_WindowedModeNeverHides(t *testing.T) {
	t.Parallel()

	cm := frontend.NewCursorManager(testSampleX, testSampleY)

	hiddenCalled := false
	onStateChange := func(_ bool) {
		hiddenCalled = true
	}

	// Stay still for 300 ticks in windowed mode
	for range 300 {
		cm.Update(false, testSampleX, testSampleY, onStateChange)
	}
	assert.False(t, cm.IsHidden())
	assert.False(t, hiddenCalled)
}

func TestCursorManager_Reset(t *testing.T) {
	t.Parallel()

	cm := frontend.NewCursorManager(testSampleX, testSampleY)
	for range testTimeoutTicks {
		cm.Update(true, testSampleX, testSampleY, func(bool) {})
	}
	assert.True(t, cm.IsHidden())

	cm.Reset()
	assert.False(t, cm.IsHidden())
}
