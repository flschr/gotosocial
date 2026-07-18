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

	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

func DeleteStatus(ctx context.Context, state *state.State, statusID string) error {
	return deleteStatus(ctx, state, statusID, true, true)
}

func deleteStatus(ctx context.Context, state *state.State, statusID string, cleanupDelivery, acquireAccountLock bool) error {
	post, err := state.DB.GetBlueskyPostByStatusID(ctx, statusID)
	if errors.Is(err, db.ErrNoEntries) {
		if cleanupDelivery {
			return state.DB.DeleteBlueskyDeliveryByStatusID(ctx, statusID)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if acquireAccountLock {
		defer lockAccount(post.AccountID)()
	}
	connection, err := state.DB.GetBlueskyConnectionByAccountID(ctx, post.AccountID)
	if errors.Is(err, db.ErrNoEntries) {
		return fmt.Errorf("Bluesky account is disconnected; remote deletion will resume after reconnect: %w", err)
	}
	if err != nil {
		return err
	}
	client, err := authenticatedClient(ctx, state, connection)
	if err != nil {
		return err
	}
	rkey := post.URI[strings.LastIndex(post.URI, "/")+1:]
	if err := deleteATRecord(ctx, client, connection.DID, "app.bsky.feed.threadgate", rkey, true); err != nil {
		return fmt.Errorf("delete Bluesky threadgate: %w", err)
	}
	if err := deleteATRecord(ctx, client, connection.DID, "app.bsky.feed.post", rkey, true); err != nil {
		return fmt.Errorf("delete Bluesky post: %w", err)
	}
	if err := state.DB.DeleteBlueskyPost(ctx, post.ID); err != nil {
		return err
	}
	if cleanupDelivery {
		return state.DB.DeleteBlueskyDeliveryByStatusID(ctx, statusID)
	}
	return nil
}

func syncThreadgate(ctx context.Context, client *atclient.APIClient, repo string, status *gtsmodel.Status, postURI string, updating bool) error {
	publicReplies := status.InteractionPolicy == nil || policyAllowsPublic(status.InteractionPolicy.CanReply)
	if publicReplies {
		if !updating {
			return nil
		}
		if err := deleteATRecord(ctx, client, repo, "app.bsky.feed.threadgate", status.ID, true); err != nil {
			return fmt.Errorf("remove Bluesky reply restriction: %w", err)
		}
		return nil
	}
	endpoint, _ := syntax.ParseNSID("com.atproto.repo.putRecord")
	if err := client.Post(ctx, endpoint, map[string]any{
		"repo": repo, "collection": "app.bsky.feed.threadgate", "rkey": status.ID,
		"record": map[string]any{"$type": "app.bsky.feed.threadgate", "post": postURI, "createdAt": status.CreatedAt.UTC().Format(time.RFC3339Nano), "allow": []any{}},
	}, nil); err != nil {
		return fmt.Errorf("apply Bluesky reply restriction: %w", err)
	}
	return nil
}

func deleteATRecord(ctx context.Context, client *atclient.APIClient, repo, collection, rkey string, ignoreMissing bool) error {
	endpoint, _ := syntax.ParseNSID("com.atproto.repo.deleteRecord")
	err := client.Post(ctx, endpoint, map[string]any{"repo": repo, "collection": collection, "rkey": rkey}, nil)
	if ignoreMissing && err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "recordnotfound") || strings.Contains(message, "record not found") {
			return nil
		}
	}
	return err
}

func policyAllowsPublic(rules *gtsmodel.PolicyRules) bool {
	if rules == nil {
		return false
	}
	for _, value := range rules.AutomaticApproval {
		if value == gtsmodel.PolicyValuePublic {
			return true
		}
	}
	return false
}
