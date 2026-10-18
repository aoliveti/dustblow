// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !darwin

package frontend

const defaultDisplayScale = 1.0

// displayScale returns 1.0 on non-macOS platforms where outside dimensions
// already represent standard device pixels.
func displayScale() float64 {
	return defaultDisplayScale
}
