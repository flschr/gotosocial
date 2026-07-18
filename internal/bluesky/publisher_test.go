// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/db"
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
	require.NoError(t, syncThreadgate(t.Context(), client, "did:plc:test", status, "at://did:plc:test/app.bsky.feed.post/status", "3kexampletid", false))
	require.Equal(t, 1, requestCount)
}

func TestDeleteATRecordTreatsMissingRecordAsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/xrpc/com.atproto.repo.deleteRecord", request.URL.Path)
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusBadRequest)
		_, _ = response.Write([]byte(`{"error":"RecordNotFound","message":"Record not found"}`))
	}))
	defer server.Close()
	client := atclient.NewAPIClient(server.URL)
	require.NoError(t, deleteATRecord(t.Context(), client, "did:plc:test", "app.bsky.feed.post", "missing", true))
	require.Error(t, deleteATRecord(t.Context(), client, "did:plc:test", "app.bsky.feed.post", "missing", false))
}

func TestConnectionErrorCode(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", &ConnectionError{Code: ErrorCodeAuth, Err: errors.New("expired")})
	require.Equal(t, ErrorCodeAuth, errorCode(err))
	require.Equal(t, ErrorCodeRemote, errorCode(errors.New("timeout")))
}

func TestInteractionCreatedAtPrefersServerObservedTime(t *testing.T) {
	indexedAt := time.Now()
	maliciousFuture := indexedAt.Add(100 * 365 * 24 * time.Hour)
	require.Equal(t, indexedAt, interactionCreatedAt(indexedAt, maliciousFuture))
}

func TestClassifyParentLookupOnlyAllowsUnmappedMentions(t *testing.T) {
	skip, err := classifyParentLookup("reply", errors.New("database unavailable"))
	require.Error(t, err)
	require.False(t, skip)

	skip, err = classifyParentLookup("reply", fmt.Errorf("missing: %w", db.ErrNoEntries))
	require.NoError(t, err)
	require.True(t, skip)

	skip, err = classifyParentLookup("mention", db.ErrNoEntries)
	require.NoError(t, err)
	require.False(t, skip)
}

func TestMissingInteractionIsRetainedUntilLocalStatusIsGone(t *testing.T) {
	require.False(t, interactionCanBeForgotten(new(gtsmodel.Status), nil))
	require.False(t, interactionCanBeForgotten(nil, errors.New("database unavailable")))
	require.True(t, interactionCanBeForgotten(nil, fmt.Errorf("gone: %w", db.ErrNoEntries)))
	stub := new(gtsmodel.Status)
	stub.Flags.SetDeleted(true)
	require.True(t, interactionCanBeForgotten(stub, nil))
}

func TestRefreshReplyReferencesUsesCurrentCIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/xrpc/app.bsky.feed.getPosts", request.URL.Path)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"posts":[{"uri":"at://did:plc:test/app.bsky.feed.post/parent","cid":"new-parent"},{"uri":"at://did:plc:test/app.bsky.feed.post/root","cid":"new-root"}]}`))
	}))
	defer server.Close()
	client := atclient.NewAPIClient(server.URL)
	target := &blueskyReplyTarget{
		ParentURI: "at://did:plc:test/app.bsky.feed.post/parent", ParentCID: "stale-parent",
		RootURI: "at://did:plc:test/app.bsky.feed.post/root", RootCID: "stale-root",
	}
	require.NoError(t, refreshReplyReferences(t.Context(), client, target))
	require.Equal(t, "new-parent", target.ParentCID)
	require.Equal(t, "new-root", target.RootCID)
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

func TestPrepareBlueskyImageRejectsExcessivePixelCount(t *testing.T) {
	var encoded bytes.Buffer
	encoded.Write([]byte("\x89PNG\r\n\x1a\n"))
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], 10_000)
	binary.BigEndian.PutUint32(ihdr[4:8], 10_000)
	ihdr[8], ihdr[9] = 8, 2
	binary.Write(&encoded, binary.BigEndian, uint32(len(ihdr)))
	encoded.WriteString("IHDR")
	encoded.Write(ihdr)
	checksum := crc32.NewIEEE()
	_, _ = checksum.Write([]byte("IHDR"))
	_, _ = checksum.Write(ihdr)
	binary.Write(&encoded, binary.BigEndian, checksum.Sum32())
	_, _, err := prepareBlueskyImage(encoded.Bytes())
	require.ErrorContains(t, err, "dimensions")
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

func TestRenderBlueskyExternalEmbedAsPrimaryReplyLink(t *testing.T) {
	var record blueskyPostRecord
	require.NoError(t, json.Unmarshal([]byte(`{
		"text":"",
		"embed":{
			"$type":"app.bsky.embed.external",
			"external":{
				"uri":"https://media.example/gladiator.gif",
				"title":"Gladiator: Hand in Wheat Field",
				"description":"ALT: Gladiator scene"
			}
		}
	}`), &record))

	content := renderInteractionContent(
		record,
		blueskyAuthor{DID: "did:plc:gilly", Handle: "gilly.berlin", DisplayName: "Gilly 🐈🇪🇺"},
		"https://bsky.app/profile/did:plc:gilly/post/reply",
	)
	require.Equal(t,
		`<p>Gilly 🐈🇪🇺 (@gilly.berlin) via Bluesky</p><p><a href="https://media.example/gladiator.gif">Gladiator: Hand in Wheat Field</a></p><p><a href="https://bsky.app/profile/did:plc:gilly/post/reply">View reply on Bluesky</a></p>`,
		content,
	)
	require.NotContains(t, content, "<strong>")
}

func TestEligibleForCrosspost(t *testing.T) {
	connection := &gtsmodel.BlueskyConnection{CrosspostPublic: true, OAuthSessionID: "session", OAuthData: []byte("encrypted")}
	status := &gtsmodel.Status{Visibility: gtsmodel.VisibilityPublic}
	status.Flags.SetFederated(true)
	require.True(t, EligibleForCrosspost(status, connection))

	status.InReplyToID = "reply"
	require.False(t, EligibleForCrosspost(status, connection))
	status.InReplyToID = ""
	status.MentionIDs = []string{"mention"}
	require.False(t, EligibleForCrosspost(status, connection))
}

func TestEligibleForExistingMappingWhileDisconnected(t *testing.T) {
	status := &gtsmodel.Status{Visibility: gtsmodel.VisibilityPublic}
	status.Flags.SetFederated(true)
	require.True(t, EligibleForExistingMapping(status, false))

	status.Visibility = gtsmodel.VisibilityFollowersOnly
	require.False(t, EligibleForExistingMapping(status, false))
	require.True(t, EligibleForExistingMapping(status, true))
}

func TestShouldUpsertMappedStatusMatrix(t *testing.T) {
	public := &gtsmodel.Status{Visibility: gtsmodel.VisibilityPublic}
	public.Flags.SetFederated(true)
	active := &gtsmodel.BlueskyConnection{
		CrosspostPublic: true, OAuthSessionID: "session", OAuthData: []byte("encrypted"),
	}
	disconnected := &gtsmodel.BlueskyConnection{CrosspostPublic: true}
	disabled := &gtsmodel.BlueskyConnection{CrosspostPublic: false}

	require.True(t, ShouldUpsertMappedStatus(public, active, false))
	require.True(t, ShouldUpsertMappedStatus(public, disconnected, false))
	require.True(t, ShouldUpsertMappedStatus(public, nil, false))
	require.True(t, ShouldUpsertMappedStatus(public, disabled, false))

	private := *public
	private.Visibility = gtsmodel.VisibilityFollowersOnly
	require.False(t, ShouldUpsertMappedStatus(&private, active, false))
	require.False(t, ShouldUpsertMappedStatus(&private, disconnected, false))
	require.True(t, ShouldUpsertMappedStatus(&private, disconnected, true))
}
