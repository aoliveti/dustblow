// Copyright (c) 2026 Andrea Oliveti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		args               []string
		expectedROM        string
		expectedFullscreen bool
		expectErr          bool
	}{
		{
			name:      "Empty args returns error",
			args:      nil,
			expectErr: true,
		},
		{
			name:      "Only flag without ROM returns error",
			args:      []string{"-f"},
			expectErr: true,
		},
		{
			name:      "Help flag returns usage error",
			args:      []string{"--help"},
			expectErr: true,
		},
		{
			name:      "Invalid flag returns error",
			args:      []string{"--invalid-flag", "game.nes"},
			expectErr: true,
		},
		{
			name:               "Valid ROM path argument",
			args:               []string{"games/super_mario.nes"},
			expectedROM:        "games/super_mario.nes",
			expectedFullscreen: false,
			expectErr:          false,
		},
		{
			name:               "Shorthand -f flag",
			args:               []string{"-f", "games/super_mario.nes"},
			expectedROM:        "games/super_mario.nes",
			expectedFullscreen: true,
			expectErr:          false,
		},
		{
			name:               "Longhand --fullscreen flag",
			args:               []string{"--fullscreen", "games/super_mario.nes"},
			expectedROM:        "games/super_mario.nes",
			expectedFullscreen: true,
			expectErr:          false,
		},
		{
			name:               "Explicit false --fullscreen=false flag",
			args:               []string{"--fullscreen=false", "games/super_mario.nes"},
			expectedROM:        "games/super_mario.nes",
			expectedFullscreen: false,
			expectErr:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := Parse(tt.args)
			if tt.expectErr {
				require.Error(t, err)
				assert.Nil(t, cfg)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cfg)
			assert.Equal(t, tt.expectedROM, cfg.ROMPath)
			assert.Equal(t, tt.expectedFullscreen, cfg.Fullscreen)
		})
	}
}
