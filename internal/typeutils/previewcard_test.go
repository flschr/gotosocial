// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package typeutils

import (
	"net/url"
	"testing"

	"code.superseriousbusiness.org/gotosocial/internal/api/model"
	"github.com/stretchr/testify/require"
)

func TestFirstPreviewCardURLSkipsMentionsAndTags(t *testing.T) {
	content := `<p><a class="mention" href="https://social.example/@alice">@alice</a> ` +
		`<a rel="tag" href="https://social.example/tags/test">#test</a> ` +
		`<a href="https://example.org/article">article</a></p>`

	found := firstPreviewCardURL(content)
	require.NotNil(t, found)
	require.Equal(t, "https://example.org/article", found.String())
}

func TestHideMatchingQuoteFallback(t *testing.T) {
	content := `<p class="quote-inline">RE: <a href="https://example.org/@alice/123">` +
		`https://example.org/@alice/123</a></p><p>Useful commentary.</p>`
	card := &model.Card{URL: "https://example.org/@alice/123"}

	require.Equal(
		t,
		`<p>Useful commentary.</p>`,
		hideMatchingQuoteFallback(content, card),
	)
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

func TestParsePreviewCard(t *testing.T) {
	target, err := url.Parse("https://example.org/articles/one")
	require.NoError(t, err)

	body := []byte(`<!doctype html><html><head>
		<title>Fallback title</title>
		<meta property="og:title" content="A useful article">
		<meta property="og:description" content="Description here">
		<meta property="og:site_name" content="Example">
		<meta property="og:image" content="/preview.jpg">
		<meta property="og:image:width" content="1200">
		<meta property="og:image:height" content="630">
	</head></html>`)

	card, err := parsePreviewCard(target, body)
	require.NoError(t, err)
	require.Equal(t, "https://example.org/articles/one", card.URL)
	require.Equal(t, "A useful article", card.Title)
	require.Equal(t, "Description here", card.Description)
	require.Equal(t, "Example", card.ProviderName)
	require.Equal(t, "https://example.org/preview.jpg", card.Image)
	require.Equal(t, 1200, card.Width)
	require.Equal(t, 630, card.Height)
}

func TestParsePreviewCardYouTubePlayer(t *testing.T) {
	target, err := url.Parse("https://youtu.be/Gg-SCpXba64")
	require.NoError(t, err)

	body := []byte(`<!doctype html><html><head>
		<meta property="og:title" content="A useful video">
		<meta property="og:image" content="https://i.ytimg.com/vi/Gg-SCpXba64/maxresdefault.jpg">
	</head></html>`)

	card, err := parsePreviewCard(target, body)
	require.NoError(t, err)
	require.Equal(t, "video", card.Type)
	require.Equal(t, 480, card.Width)
	require.Equal(t, 270, card.Height)
	require.Contains(t, card.HTML, `src="https://www.youtube-nocookie.com/embed/Gg-SCpXba64"`)
}

func TestYouTubeEmbedURLRejectsUntrustedAndInvalidURLs(t *testing.T) {
	untrusted, err := url.Parse("https://example.org/watch?v=Gg-SCpXba64")
	require.NoError(t, err)
	require.Empty(t, youtubeEmbedURL(untrusted))

	invalid, err := url.Parse("https://youtu.be/not-valid")
	require.NoError(t, err)
	require.Empty(t, youtubeEmbedURL(invalid))
}

func TestYouTubePreviewCardFromOEmbed(t *testing.T) {
	target, err := url.Parse("https://youtu.be/Gg-SCpXba64")
	require.NoError(t, err)

	card, err := youtubePreviewCardFromOEmbed(target, &youtubeOEmbed{
		Title:           "Danger Dan - Keine Angst",
		AuthorName:      "Danger Dan",
		AuthorURL:       "https://www.youtube.com/@DangerDan",
		ThumbnailURL:    "https://i.ytimg.com/vi/Gg-SCpXba64/hqdefault.jpg",
		ThumbnailWidth:  480,
		ThumbnailHeight: 360,
	})
	require.NoError(t, err)
	require.Equal(t, "video", card.Type)
	require.Equal(t, "Danger Dan - Keine Angst", card.Title)
	require.Equal(t, "https://www.youtube-nocookie.com/embed/Gg-SCpXba64", card.EmbedURL)
	require.Equal(t, "https://i.ytimg.com/vi/Gg-SCpXba64/hqdefault.jpg", card.Image)
}
