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
	} else if !errors.Is(err, db.ErrNoEntries) && err != nil {
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
	existing, existingErr := p.state.DB.GetBlueskyConnectionByAccountID(ctx, storedState.AccountID)
	if existingErr != nil && !errors.Is(existingErr, db.ErrNoEntries) {
		return "", gtserror.NewErrorInternalError(existingErr)
	}
	store.DeferSessionPersistence()
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

	if existingErr == nil && !sameBlueskyIdentity(existing, session.AccountDID.String()) {
		_ = app.Logout(ctx, session.AccountDID, session.SessionID)
		_ = store.DeleteSession(ctx, session.AccountDID, session.SessionID)
		return "", gtserror.NewErrorConflict(errors.New("different Bluesky identity"), "Reconnect the previously linked Bluesky account before switching identities")
	}
	now := time.Now()
	connection := &gtsmodel.BlueskyConnection{
		ID: id.NewULID(), AccountID: storedState.AccountID, DID: session.AccountDID.String(),
		Handle: identity.Handle.String(), PDSURL: session.HostURL,
		NotificationsSeenAt: now, OutboxCheckedAt: now,
		CrosspostPublic: false, ShowProfileFollow: true,
	}
	if existingErr == nil {
		connection = existing
		connection.Handle = identity.Handle.String()
		connection.PDSURL = session.HostURL
		if err := p.state.DB.UpdateBlueskyConnection(ctx, connection, "handle", "pds_url"); err != nil {
			return "", gtserror.NewErrorInternalError(err)
		}
	} else if err := p.state.DB.PutBlueskyConnection(ctx, connection); err != nil {
		return "", gtserror.NewErrorInternalError(err)
	}
	if err := store.PersistSession(ctx, *session); err != nil {
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
