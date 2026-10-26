// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package main provides the CLI entrypoint for the Dustblow NES emulator executable.
package main

import (
	"fmt"
	"os"

	"github.com/aoliveti/dustblow/internal/frontend"
)

func main() {
	if err := frontend.Run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "dustblow: %v\n", err)
		os.Exit(1)
	}
}
