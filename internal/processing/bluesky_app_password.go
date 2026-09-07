// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package processing

import (
	"context"
	"errors"
	"strings"
	"time"

	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/bluesky"
	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

func (p *Processor) BlueskyAppPasswordConnect(ctx context.Context, accountID string, form *apimodel.BlueskyAppPasswordRequest) (*apimodel.BlueskyConnection, gtserror.WithCode) {
	if config.GetBlueskyOAuthEncryptionKey() == "" {
		return nil, gtserror.NewErrorInternalError(errors.New("Bluesky connection key is not configured"), "Bluesky connections are not configured on this server")
	}
	identifier := strings.TrimPrefix(strings.TrimSpace(form.Identifier), "@")
	parsedIdentifier, err := syntax.ParseAtIdentifier(identifier)
	if err != nil {
		return nil, gtserror.NewErrorUnprocessableEntity(err, "Enter a valid Bluesky handle, for example fischr.org")
	}
	password := strings.TrimSpace(form.AppPassword)
	if password == "" {
		return nil, gtserror.NewErrorBadRequest(errors.New("empty Bluesky app password"), "Enter the app password created in Bluesky settings")
	}

	app, _, err := p.blueskyOAuthApp(accountID)
	if err != nil {
		return nil, gtserror.NewErrorInternalError(err, "Bluesky connections are not configured on this server")
	}
	identity, err := app.Dir.Lookup(ctx, parsedIdentifier)
	if err != nil {
		return nil, gtserror.NewErrorUnprocessableEntity(err, "We could not find that Bluesky account. Check the handle and try again")
	}
	pdsURL := identity.PDSEndpoint()
	if pdsURL == "" {
		return nil, gtserror.NewErrorUnprocessableEntity(errors.New("Bluesky identity has no PDS"), "That Bluesky account has no login provider")
	}

	existing, existingErr := p.state.DB.GetBlueskyConnectionByAccountID(ctx, accountID)
	if existingErr != nil && !errors.Is(existingErr, db.ErrNoEntries) {
		return nil, gtserror.NewErrorInternalError(existingErr)
	}
	if existingErr == nil && !sameBlueskyIdentity(existing, identity.DID.String()) {
		return nil, gtserror.NewErrorConflict(bluesky.ErrIdentityMismatch, "Use an app password for the previously linked Bluesky account, or forget the saved account first")
	}

	encrypted, err := bluesky.CreateAppPasswordData(ctx, p.state, accountID, pdsURL, identity.DID.String(), password)
	if err != nil {
		if errors.Is(err, bluesky.ErrIdentityMismatch) {
			return nil, gtserror.NewErrorConflict(err, "The app password belongs to a different Bluesky account")
		}
		return nil, gtserror.NewErrorUnauthorized(err, "Bluesky rejected that app password. Create a new app password in Bluesky settings and try again")
	}
	now := time.Now()
	candidate := &gtsmodel.BlueskyConnection{
		ID: id.NewULID(), AccountID: accountID, DID: identity.DID.String(),
		Handle: identity.Handle.String(), PDSURL: pdsURL, NotificationsSeenAt: now, OutboxCheckedAt: now,
		CrosspostPublic: false, ShowProfileFollow: true,
	}
	if _, err := bluesky.ActivateAppPassword(ctx, p.state, candidate, encrypted); err != nil {
		if errors.Is(err, bluesky.ErrIdentityMismatch) {
			return nil, gtserror.NewErrorConflict(err, "Use an app password for the previously linked Bluesky account, or forget the saved account first")
		}
		return nil, gtserror.NewErrorInternalError(err)
	}
	if err := p.state.DB.RetryBlueskyFailures(ctx, accountID, now); err != nil {
		return nil, gtserror.NewErrorInternalError(err)
	}
	return p.BlueskyConnectionGet(ctx, accountID)
}
