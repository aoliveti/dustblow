// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package storage implements secure, cross-platform persistence for
// battery-backed cartridge saves, screenshots, and machine state snapshots using os.Root sandboxing.
package storage

import (
	"errors"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// Named constants for storage layout, file permissions, and slot limits.
const (
	DirDustblowDefault = ".dustblow"
	DirSaves           = "saves"
	DirSnapshots       = "snapshots"

	FileBattery = "battery.sav"
	ExtState    = ".state"

	DirDustblowPictures = "Dustblow"

	EnvDustblowHome    = "DUSTBLOW_HOME"
	EnvScreenshotsDir  = "DUSTBLOW_SCREENSHOTS_DIR"

	MaxSnapshotSlots = 10
	HashShortLength  = 8

	PermDir  os.FileMode = 0o755
	PermFile os.FileMode = 0o644
)

// Sentinel errors for storage operations.
var (
	ErrInvalidSlot      = errors.New("storage: snapshot slot must be between 0 and 9")
	ErrSnapshotNotFound = errors.New("storage: snapshot not found")
)

var invalidNameCharsRegex = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// Option configures a Store during initialization.
type Option func(*Store)

// WithLogger sets a custom structured logger.
func WithLogger(logger *slog.Logger) Option {
	return func(s *Store) {
		if logger != nil {
			s.logger = logger
		}
	}
}

// Store coordinates sandboxed filesystem operations within the user's Dustblow directory.
type Store struct {
	root          *os.Root
	basePath      string
	screenshotDir string
	logger        *slog.Logger
}

// New initializes and returns a sandboxed Store.
// If basePath is empty, it checks DUSTBLOW_HOME or defaults to ~/.dustblow.
func New(basePath string, opts ...Option) (*Store, error) {
	resolvedPath, err := resolveBasePath(basePath)
	if err != nil {
		return nil, fmt.Errorf("resolve storage path: %w", err)
	}

	if err := os.MkdirAll(resolvedPath, PermDir); err != nil {
		return nil, fmt.Errorf("create base directory %s: %w", resolvedPath, err)
	}

	root, err := os.OpenRoot(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("open root sandbox %s: %w", resolvedPath, err)
	}

	if err := root.MkdirAll(DirSaves, PermDir); err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("create %s directory: %w", DirSaves, err)
	}

	s := &Store{
		root:     root,
		basePath: resolvedPath,
		logger:   slog.Default(),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s, nil
}

func resolveBasePath(customPath string) (string, error) {
	if customPath != "" {
		return filepath.Clean(customPath), nil
	}

	envPath := os.Getenv(EnvDustblowHome)
	if envPath != "" {
		return filepath.Clean(envPath), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("lookup home directory: %w", err)
	}

	return filepath.Join(homeDir, DirDustblowDefault), nil
}

// Close closes the underlying sandboxed root file descriptor.
func (s *Store) Close() error {
	if s.root == nil {
		return nil
	}
	return s.root.Close()
}

// BasePath returns the absolute filesystem path managed by this Store.
func (s *Store) BasePath() string {
	return s.basePath
}

// GameDir returns the sanitized, collision-free relative subpath for a game: saves/<clean_name>_<hash8>.
func (s *Store) GameDir(romName, romHash string) string {
	base := filepath.Base(romName)
	cleanName := strings.TrimSuffix(base, filepath.Ext(base))
	cleanName = invalidNameCharsRegex.ReplaceAllString(cleanName, "_")
	cleanName = strings.Trim(cleanName, "_")
	if cleanName == "" {
		cleanName = "unknown"
	}

	shortHash := romHash
	if len(shortHash) > HashShortLength {
		shortHash = shortHash[:HashShortLength]
	}

	folderName := fmt.Sprintf("%s_%s", cleanName, shortHash)
	return filepath.Join(DirSaves, folderName)
}

// SaveBattery atomically writes the 8KB battery-backed SRAM into saves/<game>/battery.sav.
func (s *Store) SaveBattery(gameDir string, sram []byte) error {
	if err := s.root.MkdirAll(gameDir, PermDir); err != nil {
		return fmt.Errorf("ensure game directory %s: %w", gameDir, err)
	}

	savePath := filepath.Join(gameDir, FileBattery)
	if err := s.root.WriteFile(savePath, sram, PermFile); err != nil {
		return fmt.Errorf("write %s: %w", savePath, err)
	}

	s.logger.Debug("saved cartridge battery SRAM", "path", savePath, "bytes", len(sram))
	return nil
}

// LoadBattery reads the 8KB SRAM from saves/<game>/battery.sav.
// Returns nil, nil if no save file exists yet (normal on first playthrough).
func (s *Store) LoadBattery(gameDir string) ([]byte, error) {
	savePath := filepath.Join(gameDir, FileBattery)
	data, err := s.root.ReadFile(savePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", savePath, err)
	}

	s.logger.Debug("loaded cartridge battery SRAM", "path", savePath, "bytes", len(data))
	return data, nil
}

// SaveSnapshot saves machine state snapshot data into saves/<game>/snapshots/slot<N>.state.
func (s *Store) SaveSnapshot(gameDir string, slot int, data []byte) error {
	if slot < 0 || slot >= MaxSnapshotSlots {
		return fmt.Errorf("%w: requested %d", ErrInvalidSlot, slot)
	}

	snapsDir := filepath.Join(gameDir, DirSnapshots)
	if err := s.root.MkdirAll(snapsDir, PermDir); err != nil {
		return fmt.Errorf("ensure snapshots directory %s: %w", snapsDir, err)
	}

	snapFile := fmt.Sprintf("slot%d%s", slot, ExtState)
	snapPath := filepath.Join(snapsDir, snapFile)
	if err := s.root.WriteFile(snapPath, data, PermFile); err != nil {
		return fmt.Errorf("write snapshot %s: %w", snapPath, err)
	}

	s.logger.Info("saved machine state snapshot", "slot", slot, "path", snapPath, "bytes", len(data))
	return nil
}

// LoadSnapshot reads machine state snapshot data from saves/<game>/snapshots/slot<N>.state.
func (s *Store) LoadSnapshot(gameDir string, slot int) ([]byte, error) {
	if slot < 0 || slot >= MaxSnapshotSlots {
		return nil, fmt.Errorf("%w: requested %d", ErrInvalidSlot, slot)
	}

	snapFile := fmt.Sprintf("slot%d%s", slot, ExtState)
	snapPath := filepath.Join(gameDir, DirSnapshots, snapFile)
	data, err := s.root.ReadFile(snapPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: slot %d", ErrSnapshotNotFound, slot)
	}
	if err != nil {
		return nil, fmt.Errorf("read snapshot %s: %w", snapPath, err)
	}

	s.logger.Info("loaded machine state snapshot", "slot", slot, "path", snapPath, "bytes", len(data))
	return data, nil
}

// DefaultScreenshotsDir resolves the default cross-platform directory for screenshots.
// On macOS: ~/Pictures/Dustblow
// On Windows: %USERPROFILE%\Pictures\Dustblow
// On Linux: $XDG_PICTURES_DIR/Dustblow or ~/Pictures/Dustblow
func DefaultScreenshotsDir() (string, error) {
	if envDir := os.Getenv(EnvScreenshotsDir); envDir != "" {
		return filepath.Clean(envDir), nil
	}

	if runtime.GOOS == "linux" {
		if xdgPictures := os.Getenv("XDG_PICTURES_DIR"); xdgPictures != "" {
			return filepath.Join(filepath.Clean(xdgPictures), DirDustblowPictures), nil
		}
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("lookup home directory: %w", err)
	}

	return filepath.Join(homeDir, "Pictures", DirDustblowPictures), nil
}

// SetScreenshotDir configures a custom directory path for saved screenshots.
func (s *Store) SetScreenshotDir(dir string) {
	s.screenshotDir = dir
}

// ScreenshotDir returns the resolved directory path where screenshots are saved.
func (s *Store) ScreenshotDir() (string, error) {
	if s.screenshotDir != "" {
		return filepath.Clean(s.screenshotDir), nil
	}
	return DefaultScreenshotsDir()
}

// SaveScreenshot saves a rendered image as a lossless PNG to the configured screenshot directory.
// Returns the absolute file path of the saved PNG image.
func (s *Store) SaveScreenshot(gameTitle string, img image.Image) (string, error) {
	targetDir, err := s.ScreenshotDir()
	if err != nil {
		return "", fmt.Errorf("resolve screenshot directory: %w", err)
	}

	if err := os.MkdirAll(targetDir, PermDir); err != nil {
		return "", fmt.Errorf("create screenshot directory %s: %w", targetDir, err)
	}

	cleanTitle := strings.TrimSuffix(filepath.Base(gameTitle), filepath.Ext(gameTitle))
	cleanTitle = invalidNameCharsRegex.ReplaceAllString(cleanTitle, "_")
	cleanTitle = strings.Trim(cleanTitle, "_")
	if cleanTitle == "" {
		cleanTitle = "game"
	}

	ts := time.Now().Format("2006-01-02_15-04-05")
	baseFilename := fmt.Sprintf("Dustblow_%s_%s", cleanTitle, ts)
	filename := baseFilename + ".png"
	fullPath := filepath.Join(targetDir, filename)

	for counter := 1; ; counter++ {
		cleanFullPath := filepath.Clean(fullPath)
		f, err := os.OpenFile(cleanFullPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, PermFile)
		if err == nil {
			encodeErr := png.Encode(f, img)
			_ = f.Close()
			if encodeErr != nil {
				return "", fmt.Errorf("encode screenshot png: %w", encodeErr)
			}
			break
		}

		if !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("create screenshot file %s: %w", fullPath, err)
		}

		filename = fmt.Sprintf("%s_%d.png", baseFilename, counter)
		fullPath = filepath.Join(targetDir, filename)
	}

	bounds := img.Bounds()
	s.logger.Info("saved screenshot", "path", fullPath, "width", bounds.Dx(), "height", bounds.Dy())
	return fullPath, nil
}
