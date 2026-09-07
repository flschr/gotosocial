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
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
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

// LockAccount serializes connection transitions with sync work in this process.
// Database compare-and-swap guards cover concurrent GoToSocial processes.
func LockAccount(accountID string) func() {
	return lockAccount(accountID)
}

// Disconnect first removes private proxy statuses, then revokes the active
// remote session when possible and removes local credentials. Post mappings
// remain so a later reconnect can resume exact edits and deletions.
func Disconnect(ctx context.Context, state *state.State, accountID string) error {
	defer lockAccount(accountID)()
	connection, err := state.DB.GetBlueskyConnectionByAccountID(ctx, accountID)
	if errors.Is(err, db.ErrNoEntries) {
		if err := stubProxyStatuses(ctx, state, accountID); err != nil {
			return err
		}
		return state.DB.DeleteBlueskyConnectionDataByAccountID(ctx, accountID)
	}
	if err != nil {
		return err
	}
	if err := stubProxyStatuses(ctx, state, accountID); err != nil {
		return err
	}
	for range 3 {
		revokeConnectionCredentialsDetached(ctx, state, connection)
		cleared, err := state.DB.ClearBlueskyConnectionData(ctx, connection)
		if err != nil {
			return err
		}
		if cleared {
			return nil
		}
		connection, err = state.DB.GetBlueskyConnectionByAccountID(ctx, accountID)
		if errors.Is(err, db.ErrNoEntries) {
			return state.DB.DeleteBlueskyConnectionDataByAccountID(ctx, accountID)
		}
		if err != nil {
			return err
		}
	}
	return ErrCredentialsChanged
}

func revokeConnectionCredentialsDetached(ctx context.Context, state *state.State, connection *gtsmodel.BlueskyConnection) {
	cleanupCtx, cancel := credentialCleanupContext(ctx)
	defer cancel()
	if len(connection.AppPasswordData) != 0 {
		_ = revokeAppPasswordData(cleanupCtx, state, connection.AccountID, connection.AppPasswordData)
	}
	if connection.OAuthSessionID == "" || len(connection.OAuthData) == 0 {
		return
	}
	did, err := syntax.ParseDID(connection.DID)
	if err != nil {
		return
	}
	app, store, err := NewOAuthClient(state, connection.AccountID)
	if err != nil {
		return
	}
	session, err := store.GetSession(cleanupCtx, did, connection.OAuthSessionID)
	if err == nil {
		_ = RevokeOAuthSession(cleanupCtx, app, *session)
	}
}

// Forget removes a disconnected account binding and all retained mappings.
// It intentionally leaves previously published records on Bluesky untouched.
func Forget(ctx context.Context, state *state.State, accountID string) error {
	defer lockAccount(accountID)()
	if err := stubProxyStatuses(ctx, state, accountID); err != nil {
		return err
	}
	return state.DB.DeleteBlueskyDataByAccountID(ctx, accountID)
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
			if err := deleteATRecord(ctx, client, connection.DID, "app.bsky.feed.post", rkey, true); err != nil {
				remoteErrors = append(remoteErrors, fmt.Errorf("delete %s: %w", post.URI, err))
			}
		}
	}
	if len(connection.AppPasswordData) != 0 {
		_ = logoutAppPassword(ctx, state, connection)
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
	return removeProxyStatuses(ctx, state, accountID, false)
}

func stubProxyStatuses(ctx context.Context, state *state.State, accountID string) error {
	return removeProxyStatuses(ctx, state, accountID, true)
}

func removeProxyStatuses(ctx context.Context, state *state.State, accountID string, stub bool) error {
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
		if err == nil && stub {
			err = state.DB.StubStatus(ctx, status, false)
		} else if err == nil {
			err = state.DB.DeleteStatus(ctx, status, false)
		}
		if err != nil {
			deleteErrors = append(deleteErrors, err)
		}
	}
	return errors.Join(deleteErrors...)
}
