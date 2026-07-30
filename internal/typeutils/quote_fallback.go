// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package typeutils

import (
	"bytes"
	"net/url"
	"strings"

	"code.superseriousbusiness.org/gotosocial/internal/api/model"
	"golang.org/x/net/html"
)

// hideMatchingQuoteFallback removes Mastodon's exact backward-compatibility
// RE wrapper only when the API response also contains a card for the same URL.
// Keeping the wrapper while the card cache is cold lets clients derive and
// display the quote themselves instead of losing it.
func hideMatchingQuoteFallback(content string, card *model.Card) string {
	if card == nil {
		return content
	}

	cleaned, target, _ := extractQuoteFallback(content)
	if target == "" || target != card.URL {
		return content
	}

	return cleaned
}

// extractQuoteFallback identifies Mastodon's backward-compatibility RE
// paragraph and returns the content without it, its link target, and whether
// Mastodon supplied the semantic quote-inline marker.
func extractQuoteFallback(content string) (cleaned string, target string, marked bool) {
	if content == "" {
		return content, "", false
	}

	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return content, "", false
	}

	body := findHTMLElement(doc, "body")
	if body == nil {
		return content, "", false
	}

	first := body.FirstChild
	for first != nil && first.Type == html.TextNode && strings.TrimSpace(first.Data) == "" {
		first = first.NextSibling
	}
	if first == nil ||
		first.Type != html.ElementNode ||
		first.Data != "p" {
		return content, "", false
	}

	quoteURL := firstHTTPLink(first)
	if quoteURL == nil || strings.TrimSpace(elementText(first)) != "RE: "+quoteURL.String() {
		return content, "", false
	}

	body.RemoveChild(first)
	var rendered bytes.Buffer
	for child := body.FirstChild; child != nil; child = child.NextSibling {
		if err := html.Render(&rendered, child); err != nil {
			return content, "", false
		}
	}
	return rendered.String(), quoteURL.String(), hasHTMLClass(first, "quote-inline")
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

func elementText(node *html.Node) string {
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			text.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return text.String()
}
