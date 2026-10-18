// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"testing"

	"github.com/dustblow/dustblow/internal/joypad"
	"github.com/hajimehoshi/ebiten/v2"
)

func TestInput_DefaultKeymap(t *testing.T) {
	t.Parallel()

	k := DefaultKeymap()
	if k.Up != ebiten.KeyArrowUp {
		t.Errorf("expected Up to be ArrowUp, got %v", k.Up)
	}
	if k.Down != ebiten.KeyArrowDown {
		t.Errorf("expected Down to be ArrowDown, got %v", k.Down)
	}
	if k.Left != ebiten.KeyArrowLeft {
		t.Errorf("expected Left to be ArrowLeft, got %v", k.Left)
	}
	if k.Right != ebiten.KeyArrowRight {
		t.Errorf("expected Right to be ArrowRight, got %v", k.Right)
	}
	if k.A != ebiten.KeyX {
		t.Errorf("expected A to be KeyX, got %v", k.A)
	}
	if k.B != ebiten.KeyZ {
		t.Errorf("expected B to be KeyZ, got %v", k.B)
	}
	if k.Select != ebiten.KeyBackspace {
		t.Errorf("expected Select to be KeyBackspace, got %v", k.Select)
	}
	if k.Start != ebiten.KeyEnter {
		t.Errorf("expected Start to be KeyEnter, got %v", k.Start)
	}
}

func TestInput_PollKeyboardButtons_NoKeys(t *testing.T) {
	t.Parallel()

	k := DefaultKeymap()
	mask := pollKeyboardButtons(k)
	if mask != 0 {
		t.Errorf("expected 0 mask with no keys pressed, got %02X", mask)
	}
}

func TestInput_PollPlayer1Input_EmptyGamepads(t *testing.T) {
	t.Parallel()

	k := DefaultKeymap()
	mask := pollPlayer1Input(k, nil)
	if mask != 0 {
		t.Errorf("expected 0 mask, got %02X", mask)
	}
}

func TestGame_ControllerMenu_ToggleAndAutoPause(t *testing.T) {
	t.Parallel()

	t.Run("opens and auto-pauses running game, then auto-resumes on close", func(t *testing.T) {
		t.Parallel()
		g := &Game{
			toast:  NewToast(),
			paused: false,
		}

		g.toggleControllerMenu()
		if !g.controllerMenuOpen {
			t.Fatalf("expected controllerMenuOpen to be true")
		}
		if !g.paused || !g.menuPausedGame {
			t.Errorf("expected game to be paused automatically by menu")
		}

		g.toggleControllerMenu()
		if g.controllerMenuOpen {
			t.Fatalf("expected controllerMenuOpen to be false after second toggle")
		}
		if g.paused || g.menuPausedGame {
			t.Errorf("expected game to auto-resume when closing menu")
		}
	})

	t.Run("preserves manual pause state when closing menu", func(t *testing.T) {
		t.Parallel()
		g := &Game{
			toast:  NewToast(),
			paused: true,
		}

		g.toggleControllerMenu()
		if !g.controllerMenuOpen {
			t.Fatalf("expected controllerMenuOpen to be true")
		}
		if g.menuPausedGame {
			t.Errorf("expected menuPausedGame to be false when game was already manually paused")
		}

		g.closeControllerMenu()
		if !g.paused {
			t.Errorf("expected game to remain paused after closing menu")
		}
	})
}

func TestGame_ControllerMenu_CycleAndConflictResolution(t *testing.T) {
	t.Parallel()

	g := &Game{
		toast:        NewToast(),
		gamepadIDs:   []ebiten.GamepadID{0, 1},
		hasP1Gamepad: true,
		p1GamepadID:  0,
		hasP2Gamepad: false,
	}
	g.openControllerMenu()

	// Row 0 (Player 1): currently on Pad 0 (option 1). Cycling right (+1) selects Pad 1 (option 2).
	g.cycleControllerMenuOption(1)
	if !g.hasP1Gamepad || g.p1GamepadID != 1 {
		t.Errorf("expected P1 to select pad 1, got has=%v id=%d", g.hasP1Gamepad, g.p1GamepadID)
	}

	// Switch to Row 1 (Player 2) and select Pad 1 (which is currently on P1).
	// Conflict resolution must automatically unassign Pad 1 from P1 (leaving P1 on Keyboard Only).
	g.controllerMenuRow = menuRowPlayer2
	g.applyPlayer2MenuOption(2)
	if !g.hasP2Gamepad || g.p2GamepadID != 1 {
		t.Errorf("expected P2 to be assigned pad 1")
	}
	if g.hasP1Gamepad {
		t.Errorf("expected P1 to fall back to Keyboard Only when P2 claims its gamepad")
	}
	if g.player1AssignmentLabel() != "Keyboard Only" {
		t.Errorf("expected P1 label 'Keyboard Only', got %q", g.player1AssignmentLabel())
	}

	// Cycling P2 back to option 0 disconnects P2.
	g.applyPlayer2MenuOption(0)
	if g.hasP2Gamepad {
		t.Errorf("expected P2 to be disconnected")
	}
	if g.player2AssignmentLabel() != "Disconnected" {
		t.Errorf("expected P2 label 'Disconnected', got %q", g.player2AssignmentLabel())
	}
}

func TestGame_ControllerMenu_DrawWithoutPanic(t *testing.T) {
	t.Parallel()

	g := &Game{
		toast:        NewToast(),
		gamepadIDs:   []ebiten.GamepadID{0},
		hasP1Gamepad: true,
		p1GamepadID:  0,
	}
	g.openControllerMenu()

	screen := ebiten.NewImage(640, 480)
	g.drawControllerMenu(screen)
}

func TestGame_DetectGamepadHotplug(t *testing.T) {
	t.Parallel()

	g := &Game{
		toast:            NewToast(),
		prevGamepadCount: 0,
	}

	// Hotplug connect 1st gamepad
	g.detectGamepadHotplug([]ebiten.GamepadID{0})
	if g.prevGamepadCount != 1 {
		t.Errorf("expected prevGamepadCount to be 1, got %d", g.prevGamepadCount)
	}
	if !g.hasP1Gamepad {
		t.Errorf("expected 1st connected gamepad to attach to P1")
	}

	// Steady state
	g.detectGamepadHotplug([]ebiten.GamepadID{0})
	if g.prevGamepadCount != 1 {
		t.Errorf("expected prevGamepadCount to remain 1, got %d", g.prevGamepadCount)
	}

	// Disconnect 1st gamepad
	g.detectGamepadHotplug(nil)
	if g.prevGamepadCount != 0 {
		t.Errorf("expected prevGamepadCount to be 0 after disconnect, got %d", g.prevGamepadCount)
	}
	if g.hasP1Gamepad {
		t.Errorf("expected hasP1Gamepad to be false after disconnect")
	}
}

func TestGame_UpdateControllerInput_Routing(t *testing.T) {
	t.Parallel()

	g := &Game{
		keymap: DefaultKeymap(),
	}

	// Calling with no gamepads and no console connected must not panic
	g.updateControllerInput()
}

func TestJoypadPort1AndPort2Independence(t *testing.T) {
	t.Parallel()

	pad1 := joypad.New()
	pad2 := joypad.New()

	pad1.SetButton(joypad.ButtonA, true)
	pad2.SetButton(joypad.ButtonB, true)

	if pad1.Buttons()&byte(joypad.ButtonA) == 0 {
		t.Errorf("expected pad1 to have ButtonA pressed")
	}
	if pad1.Buttons()&byte(joypad.ButtonB) != 0 {
		t.Errorf("pad1 should not have ButtonB pressed")
	}

	if pad2.Buttons()&byte(joypad.ButtonB) == 0 {
		t.Errorf("expected pad2 to have ButtonB pressed")
	}
	if pad2.Buttons()&byte(joypad.ButtonA) != 0 {
		t.Errorf("pad2 should not have ButtonA pressed")
	}
}

func TestGamepadButtonBindings_Separation(t *testing.T) {
	t.Parallel()

	// Verify that physical A (Right) and physical B (Bottom) map to distinct NES buttons
	for _, b := range gamepadButtonBindings {
		if b.padBtn == ebiten.StandardGamepadButtonRightRight && b.button != joypad.ButtonA {
			t.Errorf("expected StandardGamepadButtonRightRight to map to ButtonA, got %v", b.button)
		}
		if b.padBtn == ebiten.StandardGamepadButtonRightBottom && b.button != joypad.ButtonB {
			t.Errorf("expected StandardGamepadButtonRightBottom to map to ButtonB, got %v", b.button)
		}
		if b.padBtn == ebiten.StandardGamepadButtonRightTop && b.button != joypad.ButtonA {
			t.Errorf("expected StandardGamepadButtonRightTop to map to ButtonA, got %v", b.button)
		}
		if b.padBtn == ebiten.StandardGamepadButtonRightLeft && b.button != joypad.ButtonB {
			t.Errorf("expected StandardGamepadButtonRightLeft to map to ButtonB, got %v", b.button)
		}
	}
}
