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

package status

import (
	"context"
	"errors"

	"code.superseriousbusiness.org/gopkg/log"
	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/paging"
)

// QuotesGet returns a pageable response of statuses that quote the target
// status, visible to the requesting account. Paging is done on quoting-status
// ID.
func (p *Processor) QuotesGet(
	ctx context.Context,
	requestingAccount *gtsmodel.Account,
	targetStatusID string,
	page *paging.Page,
) (*apimodel.PageableResponse, gtserror.WithCode) {
	// Ensure target status exists and is visible to the requester.
	if _, errWithCode := p.c.GetVisibleTargetStatus(ctx,
		requestingAccount,
		targetStatusID,
		nil,
	); errWithCode != nil {
		return nil, errWithCode
	}

	quotes, err := p.state.DB.GetStatusQuotes(ctx, targetStatusID, page)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		err = gtserror.Newf("db error getting quotes of %s: %w", targetStatusID, err)
		return nil, gtserror.NewErrorInternalError(err)
	}

	count := len(quotes)
	if count == 0 {
		return paging.EmptyResponse(), nil
	}

	// Lowest and highest quoting-status
	// IDs in the returned page, for paging.
	lo := quotes[count-1].ID
	hi := quotes[0].ID

	items := make([]interface{}, 0, count)
	for _, quote := range quotes {
		// Only include quotes the requester is permitted to see.
		visible, err := p.visFilter.StatusVisible(ctx, requestingAccount, quote)
		if err != nil {
			log.Errorf(ctx, "error checking quote visibility: %v", err)
			continue
		}
		if !visible {
			continue
		}

		item, err := p.converter.StatusToAPIStatus(ctx, quote, requestingAccount)
		if err != nil {
			log.Errorf(ctx, "error converting quote to api: %v", err)
			continue
		}

		items = append(items, item)
	}

	return paging.PackageResponse(paging.ResponseParams{
		Items: items,
		Path:  "/api/v1/statuses/" + targetStatusID + "/quotes",
		Next:  page.Next(lo, hi),
		Prev:  page.Prev(lo, hi),
	}), nil
}
