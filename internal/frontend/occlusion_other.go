// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !darwin

package frontend

// isWindowOccluded is a no-op on non-macOS desktop platforms (Windows/Linux)
// where virtual desktops do not throttle display links or audio playback.
func isWindowOccluded() bool {
	return false
}
