// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage_test

import (
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dustblow/dustblow/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	sampleSRAMSize       = 8192
	sampleSnapshotByte42 = 0x42
	sampleSnapshotByte99 = 0x99
)

func newTestStorage(t *testing.T) (*storage.Store, string) {
	t.Helper()
	dir := t.TempDir()
	discardLogger := slog.New(slog.DiscardHandler)
	store, err := storage.New(dir, storage.WithLogger(discardLogger))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store, dir
}

func TestStore_InitializationAndClose(t *testing.T) {
	t.Parallel()

	store, dir := newTestStorage(t)
	assert.Equal(t, dir, store.BasePath())

	// Verify saves directory was created
	savesInfo, err := os.Stat(filepath.Join(dir, storage.DirSaves))
	require.NoError(t, err)
	assert.True(t, savesInfo.IsDir())

	// Closing twice must not error or panic
	assert.NoError(t, store.Close())
	assert.NoError(t, store.Close())
}

func TestStore_ResolvePath_EnvironmentOverride(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv(storage.EnvDustblowHome, tempDir)

	store, err := storage.New("")
	require.NoError(t, err)
	defer func() {
		_ = store.Close()
	}()

	assert.Equal(t, tempDir, store.BasePath())
}

func TestStore_ResolvePath_DefaultHome(t *testing.T) {
	t.Setenv(storage.EnvDustblowHome, "")
	store, err := storage.New("")
	if err == nil {
		homeDir, _ := os.UserHomeDir()
		assert.Equal(t, filepath.Join(homeDir, storage.DirDustblowDefault), store.BasePath())
		_ = store.Close()
	}
}

func TestStore_GameDir(t *testing.T) {
	t.Parallel()

	store, _ := newTestStorage(t)

	tests := []struct {
		name     string
		romName  string
		romHash  string
		expected string
	}{
		{
			name:     "clean rom name and full sha256 hash",
			romName:  "Super Mario Bros.nes",
			romHash:  "d3b14a8f90c4210a56e7",
			expected: filepath.Join(storage.DirSaves, "Super_Mario_Bros_d3b14a8f"),
		},
		{
			name:     "special characters sanitized for cross-platform compatibility",
			romName:  "Zelda - 1986 (USA) [!].nes",
			romHash:  "a1b2c3d4e5f6",
			expected: filepath.Join(storage.DirSaves, "Zelda_1986_USA_a1b2c3d4"),
		},
		{
			name:     "short hash preserved",
			romName:  "metroid.nes",
			romHash:  "abcd",
			expected: filepath.Join(storage.DirSaves, "metroid_abcd"),
		},
		{
			name:     "empty rom name falls back to unknown",
			romName:  "...nes",
			romHash:  "12345678",
			expected: filepath.Join(storage.DirSaves, "unknown_12345678"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res := store.GameDir(tt.romName, tt.romHash)
			assert.Equal(t, tt.expected, res)
		})
	}
}

func TestStore_BatterySRAM_SaveAndLoad(t *testing.T) {
	t.Parallel()

	store, dir := newTestStorage(t)
	gameDir := store.GameDir("zelda.nes", "12345678abcdef")

	// Initially no battery save exists
	initialSRAM, err := store.LoadBattery(gameDir)
	require.NoError(t, err)
	assert.Nil(t, initialSRAM)

	// Save 8KB SRAM payload
	testPayload := make([]byte, sampleSRAMSize)
	testPayload[0] = sampleSnapshotByte42
	testPayload[sampleSRAMSize-1] = sampleSnapshotByte99

	err = store.SaveBattery(gameDir, testPayload)
	require.NoError(t, err)

	// Verify file was written to disk
	filePath := filepath.Join(dir, gameDir, storage.FileBattery)
	fileInfo, err := os.Stat(filePath)
	require.NoError(t, err)
	assert.Equal(t, int64(sampleSRAMSize), fileInfo.Size())

	// Load and verify content
	loadedSRAM, err := store.LoadBattery(gameDir)
	require.NoError(t, err)
	assert.Equal(t, testPayload, loadedSRAM)
}

func TestStore_Snapshots_SaveAndLoad(t *testing.T) {
	t.Parallel()

	store, dir := newTestStorage(t)
	gameDir := store.GameDir("smb.nes", "aabbccdd11223344")

	// Loading nonexistent snapshot returns ErrSnapshotNotFound
	_, err := store.LoadSnapshot(gameDir, 0)
	require.ErrorIs(t, err, storage.ErrSnapshotNotFound)

	// Saving with invalid slots (< 0 or >= 10) returns ErrInvalidSlot
	require.ErrorIs(t, store.SaveSnapshot(gameDir, -1, []byte{1}), storage.ErrInvalidSlot)
	require.ErrorIs(t, store.SaveSnapshot(gameDir, 10, []byte{1}), storage.ErrInvalidSlot)

	_, err = store.LoadSnapshot(gameDir, -1)
	require.ErrorIs(t, err, storage.ErrInvalidSlot)
	_, err = store.LoadSnapshot(gameDir, 10)
	require.ErrorIs(t, err, storage.ErrInvalidSlot)

	// Save and load valid snapshots for slot 0 and slot 9
	payload0 := []byte{0x01, 0x02, 0x03}
	payload9 := []byte{0x09, 0x08, 0x07}

	require.NoError(t, store.SaveSnapshot(gameDir, 0, payload0))
	require.NoError(t, store.SaveSnapshot(gameDir, 9, payload9))

	loaded0, err := store.LoadSnapshot(gameDir, 0)
	require.NoError(t, err)
	assert.Equal(t, payload0, loaded0)

	loaded9, err := store.LoadSnapshot(gameDir, 9)
	require.NoError(t, err)
	assert.Equal(t, payload9, loaded9)

	// Check file location in filesystem
	snapPath0 := filepath.Join(dir, gameDir, storage.DirSnapshots, "slot0.state")
	assert.FileExists(t, snapPath0)
}

func TestStore_SandboxPathTraversalProtection(t *testing.T) {
	t.Parallel()

	store, _ := newTestStorage(t)

	// Attempting to escape sandbox with "../" must be blocked by os.Root
	err := store.SaveBattery("../../escape", []byte{1, 2, 3})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "escapes") || strings.Contains(err.Error(), "outside"))
}

func TestStore_ScreenshotDir_DefaultAndCustom(t *testing.T) {
	t.Parallel()

	store, _ := newTestStorage(t)

	// Default directory should contain Dustblow
	defaultDir, err := store.ScreenshotDir()
	require.NoError(t, err)
	assert.Contains(t, defaultDir, storage.DirDustblowPictures)

	// Custom override
	customDir := t.TempDir()
	store.SetScreenshotDir(customDir)
	resolved, err := store.ScreenshotDir()
	require.NoError(t, err)
	assert.Equal(t, customDir, resolved)
}

func TestStore_SaveScreenshot(t *testing.T) {
	t.Parallel()

	store, _ := newTestStorage(t)
	customDir := t.TempDir()
	store.SetScreenshotDir(customDir)

	const (
		imgW = 240
		imgH = 224
	)
	img := image.NewRGBA(image.Rect(0, 0, imgW, imgH))
	for y := range imgH {
		for x := range imgW {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}

	savedPath, err := store.SaveScreenshot("Super Mario Bros. (USA).nes", img)
	require.NoError(t, err)
	assert.FileExists(t, savedPath)
	assert.True(t, strings.HasPrefix(filepath.Base(savedPath), "Dustblow_Super_Mario_Bros"))
	assert.True(t, strings.HasSuffix(savedPath, ".png"))

	// Verify the saved file is a valid PNG with correct dimensions
	f, err := os.Open(filepath.Clean(savedPath))
	require.NoError(t, err)
	defer func() {
		_ = f.Close()
	}()

	decoded, err := png.Decode(f)
	require.NoError(t, err)
	assert.Equal(t, imgW, decoded.Bounds().Dx())
	assert.Equal(t, imgH, decoded.Bounds().Dy())

	// Saving second screenshot immediately should avoid filename collision
	secondPath, err := store.SaveScreenshot("Super Mario Bros. (USA).nes", img)
	require.NoError(t, err)
	assert.FileExists(t, secondPath)
}
