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
	"testing"
)

// The real federated Mastodon quote fallback that motivated this feature
// (status 01KYCG5270F60S17D832G7MH0A). Note: no quote-inline class.
const realQuoteFallback = `<p>RE: <a href="https://social.lol/@z428/116980321253943752"><span class="invisible">https://</span><span class="ellipsis">social.lol/@z428/1169803212539</span><span class="invisible">43752</span></a></p><p>Nature just never ceases to amaze me.</p>`

func TestExtractQuoteFallbackURL(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		wantURL string
		wantOK  bool
	}{
		{
			name:    "real federated fallback without class",
			content: realQuoteFallback,
			wantURL: "https://social.lol/@z428/116980321253943752",
			wantOK:  true,
		},
		{
			name:    "fallback already marked",
			content: `<p class="quote-inline">RE: <a href="https://example.org/@a/1">https://example.org/@a/1</a></p><p>hi</p>`,
			wantURL: "https://example.org/@a/1",
			wantOK:  true,
		},
		{
			name:    "user prose that merely starts with RE",
			content: `<p>RE: my earlier point, I was wrong.</p>`,
			wantURL: "",
			wantOK:  false,
		},
		{
			name:    "RE with a link but extra trailing prose",
			content: `<p>RE: <a href="https://example.org/@a/1">link</a> see this</p>`,
			wantURL: "",
			wantOK:  false,
		},
		{
			name:    "link not in leading paragraph",
			content: `<p>Some intro</p><p>RE: <a href="https://example.org/@a/1">link</a></p>`,
			wantURL: "",
			wantOK:  false,
		},
		{
			name:    "two links in leading paragraph",
			content: `<p>RE: <a href="https://example.org/@a/1">a</a> <a href="https://example.org/@a/2">b</a></p>`,
			wantURL: "",
			wantOK:  false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotURL, gotOK := ExtractQuoteFallbackURL(tc.content)
			if gotURL != tc.wantURL || gotOK != tc.wantOK {
				t.Errorf("ExtractQuoteFallbackURL() = (%q, %v), want (%q, %v)",
					gotURL, gotOK, tc.wantURL, tc.wantOK)
			}
		})
	}
}

func TestMarkQuoteInline(t *testing.T) {
	// Real fallback: first paragraph gains the class, second is untouched,
	// the link and all text are preserved.
	got := MarkQuoteInline(realQuoteFallback)
	if !strings.Contains(got, `<p class="quote-inline">RE: `) {
		t.Errorf("expected leading paragraph to gain quote-inline class, got: %s", got)
	}
	if !strings.Contains(got, "Nature just never ceases to amaze me.") {
		t.Errorf("second paragraph must be preserved, got: %s", got)
	}
	if !strings.Contains(got, "https://social.lol/@z428/116980321253943752") {
		t.Errorf("quoted link must be preserved, got: %s", got)
	}
	if strings.Count(got, "quote-inline") != 1 {
		t.Errorf("class must be added exactly once, got: %s", got)
	}

	// Idempotent: marking already-marked content changes nothing.
	if again := MarkQuoteInline(got); again != got {
		t.Errorf("MarkQuoteInline not idempotent:\n first: %s\nsecond: %s", got, again)
	}

	// Non-quote prose is returned unchanged.
	prose := `<p>RE: my earlier point.</p>`
	if out := MarkQuoteInline(prose); out != prose {
		t.Errorf("prose must be unchanged, got: %s", out)
	}
}

func TestMarkQuoteInlinePreservesContent(t *testing.T) {
	// Targeted edit: only the leading <p> opening tag changes. Entities,
	// void elements (<br>), nested spans and the second paragraph are
	// preserved byte-for-byte — no HTML round-trip.
	in := `<p>RE: <a href="https://example.org/@a/1"><span class="invisible">https://</span>example.org/@a/1</a></p><p>plain &amp; simple<br>line with &lt;code&gt; &amp; entities</p>`
	want := `<p class="quote-inline">RE: <a href="https://example.org/@a/1"><span class="invisible">https://</span>example.org/@a/1</a></p><p>plain &amp; simple<br>line with &lt;code&gt; &amp; entities</p>`
	if got := MarkQuoteInline(in); got != want {
		t.Errorf("MarkQuoteInline altered content beyond the marker class:\n got: %s\nwant: %s", got, want)
	}

	// An existing class on the RE paragraph is merged, not replaced.
	in2 := `<p class="foo">RE: <a href="https://example.org/@a/1">x</a></p>`
	want2 := `<p class="quote-inline foo">RE: <a href="https://example.org/@a/1">x</a></p>`
	if got := MarkQuoteInline(in2); got != want2 {
		t.Errorf("MarkQuoteInline class merge wrong:\n got: %s\nwant: %s", got, want2)
	}

	// A leading comment that itself contains a "<p" must NOT be mismarked:
	// the class lands on the real fallback paragraph, the comment is intact.
	in3 := `<!-- <p>x</p> --><p>RE: <a href="https://example.org/@a/1">x</a></p>`
	want3 := `<!-- <p>x</p> --><p class="quote-inline">RE: <a href="https://example.org/@a/1">x</a></p>`
	if got := MarkQuoteInline(in3); got != want3 {
		t.Errorf("comment-adjacent marking wrong:\n got: %s\nwant: %s", got, want3)
	}
}
