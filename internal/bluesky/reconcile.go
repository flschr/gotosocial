// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/ap"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/messages"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

type blueskyPostsResponse struct {
	Posts []struct {
		URI    string          `json:"uri"`
		CID    string          `json:"cid"`
		Author blueskyAuthor   `json:"author"`
		Record json.RawMessage `json:"record"`
	} `json:"posts"`
}

func reconcileInteractions(ctx context.Context, state *state.State, connection *gtsmodel.BlueskyConnection, client *atclient.APIClient) error {
	interactions, err := state.DB.GetBlueskyInteractionsForReconcile(ctx, connection.AccountID, 100)
	if err != nil || len(interactions) == 0 {
		return err
	}
	endpoint, _ := syntax.ParseNSID("app.bsky.feed.getPosts")
	client = client.WithService("did:web:api.bsky.app#bsky_appview")
	var reconcileErrors []error
	for start := 0; start < len(interactions); start += 25 {
		end := min(start+25, len(interactions))
		batch := interactions[start:end]
		uris := make([]string, 0, len(batch))
		for _, interaction := range batch {
			uris = append(uris, interaction.URI)
		}
		var response blueskyPostsResponse
		if err := client.Get(ctx, endpoint, map[string]any{"uris": uris}, &response); err != nil {
			return errors.Join(append(reconcileErrors, fmt.Errorf("reconcile Bluesky interactions: %w", err))...)
		}
		posts := make(map[string]struct {
			CID    string
			Author blueskyAuthor
			Record json.RawMessage
		}, len(response.Posts))
		for _, post := range response.Posts {
			posts[post.URI] = struct {
				CID    string
				Author blueskyAuthor
				Record json.RawMessage
			}{post.CID, post.Author, post.Record}
		}
		for _, interaction := range batch {
			post, exists := posts[interaction.URI]
			if !exists {
				status, statusErr := state.DB.GetStatusByID(ctx, interaction.StatusID)
				if statusErr == nil {
					statusErr = state.DB.PopulateStatus(ctx, status)
					if statusErr == nil {
						target, targetErr := state.DB.GetAccountByID(ctx, connection.AccountID)
						if targetErr != nil {
							statusErr = targetErr
						} else {
							state.Workers.Client.Queue.Push(&messages.FromClientAPI{
								APObjectType: ap.ObjectNote, APActivityType: ap.ActivityDelete,
								GTSModel: status, Origin: status.Account, Target: target,
							})
						}
					}
				}
				if statusErr == nil || errors.Is(statusErr, db.ErrNoEntries) {
					statusErr = state.DB.DeleteBlueskyInteraction(ctx, interaction.ID)
				}
				if statusErr != nil {
					reconcileErrors = append(reconcileErrors, statusErr)
				}
				continue
			}
			if post.CID != interaction.CID {
				var record blueskyPostRecord
				if err := json.Unmarshal(post.Record, &record); err != nil {
					reconcileErrors = append(reconcileErrors, err)
					continue
				}
				status, err := state.DB.GetStatusByID(ctx, interaction.StatusID)
				if err != nil {
					reconcileErrors = append(reconcileErrors, err)
					continue
				}
				status.Text = record.Text
				status.Content = renderInteractionContent(record, post.Author, interaction.URL)
				if err := state.DB.UpdateStatus(ctx, status, "text", "content"); err != nil {
					reconcileErrors = append(reconcileErrors, err)
					continue
				}
				if err := state.DB.PopulateStatus(ctx, status); err == nil {
					if target, err := state.DB.GetAccountByID(ctx, connection.AccountID); err == nil {
						state.Workers.Client.Queue.Push(&messages.FromClientAPI{
							APObjectType: ap.ObjectNote, APActivityType: ap.ActivityUpdate,
							GTSModel: status, Origin: status.Account, Target: target,
						})
					}
				}
				interaction.CID = post.CID
				interaction.AuthorHandle = post.Author.Handle
			}
			interaction.LastCheckedAt = time.Now()
			if err := state.DB.UpdateBlueskyInteraction(ctx, interaction, "cid", "author_handle", "last_checked_at"); err != nil {
				reconcileErrors = append(reconcileErrors, err)
			}
		}
	}
	return errors.Join(reconcileErrors...)
}
