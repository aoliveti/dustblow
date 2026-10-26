// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"image"

	"github.com/aoliveti/dustblow/internal/view"
	"github.com/hajimehoshi/ebiten/v2"
)

const (
	nesSourceWidth     = 256
	nesSourceHeight    = 240
	nesOverscanMarginX = 8
	nesOverscanMarginY = 8
	nesActiveWidth     = nesSourceWidth - (nesOverscanMarginX * 2)
	nesActiveHeight    = nesSourceHeight - (nesOverscanMarginY * 2)

	aspect4x3Width     = 4
	aspect4x3Height    = 3
	aspectSquareCropW  = 15
	aspectSquareCropH  = 14
)

// Pipeline manages off-screen rendering, aspect ratio correction, and shader application.
type Pipeline struct {
	nesImage     *ebiten.Image
	activeImage  *ebiten.Image
	sharpShader  *ebiten.Shader
	crtShader    *ebiten.Shader
	activeMode   view.Mode
	drawOptions  ebiten.DrawRectShaderOptions
	imageOptions ebiten.DrawImageOptions
	lastFrameSeq uint64
}

// NewPipeline initializes shader programs and the off-screen NES texture.
func NewPipeline() (*Pipeline, error) {
	sharp, crt, err := compileShaders()
	if err != nil {
		return nil, fmt.Errorf("initialize view pipeline: %w", err)
	}

	nesImg := ebiten.NewImage(nesSourceWidth, nesSourceHeight)
	activeImg := nesImg.SubImage(image.Rect(
		nesOverscanMarginX,
		nesOverscanMarginY,
		nesSourceWidth-nesOverscanMarginX,
		nesSourceHeight-nesOverscanMarginY,
	)).(*ebiten.Image)
	warmupShaders(sharp, crt, nesImg)

	return &Pipeline{
		nesImage:    nesImg,
		activeImage: activeImg,
		sharpShader: sharp,
		crtShader:   crt,
		activeMode:  view.ModeSharpBilinear,
	}, nil
}

func warmupShaders(sharp, crt *ebiten.Shader, nesImage *ebiten.Image) {
	dummy := ebiten.NewImage(nesSourceWidth, nesSourceHeight)
	defer dummy.Deallocate()

	var opts ebiten.DrawRectShaderOptions
	opts.Images[0] = nesImage
	dummy.DrawRectShader(nesSourceWidth, nesSourceHeight, sharp, &opts)
	dummy.DrawRectShader(nesSourceWidth, nesSourceHeight, crt, &opts)
}

// Mode returns the currently active shader mode.
func (p *Pipeline) Mode() view.Mode {
	return p.activeMode
}

// SetMode updates the active shader mode.
func (p *Pipeline) SetMode(mode view.Mode) {
	if mode >= 0 && mode < view.ModeCount {
		p.activeMode = mode
	}
}

// CycleMode advances to the next available shader mode and returns it.
func (p *Pipeline) CycleMode() view.Mode {
	p.activeMode = p.activeMode.Next()
	return p.activeMode
}

// RenderGame draws the NES frame buffer onto screen applying aspect ratio and the active shader.
// Any subsequent drawing onto screen (such as HUD or menus) remains completely unshaded and crisp.
func (p *Pipeline) RenderGame(screen *ebiten.Image, ppuBuffer []byte, frameSeq uint64) view.Viewport {
	if len(ppuBuffer) > 0 && (frameSeq != p.lastFrameSeq || p.lastFrameSeq == 0) {
		p.nesImage.WritePixels(ppuBuffer)
		p.lastFrameSeq = frameSeq
	}

	if screen == nil {
		return view.Viewport{}
	}

	winW := screen.Bounds().Dx()
	winH := screen.Bounds().Dy()

	aspectW, aspectH := aspect4x3Width, aspect4x3Height
	if p.activeMode == view.ModePixelPerfect {
		aspectW, aspectH = aspectSquareCropW, aspectSquareCropH
	}
	vp := view.CalculateViewport(winW, winH, aspectW, aspectH)

	if vp.Width <= 0 || vp.Height <= 0 {
		return vp
	}

	srcImg := p.activeImage
	srcWidth := nesActiveWidth
	srcHeight := nesActiveHeight

	scaleX := float64(vp.Width) / float64(srcWidth)
	scaleY := float64(vp.Height) / float64(srcHeight)

	switch p.activeMode {
	case view.ModeSharpBilinear:
		p.drawShader(screen, p.sharpShader, srcImg, vp, srcWidth, srcHeight, scaleX, scaleY)
	case view.ModeCRT:
		p.drawShader(screen, p.crtShader, srcImg, vp, srcWidth, srcHeight, scaleX, scaleY)
	case view.ModePixelPerfect:
		p.drawPixelPerfect(screen, srcImg, vp, scaleX, scaleY)
	}

	return vp
}

func (p *Pipeline) drawShader(screen *ebiten.Image, shader *ebiten.Shader, src *ebiten.Image, vp view.Viewport, srcW, srcH int, scaleX, scaleY float64) {
	p.drawOptions.GeoM.Reset()
	p.drawOptions.GeoM.Scale(scaleX, scaleY)
	p.drawOptions.GeoM.Translate(float64(vp.X), float64(vp.Y))
	p.drawOptions.Images[0] = src
	screen.DrawRectShader(srcW, srcH, shader, &p.drawOptions)
}

func (p *Pipeline) drawPixelPerfect(screen, src *ebiten.Image, vp view.Viewport, scaleX, scaleY float64) {
	bounds := src.Bounds()
	p.imageOptions.GeoM.Reset()
	p.imageOptions.GeoM.Translate(-float64(bounds.Min.X), -float64(bounds.Min.Y))
	p.imageOptions.GeoM.Scale(scaleX, scaleY)
	p.imageOptions.GeoM.Translate(float64(vp.X), float64(vp.Y))
	p.imageOptions.Filter = ebiten.FilterNearest
	screen.DrawImage(src, &p.imageOptions)
}
