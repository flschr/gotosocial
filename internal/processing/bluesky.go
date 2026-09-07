// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package processing

import (
	"context"
	"errors"
	"net/url"
	"time"

	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/bluesky"
	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
)

func (p *Processor) BlueskyDisconnect(ctx context.Context, accountID string) gtserror.WithCode {
	if err := bluesky.Disconnect(ctx, p.state, accountID); err != nil {
		return gtserror.NewErrorInternalError(err)
	}
	return nil
}

func (p *Processor) BlueskyForget(ctx context.Context, accountID string) gtserror.WithCode {
	connection, err := p.state.DB.GetBlueskyConnectionByAccountID(ctx, accountID)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		return gtserror.NewErrorInternalError(err)
	}
	if err == nil && connection.Active() {
		return gtserror.NewErrorConflict(errors.New("Bluesky account is connected"), "disconnect Bluesky before forgetting the saved account")
	}
	if err := bluesky.Forget(ctx, p.state, accountID); err != nil {
		return gtserror.NewErrorInternalError(err)
	}
	return nil
}

func (p *Processor) BlueskyConnectionGet(ctx context.Context, accountID string) (*apimodel.BlueskyConnection, gtserror.WithCode) {
	connection, err := p.state.DB.GetBlueskyConnectionByAccountID(ctx, accountID)
	if errors.Is(err, db.ErrNoEntries) {
		return &apimodel.BlueskyConnection{Configured: config.GetBlueskyOAuthEncryptionKey() != "", Status: "disconnected"}, nil
	}
	if err != nil {
		return nil, gtserror.NewErrorInternalError(err)
	}

	health, err := p.state.DB.GetBlueskyHealth(ctx, accountID)
	if err != nil {
		return nil, gtserror.NewErrorInternalError(err)
	}
	if health.LastError == "" && connection.LastSyncError != "" {
		health.LastError = connection.LastSyncError
		health.LastErrorCode = connection.LastSyncErrorCode
		health.LastErrorAt = connection.LastSyncAt
	}
	status, statusMessage, needsReconnect := blueskyConnectionStatus(connection, health, config.GetBlueskyOAuthEncryptionKey() != "")
	return &apimodel.BlueskyConnection{
		Connected:         connection.Active(),
		AuthMethod:        connection.AuthMethod(),
		Configured:        config.GetBlueskyOAuthEncryptionKey() != "",
		Status:            status,
		StatusMessage:     statusMessage,
		NeedsReconnect:    needsReconnect,
		Handle:            connection.Handle,
		ProfileURL:        "https://bsky.app/profile/" + url.PathEscape(connection.DID),
		CrosspostPublic:   connection.CrosspostPublic,
		ShowProfileFollow: connection.ShowProfileFollow,
		PendingDeliveries: health.PendingDeliveries,
		DeadDeliveries:    health.DeadDeliveries,
		DeadNotifications: health.DeadNotifications,
		LastErrorAt:       health.LastErrorAt,
		LastSyncAt:        connection.LastSyncAt,
	}, nil
}

func blueskyConnectionStatus(connection *gtsmodel.BlueskyConnection, health *gtsmodel.BlueskyHealth, configured bool) (string, string, bool) {
	if !configured {
		return "action_required", "Bluesky is unavailable because this server is missing its connection key. Contact the server administrator.", false
	}
	if !connection.Active() {
		return "disconnected", "Bluesky is disconnected. Reconnect the saved account to resume syncing.", true
	}
	if health.LastErrorCode == bluesky.ErrorCodeAuth {
		if connection.AuthMethod() == "app_password" {
			return "action_required", "The saved Bluesky app password is no longer valid. Replace it to resume syncing.", true
		}
		return "action_required", "Bluesky authorization is no longer valid. Disconnect and reconnect the account to resume syncing.", true
	}
	if health.LastErrorCode == bluesky.ErrorCodeConfiguration {
		return "action_required", "Bluesky credentials cannot be read with the server's current connection key. Contact the server administrator.", false
	}
	if health.DeadDeliveries+health.DeadNotifications > 0 {
		return "error", "Some Bluesky items could not be synced. Try the sync again; reconnect only if the problem continues.", false
	}
	if health.PendingDeliveries > 0 {
		return "syncing", "Bluesky is connected. Outgoing posts are waiting to be synced.", false
	}
	if connection.LastSyncError != "" {
		return "error", "Bluesky could not be reached during the last check. GoToSocial will try again automatically.", false
	}
	return "healthy", "Bluesky is connected and syncing normally.", false
}

func (p *Processor) BlueskyRetry(ctx context.Context, accountID string) gtserror.WithCode {
	if err := p.state.DB.RetryBlueskyFailures(ctx, accountID, time.Now()); err != nil {
		return gtserror.NewErrorInternalError(err)
	}
	return nil
}

func (p *Processor) BlueskySettingsUpdate(ctx context.Context, accountID string, form *apimodel.BlueskySettingsUpdateRequest) (*apimodel.BlueskyConnection, gtserror.WithCode) {
	connection, err := p.state.DB.GetBlueskyConnectionByAccountID(ctx, accountID)
	if errors.Is(err, db.ErrNoEntries) {
		return nil, gtserror.NewErrorConflict(err, "connect a Bluesky account before changing its settings")
	}
	if err != nil {
		return nil, gtserror.NewErrorInternalError(err)
	}

	columns := make([]string, 0, 2)
	if form.CrosspostPublic != nil {
		if *form.CrosspostPublic && !connection.CrosspostPublic {
			now := time.Now()
			connection.OutboxCheckedAt = now
			connection.CrosspostEnabledAt = now
			columns = append(columns, "outbox_checked_at", "crosspost_enabled_at")
		}
		connection.CrosspostPublic = *form.CrosspostPublic
		columns = append(columns, "crosspost_public")
	}
	if form.ShowProfileFollow != nil {
		connection.ShowProfileFollow = *form.ShowProfileFollow
		columns = append(columns, "show_profile_follow")
	}
	if len(columns) != 0 {
		if err := p.state.DB.UpdateBlueskyConnection(ctx, connection, columns...); err != nil {
			return nil, gtserror.NewErrorInternalError(err)
		}
	}

	return p.BlueskyConnectionGet(ctx, accountID)
}
