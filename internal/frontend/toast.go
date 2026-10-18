// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"bytes"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/gomedium"
)

const (
	toastTotalFrames   = 112 // ~1.85 seconds at 60 FPS
	toastFadeInFrames  = 12
	toastFadeOutFrames = 22

	toastFontSize     = 14.0
	toastMarginTop    = 18
	toastHeight       = 44
	toastRadius       = 22
	toastPaddingLeft  = 9
	toastPaddingRight = 20
	toastIconBadgeD   = 30
	toastDotGap       = 12
	toastSlideInPx    = 10.0
	toastSlideOutPx   = 6.0
	borderWidth       = 1.0
)

// Toast coordinates modern, non-intrusive on-screen status notifications.
type Toast struct {
	source    *text.GoTextFaceSource
	face      text.Face
	lastScale float64
	message   string
	icon      ToastIcon
	frames    int
	tick      uint64
	sticky    bool
}

// NewToast creates and initializes a ready-to-use Toast renderer with a modern TrueType vector font.
func NewToast() *Toast {
	source, err := text.NewGoTextFaceSource(bytes.NewReader(gomedium.TTF))
	if err != nil {
		return &Toast{
			face:      text.NewGoXFace(basicfont.Face7x13),
			lastScale: defaultDisplayScale,
		}
	}
	s := displayScale()
	return &Toast{
		source: source,
		face: &text.GoTextFace{
			Source: source,
			Size:   toastFontSize * s,
		},
		lastScale: s,
	}
}

func (t *Toast) updateFaceScale(scale float64) {
	if t.source == nil || scale == t.lastScale {
		return
	}
	t.face = &text.GoTextFace{
		Source: t.source,
		Size:   toastFontSize * scale,
	}
	t.lastScale = scale
}

// Show schedules a new transient on-screen toast message with the specified semantic icon.
func (t *Toast) Show(msg string, icon ToastIcon) {
	if t == nil {
		return
	}
	t.message = msg
	t.icon = icon
	t.frames = toastTotalFrames
	t.sticky = false
}

// ShowSticky displays a persistent on-screen toast that stays visible until dismissed or superseded.
func (t *Toast) ShowSticky(msg string, icon ToastIcon) {
	if t == nil {
		return
	}
	t.message = msg
	t.icon = icon
	t.frames = toastTotalFrames
	t.sticky = true
	t.tick = 0
}

// Dismiss immediately clears any active or persistent toast.
func (t *Toast) Dismiss() {
	if t == nil {
		return
	}
	t.sticky = false
	t.frames = 0
}

// IsSticky reports whether the active toast is currently set to persistent mode.
func (t *Toast) IsSticky() bool {
	if t == nil {
		return false
	}
	return t.sticky
}

// Message returns the currently set toast notification message text.
func (t *Toast) Message() string {
	if t == nil {
		return ""
	}
	return t.message
}

// Update advances the life duration or animation phase of the active toast.
func (t *Toast) Update() {
	if t == nil {
		return
	}
	t.tick++
	if t.sticky || t.frames <= 0 {
		return
	}
	t.frames--
}

// Draw renders the sleek glassmorphic dark pill centered at the top of the screen.
func (t *Toast) Draw(screen *ebiten.Image) {
	if t == nil || t.frames <= 0 || t.message == "" || screen == nil {
		return
	}

	s := displayScale()
	t.updateFaceScale(s)
	sf := float32(s)

	alpha, offsetY := t.calculateAnimation(sf)
	if alpha <= 0.01 {
		return
	}

	bounds := screen.Bounds()
	maxPillW := float32(bounds.Dx()) - 48*sf
	displayMsg := t.message
	textW, textH := text.Measure(displayMsg, t.face, 0)
	padLeft := float32(toastPaddingLeft) * sf
	padRight := float32(toastPaddingRight) * sf
	badgeD := float32(toastIconBadgeD) * sf
	dotGap := float32(toastDotGap) * sf
	pillH := float32(toastHeight) * sf
	pillW := padLeft + badgeD + dotGap + float32(textW) + padRight

	if maxPillW > 0 && pillW > maxPillW {
		for len(displayMsg) > 4 && pillW > maxPillW {
			displayMsg = displayMsg[:len(displayMsg)-4] + "..."
			textW, textH = text.Measure(displayMsg, t.face, 0)
			pillW = padLeft + badgeD + dotGap + float32(textW) + padRight
		}
	}

	x := (float32(bounds.Dx()) - pillW) / 2
	y := float32(toastMarginTop)*sf + offsetY

	t.drawBackgroundPill(screen, x, y, pillW, pillH, sf, alpha)
	t.drawStatusIcon(screen, x, y, badgeD, pillH, padLeft, sf, alpha)
	t.drawMessageText(screen, x, y, pillH, float32(textH), badgeD, dotGap, padLeft, alpha, displayMsg)
}

func cubicEaseOut(p float32) float32 {
	inv := 1.0 - p
	return 1.0 - inv*inv*inv
}

func (t *Toast) calculateAnimation(sf float32) (alpha, offsetY float32) {
	if t.sticky {
		if t.tick < toastFadeInFrames {
			p := cubicEaseOut(float32(t.tick) / float32(toastFadeInFrames))
			return p, -(1.0 - p) * toastSlideInPx * sf
		}
		return 1.0, 0
	}
	elapsed := toastTotalFrames - t.frames
	if elapsed < toastFadeInFrames {
		p := cubicEaseOut(float32(elapsed) / float32(toastFadeInFrames))
		return p, -(1.0 - p) * toastSlideInPx * sf
	}
	if t.frames <= toastFadeOutFrames {
		p := cubicEaseOut(float32(t.frames) / float32(toastFadeOutFrames))
		return p, -(1.0 - p) * toastSlideOutPx * sf
	}
	return 1.0, 0
}

func pillBackgroundColor(alpha float32) color.NRGBA {
	return color.NRGBA{
		R: 18,
		G: 20,
		B: 28,
		A: uint8(236 * alpha),
	}
}

func drawDropShadow(screen *ebiten.Image, x, y, w, h, r, sf, alpha float32) {
	layers := [...]struct {
		expand  float32
		offsetY float32
		alpha   uint8
	}{
		{expand: 4.5, offsetY: 4.0, alpha: uint8(24 * alpha)},
		{expand: 2.5, offsetY: 2.5, alpha: uint8(45 * alpha)},
		{expand: 1.0, offsetY: 1.5, alpha: uint8(70 * alpha)},
	}
	for _, l := range layers {
		exp := l.expand * sf
		dy := l.offsetY * sf
		var opts vector.DrawPathOptions
		opts.AntiAlias = true
		opts.ColorScale.ScaleWithColor(color.NRGBA{R: 0, G: 0, B: 0, A: l.alpha})
		fillRoundedRect(screen, x-exp, y+dy-exp*0.4, w+exp*2, h+exp*2, r+exp, &opts)
	}
}

func (t *Toast) drawBackgroundPill(screen *ebiten.Image, x, y, w, h, sf, alpha float32) {
	r := float32(toastRadius) * sf
	drawDropShadow(screen, x, y, w, h, r, sf, alpha)

	var pillPath vector.Path
	appendRoundedRectPath(&pillPath, x, y, w, h, r)

	var fillOpts vector.DrawPathOptions
	fillOpts.AntiAlias = true
	fillOpts.ColorScale.ScaleWithColor(pillBackgroundColor(alpha))
	vector.FillPath(screen, &pillPath, nil, &fillOpts)

	// Outer dark contour for contrast on bright backgrounds
	var outerStroke vector.DrawPathOptions
	outerStroke.AntiAlias = true
	outerStroke.ColorScale.ScaleWithColor(color.NRGBA{R: 0, G: 0, B: 0, A: uint8(135 * alpha)})
	vector.StrokePath(screen, &pillPath, &vector.StrokeOptions{Width: borderWidth * 1.4 * sf}, &outerStroke)

	// Inner glassmorphic specular border
	inset := 0.8 * sf
	var innerPath vector.Path
	appendRoundedRectPath(&innerPath, x+inset, y+inset, w-inset*2, h-inset*2, r-inset)
	var innerStroke vector.DrawPathOptions
	innerStroke.AntiAlias = true
	innerStroke.ColorScale.ScaleWithColor(color.NRGBA{R: 255, G: 255, B: 255, A: uint8(48 * alpha)})
	vector.StrokePath(screen, &innerPath, &vector.StrokeOptions{Width: borderWidth * sf}, &innerStroke)
}

func (t *Toast) drawStatusIcon(screen *ebiten.Image, x, y, badgeD, pillH, padLeft, sf, alpha float32) {
	centerX := x + padLeft + badgeD/2
	centerY := y + pillH/2

	// Subtle circular glass well behind the monochrome icon
	badgeBg := color.NRGBA{R: 255, G: 255, B: 255, A: uint8(22 * alpha)}
	vector.FillCircle(screen, centerX, centerY, badgeD/2, badgeBg, true)

	iconSize := float32(iconBaseSize) * sf * 0.88
	col := iconColor(t.icon, alpha)
	bgCol := pillBackgroundColor(alpha)
	drawVectorIcon(screen, centerX, centerY, iconSize, t.icon, col, bgCol)
}

func (t *Toast) drawMessageText(screen *ebiten.Image, x, y, pillH, textH, badgeD, dotGap, padLeft, alpha float32, msg string) {
	textX := float64(x + padLeft + badgeD + dotGap)
	textY := float64(y + (pillH-textH)/2)

	txtCol := color.NRGBA{
		R: 245,
		G: 247,
		B: 252,
		A: uint8(250 * alpha),
	}

	op := &text.DrawOptions{}
	op.GeoM.Translate(textX, textY)
	op.ColorScale.ScaleWithColor(txtCol)
	text.Draw(screen, msg, t.face, op)
}
