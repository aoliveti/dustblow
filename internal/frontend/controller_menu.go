// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"image/color"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	menuRowPlayer1 = 0
	menuRowPlayer2 = 1
	menuRowCount   = 2

	menuCardWidth    = 450
	menuCardHeight   = 226
	menuCardRadius   = 20
	menuRowHeight    = 46
	menuRowRadius    = 12
	menuPaddingX     = 24
	menuMaxNameChars = 24
)

func (g *Game) toggleControllerMenu() {
	if g.controllerMenuOpen {
		g.closeControllerMenu()
		return
	}
	g.openControllerMenu()
}

func (g *Game) openControllerMenu() {
	g.setFastForward(false)
	g.controllerMenuOpen = true
	g.controllerMenuRow = menuRowPlayer1
	g.toast.Dismiss()

	if !g.paused {
		g.paused = true
		g.menuPausedGame = true
		if g.console != nil {
			g.console.TogglePause()
		}
		g.pauseAudio()
	}
}

func (g *Game) closeControllerMenu() {
	if !g.controllerMenuOpen {
		return
	}
	g.controllerMenuOpen = false

	if g.menuPausedGame {
		g.menuPausedGame = false
		g.paused = false
		if g.console != nil {
			g.console.TogglePause()
		}
		g.resumeAudio()
	}
}

func (g *Game) updateControllerMenu() {
	g.gamepadIDs = ebiten.AppendGamepadIDs(g.gamepadIDs[:0])
	g.detectGamepadHotplug(g.gamepadIDs)

	if isMenuCloseJustPressed(g.gamepadIDs) {
		g.closeControllerMenu()
		return
	}

	if isMenuUpJustPressed(g.gamepadIDs) || isMenuDownJustPressed(g.gamepadIDs) {
		g.controllerMenuRow = (g.controllerMenuRow + 1) % menuRowCount
	}

	if isMenuLeftJustPressed(g.gamepadIDs) {
		g.cycleControllerMenuOption(-1)
	}
	if isMenuRightJustPressed(g.gamepadIDs) {
		g.cycleControllerMenuOption(1)
	}
}

func isMenuCloseJustPressed(pads []ebiten.GamepadID) bool {
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyKPEnter) {
		return true
	}
	for _, id := range pads {
		if inpututil.IsStandardGamepadButtonJustPressed(id, ebiten.StandardGamepadButtonCenterRight) ||
			inpututil.IsStandardGamepadButtonJustPressed(id, ebiten.StandardGamepadButtonRightRight) {
			return true
		}
	}
	return false
}

func isMenuUpJustPressed(pads []ebiten.GamepadID) bool {
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) || inpututil.IsKeyJustPressed(ebiten.KeyW) {
		return true
	}
	for _, id := range pads {
		if inpututil.IsStandardGamepadButtonJustPressed(id, ebiten.StandardGamepadButtonLeftTop) {
			return true
		}
	}
	return false
}

func isMenuDownJustPressed(pads []ebiten.GamepadID) bool {
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) || inpututil.IsKeyJustPressed(ebiten.KeyS) {
		return true
	}
	for _, id := range pads {
		if inpututil.IsStandardGamepadButtonJustPressed(id, ebiten.StandardGamepadButtonLeftBottom) {
			return true
		}
	}
	return false
}

func isMenuLeftJustPressed(pads []ebiten.GamepadID) bool {
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) || inpututil.IsKeyJustPressed(ebiten.KeyA) {
		return true
	}
	for _, id := range pads {
		if inpututil.IsStandardGamepadButtonJustPressed(id, ebiten.StandardGamepadButtonLeftLeft) {
			return true
		}
	}
	return false
}

func isMenuRightJustPressed(pads []ebiten.GamepadID) bool {
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) || inpututil.IsKeyJustPressed(ebiten.KeyD) {
		return true
	}
	for _, id := range pads {
		if inpututil.IsStandardGamepadButtonJustPressed(id, ebiten.StandardGamepadButtonLeftRight) {
			return true
		}
	}
	return false
}

func (g *Game) cycleControllerMenuOption(delta int) {
	totalOptions := 1 + len(g.gamepadIDs)
	if totalOptions <= 1 {
		return
	}

	if g.controllerMenuRow == menuRowPlayer1 {
		currIdx := g.currentMenuOptionIndex(g.hasP1Gamepad, g.p1GamepadID)
		nextIdx := (currIdx + delta + totalOptions) % totalOptions
		g.applyPlayer1MenuOption(nextIdx)
		return
	}

	currIdx := g.currentMenuOptionIndex(g.hasP2Gamepad, g.p2GamepadID)
	nextIdx := (currIdx + delta + totalOptions) % totalOptions
	g.applyPlayer2MenuOption(nextIdx)
}

func (g *Game) currentMenuOptionIndex(hasPad bool, padID ebiten.GamepadID) int {
	if !hasPad {
		return 0
	}
	idx := slices.Index(g.gamepadIDs, padID)
	if idx < 0 {
		return 0
	}
	return idx + 1
}

func (g *Game) applyPlayer1MenuOption(optionIdx int) {
	if optionIdx <= 0 || optionIdx > len(g.gamepadIDs) {
		g.hasP1Gamepad = false
		return
	}
	chosenID := g.gamepadIDs[optionIdx-1]
	g.hasP1Gamepad = true
	g.p1GamepadID = chosenID
	if g.hasP2Gamepad && g.p2GamepadID == chosenID {
		g.hasP2Gamepad = false
	}
}

func (g *Game) applyPlayer2MenuOption(optionIdx int) {
	if optionIdx <= 0 || optionIdx > len(g.gamepadIDs) {
		g.hasP2Gamepad = false
		return
	}
	chosenID := g.gamepadIDs[optionIdx-1]
	g.hasP2Gamepad = true
	g.p2GamepadID = chosenID
	if g.hasP1Gamepad && g.p1GamepadID == chosenID {
		g.hasP1Gamepad = false
	}
}

func formatGamepadDisplayName(id ebiten.GamepadID) string {
	name := ebiten.GamepadName(id)
	if name == "" {
		name = fmt.Sprintf("Gamepad #%d", id+1)
	}
	if len(name) > menuMaxNameChars {
		return name[:menuMaxNameChars-3] + "..."
	}
	return name
}

func (g *Game) player1AssignmentLabel() string {
	if !g.hasP1Gamepad {
		return "Keyboard Only"
	}
	return "Keyboard + " + formatGamepadDisplayName(g.p1GamepadID)
}

func (g *Game) player2AssignmentLabel() string {
	if !g.hasP2Gamepad {
		return "Disconnected"
	}
	return formatGamepadDisplayName(g.p2GamepadID)
}

func (g *Game) drawControllerMenu(screen *ebiten.Image) {
	if !g.controllerMenuOpen || screen == nil || g.toast == nil {
		return
	}

	s := displayScale()
	g.toast.updateFaceScale(s)
	sf := float32(s)

	bounds := screen.Bounds()
	sw := float32(bounds.Dx())
	sh := float32(bounds.Dy())

	// Dimmed translucent backdrop
	vector.FillRect(screen, 0, 0, sw, sh, color.NRGBA{R: 6, G: 8, B: 14, A: 150}, false)

	cardW := float32(menuCardWidth) * sf
	cardH := float32(menuCardHeight) * sf
	if cardW > sw-32*sf {
		cardW = sw - 32*sf
	}
	cardX := (sw - cardW) / 2
	cardY := (sh - cardH) / 2
	cardR := float32(menuCardRadius) * sf

	drawDropShadow(screen, cardX, cardY, cardW, cardH, cardR, sf, 1.0)

	var cardPath vector.Path
	appendRoundedRectPath(&cardPath, cardX, cardY, cardW, cardH, cardR)

	var bgFill vector.DrawPathOptions
	bgFill.AntiAlias = true
	bgFill.ColorScale.ScaleWithColor(color.NRGBA{R: 18, G: 20, B: 28, A: 244})
	vector.FillPath(screen, &cardPath, nil, &bgFill)

	var outerStroke vector.DrawPathOptions
	outerStroke.AntiAlias = true
	outerStroke.ColorScale.ScaleWithColor(color.NRGBA{R: 0, G: 0, B: 0, A: 160})
	vector.StrokePath(screen, &cardPath, &vector.StrokeOptions{Width: 1.4 * sf}, &outerStroke)

	inset := 0.8 * sf
	var innerPath vector.Path
	appendRoundedRectPath(&innerPath, cardX+inset, cardY+inset, cardW-inset*2, cardH-inset*2, cardR-inset)
	var innerStroke vector.DrawPathOptions
	innerStroke.AntiAlias = true
	innerStroke.ColorScale.ScaleWithColor(color.NRGBA{R: 255, G: 255, B: 255, A: 50})
	vector.StrokePath(screen, &innerPath, &vector.StrokeOptions{Width: 1.0 * sf}, &innerStroke)

	g.drawControllerMenuHeader(screen, cardX, cardY, cardW, sf)
	g.drawControllerMenuRow(screen, cardX, cardY+66*sf, cardW, sf, menuRowPlayer1, "Player 1", g.player1AssignmentLabel())
	g.drawControllerMenuRow(screen, cardX, cardY+120*sf, cardW, sf, menuRowPlayer2, "Player 2", g.player2AssignmentLabel())
	g.drawControllerMenuFooter(screen, cardX, cardY, cardW, cardH, sf)
}

func (g *Game) drawControllerMenuHeader(screen *ebiten.Image, cardX, cardY, cardW, sf float32) {
	padX := float32(menuPaddingX) * sf
	headerCY := cardY + 32*sf
	badgeR := 14 * sf
	iconCX := cardX + padX + badgeR

	vector.FillCircle(screen, iconCX, headerCY, badgeR, color.NRGBA{R: 255, G: 255, B: 255, A: 22}, true)
	drawVectorIcon(
		screen,
		iconCX,
		headerCY,
		17*sf,
		IconGamepad,
		color.NRGBA{R: 245, G: 247, B: 252, A: 255},
		color.NRGBA{R: 18, G: 20, B: 28, A: 244},
	)

	title := "Controller Assignment"
	_, th := text.Measure(title, g.toast.face, 0)
	op := &text.DrawOptions{}
	op.GeoM.Translate(float64(iconCX+badgeR+12*sf), float64(headerCY-float32(th)/2))
	op.ColorScale.ScaleWithColor(color.NRGBA{R: 245, G: 247, B: 252, A: 255})
	text.Draw(screen, title, g.toast.face, op)

	divY := cardY + 54*sf
	vector.FillRect(screen, cardX+padX, divY, cardW-padX*2, 1*sf, color.NRGBA{R: 255, G: 255, B: 255, A: 24}, false)
}

func (g *Game) drawControllerMenuRow(screen *ebiten.Image, cardX, rowY, cardW, sf float32, rowIdx int, portLabel, valLabel string) {
	padX := float32(menuPaddingX) * sf
	rowX := cardX + padX
	rowW := cardW - padX*2
	rowH := float32(menuRowHeight) * sf
	rowR := float32(menuRowRadius) * sf
	selected := g.controllerMenuRow == rowIdx

	if selected {
		var rowPath vector.Path
		appendRoundedRectPath(&rowPath, rowX, rowY, rowW, rowH, rowR)
		var fillOpts vector.DrawPathOptions
		fillOpts.AntiAlias = true
		fillOpts.ColorScale.ScaleWithColor(color.NRGBA{R: 255, G: 255, B: 255, A: 24})
		vector.FillPath(screen, &rowPath, nil, &fillOpts)

		var strokeOpts vector.DrawPathOptions
		strokeOpts.AntiAlias = true
		strokeOpts.ColorScale.ScaleWithColor(color.NRGBA{R: 255, G: 255, B: 255, A: 58})
		vector.StrokePath(screen, &rowPath, &vector.StrokeOptions{Width: 1.0 * sf}, &strokeOpts)
	}

	labelAlpha := uint8(180)
	if selected {
		labelAlpha = 255
	}

	_, lh := text.Measure(portLabel, g.toast.face, 0)
	portOp := &text.DrawOptions{}
	portOp.GeoM.Translate(float64(rowX+16*sf), float64(rowY+(rowH-float32(lh))/2))
	portOp.ColorScale.ScaleWithColor(color.NRGBA{R: 245, G: 247, B: 252, A: labelAlpha})
	text.Draw(screen, portLabel, g.toast.face, portOp)

	displayVal := valLabel
	if selected {
		displayVal = "<  " + valLabel + "  >"
	}
	vw, vh := text.Measure(displayVal, g.toast.face, 0)
	valOp := &text.DrawOptions{}
	valOp.GeoM.Translate(float64(rowX+rowW-16*sf)-vw, float64(rowY+(rowH-float32(vh))/2))
	valOp.ColorScale.ScaleWithColor(color.NRGBA{R: 245, G: 247, B: 252, A: labelAlpha})
	text.Draw(screen, displayVal, g.toast.face, valOp)
}

func (g *Game) drawControllerMenuFooter(screen *ebiten.Image, cardX, cardY, cardW, cardH, sf float32) {
	padX := float32(menuPaddingX) * sf
	divY := cardY + cardH - 42*sf
	vector.FillRect(screen, cardX+padX, divY, cardW-padX*2, 1*sf, color.NRGBA{R: 255, G: 255, B: 255, A: 20}, false)

	footer := "Up/Down: Port   Left/Right: Device   Enter/F6: Resume"
	fw, fh := text.Measure(footer, g.toast.face, 0)
	op := &text.DrawOptions{}
	op.GeoM.Translate(float64(cardX+(cardW-float32(fw))/2), float64(divY+(42*sf-float32(fh))/2))
	op.ColorScale.ScaleWithColor(color.NRGBA{R: 185, G: 192, B: 208, A: 215})
	text.Draw(screen, footer, g.toast.face, op)
}
