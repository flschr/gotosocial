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

	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

type notificationPage struct {
	Cursor        string                `json:"cursor"`
	Notifications []blueskyNotification `json:"notifications"`
}

type blueskyNotification struct {
	URI       string          `json:"uri"`
	CID       string          `json:"cid"`
	Reason    string          `json:"reason"`
	IndexedAt time.Time       `json:"indexedAt"`
	Author    blueskyAuthor   `json:"author"`
	Record    json.RawMessage `json:"record"`
}

type blueskyAuthor struct {
	DID         string `json:"did"`
	Handle      string `json:"handle"`
	DisplayName string `json:"displayName"`
}

type blueskyPostRecord struct {
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
	Facets    []struct {
		Index    facetIndex `json:"index"`
		Features []struct {
			Type string `json:"$type"`
			URI  string `json:"uri"`
			DID  string `json:"did"`
			Tag  string `json:"tag"`
		} `json:"features"`
	} `json:"facets"`
	Embed *struct {
		Images []struct {
			Alt   string `json:"alt"`
			Image struct {
				Ref struct {
					Link string `json:"$link"`
				} `json:"ref"`
			} `json:"image"`
		} `json:"images"`
	} `json:"embed"`
	Reply *struct {
		Root   blueskyStrongRef `json:"root"`
		Parent blueskyStrongRef `json:"parent"`
	} `json:"reply"`
}

type blueskyStrongRef struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}

type blueskyPostsResponse struct {
	Posts []struct {
		URI    string          `json:"uri"`
		CID    string          `json:"cid"`
		Author blueskyAuthor   `json:"author"`
		Record json.RawMessage `json:"record"`
	} `json:"posts"`
}

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
			if err := syncConnection(ctx, state, connection); err != nil {
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

func syncConnection(ctx context.Context, state *state.State, connection *gtsmodel.BlueskyConnection) (syncErr error) {
	defer lockAccount(connection.AccountID)()
	defer func() {
		connection.LastSyncAt = time.Now()
		if syncErr != nil {
			connection.LastSyncError = truncateUTF8(syncErr.Error(), 1000, 4000)
		} else {
			connection.LastSyncError = ""
		}
		if err := state.DB.UpdateBlueskyConnection(ctx, connection, "last_sync_at", "last_sync_error"); err != nil && syncErr == nil {
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

func processNotificationInbox(ctx context.Context, state *state.State, connection *gtsmodel.BlueskyConnection) error {
	var processingErrors []error
	for {
		items, err := state.DB.GetDueBlueskyNotifications(ctx, connection.AccountID, time.Now(), 100)
		if err != nil {
			return errors.Join(append(processingErrors, err)...)
		}
		if len(items) == 0 {
			return errors.Join(processingErrors...)
		}
		for _, item := range items {
			var notification blueskyNotification
			err := json.Unmarshal(item.Payload, &notification)
			if err == nil {
				err = importNotification(ctx, state, connection, notification)
			}
			if err == nil {
				if deleteErr := state.DB.DeleteBlueskyNotification(ctx, item.ID); deleteErr != nil {
					processingErrors = append(processingErrors, deleteErr)
				}
				continue
			}
			item.Attempts++
			item.LastError = truncateUTF8(err.Error(), 1000, 4000)
			item.DeadLetter = item.Attempts >= 10
			item.NextAttemptAt = time.Now().Add(time.Minute * time.Duration(1<<min(item.Attempts-1, 6)))
			if updateErr := state.DB.UpdateBlueskyNotification(ctx, item, "attempts", "last_error", "dead_letter", "next_attempt_at"); updateErr != nil {
				processingErrors = append(processingErrors, updateErr)
			} else {
				processingErrors = append(processingErrors, fmt.Errorf("notification %s: %w", item.URI, err))
			}
		}
		if len(items) < 100 {
			return errors.Join(processingErrors...)
		}
	}
}

func authenticatedClient(ctx context.Context, state *state.State, connection *gtsmodel.BlueskyConnection) (*atclient.APIClient, error) {
	app, _, err := NewOAuthClient(state, connection.AccountID)
	if err != nil {
		return nil, err
	}
	did, err := syntax.ParseDID(connection.DID)
	if err != nil {
		return nil, err
	}
	session, err := app.ResumeSession(ctx, did, connection.OAuthSessionID)
	if err != nil {
		return nil, fmt.Errorf("resume Bluesky OAuth session: %w", err)
	}
	client := newATClient(state, connection.PDSURL)
	client.Auth = session
	client.AccountDID = &did
	client.Headers.Set("User-Agent", "GoToSocial Plus")
	return client, nil
}
