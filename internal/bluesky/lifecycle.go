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

var ErrConnectionActive = errors.New("Bluesky account is connected")

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
		return stubProxyStatuses(ctx, state, accountID)
	}
	if err != nil {
		return err
	}
	if err := stubProxyStatuses(ctx, state, accountID); err != nil {
		return err
	}
	for range 3 {
		revokeConnectionCredentialsDetached(ctx, state, connection)
		cleanupCtx, cancel := credentialCleanupContext(ctx)
		cleared, err := state.DB.ClearBlueskyConnectionData(cleanupCtx, connection)
		if err != nil {
			cancel()
			return err
		}
		if cleared {
			cancel()
			return nil
		}
		connection, err = state.DB.GetBlueskyConnectionByAccountID(cleanupCtx, accountID)
		if errors.Is(err, db.ErrNoEntries) {
			cancel()
			return nil
		}
		cancel()
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
	for range 3 {
		connection, err := state.DB.GetBlueskyConnectionByAccountID(ctx, accountID)
		if errors.Is(err, db.ErrNoEntries) {
			return nil
		}
		if err != nil {
			return err
		}
		if connection.Active() {
			return ErrConnectionActive
		}
		if err := stubProxyStatuses(ctx, state, accountID); err != nil {
			return err
		}
		deleted, err := state.DB.DeleteBlueskyData(ctx, connection)
		if err != nil {
			return err
		}
		if deleted {
			return nil
		}
	}
	return ErrCredentialsChanged
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
	remoteErrors = append(remoteErrors, deleteProxyStatuses(ctx, state, accountID))
	for range 3 {
		cleanupCtx, cancel := credentialCleanupContext(ctx)
		connection, err = state.DB.GetBlueskyConnectionByAccountID(cleanupCtx, accountID)
		if errors.Is(err, db.ErrNoEntries) {
			remoteErrors = append(remoteErrors, state.DB.DeleteBlueskyDataByAccountID(cleanupCtx, accountID))
			cancel()
			return errors.Join(remoteErrors...)
		}
		if err != nil {
			cancel()
			remoteErrors = append(remoteErrors, err)
			return errors.Join(remoteErrors...)
		}
		cancel()
		revokeConnectionCredentialsDetached(ctx, state, connection)
		cleanupCtx, cancel = credentialCleanupContext(ctx)
		deleted, err := state.DB.DeleteBlueskyData(cleanupCtx, connection)
		cancel()
		if err != nil {
			remoteErrors = append(remoteErrors, err)
			return errors.Join(remoteErrors...)
		}
		if deleted {
			return errors.Join(remoteErrors...)
		}
	}
	remoteErrors = append(remoteErrors, ErrCredentialsChanged)
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
