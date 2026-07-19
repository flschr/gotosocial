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
	"github.com/gin-gonic/gin"
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
