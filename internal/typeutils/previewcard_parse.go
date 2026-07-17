// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package typeutils

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"code.superseriousbusiness.org/gotosocial/internal/api/model"
	"golang.org/x/net/html"
)

type youtubeOEmbed struct {
	Title           string `json:"title"`
	AuthorName      string `json:"author_name"`
	AuthorURL       string `json:"author_url"`
	ThumbnailURL    string `json:"thumbnail_url"`
	ThumbnailWidth  int    `json:"thumbnail_width"`
	ThumbnailHeight int    `json:"thumbnail_height"`
}

func parsePreviewCard(target *url.URL, body []byte) (*model.Card, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	meta := make(map[string]string)
	var title string
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch node.Data {
			case "title":
				if title == "" && node.FirstChild != nil {
					title = strings.TrimSpace(node.FirstChild.Data)
				}
			case "meta":
				key := strings.ToLower(attr(node, "property"))
				if key == "" {
					key = strings.ToLower(attr(node, "name"))
				}
				if key != "" && meta[key] == "" {
					meta[key] = strings.TrimSpace(attr(node, "content"))
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(doc)

	title = firstNonEmpty(meta["og:title"], meta["twitter:title"], title)
	if title == "" {
		return nil, fmt.Errorf("preview HTML has no title")
	}

	cardURL := target.String()
	if resolved := resolveOptionalPreviewURL(target, meta["og:url"]); resolved != "" {
		cardURL = resolved
	}
	image := resolveOptionalPreviewURL(target, firstNonEmpty(meta["og:image:secure_url"], meta["og:image"], meta["twitter:image"]))
	providerURL := &url.URL{Scheme: target.Scheme, Host: target.Host}

	card := &model.Card{
		URL:          cardURL,
		Title:        truncatePreviewText(title),
		Description:  truncatePreviewText(firstNonEmpty(meta["og:description"], meta["twitter:description"], meta["description"])),
		Type:         "link",
		AuthorName:   truncatePreviewText(firstNonEmpty(meta["article:author"], meta["author"])),
		ProviderName: truncatePreviewText(firstNonEmpty(meta["og:site_name"], target.Hostname())),
		ProviderURL:  providerURL.String(),
		Width:        parsePreviewDimension(meta["og:image:width"]),
		Height:       parsePreviewDimension(meta["og:image:height"]),
		Image:        image,
	}

	if embedURL := youtubeEmbedURL(target); embedURL != "" {
		card.Type = "video"
		card.Width = 480
		card.Height = 270
		card.EmbedURL = embedURL
		card.HTML = fmt.Sprintf(
			`<iframe src="%s" width="480" height="270" frameborder="0" allow="accelerometer; autoplay; encrypted-media; picture-in-picture" allowfullscreen></iframe>`,
			embedURL,
		)
	}

	return card, nil
}

func youtubeEmbedURL(target *url.URL) string {
	host := strings.ToLower(target.Hostname())
	var videoID string

	switch host {
	case "youtu.be":
		videoID = strings.TrimPrefix(target.EscapedPath(), "/")
	case "youtube.com", "www.youtube.com", "m.youtube.com", "music.youtube.com", "youtube-nocookie.com", "www.youtube-nocookie.com":
		switch {
		case target.Path == "/watch":
			videoID = target.Query().Get("v")
		case strings.HasPrefix(target.Path, "/shorts/"):
			videoID = strings.TrimPrefix(target.Path, "/shorts/")
		case strings.HasPrefix(target.Path, "/embed/"):
			videoID = strings.TrimPrefix(target.Path, "/embed/")
		}
	}

	if slash := strings.IndexByte(videoID, '/'); slash >= 0 {
		videoID = videoID[:slash]
	}
	if !validYouTubeVideoID(videoID) {
		return ""
	}
	return "https://www.youtube-nocookie.com/embed/" + videoID
}

func validYouTubeVideoID(value string) bool {
	if len(value) != 11 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func youtubePreviewCardFromOEmbed(target *url.URL, metadata *youtubeOEmbed) (*model.Card, error) {
	if metadata.Title == "" {
		return nil, fmt.Errorf("YouTube oEmbed response has no title")
	}
	embedURL := youtubeEmbedURL(target)
	if embedURL == "" {
		return nil, fmt.Errorf("invalid YouTube video URL")
	}

	return &model.Card{
		URL:          target.String(),
		Title:        truncatePreviewText(metadata.Title),
		Type:         "video",
		AuthorName:   truncatePreviewText(metadata.AuthorName),
		AuthorURL:    resolveOptionalPreviewURL(target, metadata.AuthorURL),
		ProviderName: "YouTube",
		ProviderURL:  "https://www.youtube.com/",
		HTML: fmt.Sprintf(
			`<iframe src="%s" width="480" height="270" frameborder="0" allow="accelerometer; autoplay; encrypted-media; picture-in-picture" allowfullscreen></iframe>`,
			embedURL,
		),
		Width:    480,
		Height:   270,
		Image:    resolveOptionalPreviewURL(target, metadata.ThumbnailURL),
		EmbedURL: embedURL,
	}, nil
}

func resolvePreviewURL(base *url.URL, value string) string {
	if value == "" {
		return base.String()
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return base.String()
	}
	resolved := base.ResolveReference(parsed)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return ""
	}
	return resolved.String()
}

func resolveOptionalPreviewURL(base *url.URL, value string) string {
	if value == "" {
		return ""
	}
	return resolvePreviewURL(base, value)
}

func parsePreviewDimension(value string) int {
	dimension, err := strconv.Atoi(value)
	if err != nil || dimension < 0 || dimension > 100000 {
		return 0
	}
	return dimension
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func truncatePreviewText(value string) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) <= previewCardMaxText {
		return value
	}
	return string([]rune(value)[:previewCardMaxText-1]) + "…"
}
