// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"strings"
	"unicode"

	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/text"
	"github.com/rivo/uniseg"
	"golang.org/x/net/html"
)

func blueskyText(status *gtsmodel.Status) (string, []facet) {
	return blueskyTextForStatus(status, false)
}

func blueskyTextForStatus(status *gtsmodel.Status, stripSystemMention bool) (string, []facet) {
	prefix := ""
	if warning := strings.TrimSpace(text.ParseHTMLToPlain(status.ContentWarning)); warning != "" {
		prefix = "CW: " + warning + "\n\n"
	}
	body, facets := htmlTextAndFacets(status.Content)
	body, facets = trimTextAndFacets(body, facets)
	if stripSystemMention && status.InReplyToAccount != nil {
		body, facets = stripLeadingAccountMention(body, facets, status.InReplyToAccount.URI, status.InReplyToAccount.URL)
	}
	for i := range facets {
		facets[i].Index.ByteStart += len(prefix)
		facets[i].Index.ByteEnd += len(prefix)
	}
	full := prefix + body
	if uniseg.GraphemeClusterCount(full) <= maxPostGraphemes && len(full) <= maxPostBytes {
		return full, facets
	}
	suffix := "… " + status.URL
	trimmed := truncateUTF8(full, maxPostGraphemes-uniseg.GraphemeClusterCount(suffix), maxPostBytes-len(suffix)) + suffix
	start := len(trimmed) - len(status.URL)
	return trimmed, []facet{{
		Index:    facetIndex{ByteStart: start, ByteEnd: len(trimmed)},
		Features: []facetFeature{{Type: "app.bsky.richtext.facet#link", URI: status.URL}},
	}}
}

func trimTextAndFacets(body string, facets []facet) (string, []facet) {
	leftTrimmed := strings.TrimLeftFunc(body, unicode.IsSpace)
	removed := len(body) - len(leftTrimmed)
	body = strings.TrimRightFunc(leftTrimmed, unicode.IsSpace)
	kept := facets[:0]
	for _, value := range facets {
		value.Index.ByteStart -= removed
		value.Index.ByteEnd -= removed
		if value.Index.ByteStart >= 0 && value.Index.ByteEnd > value.Index.ByteStart && value.Index.ByteEnd <= len(body) {
			kept = append(kept, value)
		}
	}
	return body, kept
}

func stripLeadingAccountMention(body string, facets []facet, accountURLs ...string) (string, []facet) {
	if len(facets) == 0 || facets[0].Index.ByteStart != 0 || len(facets[0].Features) == 0 {
		return body, facets
	}
	uri := facets[0].Features[0].URI
	matched := false
	for _, accountURL := range accountURLs {
		if uri != "" && uri == accountURL {
			matched = true
			break
		}
	}
	if !matched || facets[0].Index.ByteEnd > len(body) {
		return body, facets
	}
	removed := facets[0].Index.ByteEnd
	for removed < len(body) && (body[removed] == ' ' || body[removed] == '\n' || body[removed] == '\t') {
		removed++
	}
	body = body[removed:]
	remaining := facets[1:]
	for i := range remaining {
		remaining[i].Index.ByteStart -= removed
		remaining[i].Index.ByteEnd -= removed
	}
	return body, remaining
}

func htmlTextAndFacets(input string) (string, []facet) {
	doc, err := html.Parse(strings.NewReader(input))
	if err != nil {
		return text.StripHTMLFromText(input), nil
	}
	var out strings.Builder
	var facets []facet
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			out.WriteString(node.Data)
			return
		}
		block := node.Type == html.ElementNode && (node.Data == "p" || node.Data == "div" || node.Data == "li" || node.Data == "br")
		if block && out.Len() != 0 && !strings.HasSuffix(out.String(), "\n") {
			out.WriteByte('\n')
		}
		start := out.Len()
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if node.Type == html.ElementNode && node.Data == "a" {
			if href := htmlAttribute(node, "href"); out.Len() > start && (strings.HasPrefix(href, "https://") || strings.HasPrefix(href, "http://")) {
				facets = append(facets, facet{
					Index:    facetIndex{ByteStart: start, ByteEnd: out.Len()},
					Features: []facetFeature{{Type: "app.bsky.richtext.facet#link", URI: href}},
				})
			}
		}
		if block && node.Data != "br" && out.Len() != 0 && !strings.HasSuffix(out.String(), "\n") {
			out.WriteByte('\n')
		}
	}
	walk(doc)
	return out.String(), facets
}

func htmlAttribute(node *html.Node, name string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return attribute.Val
		}
	}
	return ""
}

func truncateUTF8(value string, maxGraphemes, maxBytes int) string {
	var out strings.Builder
	graphemes := uniseg.NewGraphemes(value)
	for count := 0; count < maxGraphemes && graphemes.Next(); count++ {
		cluster := graphemes.Str()
		if out.Len()+len(cluster) > maxBytes {
			break
		}
		out.WriteString(cluster)
	}
	return strings.TrimSpace(out.String())
}
