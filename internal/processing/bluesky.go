// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package processing

import (
	"context"
	"errors"
	"net/url"

	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
)

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
