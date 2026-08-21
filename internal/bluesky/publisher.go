// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

const (
	maxPostGraphemes = 300
	maxPostBytes     = 3000
	maxImages        = 4
	maxBlobBytes     = 1_000_000
)

func EligibleForCrosspost(status *gtsmodel.Status, connection *gtsmodel.BlueskyConnection) bool {
	return connection != nil &&
		connection.Active() &&
		connection.CrosspostPublic &&
		status.Visibility == gtsmodel.VisibilityPublic &&
		!status.LocalOnly() &&
		status.InReplyToID == "" &&
		status.BoostOfID == "" &&
		status.QuoteID == "" &&
		status.QuoteURI == "" &&
		status.PollID == "" &&
		len(status.MentionIDs) == 0
}

func PublishStatus(ctx context.Context, state *state.State, status *gtsmodel.Status, card *apimodel.Card) error {
	return publishStatus(ctx, state, status, card, nil)
}

// PublishReply publishes a local-only reply to an imported Bluesky interaction.
func PublishReply(ctx context.Context, state *state.State, status *gtsmodel.Status, card *apimodel.Card, interaction *gtsmodel.BlueskyInteraction) error {
	return publishStatus(ctx, state, status, card, interaction)
}

func publishStatus(ctx context.Context, state *state.State, status *gtsmodel.Status, card *apimodel.Card, interaction *gtsmodel.BlueskyInteraction) error {
	connection, err := state.DB.GetBlueskyConnectionByAccountID(ctx, status.AccountID)
	if err != nil {
		return err
	}
	if interaction != nil && interaction.AccountID != status.AccountID {
		return fmt.Errorf("Bluesky interaction belongs to a different account")
	}
	existingPost, existingErr := state.DB.GetBlueskyPostByStatusID(ctx, status.ID)
	if existingErr != nil && !errors.Is(existingErr, db.ErrNoEntries) {
		return existingErr
	}
	if interaction == nil && existingPost == nil && !EligibleForCrosspost(status, connection) {
		return nil
	}
	if interaction == nil && existingPost != nil && !ShouldUpsertMappedStatus(status, connection, false) {
		return nil
	}
	rkey, err := recordKeyForStatusID(status.ID)
	if err != nil {
		return fmt.Errorf("derive Bluesky record key: %w", err)
	}
	if err := state.DB.PopulateStatus(ctx, status); err != nil {
		return fmt.Errorf("populate status for Bluesky: %w", err)
	}

	client, err := authenticatedClient(ctx, state, connection)
	if err != nil {
		return err
	}
	var replyTarget *blueskyReplyTarget
	if interaction != nil {
		replyTarget = &blueskyReplyTarget{
			RootURI: interaction.RootURI, RootCID: interaction.RootCID,
			ParentURI: interaction.URI, ParentCID: interaction.CID,
		}
		if err := refreshReplyReferences(ctx, client, replyTarget); err != nil {
			return err
		}
	}

	postText, facets := blueskyTextForStatus(status, interaction != nil)
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
	if replyTarget != nil {
		record["reply"] = map[string]any{
			"root":   map[string]string{"uri": replyTarget.RootURI, "cid": replyTarget.RootCID},
			"parent": map[string]string{"uri": replyTarget.ParentURI, "cid": replyTarget.ParentCID},
		}
	}

	images := make([]map[string]any, 0, maxImages)
	for _, attachment := range status.Attachments {
		if len(images) == maxImages || attachment.Type != gtsmodel.FileTypeImage {
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

	// A deterministic record key makes retries idempotent even if a network
	// failure happens after Bluesky accepted the write but before we saw it.
	endpoint, _ := syntax.ParseNSID("com.atproto.repo.putRecord")
	var response createRecordResponse
	if err := client.Post(ctx, endpoint, map[string]any{
		"repo":       connection.DID,
		"collection": "app.bsky.feed.post",
		"rkey":       rkey,
		"record":     record,
	}, &response); err != nil {
		return fmt.Errorf("create Bluesky post: %w", err)
	}
	rkey = response.URI[strings.LastIndex(response.URI, "/")+1:]
	if err := syncThreadgate(ctx, client, connection.DID, status, response.URI, rkey, existingPost != nil); err != nil {
		return err
	}
	if existingPost != nil {
		existingPost.UpdatedAt = time.Now()
		existingPost.URI = response.URI
		existingPost.CID = response.CID
		existingPost.RootURI = response.URI
		existingPost.RootCID = response.CID
		existingPost.ParentURI = ""
		existingPost.ParentCID = ""
		if replyTarget != nil {
			existingPost.RootURI, existingPost.RootCID = replyTarget.RootURI, replyTarget.RootCID
			existingPost.ParentURI, existingPost.ParentCID = replyTarget.ParentURI, replyTarget.ParentCID
		}
		existingPost.URL = "https://bsky.app/profile/" + connection.DID + "/post/" + rkey
		return state.DB.UpdateBlueskyPost(ctx, existingPost, "updated_at", "uri", "cid", "root_uri", "root_cid", "parent_uri", "parent_cid", "url")
	}
	rootURI, rootCID := response.URI, response.CID
	parentURI, parentCID := "", ""
	if replyTarget != nil {
		rootURI, rootCID = replyTarget.RootURI, replyTarget.RootCID
		parentURI, parentCID = replyTarget.ParentURI, replyTarget.ParentCID
	}
	return state.DB.PutBlueskyPost(ctx, &gtsmodel.BlueskyPost{
		ID: id.NewULID(), ConnectionID: connection.ID, AccountID: status.AccountID,
		StatusID: status.ID, URI: response.URI, CID: response.CID, RootURI: rootURI, RootCID: rootCID, ParentURI: parentURI, ParentCID: parentCID,
		URL: "https://bsky.app/profile/" + connection.DID + "/post/" + rkey,
	})
}
