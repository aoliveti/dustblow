// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

const (
	// cursorHideTimeoutTicks is the duration of inactivity in frames (~1.5s at 60 FPS)
	// before hiding the mouse cursor in fullscreen mode.
	cursorHideTimeoutTicks = 90
)

// CursorManager handles automatic mouse cursor suppression in fullscreen mode,
// waking the cursor upon detected movement and restoring it upon exiting fullscreen.
type CursorManager struct {
	lastX         int
	lastY         int
	inactiveTicks int
	hidden        bool
}

// NewCursorManager constructs a CursorManager initialized to the given cursor coordinates.
func NewCursorManager(initialX, initialY int) CursorManager {
	return CursorManager{
		lastX: initialX,
		lastY: initialY,
	}
}

// Update evaluates cursor visibility based on the current window mode and coordinates.
// setHidden callback communicates state changes to the host windowing environment.
func (c *CursorManager) Update(fullscreen bool, curX, curY int, setHidden func(bool)) {
	if !fullscreen {
		if c.hidden {
			setHidden(false)
			c.hidden = false
		}
		c.inactiveTicks = 0
		c.lastX = curX
		c.lastY = curY
		return
	}

	if curX != c.lastX || curY != c.lastY {
		c.lastX = curX
		c.lastY = curY
		c.inactiveTicks = 0
		if c.hidden {
			setHidden(false)
			c.hidden = false
		}
		return
	}

	if !c.hidden {
		c.inactiveTicks++
		if c.inactiveTicks >= cursorHideTimeoutTicks {
			setHidden(true)
			c.hidden = true
		}
	}
}

// IsHidden reports whether the mouse cursor is currently suppressed.
func (c *CursorManager) IsHidden() bool {
	return c.hidden
}

// Reset clears the inactivity counter and hidden state.
func (c *CursorManager) Reset() {
	c.inactiveTicks = 0
	c.hidden = false
}
