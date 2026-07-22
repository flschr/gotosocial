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

package dereferencing

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"code.superseriousbusiness.org/gopkg/log"
	"code.superseriousbusiness.org/gotosocial/internal/ap"
	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/util"
)

// GetStatusByURI will attempt to fetch a status by its URI,
// first checking the database. In the case of a newly-met
// remote model, or a remote model whose 'last_fetched' date
// is beyond a certain interval, the status will be dereferenced.
// Upon dereferencing the status model, (and any subsequent models
// discovered during thread iteration), will be passed to the
// OnStatusDereference() hook to handle timelining, streaming and
// notification events. THOUGH DO NOTE THAT THIS WILL BE SKIPPED
// IF THE STATUS IS STILL PENDING APPROVAL.
//
// A returned AP statusable indicates the status was dereferenced.
//
// In the case of dereferencing, some low-priority status info will be
// enqueued for asynchronous fetching, e.g. dereferencing status thread.
func (d *Dereferencer) GetStatusByURI(
	ctx context.Context,
	requestUser string,
	uri *url.URL,
) (
	status *gtsmodel.Status,
	statusable ap.Statusable,
	err error,
) {
	var isNew bool

	// Fetch and dereference / update status if necessary.
	status, statusable, isNew, err = d.getStatusByURI(ctx,
		requestUser,
		uri,
	)

	if err != nil {
		if status == nil {
			// err with no existing
			// status for fallback.
			return nil, nil, err
		}

		log.Errorf(ctx, "error updating status %s: %v", uri, err)

	} else if statusable != nil {

		// Deref parents + children.
		d.dereferenceThread(ctx,
			requestUser,
			uri,
			status,
			statusable,
			isNew,
		)
	}

	return status, statusable, nil
}

// RefreshStatus is functionally equivalent to GetStatusByURI(),
// except that it requires a pre populated status model (with AT
// LEAST uri set), and ALL thread dereferencing is asynchronous.
//
// A returned AP statusable indicates the status was dereferenced.
// The returned bool indicates whether the status was new (to us).
func (d *Dereferencer) RefreshStatus(
	ctx context.Context,
	requestUser string,
	status *gtsmodel.Status,
	statusable ap.Statusable,
	window *FreshnessWindow,
) (
	latest *gtsmodel.Status,
	latestStatusable ap.Statusable,
	err error,
) {
	// If no incoming data is provided,
	// check whether status needs update.
	if statusable == nil &&
		statusFresh(status, window) {
		return status, nil, nil
	}

	// Parse the URI from status.
	uri, err := url.Parse(status.URI)
	if err != nil {
		return nil, nil, gtserror.Newf("invalid status uri %q: %w", status.URI, err)
	}

	var isNew bool

	// Try to update and dereference the passed status model.
	latest, latestStatusable, isNew, err = d.enrichAndStoreStatusSafely(ctx,
		requestUser,
		uri,
		status,
		statusable,
	)

	if latestStatusable != nil {
		// Deref parents + children.
		d.dereferenceThread(ctx,
			requestUser,
			uri,
			latest,
			latestStatusable,
			isNew,
		)
	}

	return latest, latestStatusable, err
}

// RefreshStatusForThreadContext refreshes a remote status and imports the
// first attached page of replies before returning. Remaining ancestors,
// reply pages, and nested replies continue asynchronously. Concurrent context
// requests for the same status are coalesced and failures leave the existing
// database thread available to the caller.
func (d *Dereferencer) RefreshStatusForThreadContext(
	ctx context.Context,
	requestUser string,
	status *gtsmodel.Status,
) (*gtsmodel.Status, error) {
	if status.Flags.Local() || status.Flags.Deleted() {
		return status, nil
	}
	if d.threadContextRefreshFresh(status.URI, time.Now()) {
		return status, nil
	}

	lockKey := "thread-context-refresh:" + status.URI
	unlock := d.state.ProcessingLocks.Lock(lockKey)
	defer unlock()

	// Another context request may have completed while this one waited.
	if d.threadContextRefreshFresh(status.URI, time.Now()) {
		return status, nil
	}
	markAttempt := func(success bool) {
		d.threadContextRefreshes.Set(status.URI, threadContextRefresh{
			at:      time.Now(),
			success: success,
		})
	}

	ctx, cancel := context.WithTimeout(ctx, ThreadContextRefreshTimeout)
	defer cancel()

	latest, err := d.state.DB.GetStatusByID(ctx, status.ID)
	if err != nil {
		markAttempt(false)
		return status, gtserror.Newf("error reloading thread status: %w", err)
	}

	uri, err := url.Parse(latest.URI)
	if err != nil {
		markAttempt(false)
		return latest, gtserror.Newf("invalid status uri %q: %w", latest.URI, err)
	}

	latest, statusable, isNew, err := d.enrichAndStoreStatusSafely(
		ctx,
		requestUser,
		uri,
		latest,
		nil,
	)
	if err != nil {
		markAttempt(false)
		return latest, err
	}

	if statusable == nil {
		markAttempt(false)
		return latest, nil
	}

	// Import the first reply page before the context query reads descendants.
	// Any failure is non-fatal: callers can still return the cached thread.
	firstPageErr := d.dereferenceStatusDescendantsFirstPage(
		ctx,
		requestUser,
		uri,
		statusable,
	)
	if firstPageErr != nil {
		log.Errorf(ctx, "error refreshing first reply page for %s: %v", latest.URI, firstPageErr)
	}

	// Continue the complete thread traversal outside the request path.
	d.dereferenceThread(ctx, requestUser, uri, latest, statusable, isNew)
	markAttempt(firstPageErr == nil)

	return latest, nil
}

// RefreshStatusAsync is functionally equivalent to callling RefreshStatus()
// yourself within a dereferencer worker function, except that it performs an
// optimized hand-off operation by performing freshness and validity checks
// synchronously beforehand. This prevents handing spurious tasks to the worker.
func (d *Dereferencer) RefreshStatusAsync(
	ctx context.Context,
	requestUser string,
	status *gtsmodel.Status,
	statusable ap.Statusable,
	window *FreshnessWindow,
) {
	// If no incoming data is provided,
	// check whether status needs update.
	if statusable == nil &&
		statusFresh(status, window) {
		return
	}

	// Parse the URI from status.
	uri, err := url.Parse(status.URI)
	if err != nil {
		log.Errorf(ctx, "invalid status uri %q: %v", status.URI, err)
		return
	}

	// Enqueue a worker function to enrich this status model async.
	d.state.Workers.Dereference.Queue.Push(func(ctx context.Context) {
		var isNew bool

		// Try to update and dereference the passed status model.
		latest, statusable, isNew, err := d.enrichAndStoreStatusSafely(ctx,
			requestUser,
			uri,
			status,
			statusable,
		)
		if err != nil {
			log.Errorf(ctx, "error enriching remote status: %v", err)
			return
		}

		if statusable != nil {
			// Deref parents + children.
			d.dereferenceThread(ctx,
				requestUser,
				uri,
				latest,
				statusable,
				isNew,
			)
		}
	})
}

/*
	INTERNAL / UTIL FUNCTIONS HERE
*/

// statusFresh returns true if the given status is still
// considered "fresh" according to the desired freshness
// window (falls back to default status freshness if nil).
//
// Local statuses will always be considered fresh,
// because there's no remote state that may have changed.
//
// Return value of false indicates that the status
// is not fresh and should be refreshed from remote.
func statusFresh(
	status *gtsmodel.Status,
	window *FreshnessWindow,
) bool {
	if status.Flags.Local() ||
		status.Flags.Deleted() {
		// Can't refresh deleted
		// or local statuses!
		return true
	}

	if window == nil {
		// If no window given, fallback
		// to default status freshness.
		window = &DefaultStatusFreshness
	}

	// Moment when the status is
	// considered stale according to
	// desired freshness window.
	d := time.Duration(*window)
	staleAt := status.FetchedAt.Add(d)

	// It's still fresh if the time now
	// is not past the point of staleness.
	return !time.Now().After(staleAt)
}

// getStatusByURI is a package internal form of .GetStatusByURI()
// that doesn't dereference thread on update, and may return
// an existing status with error on failed re-fetch.
func (d *Dereferencer) getStatusByURI(
	ctx context.Context,
	requestUser string,
	uri *url.URL,
) (
	status *gtsmodel.Status,
	statusable ap.Statusable,
	isNew bool,
	err error,
) {
	// Search the database
	// for existing status.
	uriStr := uri.String()
	status, err = d.getStatusFromDB(ctx, uriStr)
	if err != nil {
		return nil, nil, false, err
	}

	if status == nil {
		// Ensure not a failed search for a local
		// status, if so we know it doesn't exist.
		if uri.Host == config.GetHost() ||
			uri.Host == config.GetAccountDomain() {
			return nil, nil, false, gtserror.SetUnretrievable(err)
		}

		// Create and pass-through a bare-bones model for deref.
		return d.enrichAndStoreStatusSafely(ctx, requestUser, uri,
			&gtsmodel.Status{URI: uriStr}, nil)
	}

	if statusFresh(status, &DefaultStatusFreshness) {
		// This is an existing status that is up-to-date,
		// before returning ensure it is fully populated.
		if err := d.state.DB.PopulateStatus(ctx, status); err != nil {
			log.Errorf(ctx, "error populating existing status: %v", err)
		}

		return status, nil, false, nil
	}

	// Status not found in db or not fresh.
	// Try to deref new or update existing.
	return d.enrichAndStoreStatusSafely(ctx,
		requestUser,
		uri,
		status,
		nil,
	)
}

// enrichAndStoreStatusSafely wraps enrichStatus() to perform it within a
// State{}.FedLocks mutexmap, which protects it within per-URI mutex locks.
// This also handles necessary delete of now-deleted statuses, and updating
// fetched_at on returned HTTP errors.
func (d *Dereferencer) enrichAndStoreStatusSafely(
	ctx context.Context,
	requestUser string,
	uri *url.URL,
	status *gtsmodel.Status,
	statusable ap.Statusable,
) (
	*gtsmodel.Status,
	ap.Statusable,
	bool,
	error,
) {
	uriStr := status.URI

	// Acquire per-URI deref lock, wraping unlock
	// to safely defer in case of panic, while still
	// performing more granular unlocks when needed.
	unlock := d.state.FedLocks.Lock(uriStr)
	unlock = util.DoOnce(unlock)
	defer unlock()

	if status.ID != "" {
		var err error

		// If ID was set it means we've stored this status before.
		//
		// We reload the existing status, just to ensure we have the
		// latest version of it. e.g. another racing thread might have
		// just input a change but we still have an old status copy.
		//
		// Note: returned status will be fully populated, required below.
		status, err = d.state.DB.GetStatusByID(ctx, status.ID)
		if err != nil {
			return nil, nil, false, gtserror.Newf("error getting up-to-date existing status: %w", err)
		}
	}

	// Perform status enrichment + storage with passed vars.
	//
	// isNew may be true if this status was actually stored
	// under a different URI than the one we were given.
	latest, statusable, isNew, err := d.enrichAndStoreStatus(ctx,
		requestUser,
		uri,
		status,
		statusable,
	)

	// Check for a returned HTTP code via error.
	switch code := gtserror.StatusCode(err); {

	// Gone (410) definitely indicates deletion.
	// Remove status if it was an existing one.
	case code == http.StatusGone && !isNew:
		if err := d.state.DB.StubStatus(ctx, status, true); err != nil {
			log.Error(ctx, "error deleting gone status %s: %v", uriStr, err)
		}

		// Don't return any status.
		return nil, nil, false, err

	// Any other HTTP error mesg
	// code, with existing status.
	case code >= 400 && !isNew:

		// Update fetched_at to slow re-attempts
		// but don't return early. We can still
		// return the model we had stored already.
		status.FetchedAt = time.Now()
		if err := d.state.DB.UpdateStatus(ctx, status, "fetched_at"); err != nil {
			log.Error(ctx, "error updating %s fetched_at: %v", uriStr, err)
		}

		// See below.
		fallthrough

	// In case of error with an existing
	// status in the database, return error
	// but still return existing status.
	case err != nil && !isNew:
		latest = status
		statusable = nil
	}

	// Unlock now
	// we're done.
	unlock()

	switch {
	case err == nil:
		// Pass status to its dereferencer hook.
		d.onStatusDereference(ctx, latest, isNew)

	case errors.Is(err, db.ErrAlreadyExists):
		// We leave 'isNew' set so that caller
		// still dereferences parents, otherwise
		// the version we pass back may not have
		// these attached as inReplyTos yet (since
		// those happen OUTSIDE federator lock).
		//
		// TODO: performance-wise, this won't be
		// great. should improve this if we can!

		// DATA RACE! We likely lost out to another goroutine
		// in a call to db.Put(Status). Look again in DB by URI.
		latest, err = d.getStatusFromDB(ctx, status.URI)
		if err != nil {
			err = gtserror.Newf("error getting status %s from database after race: %w", uriStr, err)
		}
	}

	return latest, statusable, isNew, err
}

// enrichAndStoreStatus will enrich the given status, whether
// a new barebones model, or existing model from the database,
// handling necessary dereferencing. The result is then stored,
// either updating or putting a new status status in the db.
//
// The return values are the latest up-to-date model of the status,
// the statusable AP representation, and a boolean indicating whether
// the status is truly new or whether it had already been stored
// under a different URI than the one given, after dereferencing.
func (d *Dereferencer) enrichAndStoreStatus(
	ctx context.Context,
	requestUser string,
	uri *url.URL,
	status *gtsmodel.Status,
	statusable ap.Statusable,
) (
	*gtsmodel.Status, // latestStatus
	ap.Statusable, // statusable
	bool, // isNew
	error, // error
) {
	// New--ie., hasn't been stored
	// in the db--if no ID was set yet.
	isNew := status.ID == ""

	// Check whether this status URI is a blocked domain / subdomain.
	if blocked, err := d.state.DB.IsDomainBlocked(ctx, uri.Host); err != nil {
		return nil, nil, isNew, gtserror.Newf("error checking blocked domain: %w", err)
	} else if blocked {
		err := gtserror.Newf("%s is blocked", uri.Host)
		return nil, nil, isNew, gtserror.SetUnretrievable(err)
	}

	if statusable == nil {
		// Create transport on behalf of requesting username.
		tsport, err := d.transportController.NewTransportForUsername(ctx, requestUser)
		if err != nil {
			return nil, nil, isNew, gtserror.Newf("couldn't create transport: %w", err)
		}

		var existing *gtsmodel.Status

		// Dereference statusable from remote, checking if we already had this
		// status stored under a different URI (ie., the final URI after redirects).
		statusable, existing, uri, err = d.retrieveStatusable(ctx, tsport, uri)
		if err != nil {
			return nil, nil, isNew, err
		}

		// If alreadyStatus was returned, it means we
		// already had this status stored in the db under
		// its final reachable URI and not the given URI.
		//
		// Continue with this status and mark it as not
		// new so we don't try to store it again below.
		if existing != nil {
			status = existing
			isNew = false
		}
	}

	// ActivityPub model was recently dereferenced, so assume passed status
	// may contain out-of-date information. Convert AP model to our GTS model.
	latestStatus, err := d.convertStatusable(ctx, requestUser, uri, statusable)
	if err != nil {
		return nil, nil, isNew, err
	}

	if isNew {

		// Generate new status ID from the provided creation date.
		latestStatus.ID = id.NewULIDFromTime(latestStatus.CreatedAt)
	} else {

		// Ensure that status isn't trying to re-date itself.
		if !latestStatus.CreatedAt.Equal(status.CreatedAt) {
			err := gtserror.Newf("status %s 'published' changed", uri)
			return nil, nil, isNew, gtserror.SetMalformed(err)
		}

		// Reuse existing status ID.
		latestStatus.ID = status.ID
	}

	// Set latest fetch time and carry-
	// over some values from "old" status.
	latestStatus.FetchedAt = time.Now()
	pendAppr := status.Flags.PendingApproval()
	latestStatus.Flags.SetPendingApproval(pendAppr)

	// Carry-over approvals. Remote instances might not yet
	// serve statuses with the `approved_by` field, but we
	// might have marked a status as pre-approved on our side
	// based on the author's inclusion in a followers/following
	// collection, or by providing pre-approval URI on the bare
	// status passed to RefreshStatus. By carrying over previously
	// set values we can avoid marking such statuses as "pending".
	//
	// If a remote has in the meantime retracted its approval,
	// the next call to 'isPermittedStatus' will catch that.
	if latestStatus.ApprovedByURI == "" {
		latestStatus.ApprovedByURI = status.ApprovedByURI
	}

	// Check if this is a permitted status we should accept.
	// Function also sets "PendingApproval" bool as necessary,
	// and handles removal of existing statuses no longer permitted.
	permit, err := d.isPermittedStatus(ctx, requestUser, status, latestStatus, isNew)
	if err != nil {
		return nil, nil, isNew, gtserror.Newf("error checking permissibility for status %s: %w", uri, err)
	}

	if !permit {
		// Return a checkable error type that can be ignored.
		err := gtserror.Newf("dropping unpermitted status: %s", uri)
		return nil, nil, isNew, gtserror.SetNotPermitted(err)
	}

	// Handle all peripheral changes / new stuff
	// for this status: polls, mentions, media, etc.
	changes, err := d.handleStatusPeripherals(ctx,
		requestUser,
		uri,
		status,
		latestStatus,
	)
	if err != nil {
		return nil, nil, isNew, err
	}

	if isNew {
		// Simplest case, insert this new remote status into the database.
		if err := d.state.DB.PutStatus(ctx, latestStatus); err != nil {
			return nil, nil, isNew, gtserror.Newf("error inserting new status %s: %w", uri, err)
		}
	} else {
		// Check for and handle any edits to status, inserting
		// historical edit if necessary. Also determines status
		// columns that need updating in below query.
		cols, err := d.handleStatusEdit(ctx,
			status,
			latestStatus,
			changes.pollChanged,
			changes.mentionsChanged,
			changes.tagsChanged,
			changes.mediaChanged,
			changes.emojiChanged,
			changes.interactionPolicyChanged,
		)
		if err != nil {
			return nil, nil, isNew, gtserror.Newf("error handling edit for status %s: %w", uri, err)
		}

		// With returned changed columns, now update the existing status entry.
		if err := d.state.DB.UpdateStatus(ctx, latestStatus, cols...); err != nil {
			return nil, nil, isNew, gtserror.Newf("error updating existing status %s: %w", uri, err)
		}
	}

	return latestStatus, statusable, isNew, nil
}
