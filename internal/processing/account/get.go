// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package account

import (
	"context"
	"errors"
	"net/url"

	"code.superseriousbusiness.org/gopkg/log"
	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtscontext"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/typeutils"
)

// Get processes the given request for account information.
func (p *Processor) Get(ctx context.Context, requestingAccount *gtsmodel.Account, targetAccountID string) (*apimodel.Account, gtserror.WithCode) {
	targetAccount, err := p.state.DB.GetAccountByID(ctx, targetAccountID)
	if err != nil {
		if errors.Is(err, db.ErrNoEntries) {
			interaction, interactionErr := p.state.DB.GetBlueskyInteractionByAuthorAccountID(ctx, targetAccountID, requestingAccount.ID)
			if interactionErr == nil {
				instanceAccount, instanceErr := p.state.DB.GetInstanceAccount(ctx, "")
				if instanceErr != nil {
					err := gtserror.Newf("db error getting instance account: %w", instanceErr)
					return nil, gtserror.NewErrorInternalError(err)
				}
				apiAccount, convertErr := p.converter.AccountToAPIAccountPublic(ctx, instanceAccount)
				if convertErr != nil {
					err := gtserror.Newf("error converting virtual Bluesky account: %w", convertErr)
					return nil, gtserror.NewErrorInternalError(err)
				}
				return typeutils.BlueskyInteractionAccount(interaction, apiAccount), nil
			}
			if interactionErr != nil && !errors.Is(interactionErr, db.ErrNoEntries) {
				err := gtserror.Newf("db error getting virtual Bluesky account: %w", interactionErr)
				return nil, gtserror.NewErrorInternalError(err)
			}
			err := gtserror.New("account not found")
			return nil, gtserror.NewErrorNotFound(err)
		}
		err := gtserror.Newf("db error getting account: %w", err)
		return nil, gtserror.NewErrorInternalError(err)
	}

	blocked, err := p.state.DB.IsEitherBlocked(ctx, requestingAccount.ID, targetAccount.ID)
	if err != nil {
		err := gtserror.Newf("db error checking blocks: %w", err)
		return nil, gtserror.NewErrorInternalError(err)
	}

	if blocked {
		apiAccount, err := p.converter.AccountToAPIAccountBlocked(ctx, targetAccount)
		if err != nil {
			err := gtserror.Newf("error converting account: %w", err)
			return nil, gtserror.NewErrorInternalError(err)
		}
		return apiAccount, nil
	}

	if targetAccount.Domain != "" {
		targetAccountURI, err := url.Parse(targetAccount.URI)
		if err != nil {
			err := gtserror.Newf("error parsing account URI: %w", err)
			return nil, gtserror.NewErrorInternalError(err)
		}

		// Perform a last-minute fetch of target account to
		// ensure remote account header / avatar is cached.
		//
		// Match by URI only.
		latest, _, err := p.federator.GetAccountByURI(
			gtscontext.SetFastFail(ctx),
			requestingAccount.Username,
			targetAccountURI,
			false,
		)
		if err != nil {
			log.Errorf(ctx, "error fetching latest target account: %v", err)
		} else {
			// Use latest account model.
			targetAccount = latest
		}
	}

	var apiAccount *apimodel.Account
	if targetAccount.ID == requestingAccount.ID {
		// This is requester's own account,
		// show additional details.
		apiAccount, err = p.converter.AccountToAPIAccountSensitive(ctx, targetAccount)
	} else {
		// This is a different account,
		// show the "public" view.
		apiAccount, err = p.converter.AccountToAPIAccountPublic(ctx, targetAccount)
	}
	if err != nil {
		err := gtserror.Newf("error converting account: %w", err)
		return nil, gtserror.NewErrorInternalError(err)
	}

	return apiAccount, nil
}

// GetWeb returns the web model of a local account by username.
func (p *Processor) GetWeb(ctx context.Context, username string) (*apimodel.WebAccount, gtserror.WithCode) {
	targetAccount, err := p.state.DB.GetAccountByUsernameDomain(ctx, username, "")
	if err != nil {
		if errors.Is(err, db.ErrNoEntries) {
			err := gtserror.New("account not found")
			return nil, gtserror.NewErrorNotFound(err)
		}
		err := gtserror.Newf("db error getting account: %w", err)
		return nil, gtserror.NewErrorInternalError(err)
	}

	webAccount, err := p.converter.AccountToWebAccount(ctx, targetAccount, nil)
	if err != nil {
		err := gtserror.Newf("error converting account: %w", err)
		return nil, gtserror.NewErrorInternalError(err)
	}

	connection, err := p.state.DB.GetBlueskyConnectionByAccountID(ctx, targetAccount.ID)
	if err == nil && showBlueskyProfile(connection) {
		webAccount.BlueskyHandle = connection.Handle
		webAccount.BlueskyURL = "https://bsky.app/profile/" + url.PathEscape(connection.DID)
	} else if err != nil && !errors.Is(err, db.ErrNoEntries) {
		err := gtserror.Newf("db error getting Bluesky connection: %w", err)
		return nil, gtserror.NewErrorInternalError(err)
	}

	return webAccount, nil
}

func showBlueskyProfile(connection *gtsmodel.BlueskyConnection) bool {
	return connection != nil && connection.Active() && connection.ShowProfileFollow
}

// GetCustomCSSForUsername returns custom css for the given local username.
func (p *Processor) GetCustomCSSForUsername(ctx context.Context, username string) (string, gtserror.WithCode) {
	customCSS, err := p.state.DB.GetAccountCustomCSSByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, db.ErrNoEntries) {
			return "", gtserror.NewErrorNotFound(gtserror.New("account not found"))
		}
		return "", gtserror.NewErrorInternalError(gtserror.Newf("db error: %w", err))
	}

	return customCSS, nil
}
