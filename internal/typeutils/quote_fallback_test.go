// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package typeutils

import (
	"testing"

	"code.superseriousbusiness.org/gotosocial/internal/api/model"
	"github.com/stretchr/testify/require"
)

func TestHideMatchingMarkedQuoteFallback(t *testing.T) {
	content := `<p class="quote-inline">RE: <a href="https://example.org/@alice/123">` +
		`https://example.org/@alice/123</a></p><p>Useful commentary.</p>`
	card := &model.Card{URL: "https://example.org/@alice/123"}

	require.Equal(t, `<p>Useful commentary.</p>`, hideMatchingQuoteFallback(content, card))
}

func TestHideQuoteFallbackAllowsWhitespaceBeforeParagraph(t *testing.T) {
	content := "\n  " +
		`<p class="quote-inline">RE: <a href="https://example.org/@alice/123">` +
		`https://example.org/@alice/123</a></p><p>Useful commentary.</p>`

	cleaned, target, marked := extractQuoteFallback(content)
	require.Equal(t, `<p>Useful commentary.</p>`, cleaned)
	require.Equal(t, "https://example.org/@alice/123", target)
	require.True(t, marked)
}

func TestExtractUnmarkedMastodonQuoteFallback(t *testing.T) {
	content := `<p>RE: <a href="https://social.lol/@z428/116980321253943752">` +
		`<span class="invisible">https://</span>` +
		`<span class="ellipsis">social.lol/@z428/1169803212539</span>` +
		`<span class="invisible">43752</span></a></p>` +
		`<p>Nature just never ceases to amaze me.</p>`

	cleaned, target, marked := extractQuoteFallback(content)
	require.Equal(t, `<p>Nature just never ceases to amaze me.</p>`, cleaned)
	require.Equal(t, "https://social.lol/@z428/116980321253943752", target)
	require.False(t, marked)
}

func TestHideMatchingUnmarkedMastodonQuoteFallback(t *testing.T) {
	content := `<p>RE: <a href="https://social.lol/@z428/116980321253943752">` +
		`<span class="invisible">https://</span>` +
		`<span class="ellipsis">social.lol/@z428/1169803212539</span>` +
		`<span class="invisible">43752</span></a></p>` +
		`<p>Nature just never ceases to amaze me.</p>`
	card := &model.Card{URL: "https://social.lol/@z428/116980321253943752"}

	require.Equal(t, `<p>Nature just never ceases to amaze me.</p>`, hideMatchingQuoteFallback(content, card))
}

func TestHideMatchingQuoteFallbackRequiresCard(t *testing.T) {
	content := `<p>RE: <a href="https://example.org/@alice/123">` +
		`https://example.org/@alice/123</a></p><p>Useful commentary.</p>`

	require.Equal(t, content, hideMatchingQuoteFallback(content, nil))
}

func TestHideMatchingQuoteFallbackRequiresMatchingCardURL(t *testing.T) {
	content := `<p>RE: <a href="https://example.org/@alice/123">` +
		`https://example.org/@alice/123</a></p><p>Useful commentary.</p>`
	card := &model.Card{URL: "https://example.org/@bob/456"}

	require.Equal(t, content, hideMatchingQuoteFallback(content, card))
}

func TestHideQuoteFallbackRequiresHTTPLink(t *testing.T) {
	content := `<p class="quote-inline">A styled paragraph.</p><p>Useful commentary.</p>`

	cleaned, target, marked := extractQuoteFallback(content)
	require.Equal(t, content, cleaned)
	require.Empty(t, target)
	require.False(t, marked)
}

func TestHideQuoteFallbackPreservesOrdinaryContent(t *testing.T) {
	content := `<p>RE: <a href="https://example.org/@alice/123">` +
		`https://example.org/@alice/123</a> — my own context.</p><p>Useful commentary.</p>`
	card := &model.Card{URL: "https://example.org/@alice/123"}

	require.Equal(t, content, hideMatchingQuoteFallback(content, card))
}
