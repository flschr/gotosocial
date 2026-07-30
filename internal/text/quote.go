// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package text

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// quoteInlineClass is the marker class placed on the leading "RE: <link>"
// fallback paragraph of a quote post. Native-quote-aware clients (Phanpy,
// Mastodon web, ...) use it to hide the redundant fallback line once they
// render the structured quote card.
const quoteInlineClass = "quote-inline"

// ExtractQuoteFallbackURL scans status content HTML for a leading, standalone
// quote-fallback paragraph of the exact shape Mastodon-family servers emit:
//
//	<p>RE: <a href="URL">…</a></p>
//
// (optionally already carrying class="quote-inline"). If found, it returns the
// anchor's href and true. Any other shape — extra prose, multiple links, a
// non-leading position, or a different prefix — returns ("", false), so a
// user's own "RE:" prose or an unrelated link is never misclassified as a
// quote.
func ExtractQuoteFallbackURL(content string) (string, bool) {
	p := leadingParagraph(content)
	if p == nil {
		return "", false
	}
	return quoteFallbackHref(p)
}

// MarkQuoteInline returns content with class="quote-inline" added to the
// leading quote-fallback paragraph (see ExtractQuoteFallbackURL). If there is
// no such paragraph, or it already carries the class, content is returned
// unchanged. It never removes or reorders content — it only adds the marker
// class to the one fallback paragraph.
func MarkQuoteInline(content string) string {
	// Parse only to *detect* the fallback paragraph. The actual mutation is
	// a targeted string edit of the leading <p> opening tag, so the rest of
	// the (already-sanitized) content is never re-serialized — a parse/render
	// hiccup can therefore never blank or reshape the content.
	p := leadingParagraph(content)
	if p == nil {
		return content
	}
	if _, ok := quoteFallbackHref(p); !ok {
		return content
	}
	for _, a := range p.Attr {
		if a.Key == "class" && hasClass(a.Val, quoteInlineClass) {
			// Already marked; nothing to change.
			return content
		}
	}
	return injectClassIntoLeadingP(content)
}

// injectClassIntoLeadingP adds/merges class="quote-inline" into the opening
// tag of the content's leading <p> — which the caller has already confirmed
// to be the fallback paragraph — touching nothing else in the content.
func injectClassIntoLeadingP(content string) string {
	for i := 0; i < len(content); {
		// Skip over HTML comments so a "<p" living inside a comment is
		// never matched (the leading *element* was confirmed to be a <p>
		// by the caller; comments are the only thing that could carry a
		// stray "<p" ahead of it in the raw string).
		if strings.HasPrefix(content[i:], "<!--") {
			end := strings.Index(content[i:], "-->")
			if end < 0 {
				return content
			}
			i += end + len("-->")
			continue
		}

		if strings.HasPrefix(content[i:], "<p") && i+2 < len(content) {
			switch content[i+2] {
			case '>', ' ', '\t', '\n', '\r', '/':
				end := strings.IndexByte(content[i:], '>')
				if end < 0 {
					return content
				}
				tag := content[i : i+end+1]
				return content[:i] + mergeQuoteInlineClass(tag) + content[i+end+1:]
			}
		}
		i++
	}
	return content
}

// mergeQuoteInlineClass returns the given "<p ...>" opening tag with the
// quote-inline class added: merged into an existing class attribute, or a
// new class attribute inserted right after "<p" otherwise.
func mergeQuoteInlineClass(tag string) string {
	if idx := strings.Index(tag, `class="`); idx >= 0 {
		valStart := idx + len(`class="`)
		return tag[:valStart] + quoteInlineClass + " " + tag[valStart:]
	}
	return `<p class="` + quoteInlineClass + `"` + tag[2:]
}

// leadingParagraph parses content and returns its first top-level element if
// that element is a <p>, else nil. Requiring the paragraph to be the leading
// top-level element prevents a nested <p> (e.g. inside a blockquote) from
// being mistaken for the fallback line.
func leadingParagraph(content string) *html.Node {
	_, body := parseBody(content)
	if body == nil {
		return nil
	}
	p := firstChildElement(body)
	if p == nil || p.DataAtom != atom.P {
		return nil
	}
	return p
}

// firstChildElement returns n's first child that is an element node
// (skipping whitespace/text nodes), or nil.
func firstChildElement(n *html.Node) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			return c
		}
		if c.Type == html.TextNode && strings.TrimSpace(c.Data) == "" {
			// Skip insignificant whitespace.
			continue
		}
		// Anything else before the first element — non-whitespace text, or
		// a comment node that could itself contain a "<p" the targeted
		// string edit would latch onto — means the leading paragraph isn't
		// cleanly first. Bail conservatively so we never mismark content.
		return nil
	}
	return nil
}

// quoteFallbackHref reports whether p is exactly a "RE: <a>" fallback
// paragraph, returning the single anchor's href if so. The paragraph must
// contain the text "RE:" (ignoring surrounding whitespace) and exactly one
// anchor, with no other meaningful content.
func quoteFallbackHref(p *html.Node) (string, bool) {
	var (
		text    strings.Builder
		anchors []*html.Node
	)

	for c := p.FirstChild; c != nil; c = c.NextSibling {
		switch {
		case c.Type == html.TextNode:
			text.WriteString(c.Data)
		case c.Type == html.ElementNode && c.DataAtom == atom.A:
			anchors = append(anchors, c)
		case c.Type == html.ElementNode && c.DataAtom == atom.Br:
			// Ignore stray line breaks.
		default:
			// Any other element (extra prose wrapper, second
			// link, etc.) means this isn't a clean fallback.
			return "", false
		}
	}

	if len(anchors) != 1 {
		return "", false
	}
	if strings.TrimSpace(text.String()) != "RE:" {
		return "", false
	}

	for _, attr := range anchors[0].Attr {
		if attr.Key == "href" && attr.Val != "" {
			return attr.Val, true
		}
	}
	return "", false
}

// parseBody parses an HTML content fragment and returns the document root
// and its <body> node (into which the fragment content is placed).
func parseBody(content string) (root, body *html.Node) {
	root, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return nil, nil
	}
	body = firstElement(root, atom.Body)
	return root, body
}

// firstElement returns the first descendant element with the given atom,
// depth-first, or nil.
func firstElement(n *html.Node, a atom.Atom) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == a {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := firstElement(c, a); found != nil {
			return found
		}
	}
	return nil
}

// hasClass reports whether the space-separated class list contains want.
func hasClass(classList, want string) bool {
	for _, c := range strings.Fields(classList) {
		if c == want {
			return true
		}
	}
	return false
}
