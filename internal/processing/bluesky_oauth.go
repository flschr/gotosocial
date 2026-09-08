// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package processing

import (
	"context"
	"errors"
	"net/url"
	"strings"
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
	identifier, err := normalizeBlueskyIdentifier(identifier)
	if err != nil {
		return nil, gtserror.NewErrorUnprocessableEntity(err, "Enter a valid Bluesky handle, for example fischr.org")
	}
	app, _, err := p.blueskyOAuthApp(accountID)
	if err != nil {
		return nil, gtserror.NewErrorInternalError(err, "Bluesky OAuth is not configured on this instance")
	}
	authorizationURL, err := app.StartAuthFlow(ctx, identifier)
	if err != nil {
		return nil, gtserror.NewErrorUnprocessableEntity(err, "We could not find that Bluesky account or reach its login provider. Check the handle and try again")
	}
	return &apimodel.BlueskyConnectResponse{AuthorizationURL: authorizationURL}, nil
}

func normalizeBlueskyIdentifier(identifier string) (string, error) {
	identifier = strings.TrimSpace(identifier)
	identifier = strings.TrimPrefix(identifier, "@")
	if strings.HasPrefix(identifier, "https://") {
		return identifier, nil
	}
	parsed, err := syntax.ParseAtIdentifier(identifier)
	if err != nil {
		return "", err
	}
	return parsed.String(), nil
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
	defer bluesky.LockAccount(storedState.AccountID)()

	app, store, err := p.blueskyOAuthApp(storedState.AccountID)
	if err != nil {
		return "", gtserror.NewErrorInternalError(err)
	}
	existing, existingErr := p.state.DB.GetBlueskyConnectionByAccountID(ctx, storedState.AccountID)
	if existingErr != nil && !errors.Is(existingErr, db.ErrNoEntries) {
		return "", gtserror.NewErrorInternalError(existingErr)
	}
	if existingErr == nil && existing.Active() {
		_ = p.state.DB.DeleteBlueskyOAuthState(ctx, state)
		return "", gtserror.NewErrorConflict(errors.New("Bluesky account already connected"), "Another Bluesky connection completed before this authorization")
	}
	store.DeferSessionPersistence()
	session, err := app.ProcessCallback(ctx, params)
	if err != nil {
		if errors.Is(err, bluesky.ErrIdentityMismatch) {
			return "", gtserror.NewErrorConflict(err, "Reconnect the previously linked Bluesky account before switching identities")
		}
		return "", gtserror.NewErrorBadRequest(err, "Bluesky authorization failed")
	}
	persisted := false
	defer func() {
		if !persisted {
			_ = bluesky.RevokeOAuthSessionDetached(ctx, app, *session)
		}
	}()
	identity, err := app.Dir.LookupDID(ctx, session.AccountDID)
	if err != nil {
		return "", gtserror.NewErrorUnprocessableEntity(err, "could not resolve the connected Bluesky identity")
	}

	if existingErr == nil && !sameBlueskyIdentity(existing, session.AccountDID.String()) {
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
	} else if inserted, putErr := p.state.DB.PutBlueskyConnectionIfAccountExists(ctx, connection); putErr != nil || !inserted {
		current, currentErr := p.state.DB.GetBlueskyConnectionByAccountID(ctx, storedState.AccountID)
		if currentErr == nil && current.Active() {
			if !oauthRaceWinnerMatches(current, *session) {
				return "", gtserror.NewErrorConflict(errors.New("different Bluesky identity"), "Another Bluesky identity completed first; reload before trying again")
			}
			baseURL, _, _ := bluesky.OAuthURLs()
			return baseURL + "/settings/user/bluesky?connected=true", nil
		}
		if currentErr == nil {
			return "", gtserror.NewErrorConflict(errors.Join(putErr, bluesky.ErrCredentialsChanged), "Another Bluesky connection is finishing; reload and try again if needed")
		}
		if putErr == nil && errors.Is(currentErr, db.ErrNoEntries) {
			return "", gtserror.NewErrorConflict(errors.New("local account no longer exists"), "The local account changed while Bluesky authorization was finishing")
		}
		return "", gtserror.NewErrorInternalError(errors.Join(putErr, currentErr))
	}
	if err := store.PersistSession(ctx, *session); err != nil {
		if existingErr != nil {
			_, _ = p.state.DB.DeleteInactiveBlueskyConnection(ctx, connection.ID)
		}
		if errors.Is(err, bluesky.ErrCredentialsChanged) {
			current, currentErr := p.state.DB.GetBlueskyConnectionByAccountID(ctx, storedState.AccountID)
			if currentErr == nil && current.Active() {
				if !oauthRaceWinnerMatches(current, *session) {
					return "", gtserror.NewErrorConflict(errors.New("different Bluesky identity"), "Another Bluesky identity completed first; reload before trying again")
				}
				baseURL, _, _ := bluesky.OAuthURLs()
				return baseURL + "/settings/user/bluesky?connected=true", nil
			}
			return "", gtserror.NewErrorConflict(err, "Another Bluesky connection completed first; reload and try again if needed")
		}
		return "", gtserror.NewErrorInternalError(err)
	}
	persisted = true

	baseURL, _, _ := bluesky.OAuthURLs()
	return baseURL + "/settings/user/bluesky?connected=true", nil
}

func sameBlueskyIdentity(connection *gtsmodel.BlueskyConnection, did string) bool {
	return connection != nil && connection.DID == did
}

func oauthRaceWinnerMatches(connection *gtsmodel.BlueskyConnection, session oauth.ClientSessionData) bool {
	return connection.Active() && sameBlueskyIdentity(connection, session.AccountDID.String())
}
