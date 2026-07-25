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

package timeline

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"code.superseriousbusiness.org/gopkg/log"
	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	apiutil "code.superseriousbusiness.org/gotosocial/internal/api/util"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/paging"
	"code.superseriousbusiness.org/gotosocial/internal/util"
)

// NotificationsGet ...
func (p *Processor) NotificationsGet(
	ctx context.Context,
	requester *gtsmodel.Account,
	page *paging.Page,
	types []gtsmodel.NotificationType,
	excludeTypes []gtsmodel.NotificationType,
) (*apimodel.PageableResponse, gtserror.WithCode) {
	notifs, err := p.state.DB.GetAccountNotifications(ctx,
		requester.ID,
		page,
		types,
		excludeTypes,
	)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		err = fmt.Errorf("NotificationsGet: db error getting notifications: %w", err)
		return nil, gtserror.NewErrorInternalError(err)
	}

	count := len(notifs)
	if count == 0 {
		return util.EmptyPageableResponse(), nil
	}

	var (
		items = make([]interface{}, 0, count)

		// Get the lowest and highest
		// ID values, used for paging.
		lo = notifs[count-1].ID
		hi = notifs[0].ID
	)

	for _, n := range notifs {
		visible, err := p.notifVisible(ctx, n, requester)
		if err != nil {
			log.Debugf(ctx, "skipping notification %s because of an error checking notification visibility: %v", n.ID, err)
			continue
		}

		if !visible {
			continue
		}

		// Check whether notification origin account is muted.
		muted, err := p.muteFilter.AccountNotificationsMuted(ctx,
			requester.ID,
			n.OriginAccountID,
		)
		if err != nil {
			log.Errorf(ctx, "error checking account mute: %v", err)
			continue
		}

		if muted {
			continue
		}

		var filtered []apimodel.FilterResult

		if n.Status != nil {
			var hide bool

			// Check whether notification status is muted by requester.
			muted, err = p.muteFilter.StatusNotificationsMuted(ctx,
				requester,
				n.Status,
			)
			if err != nil {
				log.Errorf(ctx, "error checking status mute: %v", err)
				continue
			}

			if muted {
				continue
			}

			// Check whether notification status is filtered by requester in notifs.
			filtered, hide, err = p.statusFilter.StatusFilterResultsInContext(ctx,
				requester,
				n.Status,
				gtsmodel.FilterContextNotifications,
			)
			if err != nil {
				log.Errorf(ctx, "error checking status filtering: %v", err)
				continue
			}

			if hide {
				continue
			}
		}

		item, err := p.converter.NotificationToAPINotification(ctx, n)
		if err != nil {
			continue
		}

		if item.Status != nil {
			// Set filter results on status,
			// in case any were set above.
			item.Status.Filtered = filtered
		}

		items = append(items, item)
	}

	// Build type query string.
	query := make(url.Values)
	for _, typ := range types {
		query.Add("types[]", typ.String())
	}
	for _, typ := range excludeTypes {
		query.Add("exclude_types[]", typ.String())
	}

	return paging.PackageResponse(paging.ResponseParams{
		Items: items,
		Path:  "/api/v1/notifications",
		Next:  page.Next(lo, hi),
		Prev:  page.Prev(lo, hi),
		Query: query,
	}), nil
}

// NotificationsGetGrouped is like NotificationsGet but returns the grouped
// (Mastodon 4.3+) response shape for GET /api/v2/notifications. GoToSocial
// doesn't actually group notifications yet, so each notification is returned
// as its own single-item group. Returns the grouped payload plus the Link
// header (rewritten to point at the v2 endpoint) for paging.
func (p *Processor) NotificationsGetGrouped(
	ctx context.Context,
	requester *gtsmodel.Account,
	page *paging.Page,
	types []gtsmodel.NotificationType,
	excludeTypes []gtsmodel.NotificationType,
) (*apimodel.GroupedNotificationsResponse, string, gtserror.WithCode) {
	// Reuse the v1 fetch (visibility, mutes, filters, paging) unchanged.
	resp, errWithCode := p.NotificationsGet(ctx, requester, page, types, excludeTypes)
	if errWithCode != nil {
		return nil, "", errWithCode
	}

	grouped := &apimodel.GroupedNotificationsResponse{
		NotificationGroups: make([]apimodel.NotificationGroup, 0, len(resp.Items)),
		Accounts:           make([]*apimodel.Account, 0, len(resp.Items)),
		Statuses:           make([]*apimodel.Status, 0),
	}

	accountsSeen := make(map[string]struct{})
	statusesSeen := make(map[string]struct{})

	for _, item := range resp.Items {
		n, ok := item.(*apimodel.Notification)
		if !ok {
			continue
		}

		group := apimodel.NotificationGroup{
			GroupKey:                 n.ID,
			NotificationsCount:       1,
			Type:                     n.Type,
			MostRecentNotificationID: n.ID,
			PageMinID:                n.ID,
			PageMaxID:                n.ID,
			LatestPageNotificationAt: n.CreatedAt,
			SampleAccountIDs:         make([]string, 0, 1),
		}

		if n.Account != nil {
			group.SampleAccountIDs = append(group.SampleAccountIDs, n.Account.ID)
			if _, seen := accountsSeen[n.Account.ID]; !seen {
				accountsSeen[n.Account.ID] = struct{}{}
				grouped.Accounts = append(grouped.Accounts, n.Account)
			}
		}

		if n.Status != nil {
			id := n.Status.ID
			group.StatusID = &id
			if _, seen := statusesSeen[id]; !seen {
				statusesSeen[id] = struct{}{}
				grouped.Statuses = append(grouped.Statuses, n.Status)
			}
		}

		grouped.NotificationGroups = append(grouped.NotificationGroups, group)
	}

	// Point paging links at the v2 endpoint.
	linkHeader := strings.ReplaceAll(resp.LinkHeader, "/api/v1/notifications", "/api/v2/notifications")

	return grouped, linkHeader, nil
}

func (p *Processor) NotificationGet(ctx context.Context, account *gtsmodel.Account, targetNotifID string) (*apimodel.Notification, gtserror.WithCode) {
	notif, err := p.state.DB.GetNotificationByID(ctx, targetNotifID)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		err := gtserror.Newf("error getting from db: %w", err)
		return nil, gtserror.NewErrorInternalError(err)
	}

	if notif == nil {
		const text = "notification not found"
		return nil, gtserror.NewErrorNotFound(
			errors.New(text),
			text,
		)
	}

	if notif.TargetAccountID != account.ID {
		err := gtserror.New("requester does not match notification target")
		return nil, gtserror.NewErrorNotFound(err)
	}

	// NOTE: we specifically don't do any filtering
	// or mute checking for a notification directly
	// fetched by ID. only from timelines etc.

	apiNotif, err := p.converter.NotificationToAPINotification(ctx, notif)
	if err != nil {
		err := gtserror.Newf("error converting to api model: %w", err)
		return nil, gtserror.WrapWithCode(http.StatusInternalServerError, err)
	}

	return apiNotif, nil
}

func (p *Processor) NotificationsClear(ctx context.Context, authed *apiutil.Auth) gtserror.WithCode {
	// Delete all notifications of all types that target the authorized account.
	if err := p.state.DB.DeleteNotifications(ctx, nil, authed.Account.ID, ""); err != nil && !errors.Is(err, db.ErrNoEntries) {
		return gtserror.NewErrorInternalError(err)
	}

	return nil
}

func (p *Processor) notifVisible(
	ctx context.Context,
	n *gtsmodel.Notification,
	acct *gtsmodel.Account,
) (bool, error) {
	// If account is set, ensure it's
	// visible to notif target.
	if n.OriginAccount != nil {
		// If this is a new local account sign-up,
		// skip normal visibility checking because
		// origin account won't be confirmed yet.
		if n.NotificationType == gtsmodel.NotificationAdminSignup {
			return true, nil
		}

		visible, err := p.visFilter.AccountVisible(ctx, acct, n.OriginAccount)
		if err != nil {
			return false, err
		}

		if !visible {
			return false, nil
		}
	}

	// If status is set, ensure it's
	// visible to notif target.
	if n.Status != nil {
		visible, err := p.visFilter.StatusVisible(ctx, acct, n.Status)
		if err != nil {
			return false, err
		}

		if !visible {
			return false, nil
		}
	}

	return true, nil
}
