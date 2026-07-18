// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/rivo/uniseg"
	"github.com/stretchr/testify/require"
)

func TestOAuthScopesAreGranular(t *testing.T) {
	scopes := strings.Join(OAuthClientConfig().Scopes, " ")
	require.NotContains(t, scopes, "transition:generic")
	require.Contains(t, scopes, "repo:app.bsky.feed.post")
	require.Contains(t, scopes, "app.bsky.notification.listNotifications")
}

func TestSyncThreadgateUsesPDSRecord(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requestCount++
		require.Equal(t, "/xrpc/com.atproto.repo.putRecord", request.URL.Path)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"uri":"at://did:plc:test/app.bsky.feed.threadgate/status","cid":"bafy"}`))
	}))
	defer server.Close()
	client := atclient.NewAPIClient(server.URL)
	status := &gtsmodel.Status{ID: "status", CreatedAt: time.Now(), InteractionPolicy: &gtsmodel.InteractionPolicy{CanReply: &gtsmodel.PolicyRules{}}}
	require.NoError(t, syncThreadgate(t.Context(), client, "did:plc:test", status, "at://did:plc:test/app.bsky.feed.post/status", false))
	require.Equal(t, 1, requestCount)
}

func TestPrepareBlueskyImageReencodesAndResizes(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 2400, 1600))
	for y := 0; y < source.Bounds().Dy(); y++ {
		for x := 0; x < source.Bounds().Dx(); x++ {
			source.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 120, A: 255})
		}
	}
	var input bytes.Buffer
	require.NoError(t, png.Encode(&input, source))
	output, contentType, err := prepareBlueskyImage(input.Bytes())
	require.NoError(t, err)
	require.Equal(t, "image/jpeg", contentType)
	require.LessOrEqual(t, len(output), maxBlobBytes)
	decoded, _, err := image.Decode(bytes.NewReader(output))
	require.NoError(t, err)
	require.LessOrEqual(t, decoded.Bounds().Dx(), 2000)
	require.LessOrEqual(t, decoded.Bounds().Dy(), 2000)
}

func TestHTMLTextAndFacetsPreservesLinkedLabel(t *testing.T) {
	plain, facets := htmlTextAndFacets(`<p>Read <a href="https://example.org/post">the article</a>.</p>`)
	require.Equal(t, "Read the article.\n", plain)
	require.Len(t, facets, 1)
	require.Equal(t, len("Read "), facets[0].Index.ByteStart)
	require.Equal(t, len("Read the article"), facets[0].Index.ByteEnd)
	require.Equal(t, "https://example.org/post", facets[0].Features[0].URI)
}

func TestTrimTextKeepsFacetByteOffsets(t *testing.T) {
	body, facets := htmlTextAndFacets(`  <a href="https://example.org">link</a>  `)
	body, facets = trimTextAndFacets(body, facets)
	require.Equal(t, "link", body)
	require.Len(t, facets, 1)
	require.Equal(t, facetIndex{ByteStart: 0, ByteEnd: 4}, facets[0].Index)
}

func TestHTMLTextCreatesNativeHashtagFacet(t *testing.T) {
	plain, facets := htmlTextAndFacets(`<a href="https://example.org/tags/golang">#golang</a>`)
	require.Equal(t, "#golang", plain)
	require.Len(t, facets, 1)
	require.Equal(t, "app.bsky.richtext.facet#tag", facets[0].Features[0].Type)
	require.Equal(t, "golang", facets[0].Features[0].Tag)
}

func TestBlueskyTextTruncatesWithCanonicalLink(t *testing.T) {
	status := &gtsmodel.Status{
		Content: strings.Repeat("word ", 100),
		URL:     "https://example.org/@author/status/1",
	}
	plain, facets := blueskyText(status)
	require.LessOrEqual(t, uniseg.GraphemeClusterCount(plain), maxPostGraphemes)
	require.LessOrEqual(t, len(plain), maxPostBytes)
	require.True(t, strings.HasSuffix(plain, status.URL))
	require.Len(t, facets, 1)
}

func TestBlueskyReplyStripsInternalProxyMention(t *testing.T) {
	account := &gtsmodel.Account{URI: "https://example.org/users/instance", URL: "https://example.org/@instance"}
	status := &gtsmodel.Status{
		Content:          `<p><a href="https://example.org/@instance">@instance</a> Thanks <a href="https://example.net">friend</a></p>`,
		InReplyToAccount: account,
	}
	plain, facets := blueskyTextForStatus(status, true)
	require.Equal(t, "Thanks friend", plain)
	require.Len(t, facets, 1)
	require.Equal(t, "https://example.net", facets[0].Features[0].URI)
}

func TestRenderBlueskyRecordFacets(t *testing.T) {
	var record blueskyPostRecord
	record.Text = "Read this"
	record.Facets = append(record.Facets, struct {
		Index    facetIndex `json:"index"`
		Features []struct {
			Type string `json:"$type"`
			URI  string `json:"uri"`
			DID  string `json:"did"`
			Tag  string `json:"tag"`
		} `json:"features"`
	}{Index: facetIndex{ByteStart: 5, ByteEnd: 9}})
	record.Facets[0].Features = append(record.Facets[0].Features, struct {
		Type string `json:"$type"`
		URI  string `json:"uri"`
		DID  string `json:"did"`
		Tag  string `json:"tag"`
	}{Type: "app.bsky.richtext.facet#link", URI: "https://example.org"})
	require.Equal(t, `Read <a href="https://example.org">this</a>`, renderBlueskyRecord(record, "did:plc:test"))
}

func TestEligibleForCrosspost(t *testing.T) {
	connection := &gtsmodel.BlueskyConnection{CrosspostPublic: true}
	status := &gtsmodel.Status{Visibility: gtsmodel.VisibilityPublic}
	status.Flags.SetFederated(true)
	require.True(t, EligibleForCrosspost(status, connection))

	status.InReplyToID = "reply"
	require.False(t, EligibleForCrosspost(status, connection))
	status.InReplyToID = ""
	status.MentionIDs = []string{"mention"}
	require.False(t, EligibleForCrosspost(status, connection))
}
