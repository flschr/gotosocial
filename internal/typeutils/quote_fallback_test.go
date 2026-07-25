// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package typeutils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHideQuoteFallback(t *testing.T) {
	content := `<p class="quote-inline">RE: <a href="https://example.org/@alice/123">` +
		`https://example.org/@alice/123</a></p><p>Useful commentary.</p>`

	require.Equal(t, `<p>Useful commentary.</p>`, hideQuoteFallback(content))
}

func TestHideQuoteFallbackAllowsWhitespaceBeforeParagraph(t *testing.T) {
	content := "\n  " +
		`<p class="quote-inline">RE: <a href="https://example.org/@alice/123">` +
		`https://example.org/@alice/123</a></p><p>Useful commentary.</p>`

	require.Equal(t, `<p>Useful commentary.</p>`, hideQuoteFallback(content))
}

func TestHideQuoteFallbackRequiresHTTPLink(t *testing.T) {
	content := `<p class="quote-inline">RE: <a href="https://example.org/@alice/123">` +
		`https://example.org/@alice/123</a></p><p>Useful commentary.</p>`

	withoutLink := `<p class="quote-inline">A styled paragraph.</p><p>Useful commentary.</p>`

	require.Equal(t, `<p>Useful commentary.</p>`, hideQuoteFallback(content))
	require.Equal(t, withoutLink, hideQuoteFallback(withoutLink))
}

func TestHideQuoteFallbackPreservesOrdinaryContent(t *testing.T) {
	content := `<p>RE: <a href="https://example.org/@alice/123">` +
		`https://example.org/@alice/123</a></p><p>Useful commentary.</p>`

	require.Equal(t, content, hideQuoteFallback(content))
}
