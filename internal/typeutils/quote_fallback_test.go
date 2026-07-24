// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package typeutils

import (
	"testing"

	"code.superseriousbusiness.org/gotosocial/internal/api/model"
	"github.com/stretchr/testify/require"
)

func TestHideMatchingQuoteFallback(t *testing.T) {
	content := `<p class="quote-inline">RE: <a href="https://example.org/@alice/123">` +
		`https://example.org/@alice/123</a></p><p>Useful commentary.</p>`
	card := &model.Card{URL: "https://example.org/@alice/123"}

	require.Equal(t, `<p>Useful commentary.</p>`, hideMatchingQuoteFallback(content, card))
}

func TestHideMatchingQuoteFallbackRequiresCard(t *testing.T) {
	content := `<p class="quote-inline">RE: <a href="https://example.org/@alice/123">` +
		`https://example.org/@alice/123</a></p><p>Useful commentary.</p>`

	require.Equal(t, content, hideMatchingQuoteFallback(content, nil))
}

func TestHideMatchingQuoteFallbackRequiresMatchingCardURL(t *testing.T) {
	content := `<p class="quote-inline">RE: <a href="https://example.org/@alice/123">` +
		`https://example.org/@alice/123</a></p><p>Useful commentary.</p>`
	card := &model.Card{URL: "https://example.org/@bob/456"}

	require.Equal(t, content, hideMatchingQuoteFallback(content, card))
}

func TestHideMatchingQuoteFallbackPreservesOrdinaryContent(t *testing.T) {
	content := `<p>RE: <a href="https://example.org/@alice/123">` +
		`https://example.org/@alice/123</a></p><p>Useful commentary.</p>`
	card := &model.Card{URL: "https://example.org/@alice/123"}

	require.Equal(t, content, hideMatchingQuoteFallback(content, card))
}
