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

package surfacing

import (
	"context"
	"errors"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/email"
	"code.superseriousbusiness.org/gotosocial/internal/federation"
	"code.superseriousbusiness.org/gotosocial/internal/filter/mutes"
	"code.superseriousbusiness.org/gotosocial/internal/filter/status"
	"code.superseriousbusiness.org/gotosocial/internal/filter/visibility"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/processing/conversations"
	"code.superseriousbusiness.org/gotosocial/internal/processing/stream"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"code.superseriousbusiness.org/gotosocial/internal/typeutils"
	"code.superseriousbusiness.org/gotosocial/internal/webpush"
)

// Surfacer wraps functions for 'surfacing' the result
// of ingesting a message into the server, eg:
//   - timelining a status
//   - removing a status from timelines
//   - sending a notification to a user
//   - sending an email
type Surfacer struct {
	state         *state.State
	converter     *typeutils.Converter
	federator     *federation.Federator
	stream        *stream.Processor
	visFilter     *visibility.Filter
	muteFilter    *mutes.Filter
	statusFilter  *status.Filter
	emailSender   email.Sender
	webPushSender webpush.Sender
	conversations *conversations.Processor
}

// New returns a pointer
// to a new surfacer struct.
func New(
	state *state.State,
	converter *typeutils.Converter,
	federator *federation.Federator,
	stream *stream.Processor,
	visFilter *visibility.Filter,
	muteFilter *mutes.Filter,
	statusFilter *status.Filter,
	emailSender email.Sender,
	webPushSender webpush.Sender,
	conversations *conversations.Processor,
) *Surfacer {
	s := &Surfacer{
		state:         state,
		converter:     converter,
		federator:     federator,
		stream:        stream,
		visFilter:     visFilter,
		muteFilter:    muteFilter,
		statusFilter:  statusFilter,
		emailSender:   emailSender,
		webPushSender: webPushSender,
		conversations: conversations,
	}

	// Status status dereferencer hook using surfacer.
	federator.Dereferencer.OnStatusDereference = func(ctx context.Context, status *gtsmodel.Status, isNew bool) error {
		if status.Flags.PendingApproval() {
			// Status hasn't yet been
			// approved, it needs further
			// processing elsewhere.
			return nil
		}

		// We only timeline and notify like a new status IF:
		// - it is new to our instance (i.e. just been dereference)
		// - it was created in the last day (accounting for timezones).
		if isNew && time.Since(status.CreatedAt) < 25*time.Hour {
			return s.TimelineAndNotifyStatus(ctx, status)
		} else { //nolint
			return s.TimelineAndNotifyStatusUpdate(ctx, status)
		}
	}

	// Set media dereferencer hook using surfacer.
	federator.Dereferencer.OnMediaDereference = func(ctx context.Context, media *gtsmodel.MediaAttachment) error {
		if media.StatusID == "" {
			// we only handle this
			// for statuses for now.
			return nil
		}

		// Get the original status model that media is attached to.
		// If we can get it, it means it finished processing already
		// and the media finished after. If we can't get it (yet)
		// it means it hasn't finished processing and been stored.
		status, err := state.DB.GetStatusByID(ctx, media.StatusID)
		if err != nil && !errors.Is(err, db.ErrNoEntries) {
			return gtserror.Newf("db error getting status: %w", err)
		}

		switch {
		case status == nil:
			// Status wasn't stored yet. This means the media finished
			// processing before the status did, which means that when
			// the status is streamed to the timeline, it will be the
			// complete version with this media attachment in place.
			// In other words, we don't need to do side effects here.
			return nil

		case status.Flags.PendingApproval():
			// Status was stored but hasn't yet been approved, and
			// therefore hasn't been timelined; it needs further
			// processing elsewhere. So just return regardless.
			return nil

		default:
			// Status was stored and is not pending approval,
			// and this media finished processing *afterwards*.
			// This means a version of the status probably already
			// exists in timelines with the media set to "still
			// processing". Since it's now finished, we can stream
			// an update to the status to show the complete status
			// (with this fully dereffed media) in timelines.
			return s.TimelineAndNotifyStatusUpdate(ctx, status)
		}
	}

	return s
}
