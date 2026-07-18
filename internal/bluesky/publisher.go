// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"code.superseriousbusiness.org/gotosocial/internal/text"
	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/rivo/uniseg"
	"golang.org/x/net/html"
)

const (
	maxPostGraphemes = 300
	maxPostBytes     = 3000
	maxImages        = 4
	maxBlobBytes     = 1_000_000
)

type facet struct {
	Index    facetIndex     `json:"index"`
	Features []facetFeature `json:"features"`
}

type facetIndex struct {
	ByteStart int `json:"byteStart"`
	ByteEnd   int `json:"byteEnd"`
}

type facetFeature struct {
	Type string `json:"$type"`
	URI  string `json:"uri"`
}

type createRecordResponse struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}

type uploadBlobResponse struct {
	Blob any `json:"blob"`
}

func EligibleForCrosspost(status *gtsmodel.Status, connection *gtsmodel.BlueskyConnection) bool {
	return connection != nil &&
		connection.CrosspostPublic &&
		status.Visibility == gtsmodel.VisibilityPublic &&
		!status.LocalOnly() &&
		status.InReplyToID == "" &&
		status.BoostOfID == "" &&
		status.PollID == "" &&
		len(status.MentionIDs) == 0
}

func PublishStatus(ctx context.Context, state *state.State, status *gtsmodel.Status, card *apimodel.Card) error {
	connection, err := state.DB.GetBlueskyConnectionByAccountID(ctx, status.AccountID)
	if err != nil {
		return err
	}
	if !EligibleForCrosspost(status, connection) {
		return nil
	}
	if _, err := state.DB.GetBlueskyPostByStatusID(ctx, status.ID); err == nil {
		return nil
	}
	if err := state.DB.PopulateStatus(ctx, status); err != nil {
		return fmt.Errorf("populate status for Bluesky: %w", err)
	}

	app, _, err := NewOAuthClient(state.DB, status.AccountID)
	if err != nil {
		return err
	}
	did, err := syntax.ParseDID(connection.DID)
	if err != nil {
		return err
	}
	session, err := app.ResumeSession(ctx, did, connection.OAuthSessionID)
	if err != nil {
		return fmt.Errorf("resume Bluesky OAuth session: %w", err)
	}
	client := atclient.NewAPIClient(connection.PDSURL)
	client.Auth = session
	client.AccountDID = &did
	client.Headers.Set("User-Agent", "GoToSocial Plus")

	postText, facets := blueskyText(status)
	record := map[string]any{
		"$type":     "app.bsky.feed.post",
		"text":      postText,
		"createdAt": status.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if len(facets) != 0 {
		record["facets"] = facets
	}
	if status.Language != "" {
		record["langs"] = []string{status.Language}
	}

	images := make([]map[string]any, 0, maxImages)
	for _, attachment := range status.Attachments {
		if len(images) == maxImages || attachment.Type != gtsmodel.FileTypeImage || attachment.File.FileSize > maxBlobBytes {
			continue
		}
		blob, err := uploadStoredBlob(ctx, state, client, attachment.File.Path, attachment.File.ContentType)
		if err != nil {
			return fmt.Errorf("upload Bluesky image: %w", err)
		}
		images = append(images, map[string]any{
			"image": blob,
			"alt":   truncateUTF8(attachment.Description, 1000, 10_000),
		})
	}
	if len(images) != 0 {
		record["embed"] = map[string]any{"$type": "app.bsky.embed.images", "images": images}
	} else if card != nil && card.URL != "" {
		external := map[string]any{
			"uri":         card.URL,
			"title":       truncateUTF8(card.Title, 300, 3000),
			"description": truncateUTF8(card.Description, 300, 3000),
		}
		if card.Image != "" {
			if blob, err := uploadRemoteImage(ctx, state, client, card.Image); err == nil {
				external["thumb"] = blob
			}
		}
		record["embed"] = map[string]any{"$type": "app.bsky.embed.external", "external": external}
	}

	endpoint, _ := syntax.ParseNSID("com.atproto.repo.createRecord")
	var response createRecordResponse
	if err := client.Post(ctx, endpoint, map[string]any{
		"repo":       connection.DID,
		"collection": "app.bsky.feed.post",
		"record":     record,
	}, &response); err != nil {
		return fmt.Errorf("create Bluesky post: %w", err)
	}
	rkey := response.URI[strings.LastIndex(response.URI, "/")+1:]
	return state.DB.PutBlueskyPost(ctx, &gtsmodel.BlueskyPost{
		ID: id.NewULID(), ConnectionID: connection.ID, AccountID: status.AccountID,
		StatusID: status.ID, URI: response.URI, CID: response.CID,
		URL: "https://bsky.app/profile/" + connection.Handle + "/post/" + rkey,
	})
}

func blueskyText(status *gtsmodel.Status) (string, []facet) {
	prefix := ""
	if warning := strings.TrimSpace(text.ParseHTMLToPlain(status.ContentWarning)); warning != "" {
		prefix = "CW: " + warning + "\n\n"
	}
	body, facets := htmlTextAndFacets(status.Content)
	body = strings.TrimSpace(body)
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
			if href := htmlAttribute(node, "href"); strings.HasPrefix(href, "https://") || strings.HasPrefix(href, "http://") {
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

func uploadStoredBlob(ctx context.Context, state *state.State, client *atclient.APIClient, path, contentType string) (any, error) {
	stream, err := state.Storage.GetStream(ctx, path)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	data, err := io.ReadAll(io.LimitReader(stream, maxBlobBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBlobBytes {
		return nil, fmt.Errorf("image exceeds Bluesky blob limit")
	}
	return uploadBlob(ctx, client, bytes.NewReader(data), contentType)
}

func uploadRemoteImage(ctx context.Context, state *state.State, client *atclient.APIClient, imageURL string) (any, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := state.HTTPClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("preview image returned %s", response.Status)
	}
	contentType := response.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return nil, fmt.Errorf("preview is not an image")
	}
	limited := io.LimitReader(response.Body, maxBlobBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(data) > maxBlobBytes {
		return nil, fmt.Errorf("preview image exceeds Bluesky blob limit")
	}
	return uploadBlob(ctx, client, bytes.NewReader(data), contentType)
}

func uploadBlob(ctx context.Context, client *atclient.APIClient, reader io.Reader, contentType string) (any, error) {
	endpoint, _ := syntax.ParseNSID("com.atproto.repo.uploadBlob")
	request := atclient.NewAPIRequest(http.MethodPost, endpoint, reader)
	request.Headers.Set("Content-Type", contentType)
	var response uploadBlobResponse
	result, err := client.Do(ctx, request)
	if err != nil {
		return nil, err
	}
	defer result.Body.Close()
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return nil, fmt.Errorf("blob upload returned %s", result.Status)
	}
	if err := json.NewDecoder(result.Body).Decode(&response); err != nil {
		return nil, err
	}
	return response.Blob, nil
}
