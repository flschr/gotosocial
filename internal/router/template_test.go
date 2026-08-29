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

package router

import (
	"html/template"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/language"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/html"
)

func TestRemoteFollowCloseButtonDoesNotSubmitForm(t *testing.T) {
	markup, err := os.ReadFile("../../web/template/profile_header.tmpl")
	if err != nil {
		t.Fatalf("read profile header template: %v", err)
	}

	closeButton := `type="button" class="remote-follow-close" aria-label="Close" data-remote-follow-close`
	if !strings.Contains(string(markup), closeButton) {
		t.Fatalf("remote follow close button must not submit the required form")
	}
}

func TestRemoteFollowButtonLabelStaysStable(t *testing.T) {
	markup, err := os.ReadFile("../../web/template/profile_header.tmpl")
	if err != nil {
		t.Fatalf("read profile header template: %v", err)
	}

	if !strings.Contains(string(markup), `<span>Follow</span>`) {
		t.Fatalf("remote follow button must use its neutral label")
	}

	script, err := os.ReadFile("../../web/source/frontend/index.js")
	if err != nil {
		t.Fatalf("read frontend script: %v", err)
	}

	if strings.Contains(string(script), `openButton.querySelector("span").textContent`) {
		t.Fatalf("stored server must not replace the remote follow button label")
	}
}

func TestRemoteFollowServerChoiceStaysInsideDialog(t *testing.T) {
	markup, err := os.ReadFile("../../web/template/profile_header.tmpl")
	if err != nil {
		t.Fatalf("read profile header template: %v", err)
	}

	if strings.Contains(string(markup), "Use another server") || strings.Contains(string(markup), "data-remote-follow-change") {
		t.Fatalf("server change action must not appear below the profile follow button")
	}

	script, err := os.ReadFile("../../web/source/frontend/index.js")
	if err != nil {
		t.Fatalf("read frontend script: %v", err)
	}

	if !strings.Contains(string(script), `serverInput.value = savedServer || "";`) {
		t.Fatalf("saved server must be editable in the remote follow dialog")
	}
}

func TestStatusAttachmentMarkupIsScopedAndCSPCompatible(t *testing.T) {
	oldTemplateDir := config.GetWebTemplateBaseDir()
	config.SetWebTemplateBaseDir("../../web/template")
	t.Cleanup(func() { config.SetWebTemplateBaseDir(oldTemplateDir) })

	engine := gin.New()
	if err := LoadTemplates(engine); err != nil {
		t.Fatalf("load templates: %v", err)
	}

	renderAttachment := func(mediaType string) string {
		t.Helper()

		previewURL := "https://example.org/small.jpeg"
		attachment := &apimodel.WebAttachment{
			Attachment: &apimodel.Attachment{
				Type:       mediaType,
				PreviewURL: &previewURL,
				Meta: &apimodel.MediaMeta{
					Small: apimodel.MediaDimensions{Width: 512, Height: 384},
					Focus: &apimodel.MediaFocus{},
				},
			},
		}
		data := struct {
			Item  *apimodel.WebAttachment
			Index int
		}{Item: attachment}

		output := httptest.NewRecorder()
		if err := engine.HTMLRender.Instance("status_attachment.tmpl", data).Render(output); err != nil {
			t.Fatalf("render %s attachment: %v", mediaType, err)
		}
		return output.Body.String()
	}

	imageHTML := renderAttachment("image")
	if !strings.Contains(imageHTML, `class="media-wrapper image-media-wrapper"`) {
		t.Fatalf("image attachment missing image-specific class:\n%s", imageHTML)
	}
	if strings.Contains(imageHTML, `style=`) {
		t.Fatalf("image attachment contains CSP-incompatible inline style:\n%s", imageHTML)
	}

	videoHTML := renderAttachment("video")
	if strings.Contains(videoHTML, "image-media-wrapper") {
		t.Fatalf("video attachment received image-specific class:\n%s", videoHTML)
	}
}

// newMinimalWebStatus builds a bare-bones *apimodel.WebStatus with just
// enough fields set for status.tmpl (and the sub-templates it includes)
// to render without panicking on a nil dereference.
func newMinimalWebStatus(id string) *apimodel.WebStatus {
	return &apimodel.WebStatus{
		Status: &apimodel.Status{
			ID:         id,
			CreatedAt:  "2024-01-01T00:00:00.000Z",
			Content:    "<p>hello from " + id + "</p>",
			URL:        "https://example.org/@user_" + id + "/statuses/" + id,
			Visibility: apimodel.VisibilityPublic,
		},
		Account: &apimodel.WebAccount{
			Account: &apimodel.Account{
				ID:          "account_" + id,
				Username:    "user_" + id,
				Acct:        "user_" + id,
				DisplayName: "User " + id,
				URL:         "https://example.org/@user_" + id,
				Avatar:      "https://example.org/avatar_" + id + ".png",
			},
		},
		LanguageTag: new(language.Language),
		Local:       true,
	}
}

// maxAnchorNestingDepth returns how deeply <a> tags are nested in the
// raw markup, by tokenizing it (html.NewTokenizer) rather than building
// a DOM via html.Parse.
//
// This distinction matters: the HTML5 parsing algorithm's "adoption
// agency" step actively repairs a literally nested <a>...<a> by closing
// the outer anchor early and starting a new sibling one, so a DOM walk
// over html.Parse's output would never observe the nesting at all -- it
// self-heals before a walk could see it (verified: parsing
// `<a>outer <a>inner</a> more</a>` yields two *sibling* <a> nodes and an
// orphaned "more" text node outside any anchor, not a nested pair). That
// auto-repair is exactly the bug worth catching -- it silently splits
// one link into two and strands trailing text outside any anchor -- so
// detecting it means looking at the tag stream as the template actually
// emitted it, before a parser "fixes" it up.
func maxAnchorNestingDepth(body string) int {
	z := html.NewTokenizer(strings.NewReader(body))
	depth, max := 0, 0
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return max
		}
		tok := z.Token()
		if tok.Data != "a" {
			continue
		}
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			depth++
			if depth > max {
				max = depth
			}
		case html.EndTagToken:
			if depth > 0 {
				depth--
			}
		}
	}
}

// TestMaxAnchorNestingDepthCatchesActualNesting is a meta-test proving
// maxAnchorNestingDepth actually detects genuine nesting, since an
// html.Parse-based DOM walk (the first, wrong approach tried here) would
// not: see maxAnchorNestingDepth's comment for why that approach is a
// placebo check that always reports "no nesting found".
func TestMaxAnchorNestingDepthCatchesActualNesting(t *testing.T) {
	nested := `<a href="/outer">outer <a href="/inner">inner</a> more</a>`
	if depth := maxAnchorNestingDepth(nested); depth != 2 {
		t.Fatalf("expected nested anchors to report depth 2, got %d", depth)
	}

	siblings := `<a href="/outer">outer</a><a href="/inner">inner</a>`
	if depth := maxAnchorNestingDepth(siblings); depth != 1 {
		t.Fatalf("expected sibling anchors to report depth 1, got %d", depth)
	}
}

// assertNoNestedAnchors fails the test if the raw rendered markup ever
// opens an <a> tag before a previously-opened one has closed.
func assertNoNestedAnchors(t *testing.T, body string) {
	t.Helper()

	if depth := maxAnchorNestingDepth(body); depth > 1 {
		t.Fatalf("found <a> opened while another <a> was still open (rendered markup splits/nests links) in:\n%s", body)
	}
}

// TestStatusQuoteRendersWithoutNestedAnchorsOrDepth actually executes
// status.tmpl (via html/template, not just string-matching the template
// source) with a WebStatus that has an accepted, rendered .Quote, the way
// StatusToWebStatus populates it. Neither of these existing test suites
// caught template-execution errors before: internaltofrontend_test.go
// only exercises the Go model (never runs it through html/template), and
// this file previously only string-matched template source. This closes
// that gap for the specific concern raised in review: recursively
// including status.tmpl for .Quote nests a full interactive status
// (its own header <a> and footer <a>) inside another one, which would be
// invalid, broken-click-target markup if the two ever ended up nested
// rather than siblings.
//
// See TestMaxAnchorNestingDepthCatchesActualNesting for proof that the
// helper this test relies on actually fails on genuinely nested anchors
// (an html.Parse-based DOM walk would not: see that test's comment).
func TestStatusQuoteRendersWithoutNestedAnchorsOrDepth(t *testing.T) {
	oldTemplateDir := config.GetWebTemplateBaseDir()
	config.SetWebTemplateBaseDir("../../web/template")
	t.Cleanup(func() { config.SetWebTemplateBaseDir(oldTemplateDir) })

	engine := gin.New()
	if err := LoadTemplates(engine); err != nil {
		t.Fatalf("load templates: %v", err)
	}

	quoted := newMinimalWebStatus("quoted")
	outer := newMinimalWebStatus("outer")
	outer.Quote = &apimodel.WebQuote{WebStatus: quoted}

	output := httptest.NewRecorder()
	if err := engine.HTMLRender.Instance("status.tmpl", outer).Render(output); err != nil {
		t.Fatalf("render status.tmpl with quote: %v", err)
	}

	body := output.Body.String()
	if !strings.Contains(body, "status-quote") {
		t.Fatalf("rendered status is missing the .status-quote wrapper:\n%s", body)
	}
	if strings.Count(body, `class="status status-quote h-cite"`) != 1 {
		t.Fatalf("expected exactly one rendered quote card:\n%s", body)
	}

	assertNoNestedAnchors(t, body)
}

func TestPublicVersion(t *testing.T) {
	for input, expected := range map[string]string{
		"0.22.0-plus+git-0de01b8":                  "0.22.0-plus",
		"0.22.0-plus.1+git-43c8d97e0":              "0.22.0-plus.1",
		"0.22.0-plus.1-settings-test.4+git-43c8d":  "0.22.0-plus.1",
		"0.22.0-plus-bluesky-test.2+git-48885a7cc": "0.22.0-plus",
		"0.22.0+git-0de01b8":                       "0.22.0",
		"0.22.0":                                   "0.22.0",
	} {
		if actual := publicVersion(input); actual != expected {
			t.Errorf("publicVersion(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestPlusVersionParts(t *testing.T) {
	const version = "0.22.0-plus.1-settings-test.4+git-43c8d97e0"
	if actual := baseVersion(version); actual != "0.22.0" {
		t.Errorf("baseVersion(%q) = %q, want 0.22.0", version, actual)
	}
	if actual := plusRelease(version); actual != "1" {
		t.Errorf("plusRelease(%q) = %q, want 1", version, actual)
	}
}

func TestOutdentPreformatted(t *testing.T) {
	const html = template.HTML(`
        <div class="text">
            <div
                class="content"
                lang="en"
                title="DW from Arthur is labeled &#34;crawlers&#34;. 
        
            She&#39;s reading a sign on a door that says: &#34;robots.txt: don&#39;t crawl this website, it&#39;s not for you, please, thanks.&#34;
        
        With her hands on her hips looking annoyed she says &#34;That sign won&#39;t stop me because I can&#39;t read!&#34;"
                alt="pee pee poo poo"
            >
                <p>Here's a bunch of HTML, read it and weep, weep then!</p>
                <pre><code class="language-html">&lt;section class=&#34;about-user&#34;&gt;
                    &lt;div class=&#34;col-header&#34;&gt;
                        &lt;h2&gt;About&lt;/h2&gt;
                    &lt;/div&gt;            
                    &lt;div class=&#34;fields&#34;&gt;
                        &lt;h3 class=&#34;sr-only&#34;&gt;Fields&lt;/h3&gt;
                        &lt;dl&gt;
                            &lt;div class=&#34;field&#34;&gt;
                                &lt;dt&gt;should you follow me?&lt;/dt&gt;
                                &lt;dd&gt;maybe!&lt;/dd&gt;
                            &lt;/div&gt;
                            &lt;div class=&#34;field&#34;&gt;
                                &lt;dt&gt;age&lt;/dt&gt;
                                &lt;dd&gt;120&lt;/dd&gt;
                            &lt;/div&gt;
                        &lt;/dl&gt;
                    &lt;/div&gt;
                    &lt;div class=&#34;bio&#34;&gt;
                        &lt;h3 class=&#34;sr-only&#34;&gt;Bio&lt;/h3&gt;
                        &lt;p&gt;i post about things that concern me&lt;/p&gt;
                    &lt;/div&gt;
                    &lt;div class=&#34;sr-only&#34; role=&#34;group&#34;&gt;
                        &lt;h3 class=&#34;sr-only&#34;&gt;Stats&lt;/h3&gt;
                        &lt;span&gt;Joined in Jun, 2022.&lt;/span&gt;
                        &lt;span&gt;8 posts.&lt;/span&gt;
                        &lt;span&gt;Followed by 1.&lt;/span&gt;
                        &lt;span&gt;Following 1.&lt;/span&gt;
                    &lt;/div&gt;
                    &lt;div class=&#34;accountstats&#34; aria-hidden=&#34;true&#34;&gt;
                        &lt;b&gt;Joined&lt;/b&gt;&lt;time datetime=&#34;2022-06-04T13:12:00.000Z&#34;&gt;Jun, 2022&lt;/time&gt;
                        &lt;b&gt;Posts&lt;/b&gt;&lt;span&gt;8&lt;/span&gt;
                        &lt;b&gt;Followed by&lt;/b&gt;&lt;span&gt;1&lt;/span&gt;
                        &lt;b&gt;Following&lt;/b&gt;&lt;span&gt;1&lt;/span&gt;
                    &lt;/div&gt;
                &lt;/section&gt;
                </code></pre>
                <p>There, hope you liked that!</p>
            </div>
        </div>
        <div class="text">
            <div
                class="content"
                lang="en"
                alt="DW from Arthur is labeled &#34;crawlers&#34;. 
        
        She&#39;s reading a sign on a door that says: &#34;robots.txt: don&#39;t crawl this website, it&#39;s not for you, please, thanks.&#34;
        
        With her hands on her hips looking annoyed she says &#34;That sign won&#39;t stop me because I can&#39;t read!&#34;"
            >
                <p>Here's a bunch of HTML, read it and weep, weep then!</p>
                <pre><code class="language-html">&lt;section class=&#34;about-user&#34;&gt;
                    &lt;div class=&#34;col-header&#34;&gt;
                        &lt;h2&gt;About&lt;/h2&gt;
                    &lt;/div&gt;            
                    &lt;div class=&#34;fields&#34;&gt;
                        &lt;h3 class=&#34;sr-only&#34;&gt;Fields&lt;/h3&gt;
                        &lt;dl&gt;
                            &lt;div class=&#34;field&#34;&gt;
                                &lt;dt&gt;should you follow me?&lt;/dt&gt;
                                &lt;dd&gt;maybe!&lt;/dd&gt;
                            &lt;/div&gt;
                            &lt;div class=&#34;field&#34;&gt;
                                &lt;dt&gt;age&lt;/dt&gt;
                                &lt;dd&gt;120&lt;/dd&gt;
                            &lt;/div&gt;
                        &lt;/dl&gt;
                    &lt;/div&gt;
                    &lt;div class=&#34;bio&#34;&gt;
                        &lt;h3 class=&#34;sr-only&#34;&gt;Bio&lt;/h3&gt;
                        &lt;p&gt;i post about things that concern me&lt;/p&gt;
                    &lt;/div&gt;
                    &lt;div class=&#34;sr-only&#34; role=&#34;group&#34;&gt;
                        &lt;h3 class=&#34;sr-only&#34;&gt;Stats&lt;/h3&gt;
                        &lt;span&gt;Joined in Jun, 2022.&lt;/span&gt;
                        &lt;span&gt;8 posts.&lt;/span&gt;
                        &lt;span&gt;Followed by 1.&lt;/span&gt;
                        &lt;span&gt;Following 1.&lt;/span&gt;
                    &lt;/div&gt;
                    &lt;div class=&#34;accountstats&#34; aria-hidden=&#34;true&#34;&gt;
                        &lt;b&gt;Joined&lt;/b&gt;&lt;time datetime=&#34;2022-06-04T13:12:00.000Z&#34;&gt;Jun, 2022&lt;/time&gt;
                        &lt;b&gt;Posts&lt;/b&gt;&lt;span&gt;8&lt;/span&gt;
                        &lt;b&gt;Followed by&lt;/b&gt;&lt;span&gt;1&lt;/span&gt;
                        &lt;b&gt;Following&lt;/b&gt;&lt;span&gt;1&lt;/span&gt;
                    &lt;/div&gt;
                &lt;/section&gt;
                </code></pre>
                <p>There, hope you liked that!</p>
            </div>
        </div>
`)

	const expected = template.HTML(`
        <div class="text">
            <div
                class="content"
                lang="en"
                title="DW from Arthur is labeled &#34;crawlers&#34;. 

    She&#39;s reading a sign on a door that says: &#34;robots.txt: don&#39;t crawl this website, it&#39;s not for you, please, thanks.&#34;

With her hands on her hips looking annoyed she says &#34;That sign won&#39;t stop me because I can&#39;t read!&#34;"
                alt="pee pee poo poo"
            >
                <p>Here's a bunch of HTML, read it and weep, weep then!</p>
<pre><code class="language-html">&lt;section class=&#34;about-user&#34;&gt;
    &lt;div class=&#34;col-header&#34;&gt;
        &lt;h2&gt;About&lt;/h2&gt;
    &lt;/div&gt;            
    &lt;div class=&#34;fields&#34;&gt;
        &lt;h3 class=&#34;sr-only&#34;&gt;Fields&lt;/h3&gt;
        &lt;dl&gt;
            &lt;div class=&#34;field&#34;&gt;
&lt;dt&gt;should you follow me?&lt;/dt&gt;
&lt;dd&gt;maybe!&lt;/dd&gt;
            &lt;/div&gt;
            &lt;div class=&#34;field&#34;&gt;
&lt;dt&gt;age&lt;/dt&gt;
&lt;dd&gt;120&lt;/dd&gt;
            &lt;/div&gt;
        &lt;/dl&gt;
    &lt;/div&gt;
    &lt;div class=&#34;bio&#34;&gt;
        &lt;h3 class=&#34;sr-only&#34;&gt;Bio&lt;/h3&gt;
        &lt;p&gt;i post about things that concern me&lt;/p&gt;
    &lt;/div&gt;
    &lt;div class=&#34;sr-only&#34; role=&#34;group&#34;&gt;
        &lt;h3 class=&#34;sr-only&#34;&gt;Stats&lt;/h3&gt;
        &lt;span&gt;Joined in Jun, 2022.&lt;/span&gt;
        &lt;span&gt;8 posts.&lt;/span&gt;
        &lt;span&gt;Followed by 1.&lt;/span&gt;
        &lt;span&gt;Following 1.&lt;/span&gt;
    &lt;/div&gt;
    &lt;div class=&#34;accountstats&#34; aria-hidden=&#34;true&#34;&gt;
        &lt;b&gt;Joined&lt;/b&gt;&lt;time datetime=&#34;2022-06-04T13:12:00.000Z&#34;&gt;Jun, 2022&lt;/time&gt;
        &lt;b&gt;Posts&lt;/b&gt;&lt;span&gt;8&lt;/span&gt;
        &lt;b&gt;Followed by&lt;/b&gt;&lt;span&gt;1&lt;/span&gt;
        &lt;b&gt;Following&lt;/b&gt;&lt;span&gt;1&lt;/span&gt;
    &lt;/div&gt;
&lt;/section&gt;
</code></pre>
                <p>There, hope you liked that!</p>
            </div>
        </div>
        <div class="text">
            <div
                class="content"
                lang="en"
                alt="DW from Arthur is labeled &#34;crawlers&#34;. 

She&#39;s reading a sign on a door that says: &#34;robots.txt: don&#39;t crawl this website, it&#39;s not for you, please, thanks.&#34;

With her hands on her hips looking annoyed she says &#34;That sign won&#39;t stop me because I can&#39;t read!&#34;"
            >
                <p>Here's a bunch of HTML, read it and weep, weep then!</p>
<pre><code class="language-html">&lt;section class=&#34;about-user&#34;&gt;
    &lt;div class=&#34;col-header&#34;&gt;
        &lt;h2&gt;About&lt;/h2&gt;
    &lt;/div&gt;            
    &lt;div class=&#34;fields&#34;&gt;
        &lt;h3 class=&#34;sr-only&#34;&gt;Fields&lt;/h3&gt;
        &lt;dl&gt;
            &lt;div class=&#34;field&#34;&gt;
&lt;dt&gt;should you follow me?&lt;/dt&gt;
&lt;dd&gt;maybe!&lt;/dd&gt;
            &lt;/div&gt;
            &lt;div class=&#34;field&#34;&gt;
&lt;dt&gt;age&lt;/dt&gt;
&lt;dd&gt;120&lt;/dd&gt;
            &lt;/div&gt;
        &lt;/dl&gt;
    &lt;/div&gt;
    &lt;div class=&#34;bio&#34;&gt;
        &lt;h3 class=&#34;sr-only&#34;&gt;Bio&lt;/h3&gt;
        &lt;p&gt;i post about things that concern me&lt;/p&gt;
    &lt;/div&gt;
    &lt;div class=&#34;sr-only&#34; role=&#34;group&#34;&gt;
        &lt;h3 class=&#34;sr-only&#34;&gt;Stats&lt;/h3&gt;
        &lt;span&gt;Joined in Jun, 2022.&lt;/span&gt;
        &lt;span&gt;8 posts.&lt;/span&gt;
        &lt;span&gt;Followed by 1.&lt;/span&gt;
        &lt;span&gt;Following 1.&lt;/span&gt;
    &lt;/div&gt;
    &lt;div class=&#34;accountstats&#34; aria-hidden=&#34;true&#34;&gt;
        &lt;b&gt;Joined&lt;/b&gt;&lt;time datetime=&#34;2022-06-04T13:12:00.000Z&#34;&gt;Jun, 2022&lt;/time&gt;
        &lt;b&gt;Posts&lt;/b&gt;&lt;span&gt;8&lt;/span&gt;
        &lt;b&gt;Followed by&lt;/b&gt;&lt;span&gt;1&lt;/span&gt;
        &lt;b&gt;Following&lt;/b&gt;&lt;span&gt;1&lt;/span&gt;
    &lt;/div&gt;
&lt;/section&gt;
</code></pre>
                <p>There, hope you liked that!</p>
            </div>
        </div>
`)

	out := outdentPreformatted(html)
	if out != expected {
		t.Fatalf("unexpected output:\n`%s`\n", out)
	}
}

func TestOutdentOGMeta(t *testing.T) {
	const html = template.HTML(`<html lang="en">
    <head>
        <meta property="og:description" content="here is
        a
        
        multiline toot
        with some
        significant whitespace!
        
        &lt;3 &lt;3 &lt;3">
    </head>`)

	const expected = template.HTML(`<html lang="en">
    <head>
        <meta property="og:description" content="here is
a

multiline toot
with some
significant whitespace!

&lt;3 &lt;3 &lt;3">
    </head>`)

	out := outdentOGMeta(html)
	if out != expected {
		t.Fatalf("unexpected output:\n`%s`\n", out)
	}
}
