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
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"github.com/bluesky-social/indigo/atproto/auth/oauth"
)

const blueskyOAuthStateLifetime = 10 * time.Minute

func (p *Processor) blueskyOAuthApp(accountID string) (*oauth.ClientApp, *bluesky.OAuthStore, error) {
	return bluesky.NewOAuthClient(p.state, accountID)
}

func (p *Processor) BlueskyClientMetadata() (*oauth.ClientMetadata, error) {
	clientConfig := bluesky.OAuthClientConfig()
	metadata := clientConfig.ClientMetadata()
	clientName := "GoToSocial Plus"
	baseURL, _, _ := bluesky.OAuthURLs()
	metadata.ClientName = &clientName
	metadata.ClientURI = &baseURL
	return &metadata, nil
}

func (p *Processor) BlueskyConnectStart(ctx context.Context, accountID, identifier string) (*apimodel.BlueskyConnectResponse, gtserror.WithCode) {
	if connection, err := p.state.DB.GetBlueskyConnectionByAccountID(ctx, accountID); err == nil && connection.Active() {
		return nil, gtserror.NewErrorConflict(errors.New("Bluesky account already connected"))
	} else if !errors.Is(err, db.ErrNoEntries) {
		if err != nil {
			return nil, gtserror.NewErrorInternalError(err)
		}
	}
	app, _, err := p.blueskyOAuthApp(accountID)
	if err != nil {
		return nil, gtserror.NewErrorInternalError(err, "Bluesky OAuth is not configured on this instance")
	}
	authorizationURL, err := app.StartAuthFlow(ctx, identifier)
	if err != nil {
		return nil, gtserror.NewErrorUnprocessableEntity(err, "could not start Bluesky authorization")
	}
	return &apimodel.BlueskyConnectResponse{AuthorizationURL: authorizationURL}, nil
}

func (p *Processor) BlueskyConnectCallback(ctx context.Context, params url.Values) (string, gtserror.WithCode) {
	state := params.Get("state")
	storedState, err := p.state.DB.GetBlueskyOAuthState(ctx, state)
	if err != nil {
		return "", gtserror.NewErrorBadRequest(err, "invalid or expired Bluesky authorization state")
	}
	if time.Since(storedState.CreatedAt) > blueskyOAuthStateLifetime {
		_ = p.state.DB.DeleteBlueskyOAuthState(ctx, state)
		return "", gtserror.NewErrorBadRequest(errors.New("expired OAuth state"), "Bluesky authorization expired; please try again")
	}

	app, store, err := p.blueskyOAuthApp(storedState.AccountID)
	if err != nil {
		return "", gtserror.NewErrorInternalError(err)
	}
	session, err := app.ProcessCallback(ctx, params)
	if err != nil {
		if errors.Is(err, bluesky.ErrIdentityMismatch) {
			return "", gtserror.NewErrorConflict(err, "Reconnect the previously linked Bluesky account before switching identities")
		}
		return "", gtserror.NewErrorBadRequest(err, "Bluesky authorization failed")
	}
	identity, err := app.Dir.LookupDID(ctx, session.AccountDID)
	if err != nil {
		return "", gtserror.NewErrorUnprocessableEntity(err, "could not resolve the connected Bluesky identity")
	}

	existing, existingErr := p.state.DB.GetBlueskyConnectionByAccountID(ctx, storedState.AccountID)
	if existingErr == nil && !sameBlueskyIdentity(existing, session.AccountDID.String()) {
		_ = app.Logout(ctx, session.AccountDID, session.SessionID)
		_ = store.DeleteSession(ctx, session.AccountDID, session.SessionID)
		return "", gtserror.NewErrorConflict(errors.New("different Bluesky identity"), "Reconnect the previously linked Bluesky account before switching identities")
	}
	if existingErr != nil && !errors.Is(existingErr, db.ErrNoEntries) {
		return "", gtserror.NewErrorInternalError(existingErr)
	}

	connection := &gtsmodel.BlueskyConnection{
		ID:                  id.NewULID(),
		AccountID:           storedState.AccountID,
		DID:                 session.AccountDID.String(),
		Handle:              identity.Handle.String(),
		PDSURL:              session.HostURL,
		NotificationsSeenAt: time.Now(),
		CrosspostPublic:     false,
		ShowProfileFollow:   true,
	}
	if existingErr == nil {
		connection = existing
		connection.Handle = identity.Handle.String()
		connection.PDSURL = session.HostURL
		if err := p.state.DB.UpdateBlueskyConnection(ctx, connection, "handle", "pds_url"); err != nil {
			return "", gtserror.NewErrorInternalError(err)
		}
	} else {
		if err := p.state.DB.PutBlueskyConnection(ctx, connection); err != nil {
			return "", gtserror.NewErrorInternalError(err)
		}
	}
	if err := store.SaveSession(ctx, *session); err != nil {
		if existingErr != nil {
			_ = p.state.DB.DeleteBlueskyConnection(ctx, connection.ID)
		}
		return "", gtserror.NewErrorInternalError(err)
	}

	baseURL, _, _ := bluesky.OAuthURLs()
	return baseURL + "/settings/user/bluesky?connected=true", nil
}

func sameBlueskyIdentity(connection *gtsmodel.BlueskyConnection, did string) bool {
	return connection != nil && connection.DID == did
}

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
