// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"github.com/aoliveti/dustblow/internal/joypad"
	"github.com/hajimehoshi/ebiten/v2"
)

// Keymap defines the host keyboard key assignments for standard NES controller buttons.
type Keymap struct {
	Up     ebiten.Key
	Down   ebiten.Key
	Left   ebiten.Key
	Right  ebiten.Key
	A      ebiten.Key
	B      ebiten.Key
	Select ebiten.Key
	Start  ebiten.Key
}

// DefaultKeymap provides the standard keyboard layout for Player 1.
// Gold standard retro convention: Z for B (Run/Attack), X for A (Jump/Action).
func DefaultKeymap() Keymap {
	return Keymap{
		Up:     ebiten.KeyArrowUp,
		Down:   ebiten.KeyArrowDown,
		Left:   ebiten.KeyArrowLeft,
		Right:  ebiten.KeyArrowRight,
		A:      ebiten.KeyX,
		B:      ebiten.KeyZ,
		Select: ebiten.KeyBackspace,
		Start:  ebiten.KeyEnter,
	}
}

// pollKeyboardButtons checks pressed keys according to the active keymap and returns a bitmask.
func pollKeyboardButtons(k Keymap) byte {
	var mask byte

	// Button A (Jump / Action): primary key k.A (X) plus WASD home-row alias (K)
	if ebiten.IsKeyPressed(k.A) || ebiten.IsKeyPressed(ebiten.KeyK) {
		mask |= byte(joypad.ButtonA)
	}

	// Button B (Run / Attack): primary key k.B (Z) plus WASD home-row alias (J)
	if ebiten.IsKeyPressed(k.B) || ebiten.IsKeyPressed(ebiten.KeyJ) {
		mask |= byte(joypad.ButtonB)
	}

	// Select: primary key k.Select (Backspace) plus Right Shift alias
	if ebiten.IsKeyPressed(k.Select) || ebiten.IsKeyPressed(ebiten.KeyShiftRight) {
		mask |= byte(joypad.ButtonSelect)
	}

	// Start: primary key k.Start
	if ebiten.IsKeyPressed(k.Start) {
		mask |= byte(joypad.ButtonStart)
	}

	// D-Pad directions: primary arrows plus WASD aliases
	if ebiten.IsKeyPressed(k.Up) || ebiten.IsKeyPressed(ebiten.KeyW) {
		mask |= byte(joypad.ButtonUp)
	}
	if ebiten.IsKeyPressed(k.Down) || ebiten.IsKeyPressed(ebiten.KeyS) {
		mask |= byte(joypad.ButtonDown)
	}
	if ebiten.IsKeyPressed(k.Left) || ebiten.IsKeyPressed(ebiten.KeyA) {
		mask |= byte(joypad.ButtonLeft)
	}
	if ebiten.IsKeyPressed(k.Right) || ebiten.IsKeyPressed(ebiten.KeyD) {
		mask |= byte(joypad.ButtonRight)
	}

	return mask
}

var gamepadButtonBindings = [...]struct {
	button joypad.Button
	padBtn ebiten.StandardGamepadButton
}{
	// Action button A (Jump): East (A on 8BitDo/SNES) + North (X on 8BitDo/SNES)
	{joypad.ButtonA, ebiten.StandardGamepadButtonRightRight},
	{joypad.ButtonA, ebiten.StandardGamepadButtonRightTop},

	// Action button B (Run/Fireball): South (B on 8BitDo/SNES) + West (Y on 8BitDo/SNES)
	{joypad.ButtonB, ebiten.StandardGamepadButtonRightBottom},
	{joypad.ButtonB, ebiten.StandardGamepadButtonRightLeft},

	// System buttons
	{joypad.ButtonSelect, ebiten.StandardGamepadButtonCenterLeft},
	{joypad.ButtonStart, ebiten.StandardGamepadButtonCenterRight},

	// D-Pad buttons
	{joypad.ButtonUp, ebiten.StandardGamepadButtonLeftTop},
	{joypad.ButtonDown, ebiten.StandardGamepadButtonLeftBottom},
	{joypad.ButtonLeft, ebiten.StandardGamepadButtonLeftLeft},
	{joypad.ButtonRight, ebiten.StandardGamepadButtonLeftRight},
}

// pollGamepadPort checks a specific connected gamepad and returns its NES button bitmask.
func pollGamepadPort(id ebiten.GamepadID) byte {
	var mask byte
	if ebiten.IsStandardGamepadLayoutAvailable(id) {
		for _, binding := range gamepadButtonBindings {
			if ebiten.IsStandardGamepadButtonPressed(id, binding.padBtn) {
				mask |= byte(binding.button)
			}
		}
		return mask
	}

	return pollRawGamepadButtons(id)
}

const (
	rawGamepadBtnSouth      = 0
	rawGamepadBtnEast       = 1
	rawGamepadBtnWest       = 2
	rawGamepadBtnNorth      = 3
	rawGamepadBtnLeftBumper = 4
	rawGamepadBtnSelectSDL  = 6
	rawGamepadBtnStartSDL   = 7
	rawGamepadBtnSelectHID  = 8
	rawGamepadBtnStartHID   = 9
	rawGamepadBtnSelectAlt  = 10
	rawGamepadBtnStartAlt   = 11
)

func pollRawGamepadButtons(id ebiten.GamepadID) byte {
	var mask byte
	btnCount := ebiten.GamepadButtonCount(id)
	if btnCount == 0 {
		return 0
	}

	// Action button A: East/A or North/X
	if isRawButtonPressed(id, rawGamepadBtnEast, btnCount) || isRawButtonPressed(id, rawGamepadBtnNorth, btnCount) {
		mask |= byte(joypad.ButtonA)
	}
	// Action button B: South/B, West/Y, or L1
	if isRawButtonPressed(id, rawGamepadBtnSouth, btnCount) || isRawButtonPressed(id, rawGamepadBtnWest, btnCount) || isRawButtonPressed(id, rawGamepadBtnLeftBumper, btnCount) {
		mask |= byte(joypad.ButtonB)
	}
	// Select & Start
	if isRawButtonPressed(id, rawGamepadBtnSelectSDL, btnCount) || isRawButtonPressed(id, rawGamepadBtnSelectHID, btnCount) || isRawButtonPressed(id, rawGamepadBtnSelectAlt, btnCount) {
		mask |= byte(joypad.ButtonSelect)
	}
	if isRawButtonPressed(id, rawGamepadBtnStartSDL, btnCount) || isRawButtonPressed(id, rawGamepadBtnStartHID, btnCount) || isRawButtonPressed(id, rawGamepadBtnStartAlt, btnCount) {
		mask |= byte(joypad.ButtonStart)
	}

	return mask
}

func isRawButtonPressed(id ebiten.GamepadID, btn, count int) bool {
	return btn < count && ebiten.IsGamepadButtonPressed(id, ebiten.GamepadButton(btn))
}

// pollPlayer1Input aggregates keyboard and connected Player 1 gamepad input.
func pollPlayer1Input(k Keymap, gamepadIDs []ebiten.GamepadID) byte {
	kbMask := pollKeyboardButtons(k)
	var gpMask byte
	if len(gamepadIDs) > 0 {
		gpMask = pollGamepadPort(gamepadIDs[0])
	}
	return kbMask | gpMask
}

