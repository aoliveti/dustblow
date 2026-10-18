// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
)

const sharpBilinearSrc = `//kage:unit pixels
package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	texel := floor(srcPos)
	sub := srcPos - texel
	w := fwidth(srcPos)
	sub = clamp((sub - 0.5 + 0.5 * w) / w, 0.0, 1.0)
	return imageSrc0At(texel + sub)
}
`

const crtScanlinesSrc = `//kage:unit pixels
package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	texel := floor(srcPos)
	sub := srcPos - texel
	w := fwidth(srcPos)
	sub = clamp((sub - 0.5 + 0.5 * w) / w, 0.0, 1.0)
	c := imageSrc0At(texel + sub)

	// Horizontal scanline factor aligned to the 240 scanlines
	scan := 0.85 + 0.15 * sin(srcPos.y * 3.14159265 * 2.0)

	// Subtle RGB phosphor aperture grille pattern
	mask := vec3(0.95, 0.95, 1.05)
	idx := int(mod(dstPos.x, 3.0))
	if idx == 0 {
		mask = vec3(1.05, 0.95, 0.95)
	}
	if idx == 1 {
		mask = vec3(0.95, 1.05, 0.95)
	}

	rgb := c.rgb * scan * mask
	return vec4(rgb, c.a)
}
`

func compileShaders() (sharpShader, crtShader *ebiten.Shader, err error) {
	sharp, err := ebiten.NewShader([]byte(sharpBilinearSrc))
	if err != nil {
		return nil, nil, fmt.Errorf("compile sharp bilinear shader: %w", err)
	}

	crt, err := ebiten.NewShader([]byte(crtScanlinesSrc))
	if err != nil {
		return nil, nil, fmt.Errorf("compile CRT scanlines shader: %w", err)
	}

	return sharp, crt, nil
}
