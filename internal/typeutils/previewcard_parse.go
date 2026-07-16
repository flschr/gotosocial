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

	return &model.Card{
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
