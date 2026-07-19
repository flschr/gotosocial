// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"testing"

	"code.superseriousbusiness.org/gotosocial/internal/config"
)

func TestDefaultThumbnailSize(t *testing.T) {
	const expectedMax = 1024
	if actual := config.Defaults.Media.ThumbMaxPixels; actual != expectedMax {
		t.Fatalf("default thumbnail maximum = %d, want %d", actual, expectedMax)
	}

	width, height := thumbSize(
		config.Defaults.Media.ThumbMaxPixels,
		4000,
		3000,
		4.0/3.0,
	)
	if width != 1024 || height != 768 {
		t.Fatalf("thumbnail dimensions = %dx%d, want 1024x768", width, height)
	}
}
