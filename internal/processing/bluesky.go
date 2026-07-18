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
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

const blueskyOAuthStateLifetime = 10 * time.Minute

func (p *Processor) blueskyOAuthApp(accountID string) (*oauth.ClientApp, *bluesky.OAuthStore, error) {
	return bluesky.NewOAuthClient(p.state.DB, accountID)
}

func (p *Processor) BlueskyClientMetadata() (*oauth.ClientMetadata, error) {
	app, _, err := p.blueskyOAuthApp("")
	if err != nil {
		return nil, err
	}
	metadata := app.Config.ClientMetadata()
	clientName := "GoToSocial Plus"
	baseURL, _, _ := bluesky.OAuthURLs()
	metadata.ClientName = &clientName
	metadata.ClientURI = &baseURL
	return &metadata, nil
}

func (p *Processor) BlueskyConnectStart(ctx context.Context, accountID, identifier string) (*apimodel.BlueskyConnectResponse, gtserror.WithCode) {
	if _, err := p.state.DB.GetBlueskyConnectionByAccountID(ctx, accountID); err == nil {
		return nil, gtserror.NewErrorConflict(errors.New("Bluesky account already connected"))
	} else if !errors.Is(err, db.ErrNoEntries) {
		return nil, gtserror.NewErrorInternalError(err)
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
		return "", gtserror.NewErrorBadRequest(err, "Bluesky authorization failed")
	}
	identity, err := app.Dir.LookupDID(ctx, session.AccountDID)
	if err != nil {
		return "", gtserror.NewErrorUnprocessableEntity(err, "could not resolve the connected Bluesky identity")
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
	if err := p.state.DB.PutBlueskyConnection(ctx, connection); err != nil {
		return "", gtserror.NewErrorInternalError(err)
	}
	if err := store.SaveSession(ctx, *session); err != nil {
		_ = p.state.DB.DeleteBlueskyConnection(ctx, connection.ID)
		return "", gtserror.NewErrorInternalError(err)
	}

	baseURL, _, _ := bluesky.OAuthURLs()
	return baseURL + "/settings/user/bluesky?connected=true", nil
}

func (p *Processor) BlueskyDisconnect(ctx context.Context, accountID string) gtserror.WithCode {
	connection, err := p.state.DB.GetBlueskyConnectionByAccountID(ctx, accountID)
	if errors.Is(err, db.ErrNoEntries) {
		return nil
	}
	if err != nil {
		return gtserror.NewErrorInternalError(err)
	}
	app, _, err := p.blueskyOAuthApp(accountID)
	if err == nil && connection.OAuthSessionID != "" {
		// Revocation is best-effort inside the official client. Local removal
		// still proceeds so disconnect always has a deterministic result.
		if did, parseErr := syntax.ParseDID(connection.DID); parseErr == nil {
			_ = app.Logout(ctx, did, connection.OAuthSessionID)
		}
	}
	if err := p.state.DB.DeleteBlueskyConnection(ctx, connection.ID); err != nil {
		return gtserror.NewErrorInternalError(err)
	}
	return nil
}

func (p *Processor) BlueskyConnectionGet(ctx context.Context, accountID string) (*apimodel.BlueskyConnection, gtserror.WithCode) {
	connection, err := p.state.DB.GetBlueskyConnectionByAccountID(ctx, accountID)
	if errors.Is(err, db.ErrNoEntries) {
		return &apimodel.BlueskyConnection{}, nil
	}
	if err != nil {
		return nil, gtserror.NewErrorInternalError(err)
	}

	return &apimodel.BlueskyConnection{
		Connected:         true,
		Handle:            connection.Handle,
		ProfileURL:        "https://bsky.app/profile/" + url.PathEscape(connection.Handle),
		CrosspostPublic:   connection.CrosspostPublic,
		ShowProfileFollow: connection.ShowProfileFollow,
	}, nil
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
