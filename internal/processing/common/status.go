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

package common

import (
	"context"
	"errors"

	"code.superseriousbusiness.org/gopkg/log"
	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/federation/dereferencing"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
)

// GetOwnStatus fetches the given status with ID,
// and ensures that it belongs to given requester.
func (p *Processor) GetOwnStatus(
	ctx context.Context,
	requester *gtsmodel.Account,
	targetID string,
) (
	*gtsmodel.Status,
	gtserror.WithCode,
) {
	target, err := p.state.DB.GetStatusByID(ctx, targetID)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		err := gtserror.Newf("error getting from db: %w", err)
		return nil, gtserror.NewErrorInternalError(err)
	}

	switch {
	case target == nil:
		const text = "target status not found"
		return nil, gtserror.NewErrorNotFound(
			errors.New(text),
			text,
		)

	case target.AccountID != requester.ID:
		return nil, gtserror.NewErrorNotFound(
			errors.New("status does not belong to requester"),
			"target status not found",
		)
	}

	return target, nil
}

// GetVisibleTargetStatus calls GetTargetStatusBy(),
// but converts a non-visible result to not-found error.
//
// window can be used to force refresh of the target if it's
// deemed to be stale. Falls back to default window if nil.
func (p *Processor) GetVisibleTargetStatusBy(
	ctx context.Context,
	requester *gtsmodel.Account,
	getTargetFromDB func() (*gtsmodel.Status, error),
	window *dereferencing.FreshnessWindow,
) (
	status *gtsmodel.Status,
	errWithCode gtserror.WithCode,
) {
	return p.getVisibleTargetStatusBy(ctx,
		requester,
		getTargetFromDB,
		window,
		false, // standard status refresh

		// don't allow deleted
		// status stubs.
		false,
	)
}

// GetVisibleTargetStatus calls GetVisibleTargetStatusBy(),
// passing in a database function that fetches by status ID.
//
// window can be used to force refresh of the target if it's
// deemed to be stale. Falls back to default window if nil.
func (p *Processor) GetVisibleTargetStatus(
	ctx context.Context,
	requester *gtsmodel.Account,
	targetID string,
	window *dereferencing.FreshnessWindow,
) (
	status *gtsmodel.Status,
	errWithCode gtserror.WithCode,
) {
	return p.GetVisibleTargetStatusBy(ctx,
		requester,
		func() (*gtsmodel.Status, error) { return p.state.DB.GetStatusByID(ctx, targetID) },
		window,
	)
}

// GetVisibleTargetStatusInThread is functionally similar to
// GetVisibleTargetStatus(), but permits returning deleted stubs.
func (p *Processor) GetVisibleTargetStatusInThread(
	ctx context.Context,
	requester *gtsmodel.Account,
	targetID string,
	window *dereferencing.FreshnessWindow,
) (
	status *gtsmodel.Status,
	errWithCode gtserror.WithCode,
) {
	return p.getVisibleTargetStatusBy(ctx,
		requester,
		func() (*gtsmodel.Status, error) { return p.state.DB.GetStatusByID(ctx, targetID) },
		window,
		true, // synchronously refresh first remote reply page

		// allow deleted
		// status stubs.
		true,
	)
}

// UnwrapIfBoost "unwraps" the given status if
// it's a boost wrapper, by returning the boosted
// status it targets (pending visibility checks).
//
// Just returns the input status if it's not a boost.
func (p *Processor) UnwrapIfBoost(
	ctx context.Context,
	requester *gtsmodel.Account,
	status *gtsmodel.Status,
) (*gtsmodel.Status, gtserror.WithCode) {
	if status.BoostOfID == "" {
		return status, nil
	}
	return p.GetVisibleTargetStatus(ctx,
		requester,
		status.BoostOfID,
		nil,
	)
}

func (p *Processor) getVisibleTargetStatusBy(
	ctx context.Context,
	requester *gtsmodel.Account,
	getTargetFromDB func() (*gtsmodel.Status, error),
	window *dereferencing.FreshnessWindow,
	refreshThreadContext bool,
	allowDeleted bool,
) (
	status *gtsmodel.Status,
	errWithCode gtserror.WithCode,
) {
	// Fetch the target status by ID from the database.
	target, visible, errWithCode := p.getTargetStatusBy(ctx,
		requester,
		getTargetFromDB,
		window,
		refreshThreadContext,
		allowDeleted,
	)
	if errWithCode != nil {
		return nil, errWithCode
	}

	if !visible {
		// Wrap generic error in NotVisible so intermediate
		// functions can know this error was returned as a
		// result of status not being visible to requester,
		// not because the status wasn't found in the db.
		const text = "target status not visible"
		err := errors.New(text)
		err = gtserror.SetNotVisible(err)

		if requester == nil {
			// Not visible unauthed, return 401.
			return nil, gtserror.NewErrorUnauthorized(err, text)
		} else { // nolint
			// Not visible to authed requester, return 404.
			return nil, gtserror.NewErrorNotFound(err, text)
		}
	}

	return target, nil
}

// getTargetStatusBy fetches the target status with db load
// function, given the authorized (or, nil) requester's
// account. This returns an approprate gtserror.WithCode
// accounting for not found and visibility to requester.
//
// 'window' can be used to force refresh of the target if it's
// deemed to be stale. Falls back to default window if nil.
//
// 'allowDeleted' determines whether to permit returning deleted
// stubs left remaining of statuses, useful for threading.
func (p *Processor) getTargetStatusBy(
	ctx context.Context,
	requester *gtsmodel.Account,
	getTargetFromDB func() (*gtsmodel.Status, error),
	window *dereferencing.FreshnessWindow,
	refreshThreadContext bool,
	allowDeleted bool,
) (
	status *gtsmodel.Status,
	visible bool,
	errWithCode gtserror.WithCode,
) {
	// Fetch the target status from db.
	target, err := getTargetFromDB()
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		err := gtserror.Newf("error getting from db: %w", err)
		return nil, false, gtserror.NewErrorInternalError(err)
	}

	// Check it exists.
	if target == nil {
		const text = "target status not found"
		return nil, false, gtserror.NewErrorNotFound(
			errors.New(text),
			text,
		)
	}

	// Return error on deleted status unless
	// specifically requested and authenticated.
	if (requester == nil || !allowDeleted) && target.Flags.Deleted() {
		// Wrap generic error in Deleted so intermediate
		// functions can know this error was returned as
		// a result of status being deleted/stubbed, not
		// because the status wasn't found in the db.
		const text = "status has been deleted"
		err := errors.New(text)
		err = gtserror.SetDeleted(err)

		if target.Account.IsLocal() {
			// We can return 410 gone for our own
			// status as we can confidently say it
			// won't exist again with this ID.
			//
			// https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Status/410
			return nil, false, gtserror.NewErrorGone(err, text)
		} else { // nolint
			// For remote statuses just return 404
			// as it's not our place to return 410.
			return nil, false, gtserror.NewErrorNotFound(err, text)
		}
	}

	// Check whether target status is visible to requesting account.
	visible, err = p.visFilter.StatusVisible(ctx, requester, target)
	if err != nil {
		err := gtserror.Newf("error checking visibility: %w", err)
		return nil, false, gtserror.NewErrorInternalError(err)
	}

	if requester != nil && visible {
		// Only refresh status if visible to requester,
		// and there is *authorized* requester to prevent
		// a possible DOS vector for unauthorized clients.
		var latest *gtsmodel.Status
		var err error
		if refreshThreadContext {
			latest, err = p.federator.Dereferencer.RefreshStatusForThreadContext(
				ctx,
				requester.Username,
				target,
			)
		} else {
			latest, _, err = p.federator.RefreshStatus(ctx,
				requester.Username,
				target,
				nil,
				window,
			)
		}
		if err != nil {
			log.Errorf(ctx, "error refreshing target %s: %v", target.URI, err)
			return target, visible, nil
		}

		// Set latest.
		target = latest
	}

	return target, visible, nil
}

// GetAPIStatus fetches the appropriate API status model for target.
func (p *Processor) GetAPIStatus(
	ctx context.Context,
	requester *gtsmodel.Account,
	target *gtsmodel.Status,
) (
	apiStatus *apimodel.Status,
	errWithCode gtserror.WithCode,
) {
	// Convert the target status to frontend API model.
	apiStatus, err := p.converter.StatusToAPIStatus(ctx,
		target,
		requester,
	)
	if err != nil {
		err := gtserror.Newf("error converting: %w", err)
		return nil, gtserror.NewErrorInternalError(err)
	}

	return apiStatus, nil
}

// GetVisibleAPIStatuses converts a slice of statuses to API
// model statuses, filtering according to visibility to requester
// along with given filter context, filters and user mutes.
//
// Please note that all errors will be logged at ERROR level,
// but will not be returned. Callers are likely to run into
// show-stopping errors in the lead-up to this function.
func (p *Processor) GetVisibleAPIStatuses(
	ctx context.Context,
	requester *gtsmodel.Account,
	statuses []*gtsmodel.Status,
	filterCtx gtsmodel.FilterContext,
) []apimodel.Status {

	// Start new log entry with
	// the calling function name
	// as a field in each entry.
	l := log.WithContext(ctx).
		WithField("caller", log.Caller(3))

	// Iterate filtered statuses for conversion to API model.
	apiStatuses := make([]apimodel.Status, 0, len(statuses))
	for _, status := range statuses {

		// Check whether status is visible to requester.
		visible, err := p.visFilter.StatusVisible(ctx,
			requester,
			status,
		)
		if err != nil {
			l.Errorf("error checking visibility: %v", err)
			continue
		}

		if !visible {
			continue
		}

		var filtered []apimodel.FilterResult
		if filterCtx != 0 {
			var hide bool

			// Check whether this status is muted by requesting account.
			muted, err := p.muteFilter.StatusMuted(ctx, requester, status)
			if err != nil {
				log.Errorf(ctx, "error checking mute: %v", err)
				continue
			}

			if muted {
				continue
			}

			// Check whether status is filtered in context by requesting account.
			filtered, hide, err = p.statusFilter.StatusFilterResultsInContext(ctx,
				requester,
				status,
				filterCtx,
			)
			if err != nil {
				l.Errorf("error filtering: %v", err)
				continue
			}

			if hide {
				continue
			}
		}

		// Not muted or "hide" filtered. Convert to API status.
		apiStatus, err := p.converter.StatusToAPIStatus(ctx,
			status,
			requester,
		)
		if err != nil {
			l.Errorf("error converting: %v", err)
			continue
		}

		// Set filter results on status.
		apiStatus.Filtered = filtered

		// Append converted status to return slice.
		apiStatuses = append(apiStatuses, *apiStatus)
	}

	return apiStatuses
}
