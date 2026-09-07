// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

// SyncInteractions imports replies and mentions as private, local-only statuses.
func SyncInteractions(ctx context.Context, state *state.State) error {
	connections, err := state.DB.GetBlueskyConnections(ctx)
	if err != nil {
		return err
	}
	errChannel := make(chan error, len(connections))
	semaphore := make(chan struct{}, 4)
	var wait sync.WaitGroup
	for _, connection := range connections {
		wait.Add(1)
		go func(connection *gtsmodel.BlueskyConnection) {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				errChannel <- ctx.Err()
				return
			}
			now := time.Now()
			claimedUntil := now.Add(2 * time.Minute)
			claimed, err := state.DB.ClaimBlueskyConnection(ctx, connection.ID, now, claimedUntil)
			if err != nil {
				errChannel <- fmt.Errorf("claim %s: %w", connection.Handle, err)
				return
			}
			if !claimed {
				return
			}
			connection.SyncClaimedUntil = claimedUntil
			if err := syncConnectionWithLease(ctx, state, connection); err != nil {
				errChannel <- fmt.Errorf("%s: %w", connection.Handle, err)
			}
		}(connection)
	}
	wait.Wait()
	close(errChannel)
	var syncErrors []error
	for err := range errChannel {
		syncErrors = append(syncErrors, err)
	}
	return errors.Join(syncErrors...)
}

func syncConnectionWithLease(ctx context.Context, state *state.State, connection *gtsmodel.BlueskyConnection) error {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan time.Time, 1)
	go func() {
		expected := connection.SyncClaimedUntil
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		defer func() { done <- expected }()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				next := time.Now().Add(2 * time.Minute)
				renewed, err := state.DB.RenewBlueskyConnectionClaim(ctx, connection.ID, expected, next)
				if err != nil || !renewed {
					cancel()
					return
				}
				expected = next
			}
		}
	}()
	err := syncConnection(ctx, state, connection)
	cancel()
	expected := <-done
	if releaseErr := state.DB.ReleaseBlueskyConnectionClaim(context.WithoutCancel(ctx), connection.ID, expected); releaseErr != nil && err == nil {
		err = releaseErr
	}
	return err
}

func syncConnection(ctx context.Context, state *state.State, connection *gtsmodel.BlueskyConnection) (syncErr error) {
	defer lockAccount(connection.AccountID)()
	claimedID := connection.ID
	connection, err := state.DB.GetBlueskyConnectionByAccountID(ctx, connection.AccountID)
	if errors.Is(err, db.ErrNoEntries) {
		return nil
	}
	if err != nil {
		return err
	}
	if connection.ID != claimedID || !connection.Active() {
		return nil
	}
	defer func() {
		connection.LastSyncAt = time.Now()
		if syncErr != nil {
			connection.LastSyncError = truncateUTF8(syncErr.Error(), 1000, 4000)
			connection.LastSyncErrorCode = errorCode(syncErr)
		} else {
			connection.LastSyncError = ""
			connection.LastSyncErrorCode = ""
		}
		if err := state.DB.UpdateBlueskyConnection(ctx, connection, "last_sync_at", "last_sync_error", "last_sync_error_code"); err != nil && syncErr == nil {
			syncErr = err
		}
	}()
	if connection.LastSyncAt.IsZero() || time.Since(connection.LastSyncAt) >= time.Hour {
		if app, _, err := NewOAuthClient(state, connection.AccountID); err == nil {
			if did, err := syntax.ParseDID(connection.DID); err == nil {
				if identity, err := app.Dir.LookupDID(ctx, did); err == nil && identity.Handle.String() != connection.Handle {
					connection.Handle = identity.Handle.String()
					if err := state.DB.UpdateBlueskyConnection(ctx, connection, "handle"); err != nil {
						return err
					}
				}
			}
		}
	}
	client, err := authenticatedClient(ctx, state, connection)
	if err != nil {
		return err
	}
	endpoint, _ := syntax.ParseNSID("app.bsky.notification.listNotifications")
	client = client.WithService("did:web:api.bsky.app#bsky_appview")

	latest := connection.NotificationsSeenAt
	cursor := ""
	seenCursors := make(map[string]struct{})
	for {
		params := map[string]any{"limit": 100}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var page notificationPage
		if err := client.Get(ctx, endpoint, params, &page); err != nil {
			return fmt.Errorf("list notifications: %w", err)
		}
		stop := false
		for _, notification := range page.Notifications {
			// Include equal timestamps again: notification timestamps are not
			// unique, while URI de-duplication makes a small overlap harmless.
			if !connection.NotificationsSeenAt.IsZero() && notification.IndexedAt.Before(connection.NotificationsSeenAt) {
				stop = true
				break
			}
			if notification.IndexedAt.After(latest) {
				latest = notification.IndexedAt
			}
			if notification.Reason == "reply" || notification.Reason == "mention" {
				payload, marshalErr := json.Marshal(notification)
				if marshalErr != nil {
					return fmt.Errorf("encode notification inbox item: %w", marshalErr)
				}
				if err := state.DB.PutBlueskyNotification(ctx, &gtsmodel.BlueskyNotification{
					ID: id.NewULID(), CreatedAt: notification.IndexedAt, AccountID: connection.AccountID,
					URI: notification.URI, Payload: payload, NextAttemptAt: time.Now(),
				}); err != nil {
					return fmt.Errorf("store notification inbox item: %w", err)
				}
			}
		}
		if stop || page.Cursor == "" {
			break
		}
		if _, duplicate := seenCursors[page.Cursor]; duplicate {
			return fmt.Errorf("notification pagination returned repeated cursor")
		}
		seenCursors[page.Cursor] = struct{}{}
		cursor = page.Cursor
	}

	if latest.After(connection.NotificationsSeenAt) {
		connection.NotificationsSeenAt = latest
		if err := state.DB.UpdateBlueskyConnection(ctx, connection, "notifications_seen_at"); err != nil {
			return err
		}
	}
	return errors.Join(
		processNotificationInbox(ctx, state, connection),
		reconcileInteractions(ctx, state, connection, client),
	)
}
