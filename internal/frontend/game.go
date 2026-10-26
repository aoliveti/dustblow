// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/aoliveti/dustblow/internal/console"
	"github.com/aoliveti/dustblow/internal/joypad"
	"github.com/aoliveti/dustblow/internal/storage"
	"github.com/aoliveti/dustblow/internal/view"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

const (
	defaultWindowWidth     = 960
	defaultWindowHeight    = 720
	batteryFlushInterval   = 600 // Periodic auto-flush interval (10 seconds at 60 FPS)
	fastForwardMultiplier  = 3
	fastForwardToastMsg    = "Fast-Forward (3x)"
	occludedDeltaThreshold = 35 * time.Millisecond
	sleepWakeThreshold     = time.Second
	minWarmupFrames        = 60
)

// Renderer abstracts the video presentation and shader pipeline from the game loop.
type Renderer interface {
	RenderGame(screen *ebiten.Image, ppuBuffer []byte, frameSeq uint64) view.Viewport
	CycleMode() view.Mode
}

// Game implements the ebiten.Game interface for NES emulation.
type Game struct {
	console             *console.Console
	renderer            Renderer
	audioPlayer         *audio.Player
	store               *storage.Store
	toast               *Toast
	gamepadIDs          []ebiten.GamepadID
	title               string
	gameDir             string
	keymap              Keymap
	lastFrame           []byte
	lastTickTime        time.Time
	frameSeq            uint64
	inputLatchP1        joypad.Latch
	inputLatchP2        joypad.Latch
	hasP1Gamepad        bool
	p1GamepadID         ebiten.GamepadID
	hasP2Gamepad        bool
	p2GamepadID         ebiten.GamepadID
	prevGamepadCount    int
	activeSlot          int
	batteryFlushTimer   int
	controllerMenuRow   int
	cursor              CursorManager
	paused              bool
	muted               bool
	fastForward         bool
	audioStarted        bool
	occluded            bool
	screenshotRequested bool
	controllerMenuOpen  bool
	menuPausedGame      bool
}

// NewGameWithRenderer constructs a Game instance with a custom or mocked renderer.
func NewGameWithRenderer(c *console.Console, r Renderer, title string) *Game {
	var initialFrame []byte
	if c != nil && c.PPU != nil {
		initialFrame = c.PPU.FrameBuffer()
	}

	gamepads := ebiten.AppendGamepadIDs(nil)
	curX, curY := ebiten.CursorPosition()

	g := &Game{
		console:          c,
		renderer:         r,
		title:            title,
		keymap:           DefaultKeymap(),
		lastFrame:        initialFrame,
		gamepadIDs:       gamepads,
		prevGamepadCount: len(gamepads),
		toast:            NewToast(),
		cursor:           NewCursorManager(curX, curY),
	}
	g.resolveConnectedGamepads()
	return g
}

// SetStorage connects disk storage persistence for snapshots and SRAM saves.
func (g *Game) SetStorage(s *storage.Store, gameDir string) {
	g.store = s
	g.gameDir = gameDir
}

// SetAudioPlayer connects the live host audio player for synchronized pause/resume.
func (g *Game) SetAudioPlayer(p *audio.Player) {
	g.audioPlayer = p
}

// Close gracefully releases any active runtime resources held by the Game instance and persists SRAM.
func (g *Game) Close() {
	if g == nil {
		return
	}
	ebiten.SetCursorMode(ebiten.CursorModeVisible)
	if g.audioPlayer != nil {
		g.audioPlayer.PauseAndStopReading()
		g.audioPlayer = nil
	}
	g.flushBatterySRAM()
	if g.store != nil {
		_ = g.store.Close()
		g.store = nil
	}
}

// IsCursorHidden reports whether the mouse cursor is currently suppressed.
func (g *Game) IsCursorHidden() bool {
	if g == nil {
		return false
	}
	return g.cursor.IsHidden()
}

// IsPaused reports whether the emulation loop is currently paused.
func (g *Game) IsPaused() bool {
	if g == nil {
		return false
	}
	return g.paused
}

// IsMuted reports whether host audio output is currently muted.
func (g *Game) IsMuted() bool {
	if g == nil {
		return false
	}
	return g.muted
}

// ToggleMute toggles audio output between muted (silence) and unmuted.
func (g *Game) ToggleMute() {
	if g == nil {
		return
	}
	g.toggleMute()
}

// IsFastForward reports whether 3x turbo emulation speed is currently active.
func (g *Game) IsFastForward() bool {
	if g == nil {
		return false
	}
	return g.fastForward
}

// SetFastForward enables or disables 3x turbo emulation speed and updates audio/HUD accordingly.
func (g *Game) SetFastForward(enabled bool) {
	if g == nil {
		return
	}
	g.setFastForward(enabled)
}

// Toast returns the active toast notification coordinator.
func (g *Game) Toast() *Toast {
	if g == nil {
		return nil
	}
	return g.toast
}

// TogglePause toggles the emulation pause state and updates toast/audio accordingly.
func (g *Game) TogglePause() {
	if g == nil {
		return
	}
	g.togglePause()
}

// Reset restores the console to its power-on reset state and resumes execution if paused.
func (g *Game) Reset() {
	if g == nil {
		return
	}
	g.handleReset()
}

// NewGame constructs a new Game instance wrapping the provided console and initializing the live GPU shader pipeline.
func NewGame(c *console.Console, title string) (*Game, error) {
	p, err := NewPipeline()
	if err != nil {
		return nil, fmt.Errorf("create view pipeline: %w", err)
	}

	return NewGameWithRenderer(c, p, title), nil
}

// Update processes per-tick host input and advances the emulation by one video frame.
func (g *Game) Update() error {
	g.toast.Update()
	g.updateCursorVisibility()

	if err := g.handleShortcuts(); err != nil {
		return err
	}

	if g.controllerMenuOpen {
		g.updateControllerMenu()
		return nil
	}

	if g.paused || g.handleDesktopOcclusion() {
		return nil
	}

	g.updateFastForward()
	g.updateBatteryAutoFlush()
	g.updateControllerInput()
	g.stepFrame()

	return nil
}

func (g *Game) handleShortcuts() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyF6) {
		g.toggleControllerMenu()
		return nil
	}

	if g.controllerMenuOpen {
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
			g.closeControllerMenu()
		}
		return nil
	}

	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		if g.audioPlayer != nil {
			g.audioPlayer.PauseAndStopReading()
		}
		ebiten.SetCursorMode(ebiten.CursorModeVisible)
		return ebiten.Termination
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyF) {
		g.toggleFullscreen()
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyP) {
		g.togglePause()
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyM) {
		g.toggleMute()
	}

	if isResetRequested() {
		g.handleReset()
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyF4) && g.renderer != nil {
		newMode := g.renderer.CycleMode()
		g.toast.Show(formatVideoMode(newMode), IconDisplay)
	}

	if slot, pressed := checkSlotKeys(); pressed {
		g.activeSlot = slot
		g.toast.Show(fmt.Sprintf("Slot %d selected", slot), IconSlot)
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyF5) {
		g.saveCurrentState()
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyF8) {
		g.loadCurrentState()
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyF12) {
		g.screenshotRequested = true
	}

	return nil
}

func (g *Game) handleReset() {
	g.flushBatterySRAM()
	g.fastForward = false
	if g.paused {
		g.paused = false
		g.resumeAudio()
	}
	g.syncAudioVolume()
	g.console.Reset()
	g.console.FlushAudio()
	g.toast.Show("Console reset", IconReset)
}

func (g *Game) resolveConnectedGamepads() {
	pads := ebiten.AppendGamepadIDs(g.gamepadIDs[:0])
	numPads := len(pads)

	switch {
	case numPads >= 2:
		g.hasP1Gamepad = true
		g.p1GamepadID = pads[0]
		g.hasP2Gamepad = true
		g.p2GamepadID = pads[1]
	case numPads == 1:
		g.hasP1Gamepad = true
		g.p1GamepadID = pads[0]
		g.hasP2Gamepad = false
	default:
		g.hasP1Gamepad = false
		g.hasP2Gamepad = false
	}
}

func (g *Game) detectGamepadHotplug(current []ebiten.GamepadID) {
	currCount := len(current)
	if currCount == g.prevGamepadCount {
		return
	}

	if currCount > g.prevGamepadCount {
		g.handleGamepadConnected(current)
	}
	if currCount < g.prevGamepadCount {
		g.handleGamepadDisconnected(current)
	}

	g.prevGamepadCount = currCount
}

func (g *Game) handleGamepadConnected(current []ebiten.GamepadID) {
	newPad := current[len(current)-1]
	name := ebiten.GamepadName(newPad)
	if name == "" {
		name = "Gamepad"
	}

	if !g.hasP1Gamepad {
		g.hasP1Gamepad = true
		g.p1GamepadID = newPad
		g.toast.Show(fmt.Sprintf("Connected: %s (P1)", name), IconGamepad)
		return
	}

	if !g.hasP2Gamepad {
		g.hasP2Gamepad = true
		g.p2GamepadID = newPad
		g.toast.Show(fmt.Sprintf("Connected: %s (P2)", name), IconGamepad)
		return
	}

	g.toast.Show(fmt.Sprintf("Connected: %s (Press F6 to assign)", name), IconGamepad)
}

func (g *Game) handleGamepadDisconnected(current []ebiten.GamepadID) {
	if g.hasP1Gamepad && !slices.Contains(current, g.p1GamepadID) {
		g.hasP1Gamepad = false
		g.toast.Show("P1 gamepad disconnected (using Keyboard)", IconWarning)
	}

	if g.hasP2Gamepad && !slices.Contains(current, g.p2GamepadID) {
		g.hasP2Gamepad = false
		g.toast.Show("P2 gamepad disconnected", IconWarning)
	}
}

func (g *Game) updateControllerInput() {
	g.gamepadIDs = ebiten.AppendGamepadIDs(g.gamepadIDs[:0])
	g.detectGamepadHotplug(g.gamepadIDs)

	rawMaskP1 := pollKeyboardButtons(g.keymap)
	var rawMaskP2 byte

	if g.hasP1Gamepad {
		rawMaskP1 |= pollGamepadPort(g.p1GamepadID)
	}
	if g.hasP2Gamepad {
		rawMaskP2 = pollGamepadPort(g.p2GamepadID)
	}

	maskP1 := g.inputLatchP1.Process(rawMaskP1)
	maskP2 := g.inputLatchP2.Process(rawMaskP2)

	if g.console != nil {
		g.console.SetButtons(joypad.Port1, maskP1)
		g.console.SetButtons(joypad.Port2, maskP2)
	}
}

func (g *Game) updateFastForward() {
	g.setFastForward(ebiten.IsKeyPressed(ebiten.KeyTab))
}

func (g *Game) setFastForward(enabled bool) {
	if enabled && g.paused {
		return
	}
	if g.fastForward == enabled {
		return
	}
	g.fastForward = enabled
	g.console.FlushAudio()
	g.syncAudioVolume()

	if enabled {
		g.toast.ShowSticky(fastForwardToastMsg, IconFastForward)
		return
	}
	if !g.paused && g.toast.IsSticky() && g.toast.Message() == fastForwardToastMsg {
		g.toast.Dismiss()
	}
}

func (g *Game) syncAudioVolume() {
	if g.audioPlayer == nil {
		return
	}
	vol := 1.0
	if g.muted || g.fastForward {
		vol = 0.0
	}
	g.audioPlayer.SetVolume(vol)
}

func (g *Game) stepFrame() {
	steps := 1
	if g.fastForward {
		steps = fastForwardMultiplier
	}
	for range steps {
		g.lastFrame = g.console.StepFrame()
		g.frameSeq++
	}
	if g.fastForward {
		g.console.FlushAudio()
	}

	if !g.audioStarted && g.audioPlayer != nil {
		g.syncAudioVolume()
		g.audioPlayer.Play()
		g.audioStarted = true
	}
}

func (g *Game) toggleFullscreen() {
	fullscreen := !ebiten.IsFullscreen()
	ebiten.SetFullscreen(fullscreen)
	if !fullscreen {
		ebiten.SetCursorMode(ebiten.CursorModeVisible)
		g.cursor.Reset()
	}
}

func (g *Game) updateCursorVisibility() {
	curX, curY := ebiten.CursorPosition()
	g.cursor.Update(ebiten.IsFullscreen(), curX, curY, func(hidden bool) {
		if hidden {
			ebiten.SetCursorMode(ebiten.CursorModeHidden)
			return
		}
		ebiten.SetCursorMode(ebiten.CursorModeVisible)
	})
}

func (g *Game) togglePause() {
	g.fastForward = false
	g.paused = !g.paused
	g.console.TogglePause()
	if g.paused {
		g.pauseAudio()
		g.toast.ShowSticky("Paused", IconPause)
		return
	}
	g.resumeAudio()
	g.toast.Dismiss()
}

func (g *Game) toggleMute() {
	g.muted = !g.muted
	g.syncAudioVolume()
	if g.muted {
		g.toast.Show("Muted", IconMute)
		return
	}
	g.toast.Show("Unmuted", IconUnmute)
}

func (g *Game) pauseAudio() {
	g.console.FlushAudio()
	if g.audioPlayer != nil {
		g.audioPlayer.Pause()
	}
}

func (g *Game) resumeAudio() {
	g.console.FlushAudio()
	if g.audioPlayer != nil {
		g.syncAudioVolume()
		g.audioPlayer.Play()
	}
}

func (g *Game) handleDesktopOcclusion() bool {
	now := time.Now()
	if g.lastTickTime.IsZero() {
		g.lastTickTime = now
		return false
	}
	delta := now.Sub(g.lastTickTime)
	g.lastTickTime = now

	// Discard any stale audio buffer leftover from a system sleep or long freeze (> 1s).
	if delta > sleepWakeThreshold && g.console != nil {
		g.console.FlushAudio()
	}

	// 1. isWindowOccluded(): Native Cocoa detection on macOS (Space switch, Mission Control).
	// 2. ebiten.IsWindowMinimized(): Cross-platform detection on Windows, Linux, and macOS.
	// 3. delta > occludedDeltaThreshold: Fallback guard against display freeze or deep sleep.
	isOccluded := isWindowOccluded() || ebiten.IsWindowMinimized() ||
		(!ebiten.IsFocused() && g.frameSeq > minWarmupFrames && delta > occludedDeltaThreshold)

	if isOccluded {
		if !g.occluded {
			g.occluded = true
			g.pauseAudio()
		}
		return true
	}

	if g.occluded {
		g.occluded = false
		if !g.paused {
			g.resumeAudio()
		}
	}
	return false
}

// Draw renders the NES game into screen via the shader pipeline.
func (g *Game) Draw(screen *ebiten.Image) {
	if g.renderer == nil || g.lastFrame == nil {
		return
	}

	vp := g.renderer.RenderGame(screen, g.lastFrame, g.frameSeq)

	if g.screenshotRequested {
		g.screenshotRequested = false
		g.handleScreenshot(screen, vp)
	}

	g.drawControllerMenu(screen)
	g.toast.Draw(screen)
}

// RequestScreenshot schedules a lossless PNG screenshot to be captured on the next rendered frame.
func (g *Game) RequestScreenshot() {
	if g == nil {
		return
	}
	g.screenshotRequested = true
}

func (g *Game) handleScreenshot(screen *ebiten.Image, vp view.Viewport) {
	img := ExtractViewportImage(screen, vp)
	if img == nil {
		return
	}

	g.toast.Show("Screenshot saved", IconCamera)

	title := g.title
	store := g.store
	go func() {
		if store == nil {
			var err error
			store, err = storage.New("")
			if err != nil {
				slog.Error("failed to initialize storage for screenshot", "error", err)
				return
			}
			defer func() {
				_ = store.Close()
			}()
		}
		if _, err := store.SaveScreenshot(title, img); err != nil {
			slog.Error("failed to save screenshot", "error", err)
		}
	}()
}

// Layout returns the host window dimensions for crisp scaling.
// On macOS Retina displays, it accounts for the Retina device scale factor so that
// rendering shaders and screenshots operate at true physical display resolution.
// On other platforms and non-Retina displays, it remains 1:1.
func (g *Game) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	scale := displayScale()
	return int(float64(outsideWidth) * scale), int(float64(outsideHeight) * scale)
}

func (g *Game) saveCurrentState() {
	if g.store == nil || g.console == nil {
		return
	}
	data, err := g.console.SaveState()
	if err != nil {
		g.toast.Show("Save state failed", IconError)
		return
	}
	if err := g.store.SaveSnapshot(g.gameDir, g.activeSlot, data); err != nil {
		g.toast.Show("Save state error", IconError)
		return
	}
	g.toast.Show(fmt.Sprintf("State saved (slot %d)", g.activeSlot), IconCheckmark)
}

func (g *Game) loadCurrentState() {
	if g.store == nil || g.console == nil {
		return
	}
	data, err := g.store.LoadSnapshot(g.gameDir, g.activeSlot)
	if errors.Is(err, storage.ErrSnapshotNotFound) {
		g.toast.Show(fmt.Sprintf("Slot %d is empty", g.activeSlot), IconError)
		return
	}
	if err != nil {
		g.toast.Show("Load state error", IconError)
		return
	}
	if err := g.console.LoadState(data); err != nil {
		g.toast.Show("Corrupt state data", IconError)
		return
	}
	g.toast.Show(fmt.Sprintf("State loaded (slot %d)", g.activeSlot), IconDownload)
}

func checkSlotKeys() (int, bool) {
	slotKeys := [...]ebiten.Key{
		ebiten.Key0,
		ebiten.Key1,
		ebiten.Key2,
		ebiten.Key3,
		ebiten.Key4,
		ebiten.Key5,
		ebiten.Key6,
		ebiten.Key7,
		ebiten.Key8,
		ebiten.Key9,
	}
	for slot, key := range slotKeys {
		if inpututil.IsKeyJustPressed(key) {
			return slot, true
		}
	}
	return 0, false
}

func isResetRequested() bool {
	if inpututil.IsKeyJustPressed(ebiten.KeyF2) {
		return true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		return ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta)
	}
	return false
}

func formatVideoMode(m view.Mode) string {
	switch m {
	case view.ModeSharpBilinear:
		return "Video: Sharp bilinear (4:3)"
	case view.ModeCRT:
		return "Video: CRT scanlines (4:3)"
	case view.ModePixelPerfect:
		return "Video: Pixel perfect (1:1)"
	default:
		return "Video: Unknown"
	}
}

func (g *Game) updateBatteryAutoFlush() {
	if g.store == nil || g.console == nil || g.console.Cartridge == nil {
		return
	}
	cart := g.console.Cartridge
	if !cart.HasBattery || !cart.IsSRAMDirty() {
		g.batteryFlushTimer = 0
		return
	}

	g.batteryFlushTimer++
	if g.batteryFlushTimer >= batteryFlushInterval {
		g.batteryFlushTimer = 0
		g.flushBatterySRAM()
	}
}

func (g *Game) flushBatterySRAM() {
	if g.store == nil || g.console == nil || g.console.Cartridge == nil {
		return
	}
	cart := g.console.Cartridge
	if cart.HasBattery && cart.IsSRAMDirty() {
		_ = g.store.SaveBattery(g.gameDir, cart.BatterySRAM())
		cart.ClearSRAMDirty()
	}
}
