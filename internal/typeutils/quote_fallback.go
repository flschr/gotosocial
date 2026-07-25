// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package typeutils

import (
	"bytes"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// hideQuoteFallback removes Mastodon's backward-compatibility quote-inline
// paragraph. The quote-inline class is Mastodon's semantic marker for this
// fallback; a preview card is not guaranteed even when clients can render the
// quoted status through Mastodon's quote API.
func hideQuoteFallback(content string) string {
	if content == "" {
		return content
	}

	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return content
	}

	body := findHTMLElement(doc, "body")
	if body == nil {
		return content
	}

	first := body.FirstChild
	for first != nil && first.Type == html.TextNode && strings.TrimSpace(first.Data) == "" {
		first = first.NextSibling
	}
	if first == nil ||
		first.Type != html.ElementNode ||
		first.Data != "p" ||
		!hasHTMLClass(first, "quote-inline") {
		return content
	}

	if firstHTTPLink(first) == nil {
		return content
	}

	body.RemoveChild(first)
	var rendered bytes.Buffer
	for child := body.FirstChild; child != nil; child = child.NextSibling {
		if err := html.Render(&rendered, child); err != nil {
			return content
		}
	}
	return rendered.String()
}

func findHTMLElement(node *html.Node, name string) *html.Node {
	if node.Type == html.ElementNode && node.Data == name {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findHTMLElement(child, name); found != nil {
			return found
		}
	}
	return nil
}

func hasHTMLClass(node *html.Node, class string) bool {
	for _, item := range strings.Fields(attr(node, "class")) {
		if item == class {
			return true
		}
	}
	return false
}

func firstHTTPLink(node *html.Node) *url.URL {
	if node.Type == html.ElementNode && node.Data == "a" {
		parsed, err := url.Parse(attr(node, "href"))
		if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" {
			return parsed
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := firstHTTPLink(child); found != nil {
			return found
		}
	}
	return nil
}
