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

	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

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
			item.LastError, item.UpdatedAt = truncateUTF8(err.Error(), 1000, 4000), time.Now()
			item.LastErrorCode = errorCode(err)
			item.DeadLetter = item.Attempts >= 10
			item.NextAttemptAt = time.Now().Add(retryDelay(item.Attempts))
			if updateErr := state.DB.UpdateBlueskyNotification(ctx, item, "attempts", "last_error", "last_error_code", "updated_at", "dead_letter", "next_attempt_at"); updateErr != nil {
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
	if connection.AuthMethod() == "app_password" {
		return newAppPasswordClient(state, connection)
	}
	app, store, err := NewOAuthClient(state, connection.AccountID)
	if err != nil {
		return nil, &ConnectionError{Code: ErrorCodeConfiguration, Err: err}
	}
	did, err := syntax.ParseDID(connection.DID)
	if err != nil {
		return nil, err
	}
	store.TrackConnection(connection)
	session, err := app.ResumeSession(ctx, did, connection.OAuthSessionID)
	if err != nil {
		return nil, &ConnectionError{Code: ErrorCodeAuth, Err: fmt.Errorf("resume OAuth session: %w", err)}
	}
	client := newATClient(state, connection.PDSURL)
	client.Auth, client.AccountDID = session, &did
	client.Headers.Set("User-Agent", "GoToSocial Plus")
	return client, nil
}
