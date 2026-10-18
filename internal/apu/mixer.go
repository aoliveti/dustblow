// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package apu

const (
	maxPulseMixIndex    = 30
	maxTriangleMixIndex = 15
	maxNoiseMixIndex    = 15
	maxDMCMixIndex      = 127

	pulseTableSize    = 31
	triangleTableSize = 16
	noiseTableSize    = 16
	dmcTableSize      = 128

	// Nesdev APU non-linear mixing approximation coefficients
	pulseWeightNumerator   float32 = 95.88
	pulseWeightDenominator float32 = 8128.0
	pulseWeightOffset      float32 = 100.0

	tndWeightNumerator float32 = 159.79
	tndTriangleDivider float32 = 8227.0
	tndNoiseDivider    float32 = 12241.0
	tndDMCDivider      float32 = 22638.0
	tndWeightOffset    float32 = 100.0
)

var (
	pulseTable [pulseTableSize]float32
	tndTable   [triangleTableSize][noiseTableSize][dmcTableSize]float32
)

func init() {
	// Pre-calculate pulse non-linear mixing table according to Nesdev formula
	for i := 1; i < pulseTableSize; i++ {
		pulseTable[i] = pulseWeightNumerator / ((pulseWeightDenominator / float32(i)) + pulseWeightOffset)
	}

	// Pre-calculate triangle, noise, and DMC non-linear mixing table
	for tri := range triangleTableSize {
		for noise := range noiseTableSize {
			for dmc := range dmcTableSize {
				if tri == 0 && noise == 0 && dmc == 0 {
					continue
				}
				term := (float32(tri) / tndTriangleDivider) + (float32(noise) / tndNoiseDivider) + (float32(dmc) / tndDMCDivider)
				tndTable[tri][noise][dmc] = tndWeightNumerator / ((1.0 / term) + tndWeightOffset)
			}
		}
	}
}

// MixChannels computes the non-linear analog sum of all 5 channels.
func MixChannels(p1, p2, tri, noise, dmc byte) float32 {
	pulseSum := min(p1+p2, maxPulseMixIndex)
	tri = min(tri, maxTriangleMixIndex)
	noise = min(noise, maxNoiseMixIndex)
	dmc = min(dmc, maxDMCMixIndex)

	return pulseTable[pulseSum] + tndTable[tri][noise][dmc]
}
