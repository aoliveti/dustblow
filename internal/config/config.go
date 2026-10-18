// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package config implements CLI argument parsing and runtime configuration for Dustblow.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

// ErrMissingROMPath is returned when no ROM file path argument is provided.
var ErrMissingROMPath = errors.New("missing ROM path argument")

// Config contains validated startup parameters.
type Config struct {
	ROMPath    string
	Fullscreen bool
}

// Parse validates command-line arguments and returns an initialized Config.
func Parse(args []string) (*Config, error) {
	fs := flag.NewFlagSet("dustblow", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var fullscreen bool
	fs.BoolVar(&fullscreen, "fullscreen", false, "Start in fullscreen mode")
	fs.BoolVar(&fullscreen, "f", false, "Start in fullscreen mode (shorthand)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, fmt.Errorf("usage: dustblow [-f|--fullscreen] <path-to-game.nes>")
		}
		return nil, fmt.Errorf("parse arguments: %w", err)
	}

	if fs.NArg() == 0 {
		return nil, fmt.Errorf("%w\nUsage: dustblow [-f|--fullscreen] <path-to-game.nes>", ErrMissingROMPath)
	}

	return &Config{
		ROMPath:    fs.Arg(0),
		Fullscreen: fullscreen,
	}, nil
}
