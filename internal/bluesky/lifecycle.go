// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

var accountLocks sync.Map

func lockAccount(accountID string) func() {
	value, _ := accountLocks.LoadOrStore(accountID, new(sync.Mutex))
	mutex := value.(*sync.Mutex)
	mutex.Lock()
	return mutex.Unlock
}

// Disconnect revokes the active OAuth session when possible and always
// removes every locally stored Bluesky credential and mapping for accountID.
func Disconnect(ctx context.Context, state *state.State, accountID string) error {
	defer lockAccount(accountID)()
	connection, err := state.DB.GetBlueskyConnectionByAccountID(ctx, accountID)
	if errors.Is(err, db.ErrNoEntries) {
		return errors.Join(deleteProxyStatuses(ctx, state, accountID), state.DB.DeleteBlueskyDataByAccountID(ctx, accountID))
	}
	if err != nil {
		return err
	}
	if app, _, appErr := NewOAuthClient(state, accountID); appErr == nil && connection.OAuthSessionID != "" {
		if did, parseErr := syntax.ParseDID(connection.DID); parseErr == nil {
			_ = app.Logout(ctx, did, connection.OAuthSessionID)
		}
	}
	proxyErr := deleteProxyStatuses(ctx, state, accountID)
	return errors.Join(proxyErr, state.DB.DeleteBlueskyDataByAccountID(ctx, accountID))
}

// DeleteAccount removes crossposts created by GoToSocial before revoking and
// erasing the connection. Local credential cleanup still happens if a remote
// record cannot be removed.
func DeleteAccount(ctx context.Context, state *state.State, accountID string) error {
	defer lockAccount(accountID)()
	connection, err := state.DB.GetBlueskyConnectionByAccountID(ctx, accountID)
	if errors.Is(err, db.ErrNoEntries) {
		return errors.Join(deleteProxyStatuses(ctx, state, accountID), state.DB.DeleteBlueskyDataByAccountID(ctx, accountID))
	}
	if err != nil {
		return err
	}
	var remoteErrors []error
	client, clientErr := authenticatedClient(ctx, state, connection)
	posts, postsErr := state.DB.GetBlueskyPostsByAccountID(ctx, accountID)
	if clientErr != nil {
		remoteErrors = append(remoteErrors, clientErr)
	} else if postsErr != nil {
		remoteErrors = append(remoteErrors, postsErr)
	} else {
		for _, post := range posts {
			rkey := post.URI[strings.LastIndex(post.URI, "/")+1:]
			if err := deleteATRecord(ctx, client, connection.DID, "app.bsky.feed.threadgate", rkey, true); err != nil {
				remoteErrors = append(remoteErrors, fmt.Errorf("delete threadgate for %s: %w", post.URI, err))
			}
			if err := deleteATRecord(ctx, client, connection.DID, "app.bsky.feed.post", rkey, false); err != nil {
				remoteErrors = append(remoteErrors, fmt.Errorf("delete %s: %w", post.URI, err))
			}
		}
	}
	if app, _, appErr := NewOAuthClient(state, accountID); appErr == nil && connection.OAuthSessionID != "" {
		if did, parseErr := syntax.ParseDID(connection.DID); parseErr == nil {
			_ = app.Logout(ctx, did, connection.OAuthSessionID)
		}
	}
	remoteErrors = append(remoteErrors, deleteProxyStatuses(ctx, state, accountID))
	remoteErrors = append(remoteErrors, state.DB.DeleteBlueskyDataByAccountID(ctx, accountID))
	return errors.Join(remoteErrors...)
}

func deleteProxyStatuses(ctx context.Context, state *state.State, accountID string) error {
	interactions, err := state.DB.GetBlueskyInteractionsForReconcile(ctx, accountID, 0)
	if err != nil {
		return err
	}
	var deleteErrors []error
	for _, interaction := range interactions {
		status, err := state.DB.GetStatusByID(ctx, interaction.StatusID)
		if errors.Is(err, db.ErrNoEntries) {
			continue
		}
		if err == nil {
			err = state.DB.DeleteStatus(ctx, status, false)
		}
		if err != nil {
			deleteErrors = append(deleteErrors, err)
		}
	}
	return errors.Join(deleteErrors...)
}
