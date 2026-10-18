// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// ToastIcon defines the semantic vector glyph rendered inside the HUD pill.
type ToastIcon int

// Toast icons supported by the HUD pill.
const (
	IconCheckmark   ToastIcon = iota // State saved
	IconDownload                     // State loaded
	IconPause                        // Emulation paused
	IconCamera                       // Screenshot captured
	IconGamepad                      // Controller connected / mapped
	IconMute                         // Audio muted
	IconUnmute                       // Audio unmuted
	IconFastForward                  // Turbo emulation
	IconReset                        // Console reset
	IconError                        // Error / empty slot / fail
	IconWarning                      // Warning alert
	IconSlot                         // Save state slot selected
	IconDisplay                      // Video shader mode cycled
)

const (
	iconBaseSize float32 = 20.0
)

// iconColor returns a crisp monochrome off-white/silver tint matching FontAwesome / Apple HUD aesthetics.
func iconColor(_ ToastIcon, alpha float32) color.NRGBA {
	return color.NRGBA{
		R: 245,
		G: 247,
		B: 252,
		A: uint8(250 * alpha),
	}
}

func appendRoundedRectPath(p *vector.Path, x, y, w, h, r float32) {
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	p.MoveTo(x+r, y)
	p.LineTo(x+w-r, y)
	p.Arc(x+w-r, y+r, r, -math.Pi/2, 0, vector.Clockwise)
	p.LineTo(x+w, y+h-r)
	p.Arc(x+w-r, y+h-r, r, 0, math.Pi/2, vector.Clockwise)
	p.LineTo(x+r, y+h)
	p.Arc(x+r, y+h-r, r, math.Pi/2, math.Pi, vector.Clockwise)
	p.LineTo(x, y+r)
	p.Arc(x+r, y+r, r, math.Pi, 3*math.Pi/2, vector.Clockwise)
	p.Close()
}

func fillRoundedRect(screen *ebiten.Image, x, y, w, h, r float32, opts *vector.DrawPathOptions) {
	var p vector.Path
	appendRoundedRectPath(&p, x, y, w, h, r)
	vector.FillPath(screen, &p, nil, opts)
}

func roundStrokeOpts(width float32) *vector.StrokeOptions {
	return &vector.StrokeOptions{
		Width:    width,
		LineCap:  vector.LineCapRound,
		LineJoin: vector.LineJoinRound,
	}
}

func drawVectorIcon(screen *ebiten.Image, cx, cy, size float32, icon ToastIcon, col, bgCol color.NRGBA) {
	hs := size / 2

	var drawOpts vector.DrawPathOptions
	drawOpts.AntiAlias = true
	drawOpts.ColorScale.ScaleWithColor(col)

	var bgOpts vector.DrawPathOptions
	bgOpts.AntiAlias = true
	bgOpts.ColorScale.ScaleWithColor(bgCol)

	switch icon {
	case IconPause:
		drawIconPause(screen, cx, cy, size, &drawOpts)
	case IconCheckmark:
		drawIconCheckmark(screen, cx, cy, hs, size, &drawOpts)
	case IconDownload:
		drawIconDownload(screen, cx, cy, hs, size, &drawOpts)
	case IconCamera:
		drawIconCamera(screen, cx, cy, size, col, bgCol, &drawOpts)
	case IconGamepad:
		drawIconGamepad(screen, cx, cy, size, bgCol, &drawOpts, &bgOpts)
	case IconMute:
		drawIconMute(screen, cx, cy, hs, size, &drawOpts)
	case IconUnmute:
		drawIconUnmute(screen, cx, cy, hs, size, &drawOpts)
	case IconFastForward:
		drawIconFastForward(screen, cx, cy, hs, &drawOpts)
	case IconReset:
		drawIconReset(screen, cx, cy, hs, size, &drawOpts)
	case IconError:
		drawIconError(screen, cx, cy, hs, size, &drawOpts)
	case IconWarning:
		drawIconWarning(screen, cx, cy, hs, size, bgCol, &drawOpts, &bgOpts)
	case IconSlot:
		drawIconSlot(screen, cx, cy, size, &drawOpts, &bgOpts)
	case IconDisplay:
		drawIconDisplay(screen, cx, cy, size, &drawOpts, &bgOpts)
	}
}

func drawIconPause(screen *ebiten.Image, cx, cy, size float32, drawOpts *vector.DrawPathOptions) {
	barW := size * 0.22
	barH := size * 0.76
	gap := size * 0.20
	offset := gap/2 + barW/2
	fillRoundedRect(screen, cx-offset-barW/2, cy-barH/2, barW, barH, barW/2, drawOpts)
	fillRoundedRect(screen, cx+offset-barW/2, cy-barH/2, barW, barH, barW/2, drawOpts)
}

func drawIconCheckmark(screen *ebiten.Image, cx, cy, hs, size float32, drawOpts *vector.DrawPathOptions) {
	var p vector.Path
	p.MoveTo(cx-hs*0.72, cy+hs*0.04)
	p.LineTo(cx-hs*0.18, cy+hs*0.58)
	p.LineTo(cx+hs*0.76, cy-hs*0.52)
	vector.StrokePath(screen, &p, roundStrokeOpts(size*0.19), drawOpts)
}

func drawIconDownload(screen *ebiten.Image, cx, cy, hs, size float32, drawOpts *vector.DrawPathOptions) {
	strokeW := size * 0.17
	var arrow vector.Path
	arrow.MoveTo(cx, cy-hs*0.74)
	arrow.LineTo(cx, cy+hs*0.32)
	arrow.MoveTo(cx-hs*0.46, cy-hs*0.10)
	arrow.LineTo(cx, cy+hs*0.36)
	arrow.LineTo(cx+hs*0.46, cy-hs*0.10)
	vector.StrokePath(screen, &arrow, roundStrokeOpts(strokeW), drawOpts)

	var tray vector.Path
	tray.MoveTo(cx-hs*0.74, cy+hs*0.38)
	tray.LineTo(cx-hs*0.74, cy+hs*0.72)
	tray.LineTo(cx+hs*0.74, cy+hs*0.72)
	tray.LineTo(cx+hs*0.74, cy+hs*0.38)
	vector.StrokePath(screen, &tray, roundStrokeOpts(strokeW*0.9), drawOpts)
}

func drawIconCamera(screen *ebiten.Image, cx, cy, size float32, col, bgCol color.NRGBA, drawOpts *vector.DrawPathOptions) {
	bodyW := size * 0.94
	bodyH := size * 0.64
	topW := size * 0.42
	topH := size * 0.18
	r := size * 0.15
	bodyY := cy - bodyH/2 + topH*0.35

	var p vector.Path
	p.MoveTo(cx-bodyW/2+r, bodyY)
	p.LineTo(cx-topW/2, bodyY)
	p.LineTo(cx-topW*0.32, bodyY-topH)
	p.LineTo(cx+topW*0.32, bodyY-topH)
	p.LineTo(cx+topW/2, bodyY)
	p.LineTo(cx+bodyW/2-r, bodyY)
	p.Arc(cx+bodyW/2-r, bodyY+r, r, -math.Pi/2, 0, vector.Clockwise)
	p.LineTo(cx+bodyW/2, bodyY+bodyH-r)
	p.Arc(cx+bodyW/2-r, bodyY+bodyH-r, r, 0, math.Pi/2, vector.Clockwise)
	p.LineTo(cx-bodyW/2+r, bodyY+bodyH)
	p.Arc(cx-bodyW/2+r, bodyY+bodyH-r, r, math.Pi/2, math.Pi, vector.Clockwise)
	p.LineTo(cx-bodyW/2, bodyY+r)
	p.Arc(cx-bodyW/2+r, bodyY+r, r, math.Pi, 3*math.Pi/2, vector.Clockwise)
	p.Close()
	vector.FillPath(screen, &p, nil, drawOpts)

	lensY := bodyY + bodyH*0.52
	vector.FillCircle(screen, cx, lensY, size*0.21, bgCol, true)
	vector.FillCircle(screen, cx, lensY, size*0.11, col, true)
}

func drawIconGamepad(screen *ebiten.Image, cx, cy, size float32, bgCol color.NRGBA, drawOpts, bgOpts *vector.DrawPathOptions) {
	padW := size * 0.98
	padH := size * 0.62
	fillRoundedRect(screen, cx-padW/2, cy-padH/2, padW, padH, padH*0.42, drawOpts)

	dpadX := cx - padW*0.23
	dpadArm := size * 0.26
	dpadThick := size * 0.10
	fillRoundedRect(screen, dpadX-dpadArm/2, cy-dpadThick/2, dpadArm, dpadThick, dpadThick*0.3, bgOpts)
	fillRoundedRect(screen, dpadX-dpadThick/2, cy-dpadArm/2, dpadThick, dpadArm, dpadThick*0.3, bgOpts)

	btnX := cx + padW*0.22
	bOffset := size * 0.08
	bR := size * 0.065
	vector.FillCircle(screen, btnX-bOffset, cy+bOffset*0.6, bR, bgCol, true)
	vector.FillCircle(screen, btnX+bOffset, cy-bOffset*0.6, bR, bgCol, true)
}

func drawIconSpeakerCone(screen *ebiten.Image, cx, cy, hs float32, drawOpts *vector.DrawPathOptions) {
	var cone vector.Path
	cone.MoveTo(cx-hs*0.82, cy-hs*0.26)
	cone.LineTo(cx-hs*0.42, cy-hs*0.26)
	cone.LineTo(cx+hs*0.02, cy-hs*0.66)
	cone.LineTo(cx+hs*0.02, cy+hs*0.66)
	cone.LineTo(cx-hs*0.42, cy+hs*0.26)
	cone.LineTo(cx-hs*0.82, cy+hs*0.26)
	cone.Close()
	vector.FillPath(screen, &cone, nil, drawOpts)
}

func drawIconMute(screen *ebiten.Image, cx, cy, hs, size float32, drawOpts *vector.DrawPathOptions) {
	drawIconSpeakerCone(screen, cx, cy, hs, drawOpts)

	// FontAwesome fa-volume-xmark style: clean X to the right of the speaker cone
	xCenter := cx + hs*0.48
	arm := hs * 0.28
	var xmark vector.Path
	xmark.MoveTo(xCenter-arm, cy-arm)
	xmark.LineTo(xCenter+arm, cy+arm)
	xmark.MoveTo(xCenter+arm, cy-arm)
	xmark.LineTo(xCenter-arm, cy+arm)
	vector.StrokePath(screen, &xmark, roundStrokeOpts(size*0.15), drawOpts)
}

func drawIconUnmute(screen *ebiten.Image, cx, cy, hs, size float32, drawOpts *vector.DrawPathOptions) {
	drawIconSpeakerCone(screen, cx, cy, hs, drawOpts)

	waveOpts := roundStrokeOpts(size * 0.14)
	var wave1 vector.Path
	wave1.Arc(cx+hs*0.04, cy, hs*0.38, -math.Pi/3.4, math.Pi/3.4, vector.Clockwise)
	vector.StrokePath(screen, &wave1, waveOpts, drawOpts)

	var wave2 vector.Path
	wave2.Arc(cx+hs*0.04, cy, hs*0.70, -math.Pi/3.2, math.Pi/3.2, vector.Clockwise)
	vector.StrokePath(screen, &wave2, waveOpts, drawOpts)
}

func drawIconFastForward(screen *ebiten.Image, cx, cy, hs float32, drawOpts *vector.DrawPathOptions) {
	triW := hs * 0.68
	triH := hs * 0.64
	var p vector.Path
	p.MoveTo(cx-hs*0.72, cy-triH)
	p.LineTo(cx-hs*0.72+triW, cy)
	p.LineTo(cx-hs*0.72, cy+triH)
	p.Close()
	p.MoveTo(cx-hs*0.02, cy-triH)
	p.LineTo(cx-hs*0.02+triW, cy)
	p.LineTo(cx-hs*0.02, cy+triH)
	p.Close()
	vector.FillPath(screen, &p, nil, drawOpts)
}

// drawIconReset renders a FontAwesome fa-rotate-right circular reload arrow.
func drawIconReset(screen *ebiten.Image, cx, cy, hs, size float32, drawOpts *vector.DrawPathOptions) {
	arcR := hs * 0.66
	strokeW := size * 0.16

	// Clockwise circular arc of ~285 degrees ending in the top-right quadrant
	const startAngle = math.Pi * 0.22 // ~40 degrees (bottom-right)
	const endAngle = math.Pi * 1.76   // ~317 degrees (-43 degrees, top-right)

	var arc vector.Path
	arc.Arc(cx, cy, arcR, startAngle, endAngle, vector.Clockwise)
	vector.StrokePath(screen, &arc, roundStrokeOpts(strokeW), drawOpts)

	// Orthonormal basis at endAngle: outward normal (nx, ny) and clockwise tangent (tx, ty)
	cosA := float32(math.Cos(endAngle))
	sinA := float32(math.Sin(endAngle))
	nx, ny := cosA, sinA
	tx, ty := -sinA, cosA

	tipX := cx + arcR*cosA
	tipY := cy + arcR*sinA
	arrowNose := hs * 0.36
	arrowBack := hs * 0.18
	arrowWing := hs * 0.36

	var head vector.Path
	head.MoveTo(tipX+tx*arrowNose, tipY+ty*arrowNose)
	head.LineTo(tipX-tx*arrowBack+nx*arrowWing, tipY-ty*arrowBack+ny*arrowWing)
	head.LineTo(tipX-tx*arrowBack-nx*arrowWing, tipY-ty*arrowBack-ny*arrowWing)
	head.Close()
	vector.FillPath(screen, &head, nil, drawOpts)
}

func drawIconError(screen *ebiten.Image, cx, cy, hs, size float32, drawOpts *vector.DrawPathOptions) {
	arm := hs * 0.56
	var p vector.Path
	p.MoveTo(cx-arm, cy-arm)
	p.LineTo(cx+arm, cy+arm)
	p.MoveTo(cx+arm, cy-arm)
	p.LineTo(cx-arm, cy+arm)
	vector.StrokePath(screen, &p, roundStrokeOpts(size*0.19), drawOpts)
}

func drawIconWarning(screen *ebiten.Image, cx, cy, hs, size float32, bgCol color.NRGBA, drawOpts, bgOpts *vector.DrawPathOptions) {
	var p vector.Path
	p.MoveTo(cx, cy-hs*0.76)
	p.LineTo(cx+hs*0.82, cy+hs*0.70)
	p.LineTo(cx-hs*0.82, cy+hs*0.70)
	p.Close()
	vector.FillPath(screen, &p, nil, drawOpts)
	vector.StrokePath(screen, &p, roundStrokeOpts(size*0.10), drawOpts)

	markW := size * 0.11
	markH := hs * 0.48
	fillRoundedRect(screen, cx-markW/2, cy-hs*0.28, markW, markH, markW/2, bgOpts)
	vector.FillCircle(screen, cx, cy+hs*0.44, size*0.065, bgCol, true)
}

func drawIconSlot(screen *ebiten.Image, cx, cy, size float32, drawOpts, bgOpts *vector.DrawPathOptions) {
	w := size * 0.74
	h := size * 0.88
	notch := size * 0.20
	r := size * 0.12

	var p vector.Path
	p.MoveTo(cx-w/2+r, cy-h/2)
	p.LineTo(cx+w/2-notch, cy-h/2)
	p.LineTo(cx+w/2, cy-h/2+notch)
	p.LineTo(cx+w/2, cy+h/2-r)
	p.Arc(cx+w/2-r, cy+h/2-r, r, 0, math.Pi/2, vector.Clockwise)
	p.LineTo(cx-w/2+r, cy+h/2)
	p.Arc(cx-w/2+r, cy+h/2-r, r, math.Pi/2, math.Pi, vector.Clockwise)
	p.LineTo(cx-w/2, cy-h/2+r)
	p.Arc(cx-w/2+r, cy-h/2+r, r, math.Pi, 3*math.Pi/2, vector.Clockwise)
	p.Close()
	vector.FillPath(screen, &p, nil, drawOpts)

	fillRoundedRect(screen, cx-w*0.30, cy-h*0.08, w*0.60, h*0.42, size*0.05, bgOpts)
	fillRoundedRect(screen, cx-w*0.26, cy-h*0.38, w*0.44, size*0.10, size*0.03, bgOpts)
}

func drawIconDisplay(screen *ebiten.Image, cx, cy, size float32, drawOpts, bgOpts *vector.DrawPathOptions) {
	dispW := size * 0.94
	dispH := size * 0.64
	topY := cy - dispH/2 - size*0.08
	fillRoundedRect(screen, cx-dispW/2, topY, dispW, dispH, size*0.13, drawOpts)

	inW := dispW * 0.80
	inH := dispH * 0.68
	fillRoundedRect(screen, cx-inW/2, topY+(dispH-inH)/2, inW, inH, size*0.05, bgOpts)

	stemW := size * 0.14
	stemH := size * 0.16
	fillRoundedRect(screen, cx-stemW/2, topY+dispH, stemW, stemH, size*0.02, drawOpts)

	standW := dispW * 0.46
	standH := size * 0.11
	fillRoundedRect(screen, cx-standW/2, topY+dispH+stemH-size*0.02, standW, standH, standH/2, drawOpts)
}
