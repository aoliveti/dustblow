// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin

package frontend

import (
	"sync"

	"github.com/ebitengine/purego/objc"
)

const defaultDisplayScale = 1.0

var (
	cocoaScaleOnce        sync.Once
	classNSScreen         objc.Class
	selMainWindow         objc.SEL
	selScreen             objc.SEL
	selMainScreen         objc.SEL
	selBackingScaleFactor objc.SEL
)

func initCocoaScale() {
	initCocoaOcclusion()
	classNSScreen = objc.GetClass("NSScreen")
	selMainWindow = objc.RegisterName("mainWindow")
	selScreen = objc.RegisterName("screen")
	selMainScreen = objc.RegisterName("mainScreen")
	selBackingScaleFactor = objc.RegisterName("backingScaleFactor")
}

// displayScale returns the native monitor scale factor on macOS (Retina displays).
// On Retina screens it returns 2.0 (or matching backingScaleFactor); on standard
// external non-Retina monitors it returns 1.0.
func displayScale() float64 {
	cocoaScaleOnce.Do(initCocoaScale)

	screen := activeWindowScreen()
	if screen == 0 && classNSScreen != 0 {
		screen = objc.ID(classNSScreen).Send(selMainScreen)
	}

	if screen == 0 {
		return defaultDisplayScale
	}

	scale := objc.Send[float64](screen, selBackingScaleFactor)
	if scale > defaultDisplayScale {
		return scale
	}

	return defaultDisplayScale
}

func activeWindowScreen() objc.ID {
	if classNSApplication == 0 {
		return 0
	}
	app := objc.ID(classNSApplication).Send(selSharedApplication)
	if app == 0 {
		return 0
	}
	win := app.Send(selMainWindow)
	if win == 0 {
		return 0
	}
	return win.Send(selScreen)
}
