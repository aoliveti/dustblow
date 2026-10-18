// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin

package frontend

import (
	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var (
	classNSApplication   objc.Class
	selSharedApplication objc.SEL
	selWindows           objc.SEL
	selCount             objc.SEL
	selObjectAtIndex     objc.SEL
	selOcclusionState    objc.SEL
	occlusionInitDone    bool
)

const nsWindowOcclusionStateVisible = 1 << 1

func initCocoaOcclusion() {
	if occlusionInitDone {
		return
	}
	_, _ = purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	classNSApplication = objc.GetClass("NSApplication")
	selSharedApplication = objc.RegisterName("sharedApplication")
	selWindows = objc.RegisterName("windows")
	selCount = objc.RegisterName("count")
	selObjectAtIndex = objc.RegisterName("objectAtIndex:")
	selOcclusionState = objc.RegisterName("occlusionState")
	occlusionInitDone = true
}

// isWindowOccluded reports whether the primary application window is occluded on macOS
// (e.g. moved to an inactive Space, Mission Control, or minimized).
func isWindowOccluded() bool {
	if !occlusionInitDone {
		initCocoaOcclusion()
	}
	if classNSApplication == 0 {
		return false
	}
	app := objc.ID(classNSApplication).Send(selSharedApplication)
	if app == 0 {
		return false
	}
	windows := app.Send(selWindows)
	if windows == 0 {
		return false
	}
	count := int(windows.Send(selCount))
	if count == 0 {
		return false
	}
	win := windows.Send(selObjectAtIndex, 0)
	if win == 0 {
		return false
	}
	state := uintptr(win.Send(selOcclusionState))
	return (state & nsWindowOcclusionStateVisible) == 0
}
