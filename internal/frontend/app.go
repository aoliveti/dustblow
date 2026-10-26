// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package frontend provides the Ebitengine-based GUI frontend for the Dustblow NES emulator.
package frontend

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aoliveti/dustblow/internal/cartridge"
	"github.com/aoliveti/dustblow/internal/config"
	"github.com/aoliveti/dustblow/internal/console"
	"github.com/aoliveti/dustblow/internal/storage"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
)

// NewGameFromROM loads a .nes file from disk and initializes a Game instance with the live GPU shader pipeline.
func NewGameFromROM(romPath string) (*Game, error) {
	cleanPath := filepath.Clean(romPath)
	f, err := os.Open(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("open ROM: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	cart, err := cartridge.New(f)
	if err != nil {
		return nil, fmt.Errorf("load cartridge: %w", err)
	}

	c := console.New(cart)
	title := filepath.Base(romPath)
	game, err := NewGame(c, title)
	if err != nil {
		return nil, err
	}

	store, err := storage.New("")
	if err != nil {
		return game, nil
	}

	gameDir := store.GameDir(title, cart.Hash)
	game.SetStorage(store, gameDir)

	if cart.HasBattery {
		sram, loadErr := store.LoadBattery(gameDir)
		if loadErr == nil && len(sram) > 0 {
			cart.LoadBatterySRAM(sram)
		}
	}

	return game, nil
}

// Run coordinates argument parsing, ROM loading, and launching the Ebitengine window.
func Run(args []string) error {
	cfg, err := config.Parse(args)
	if err != nil {
		return err
	}

	game, err := NewGameFromROM(cfg.ROMPath)
	if err != nil {
		return err
	}

	defer func() {
		if game != nil {
			game.Close()
		}
	}()

	const audioSampleRate = 44100
	audioCtx := audio.NewContext(audioSampleRate)
	if err := initAudio(audioCtx, game); err != nil {
		return err
	}

	if cfg.Fullscreen {
		ebiten.SetFullscreen(true)
	}

	ebiten.SetWindowSize(defaultWindowWidth, defaultWindowHeight)
	ebiten.SetWindowTitle("Dustblow - " + game.title)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetPreferredColorMode(ebiten.ColorModeDark)

	return ebiten.RunGame(game)
}

const lowLatencyBufferSize = 45 * time.Millisecond

func initAudio(audioCtx *audio.Context, game *Game) error {
	if game.console.APU == nil {
		return nil
	}

	player, err := audioCtx.NewPlayer(game.console.APU.AudioStream())
	if err != nil {
		return fmt.Errorf("create audio player: %w", err)
	}
	player.SetBufferSize(lowLatencyBufferSize)
	game.SetAudioPlayer(player)
	return nil
}
