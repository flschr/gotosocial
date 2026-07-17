// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package typeutils

import (
	"net/url"
	"testing"

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
