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

package bundb

import (
	"context"
	"errors"
	"slices"
	"strings"

	"code.superseriousbusiness.org/gopkg/log"
	"code.superseriousbusiness.org/gopkg/xslices"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtscontext"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/paging"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"github.com/uptrace/bun"
)

type statusDB struct {
	db    *bun.DB
	state *state.State
}

func (s *statusDB) GetStatusByID(ctx context.Context, id string) (*gtsmodel.Status, error) {
	return s.getStatus(
		ctx,
		"ID",
		func(status *gtsmodel.Status) error {
			return s.db.NewSelect().Model(status).Where("? = ?", bun.Ident("status.id"), id).Scan(ctx)
		},
		id,
	)
}

func (s *statusDB) GetStatusByIdempotencyKey(
	ctx context.Context,
	accountID string,
	applicationID string,
	key string,
) (*gtsmodel.Status, error) {
	status := new(gtsmodel.Status)
	if err := s.db.NewSelect().
		Model(status).
		Where("? = ?", bun.Ident("account_id"), accountID).
		Where("? = ?", bun.Ident("created_with_application_id"), applicationID).
		Where("? = ?", bun.Ident("idempotency_key"), key).
		Scan(ctx); err != nil {
		return nil, err
	}

	if gtscontext.Barebones(ctx) {
		return status, nil
	}

	if err := s.PopulateStatus(ctx, status); err != nil {
		return nil, err
	}

	return status, nil
}

func (s *statusDB) GetStatusesByIDs(ctx context.Context, ids []string) ([]*gtsmodel.Status, error) {
	// Load all input status IDs via cache loader callback.
	statuses, err := s.state.Caches.DB.Status.LoadIDs("ID",
		ids,
		func(uncached []string) ([]*gtsmodel.Status, error) {
			// Preallocate expected length of uncached statuses.
			statuses := make([]*gtsmodel.Status, 0, len(uncached))

			// Perform database query scanning
			// the remaining (uncached) status IDs.
			if err := s.db.NewSelect().
				Model(&statuses).
				Where("? IN (?)", bun.Ident("id"), bun.List(uncached)).
				Scan(ctx); err != nil {
				return nil, err
			}

			return statuses, nil
		},
	)
	if err != nil {
		return nil, err
	}

	// Reorder the statuses by their
	// IDs to ensure in correct order.
	getID := func(s *gtsmodel.Status) string { return s.ID }
	xslices.OrderBy(statuses, ids, getID)

	if gtscontext.Barebones(ctx) {
		// no need to fully populate.
		return statuses, nil
	}

	// Populate all loaded statuses, removing those we fail to
	// populate (removes needing so many nil checks everywhere).
	statuses = slices.DeleteFunc(statuses, func(status *gtsmodel.Status) bool {
		if err := s.PopulateStatus(ctx, status); err != nil {
			log.Errorf(ctx, "error populating status %s: %v", status.ID, err)
			return true
		}
		return false
	})

	return statuses, nil
}

func (s *statusDB) GetStatusByURI(ctx context.Context, uri string) (*gtsmodel.Status, error) {
	return s.getStatus(
		ctx,
		"URI",
		func(status *gtsmodel.Status) error {
			return s.db.NewSelect().Model(status).Where("? = ?", bun.Ident("status.uri"), uri).Scan(ctx)
		},
		uri,
	)
}

func (s *statusDB) GetStatusByURL(ctx context.Context, url string) (*gtsmodel.Status, error) {
	return s.getStatus(
		ctx,
		"URL",
		func(status *gtsmodel.Status) error {
			return s.db.NewSelect().Model(status).Where("? = ?", bun.Ident("status.url"), url).Scan(ctx)
		},
		url,
	)
}

func (s *statusDB) GetStatusByPollID(ctx context.Context, pollID string) (*gtsmodel.Status, error) {
	return s.getStatus(
		ctx,
		"PollID",
		func(status *gtsmodel.Status) error {
			return s.db.NewSelect().Model(status).Where("? = ?", bun.Ident("status.poll_id"), pollID).Scan(ctx)
		},
		pollID,
	)
}

func (s *statusDB) GetStatusBoost(ctx context.Context, boostOfID string, byAccountID string) (*gtsmodel.Status, error) {
	return s.getStatus(
		ctx,
		"BoostOfID,AccountID",
		func(status *gtsmodel.Status) error {
			return s.db.NewSelect().Model(status).
				Where("status.boost_of_id = ?", boostOfID).
				Where("status.account_id = ?", byAccountID).

				// Our old code actually allowed a status to
				// be boosted multiple times by the same author,
				// so limit our query + order to fetch latest.
				Order("status.id DESC"). // our IDs are timestamped
				Limit(1).
				Scan(ctx)
		},
		boostOfID, byAccountID,
	)
}

func (s *statusDB) getStatus(ctx context.Context, lookup string, dbQuery func(*gtsmodel.Status) error, keyParts ...any) (*gtsmodel.Status, error) {
	// Fetch status from database cache with loader callback
	status, err := s.state.Caches.DB.Status.LoadOne(lookup, func() (*gtsmodel.Status, error) {
		var status gtsmodel.Status

		// Not cached! Perform database query.
		if err := dbQuery(&status); err != nil {
			return nil, err
		}

		return &status, nil
	}, keyParts...)
	if err != nil {
		return nil, err
	}

	if gtscontext.Barebones(ctx) {
		// no need to fully populate.
		return status, nil
	}

	// Further populate the status fields where applicable.
	if err := s.PopulateStatus(ctx, status); err != nil {
		return nil, err
	}

	return status, nil
}

func (s *statusDB) PopulateStatus(ctx context.Context, status *gtsmodel.Status) error {
	var (
		err  error
		errs gtserror.MultiError
	)

	if status.Account == nil {
		// Status author is not set, fetch from database.
		status.Account, err = s.state.DB.GetAccountByID(
			gtscontext.SetBarebones(ctx),
			status.AccountID,
		)
		if err != nil {
			errs.Appendf("error populating status author: %w", err)
		}
	}

	if status.InReplyToID != "" {
		if status.InReplyTo == nil {
			// Status parent is not set, fetch from database.
			status.InReplyTo, err = s.GetStatusByID(
				gtscontext.SetBarebones(ctx),
				status.InReplyToID,
			)
			if err != nil && !errors.Is(err, db.ErrNoEntries) {
				errs.Appendf("error populating status parent: %w", err)
			}
		}

		if status.InReplyToAccount == nil {
			// Status parent author is not set, fetch from database.
			status.InReplyToAccount, err = s.state.DB.GetAccountByID(
				gtscontext.SetBarebones(ctx),
				status.InReplyToAccountID,
			)
			if err != nil {
				errs.Appendf("error populating status parent author: %w", err)
			}
		}
	}

	if status.BoostOfID != "" {
		if status.BoostOf == nil {
			// Status boost is not set, fetch from database.
			status.BoostOf, err = s.GetStatusByID(
				gtscontext.SetBarebones(ctx),
				status.BoostOfID,
			)
			if err != nil {
				errs.Appendf("error populating status boost: %w", err)
			}
		}

		if status.BoostOfAccount == nil {
			// Status boost author is not set, fetch from database.
			status.BoostOfAccount, err = s.state.DB.GetAccountByID(
				gtscontext.SetBarebones(ctx),
				status.BoostOfAccountID,
			)
			if err != nil {
				errs.Appendf("error populating status boost author: %w", err)
			}
		}
	}

	if status.PollID != "" && status.Poll == nil {
		// Status poll is not set, fetch from database.
		status.Poll, err = s.state.DB.GetPollByID(
			gtscontext.SetBarebones(ctx),
			status.PollID,
		)
		if err != nil {
			errs.Appendf("error populating status poll: %w", err)
		}
	}

	if !status.AttachmentsPopulated() {
		// Status attachments are out-of-date with IDs, repopulate.
		status.Attachments, err = s.state.DB.GetAttachmentsByIDs(
			gtscontext.SetBarebones(ctx),
			status.AttachmentIDs,
		)
		if err != nil {
			errs.Appendf("error populating status attachments: %w", err)
		}
	}

	if !status.TagsPopulated() {
		// Status tags are out-of-date with IDs, repopulate.
		status.Tags, err = s.state.DB.GetTags(
			gtscontext.SetBarebones(ctx),
			status.TagIDs,
		)
		if err != nil {
			errs.Appendf("error populating status tags: %w", err)
		}
	}

	if !status.MentionsPopulated() {
		// Status mentions are out-of-date with IDs, repopulate.
		status.Mentions, err = s.state.DB.GetMentions(
			ctx, // TODO: manually populate mentions for places expecting these populated
			status.MentionIDs,
		)
		if err != nil {
			errs.Appendf("error populating status mentions: %w", err)
		}
	}

	if !status.EmojisPopulated() {
		// Status emojis are out-of-date with IDs, repopulate.
		status.Emojis, err = s.state.DB.GetEmojisByIDs(
			gtscontext.SetBarebones(ctx),
			status.EmojiIDs,
		)
		if err != nil {
			errs.Appendf("error populating status emojis: %w", err)
		}
	}

	if status.CreatedWithApplicationID != "" && status.CreatedWithApplication == nil {
		// Populate the status' expected CreatedWithApplication (not always set).
		// Don't error on ErrNoEntries, as the application may have been cleaned up.
		status.CreatedWithApplication, err = s.state.DB.GetApplicationByID(
			gtscontext.SetBarebones(ctx),
			status.CreatedWithApplicationID,
		)
		if err != nil && !errors.Is(err, db.ErrNoEntries) {
			errs.Appendf("error populating status application: %w", err)
		}
	}

	return errs.Combine()
}

func (s *statusDB) PopulateStatusEdits(ctx context.Context, status *gtsmodel.Status) error {
	var err error

	if !status.EditsPopulated() {
		// Status edits are out-of-date with IDs, repopulate.
		status.Edits, err = s.state.DB.GetStatusEditsByIDs(
			gtscontext.SetBarebones(ctx),
			status.EditIDs,
		)
		if err != nil {
			return gtserror.Newf("error populating status edits: %w", err)
		}
	}

	return nil
}

func (s *statusDB) PutStatus(ctx context.Context, status *gtsmodel.Status) error {
	return s.state.Caches.DB.Status.Store(status, func() error {
		// It is safe to run this database transaction within cache.Store
		// as the cache does not attempt a mutex lock until AFTER hook.
		//
		return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			if status.BoostOfID != "" {
				var threadID string

				// Boost wrappers always inherit thread
				// of the origin status they're boosting.
				if err := tx.
					NewSelect().
					Table("statuses").
					Column("thread_id").
					Where("? = ?", bun.Ident("id"), status.BoostOfID).
					Scan(ctx, &threadID); err != nil {
					return gtserror.Newf("error selecting boosted status: %w", err)
				}

				// Set the selected thread.
				status.ThreadID = threadID

				// They also require no further
				// checks! Simply insert status here.
				return insertStatus(ctx, tx, status)
			}

			// Gather a list of possible thread IDs
			// of all the possible related statuses
			// to this one. If one exists we can use
			// the end result, and if too many exist
			// we can fix the status threading.
			var threadIDs []string

			if status.InReplyToID != "" {
				var threadID string

				// A stored parent status exists,
				// select its thread ID to ideally
				// inherit this for status.
				if err := tx.
					NewSelect().
					Table("statuses").
					Column("thread_id").
					Where("? = ?", bun.Ident("id"), status.InReplyToID).
					Scan(ctx, &threadID); err != nil {
					return gtserror.Newf("error selecting status parent: %w", err)
				}

				// Append possible ID to threads slice.
				threadIDs = append(threadIDs, threadID)

			} else if status.InReplyToURI != "" {
				var ids []string

				// A parent status exists but is not
				// yet stored. See if any siblings for
				// this shared parent exist with their
				// own thread IDs.
				if err := tx.
					NewSelect().
					Table("statuses").
					Column("thread_id").
					Where("? = ?", bun.Ident("in_reply_to_uri"), status.InReplyToURI).
					Scan(ctx, &ids); err != nil && !errors.Is(err, db.ErrNoEntries) {
					return gtserror.Newf("error selecting status siblings: %w", err)
				}

				// Append possible IDs to threads slice.
				threadIDs = append(threadIDs, ids...)
			}

			if !status.Flags.Local() {
				var ids []string

				// For remote statuses specifically, check to
				// see if any children are stored for this new
				// stored parent with their own thread IDs.
				if err := tx.
					NewSelect().
					Table("statuses").
					Column("thread_id").
					Where("? = ?", bun.Ident("in_reply_to_uri"), status.URI).
					Scan(ctx, &ids); err != nil && !errors.Is(err, db.ErrNoEntries) {
					return gtserror.Newf("error selecting status children: %w", err)
				}

				// Append possible IDs to threads slice.
				threadIDs = append(threadIDs, ids...)
			}

			// Ensure only *unique* posssible thread IDs.
			threadIDs = xslices.Deduplicate(threadIDs)
			switch len(threadIDs) {

			case 0:
				// No related status with thread ID already exists,
				// so create new thread ID from status creation time.
				threadID := id.NewULIDFromTime(status.CreatedAt)

				// Insert new thread.
				if _, err := tx.
					NewInsert().
					Model(&gtsmodel.Thread{ID: threadID}).
					Exec(ctx); err != nil {
					return gtserror.Newf("error inserting thread: %w", err)
				}

				// Update status thread ID.
				status.ThreadID = threadID

			case 1:
				// Inherit single known thread.
				status.ThreadID = threadIDs[0]

			default:
				var err error
				log.Infof(ctx, "reconciling status threading for %s: [%s]", status.URI, strings.Join(threadIDs, ","))
				status.ThreadID, err = s.fixStatusThreading(ctx, tx, threadIDs)
				if err != nil {
					return err
				}
			}

			// And after threading, insert status.
			// This will error if ThreadID is unset.
			return insertStatus(ctx, tx, status)
		})
	})
}

// fixStatusThreading can be called to reconcile statuses in the same thread but known to be using multiple given threads.
func (s *statusDB) fixStatusThreading(ctx context.Context, tx bun.Tx, threadIDs []string) (string, error) {
	if len(threadIDs) <= 1 {
		panic("invalid call to fixStatusThreading()")
	}

	// Sort ascending, i.e.
	// oldest thread ID first.
	slices.Sort(threadIDs)

	// Drop the oldest thread ID
	// from slice, we'll keep this.
	threadID := threadIDs[0]
	threadIDs = threadIDs[1:]

	// On updates, gather IDs of changed model
	// IDs for later stage of cache invalidation,
	// preallocating slices for worst-case scenarios.
	statusIDs := make([]string, 0, 4*len(threadIDs))
	muteIDs := make([]string, 0, 4*len(threadIDs))

	// Update all statuses with
	// thread IDs to use oldest.
	if _, err := tx.
		NewUpdate().
		Table("statuses").
		Where("? IN (?)", bun.Ident("thread_id"), bun.List(threadIDs)).
		Set("? = ?", bun.Ident("thread_id"), threadID).
		Returning("?", bun.Ident("id")).
		Exec(ctx, &statusIDs); err != nil && !errors.Is(err, db.ErrNoEntries) {
		return "", gtserror.Newf("error updating statuses: %w", err)
	}

	// Update all thread mutes with
	// thread IDs to use oldest.
	if _, err := tx.
		NewUpdate().
		Table("thread_mutes").
		Where("? IN (?)", bun.Ident("thread_id"), bun.List(threadIDs)).
		Set("? = ?", bun.Ident("thread_id"), threadID).
		Returning("?", bun.Ident("id")).
		Exec(ctx, &muteIDs); err != nil && !errors.Is(err, db.ErrNoEntries) {
		return "", gtserror.Newf("error updating thread mutes: %w", err)
	}

	// Delete all now
	// unused thread IDs.
	if _, err := tx.
		NewDelete().
		Table("threads").
		Where("? IN (?)", bun.Ident("id"), bun.List(threadIDs)).
		Exec(ctx); err != nil {
		return "", gtserror.Newf("error deleting threads: %w", err)
	}

	// Invalidate caches for changed statuses and mutes.
	s.state.Caches.DB.Status.InvalidateIDs("ID", statusIDs)
	s.state.Caches.DB.ThreadMute.InvalidateIDs("ID", muteIDs)

	return threadID, nil
}

// insertStatus handles the base status insert logic, that is the status itself,
// any intermediary table links, and updating media attachments to point to status.
func insertStatus(ctx context.Context, tx bun.Tx, status *gtsmodel.Status) error {

	// create links between this
	// status and any emojis it uses
	for _, id := range status.EmojiIDs {
		if _, err := tx.
			NewInsert().
			Model(&gtsmodel.StatusToEmoji{
				StatusID: status.ID,
				EmojiID:  id,
			}).
			Exec(ctx); err != nil {
			return gtserror.Newf("error inserting status_to_emoji: %w", err)
		}
	}

	// create links between this
	// status and any tags it uses
	for _, id := range status.TagIDs {
		if _, err := tx.
			NewInsert().
			Model(&gtsmodel.StatusToTag{
				StatusID: status.ID,
				TagID:    id,
			}).
			Exec(ctx); err != nil {
			return gtserror.Newf("error inserting status_to_tag: %w", err)
		}
	}

	// change the status ID of the media
	// attachments to the current status
	for _, a := range status.Attachments {
		a.StatusID = status.ID
		a.ScheduledStatusID = ""
		if _, err := tx.
			NewUpdate().
			Model(a).
			Column("status_id", "scheduled_status_id").
			Where("? = ?", bun.Ident("media_attachment.id"), a.ID).
			Exec(ctx); err != nil {
			return gtserror.Newf("error updating media: %w", err)
		}
	}

	// Finally, insert the status
	if _, err := tx.NewInsert().
		Model(status).
		Exec(ctx); err != nil {
		return gtserror.Newf("error inserting status: %w", err)
	}

	// Increment status author statistics.
	return incrementAccountStats(ctx, tx,
		"statuses_count",
		status.AccountID,
	)
}

func (s *statusDB) UpdateStatus(ctx context.Context, status *gtsmodel.Status, columns ...string) error {
	return s.state.Caches.DB.Status.Store(status, func() error {
		// It is safe to run this database transaction within cache.Store
		// as the cache does not attempt a mutex lock until AFTER hook.
		//
		return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {

			// create links between this
			// status and any emojis it uses
			for _, id := range status.EmojiIDs {
				if _, err := tx.
					NewInsert().
					Model(&gtsmodel.StatusToEmoji{
						StatusID: status.ID,
						EmojiID:  id,
					}).
					On("CONFLICT (?, ?) DO NOTHING", bun.Ident("status_id"), bun.Ident("emoji_id")).
					Exec(ctx); err != nil {
					return err
				}
			}

			// create links between this
			// status and any tags it uses
			for _, id := range status.TagIDs {
				if _, err := tx.
					NewInsert().
					Model(&gtsmodel.StatusToTag{
						StatusID: status.ID,
						TagID:    id,
					}).
					On("CONFLICT (?, ?) DO NOTHING", bun.Ident("status_id"), bun.Ident("tag_id")).
					Exec(ctx); err != nil {
					return err
				}
			}

			// change the status ID of the media
			// attachments to the current status.
			for _, a := range status.Attachments {
				a.StatusID = status.ID
				if _, err := tx.
					NewUpdate().
					Model(a).
					Column("status_id").
					Where("? = ?", bun.Ident("media_attachment.id"), a.ID).
					Exec(ctx); err != nil {
					return err
				}
			}

			// Finally, update the status
			if _, err := tx.NewUpdate().
				Model(status).
				Column(columns...).
				Where("? = ?", bun.Ident("status.id"), status.ID).
				Exec(ctx); err != nil {
				return err
			}

			return nil
		})
	})
}

func (s *statusDB) StubStatus(ctx context.Context, status *gtsmodel.Status, includeMedia bool) error {
	// Delete status related models before anything.
	s.preStatusDelete(ctx, status, includeMedia)

	// Take pointer to original
	// status before changes, used
	// for later cache invalidation.
	original := status

	if err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Stub out status model.
		stub := status.Stub()
		status = &stub

		// Update ALL status columns.
		if res, err := tx.NewUpdate().
			Model(status).
			Where("? = ?", bun.Ident("id"), status.ID).

			// Specifically check the "DELETED" flag in the database hasn't been
			// set (i.e. we have an out-of-date model) otherwise our later handling
			// of status delete side-effects will cause status to get out of sync.
			Where(db.BitNotSet("flags", gtsmodel.StatusFlagDeleted)).
			Exec(ctx); err != nil {
			return err

		} else if n, _ := res.RowsAffected(); n < 1 {
			// Status already stubbed, return
			// early to not perform spurious
			// account status side-effects.
			return nil
		}

		// Handle remaining delete side-effects.
		return s.onStatusDelete(ctx, tx, status)
	}); err != nil {
		return err
	}

	// Invalidate cached status by its ID, manually
	// call the invalidate hook in case not cached.
	s.state.Caches.DB.Status.Invalidate("ID", status.ID)
	s.state.Caches.OnInvalidateStatus(original)

	return nil
}

func (s *statusDB) DeleteStatus(ctx context.Context, status *gtsmodel.Status, includeMedia bool) error {
	s.preStatusDelete(ctx, status, includeMedia)
	return s.deleteStatus(ctx, status)
}

func (s *statusDB) DeleteStatusBoost(ctx context.Context, boost *gtsmodel.Status) error {
	if boost.BoostOfID == "" {
		return gtserror.New("not a status boost")
	}
	// note: there are no related models to delete
	return s.deleteStatus(ctx, boost)
}

func (s *statusDB) deleteStatus(ctx context.Context, status *gtsmodel.Status) error {
	if err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Actually delete the status.
		if res, err := tx.NewDelete().
			Model(status).
			Where("? = ?", bun.Ident("id"), status.ID).
			Exec(ctx); err != nil {
			return err

		} else if n, _ := res.RowsAffected(); n < 1 {
			// Status already deleted, return
			// early to not perform spurious
			// account status side-effects.
			return nil
		}

		// Handle remaining delete side-effects.
		return s.onStatusDelete(ctx, tx, status)
	}); err != nil {
		return err
	}

	// Invalidate cached status by its ID, manually
	// call the invalidate hook in case not cached.
	s.state.Caches.DB.Status.Invalidate("ID", status.ID)
	s.state.Caches.OnInvalidateStatus(status)

	return nil
}

// preStatusDelete performs common status deletion
// logic, deleting related models before the actual
// status is deleted. no error is returned as this
// all is expected to happen in a non-error-blocking
// manner, to ensure status itself is deleted.
func (s *statusDB) preStatusDelete(
	ctx context.Context,
	status *gtsmodel.Status,
	includeMedia bool,
) {
	log := log.New().
		WithContext(ctx).
		WithField("uri", status.URI)

	// Delete all notifications referencing this status.
	if err := s.state.DB.DeleteNotificationsForStatus(ctx,
		status.ID); err != nil {
		log.Errorf("db error deleting notifications: %v", err)
	}

	// Before handling media, ensure
	// historic edits are populated.
	if !status.EditsPopulated() {
		var err error

		// Fetch all historic edits of status from database.
		status.Edits, err = s.state.DB.GetStatusEditsByIDs(
			gtscontext.SetBarebones(ctx),
			status.EditIDs,
		)
		if err != nil {
			log.Errorf("db error getting status edits: %v", err)
		}
	}

	if includeMedia {
		// Media included, delete all attachments for this status (including edits).
		err := s.state.DB.DeleteAttachments(ctx, status.AllAttachmentIDs()...)
		if err != nil {
			log.Errorf("db error deleting media: %v", err)
		}
	} else {
		// Media not included, simply unattach all attachments from this status.
		err := s.state.DB.UnattachAttachments(ctx, status.AllAttachmentIDs()...)
		if err != nil {
			log.Errorf("db error unattaching media: %v", err)
		}
	}

	// Delete all historical edits of status.
	if ids := status.EditIDs; len(ids) > 0 {
		if err := s.state.DB.DeleteStatusEdits(ctx, ids); err != nil {
			log.Errorf("db error deleting edits: %v", err)
		}
	}

	// Delete any mentions attached to status.
	if ids := status.MentionIDs; len(ids) > 0 {
		if err := s.state.DB.DeleteMentions(ctx, ids...); err != nil {
			log.Errorf("db error deleting mentions: %v", err)
		}
	}

	// Delete all local bookmarks targetting this status.
	if err := s.state.DB.DeleteStatusBookmarksForStatus(ctx,
		status.ID); err != nil {
		log.Errorf("db error deleting bookmarks: %v", err)
	}

	// Delete any status pin targetting this status.
	if err := s.state.DB.DeleteStatusPin(ctx, status.ID); //
	err != nil {
		log.Errorf("db error deleting pin: %v", err)
	}

	// Delete all stored favourites targetting status.
	if err := s.state.DB.DeleteStatusFavesForStatus(ctx,
		status.ID); err != nil {
		log.Errorf("db error deleting faves: %v", err)
	}

	if id := status.PollID; id != "" {
		// Delete stored poll attached to this status.
		if err := s.state.DB.DeletePollByID(ctx, id); //
		err != nil {
			log.Errorf("db error deleting poll %s: %v", id, err)
		}
	}

	// Get IDs of any boosts referencing this status.
	boostIDs, err := s.state.DB.GetStatusBoostIDs(ctx,
		status.ID)
	if err != nil {
		log.Errorf("db error getting boosts: %v", err)
	}

	if len(boostIDs) > 0 {
		// Delete all boosts with the given selected IDs.
		if err := s.DeleteStatusBoosts(ctx, boostIDs...); //
		err != nil {
			log.Errorf("db error deleting boosts: %v", err)
		}
	}

	// Delete this status from direct message conversations it's part of.
	if err := s.state.DB.DeleteStatusFromConversations(ctx, status.ID); //
	err != nil {
		log.Errorf("db error deleting status from conversations: %v", err)
	}
}

// onStatusDelete handles shared side-effects
// on deletion (or stubbing) of a status model.
func (s *statusDB) onStatusDelete(
	ctx context.Context,
	tx bun.Tx,
	status *gtsmodel.Status,
) error {

	// delete links between this
	// status and any emojis it uses
	if _, err := tx.NewDelete().
		Table("status_to_emojis").
		Where("? = ?", bun.Ident("status_id"), status.ID).
		Exec(ctx); err != nil {
		return gtserror.Newf("error deleting emoji links: %w", err)
	}

	// delete links between this
	// status and any tags it uses
	if _, err := tx.NewDelete().
		Table("status_to_tags").
		Where("? = ?", bun.Ident("status_id"), status.ID).
		Exec(ctx); err != nil {
		return gtserror.Newf("error deleting tag links: %w", err)
	}

	// decrement status author statistics.
	return decrementAccountStats(ctx, tx,
		"statuses_count",
		status.AccountID,
	)
}

func (s *statusDB) DeleteStatusBoosts(ctx context.Context, ids ...string) error {
	deleted := make([]*gtsmodel.Status, 0, len(ids))

	// Delete boosts by their ID.
	if _, err := s.db.NewDelete().
		Model(&deleted).
		Where("? IN (?)", bun.Ident("id"), bun.List(ids)).
		Where("? IS NOT NULL", bun.Ident("boost_of_id")). // to double check
		Returning("?, ?, ?",
			bun.Ident("id"),
			bun.Ident("account_id"),
			bun.Ident("thread_id"),
		).Exec(ctx); err != nil && !errors.Is(err, db.ErrNoEntries) {
		return err
	}

	// Invalidate related DB caches.
	for _, status := range deleted {
		s.state.Caches.OnInvalidateStatus(status)
	}

	return nil
}

func (s *statusDB) DeleteOldRemoteStatuses(
	ctx context.Context,
	olderThan id.ULID,
	page *paging.Page,
) (
	int, // count
	*paging.Page, // next page
	error,
) {
	if page == nil || page.Limit < 1 {
		panic("paging is required")
	}

	// An example of this query:
	// SELECT DISTINCT "statuses"."thread_id" FROM "statuses"
	// WHERE (NOT EXISTS (SELECT 1 FROM "statuses" AS "inner"
	// WHERE ("inner"."thread_id" = "statuses"."thread_id") AND
	// (("inner"."flags" & 8 != 0) OR ("inner"."fetched_at" > '2020-05-16 11:06:18.023+00:00'))))
	// AND ("statuses"."thread_id" < '01DB2QFDE7N8KMTSCX1XYQM260')
	// ORDER BY "statuses"."thread_id" DESC LIMIT 50

	// A quick type alias
	// to make things a
	// bit more concise.
	type ID = bun.Ident

	// Extract page params.
	maxID := page.Max.Value
	limit := page.Limit
	if page.Order() != paging.OrderDescending {
		panic("must be descending page order")
	} else if limit <= 0 {
		panic("a limit must be supplied")
	}

	// Extract timestamp from 'olderThan'.
	olderThanTs := olderThan.Timestamp()

	// Preallocate slice to store threads.
	threadIDs := make([]string, 0, limit)

	// Start the main query, SELECT
	// distinct `thread_id` entries.
	q := s.db.NewSelect()
	q = q.Distinct()
	q = q.Table("statuses")
	q = q.Column("statuses.thread_id")

	// Start the sub query, SELECTing
	// a 1 return where the local flag
	// is set OR fetched_at is recent.
	//
	// i.e. in the grand scheme of the
	// query this lets us filter threads
	// by those not fetched recently and
	// not containing any local statuses.
	sub := q.NewSelect()
	sub = sub.TableExpr("? AS ?", ID("statuses"), ID("inner"))
	sub = sub.ColumnExpr("1")
	sub = sub.Where("? = ?", ID("inner.thread_id"), ID("statuses.thread_id"))
	sub = sub.WhereGroup("AND", func(sub *bun.SelectQuery) *bun.SelectQuery {
		sub = sub.Where(db.BitIsSet("inner.flags", gtsmodel.StatusFlagLocal))
		sub = sub.WhereOr("? > ?", ID("inner.fetched_at"), olderThanTs)
		return sub
	})

	// Apply the sub-query as a WHERE
	// NOT EXISTS to the main query,
	// and apply paging parameters to
	// thread_id to given age window.
	q = q.Where("NOT EXISTS (?)", sub)
	q = q.Where("? < ?", ID("statuses.thread_id"), maxID)
	q = q.OrderExpr("? DESC", ID("statuses.thread_id"))
	q = q.Limit(limit)

	// Scan query results into slice.
	err := q.Scan(ctx, &threadIDs)
	if err != nil {
		return 0, nil, gtserror.Newf("error selecting remote threads: %w", err)
	}

	// Check for a return.
	if len(threadIDs) == 0 {
		return 0, nil, nil
	}

	// Return a copy of page
	// with updated maxID value.
	next := new(paging.Page)
	(*next) = (*page)
	next.Max.Value = threadIDs[0]

	// Create an ID lookup map to store returned
	// thread IDs that we're going to filter out.
	withInteractions := make(idmap, len(threadIDs))

	// Convert our list of threadIDs to
	// a scannable struct with CTE tags.
	values := toThreadIDValues(threadIDs)

	// Scan database for statuses contained
	// in each thread ID, looking for threads
	// that contain statuses with favourites
	// by local accounts. This query returns
	// only threads with local favourites,
	// i.e. the inverse of what we want.
	q = s.db.NewSelect()
	q = q.With("threads", q.NewValues(&values))
	q = q.Table("threads")
	q = q.Column("threads.thread_id")
	q = q.Join("JOIN ? ON ? = ?", ID("statuses"), ID("statuses.thread_id"), ID("threads.thread_id"))
	q = q.Join("JOIN ? ON ? = ?", ID("status_faves"), ID("status_faves.status_id"), ID("statuses.id"))
	q = q.Join("JOIN ? ON ? = ?", ID("accounts"), ID("accounts.id"), ID("status_faves.account_id"))
	q = q.Where("? IS NULL", ID("accounts.domain"))
	if err := q.Scan(ctx, &withInteractions); err != nil {
		return 0, nil, gtserror.Newf("error selecting threads with local faves: %w", err)
	}

	// Filter thread IDs to delete those that have been locally faved.
	if threadIDs = slices.DeleteFunc(threadIDs, func(id string) bool {
		_, ok := withInteractions[id]
		return ok
	}); len(threadIDs) == 0 {

		// i.e. nothing to delete,
		// just return next page.
		return 0, next, nil
	}

	// Convert our remaining list of threadIDs
	// to scannable struct type with CTE tags.
	values = toThreadIDValues(threadIDs)

	// Reset interaction map
	// for use again below.
	clear(withInteractions)

	// Scan database for statuses contained
	// in each thread ID, looking for threads
	// that contain statuses with bookmarks
	// by local accounts. This query returns
	// only threads with local bookmarks,
	// i.e. the inverse of what we want.
	q = s.db.NewSelect()
	q = q.With("threads", q.NewValues(&values))
	q = q.Table("threads")
	q = q.Column("threads.thread_id")
	q = q.Join("JOIN ? ON ? = ?", ID("statuses"), ID("statuses.thread_id"), ID("threads.thread_id"))
	q = q.Join("JOIN ? ON ? = ?", ID("status_bookmarks"), ID("status_bookmarks.status_id"), ID("statuses.id"))
	if err := q.Scan(ctx, &withInteractions); err != nil {
		return 0, nil, gtserror.Newf("error selecting threads with bookmarks: %w", err)
	}

	// Filter our thread IDs to delete those that have been bookmarked.
	if threadIDs = slices.DeleteFunc(threadIDs, func(id string) bool {
		_, ok := withInteractions[id]
		return ok
	}); len(threadIDs) == 0 {

		// i.e. nothing to delete,
		// just return next page.
		return 0, next, nil
	}

	// Prepare slice to store returned deleted statuses.
	statuses := make([]*gtsmodel.Status, 0, len(threadIDs))

	// Delete statuses contained in determined threads.
	statuses, err = s.deleteStatuses(ctx, statuses,
		func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("? IN (?)", bun.Ident("thread_id"), bun.List(threadIDs))
		})
	if err != nil {
		return 0, nil, err
	}

	return len(statuses), next, err
}

func (s *statusDB) DeleteLeafStubStatuses(ctx context.Context, page *paging.Page) (int, *paging.Page, error) {
	return s.deleteStatusPage(ctx, "id", func(q *bun.SelectQuery) *bun.SelectQuery {

		// Only select stubbed statuses.
		q = q.Where(db.BitIsSet("flags",
			gtsmodel.StatusFlagDeleted))

		// Append WHERE clause selecting
		// only stubbed statuses that have
		// zero replies replies to them.
		q = q.Where("(?) = 0",
			q.NewRaw("SELECT COUNT(1) FROM ? AS ? WHERE ? = ?",
				bun.Ident("statuses"),
				bun.Ident("inner"),
				bun.Ident("status.id"),
				bun.Ident("inner.in_reply_to_id"),
			))

		return q
	}, page)
}

// deleteStatusPage is a wrapper around deleteStatuses()
// that adds paging on the given column name, returning
// the number of deleted statuses and page for the next
// determined page of statuses based on just deleted.
func (s *statusDB) deleteStatusPage(
	ctx context.Context,
	col string,
	where func(*bun.SelectQuery) *bun.SelectQuery,
	page *paging.Page,
) (
	int, // deleted count
	*paging.Page, // next page
	error,
) {
	if page == nil || page.Limit < 1 {
		panic("paging is required")
	}

	// Extract page params.
	minID := page.Min.Value
	maxID := page.Max.Value
	limit := page.Limit
	order := page.Order()

	// Prepare slice to store returned statuses.
	statuses := make([]*gtsmodel.Status, 0, limit)

	// Finally, delete the statuses, within page.
	statuses, err := s.deleteStatuses(ctx, statuses,
		func(q *bun.SelectQuery) *bun.SelectQuery {

			// Apply caller specific
			// select query filtering.
			q = where(q)

			if maxID != "" {
				// Set a maximum ID boundary if was given.
				q = q.Where("? < ?", bun.Ident(col), maxID)
			}

			if minID != "" {
				// Set a minimum ID boundary if was given.
				q = q.Where("? > ?", bun.Ident(col), minID)
			}

			// Set query ordering.
			if order.Ascending() {
				q = q.OrderExpr("? ASC", bun.Ident(col))
			} else /* i.e. descending */ {
				q = q.OrderExpr("? DESC", bun.Ident(col))
			}

			// A limit should always
			// be supplied for this.
			q = q.Limit(limit)

			return q
		})
	if err != nil || len(statuses) == 0 {
		return 0, nil, err
	}

	// Ensure statuses are
	// in expected order.
	if order.Ascending() {
		slices.Reverse(statuses)
	}

	// Calculate and return the
	// next page up for deletion.
	lo := statuses[len(statuses)-1].ID
	return len(statuses), page.Next(lo, ""), err
}

// deleteStatuses encapsulates common logic for
// deleting batches of statuses and handling any
// necessary cache eviction, returning the mostly
// bare (except for core fields) deleted statuses.
func (s *statusDB) deleteStatuses(
	ctx context.Context,
	dst []*gtsmodel.Status, // preallocated delete destination slice
	where func(*bun.SelectQuery) *bun.SelectQuery,
) (
	[]*gtsmodel.Status,
	error,
) {
	// Empty the
	// input slice.
	dst = dst[:0]

	// Start preparing the SELECT query.
	q := s.db.NewSelect().Model(&dst)

	// Apply caller specific
	// select query filtering.
	q = where(q)

	// Perform the actual database query.
	if err := q.Scan(ctx); err != nil {
		return dst, err
	}

	// Check for values.
	if len(dst) == 0 {
		return dst, nil
	}

	// Perform "pre-delete" hooks for status,
	// cleaning up any associated models.
	for _, status := range dst {
		const media = true // include media
		s.preStatusDelete(ctx, status, media)
	}

	// Get status IDs for the actual delete query
	// and to later minimize cache mutex unlocks.
	statusIDs := make([]string, 0, len(dst))
	statusIDs = xslices.Gather(statusIDs, dst,
		func(s *gtsmodel.Status) string { return s.ID })

	// Now actually DELETE the statuses by their IDs! This has
	// to be a separate query as both PG and SQLite (in our form)
	// DO NOT support ordered / limited DELETE queries, oddly.
	if _, err := s.db.NewDelete().
		Table("statuses").
		Where("? IN (?)", bun.Ident("id"), bun.List(statusIDs)).
		Exec(ctx); err != nil && !errors.Is(err, db.ErrNoEntries) {
		return dst, err
	}

	// Invalidate all status IDs from cache in one call.
	s.state.Caches.DB.Status.InvalidateIDs("ID", statusIDs)

	// Manually call invalidate hooks
	// for statuses in case not cached.
	for _, status := range dst {
		s.state.Caches.OnInvalidateStatus(status)
	}

	return dst, nil
}

func (s *statusDB) GetStatusesUsingEmoji(ctx context.Context, emojiID string) ([]*gtsmodel.Status, error) {
	var statusIDs []string

	// SELECT all statuses using this emoji,
	// using a relational table for improved perf.
	if _, err := s.db.NewSelect().
		Table("status_to_emojis").
		Column("status_id").
		Where("? = ?", bun.Ident("emoji_id"), emojiID).
		Exec(ctx, &statusIDs); err != nil {
		return nil, err
	}

	// Convert status IDs into status objects.
	return s.GetStatusesByIDs(ctx, statusIDs)
}

func (s *statusDB) GetStatusParents(ctx context.Context, status *gtsmodel.Status) ([]*gtsmodel.Status, error) {
	var parents []*gtsmodel.Status

	for id := status.InReplyToID; id != ""; {
		parent, err := s.GetStatusByID(ctx, id)
		if err != nil && !errors.Is(err, db.ErrNoEntries) {
			return nil, err
		}

		if parent == nil {
			// Parent status not
			// found (e.g. deleted)
			break
		}

		// Append parent status to slice
		parents = append(parents, parent)

		// Set the next parent ID
		id = parent.InReplyToID
	}

	return parents, nil
}

func (s *statusDB) GetStatusChildren(ctx context.Context, statusID string) ([]*gtsmodel.Status, error) {
	// Get all replies for the currently set status.
	replies, err := s.GetStatusReplies(ctx, statusID)
	if err != nil {
		return nil, err
	}

	// Make estimated preallocation based on direct replies.
	children := make([]*gtsmodel.Status, 0, len(replies)*2)

	for _, status := range replies {
		// Append status to children.
		children = append(children, status)

		// Further, recursively get all children for this reply.
		grandChildren, err := s.GetStatusChildren(ctx, status.ID)
		if err != nil {
			return nil, err
		}

		// Append all sub children after status.
		children = append(children, grandChildren...)
	}

	return children, nil
}

func (s *statusDB) GetStatusReplies(ctx context.Context, statusID string) ([]*gtsmodel.Status, error) {
	statusIDs, err := s.getStatusReplyIDs(ctx, statusID)
	if err != nil {
		return nil, err
	}
	return s.GetStatusesByIDs(ctx, statusIDs)
}

func (s *statusDB) CountStatusReplies(ctx context.Context, statusID string) (int, error) {
	return s.state.Caches.DB.InReplyToIDs.Count(statusID, func() ([]string, error) {
		return getStatusReplyIDs(ctx, s.db, statusID)
	})
}

func (s *statusDB) getStatusReplyIDs(ctx context.Context, statusID string) ([]string, error) {
	return s.state.Caches.DB.InReplyToIDs.Load(statusID, func() ([]string, error) {
		return getStatusReplyIDs(ctx, s.db, statusID)
	})
}

func getStatusReplyIDs(ctx context.Context, bundb *bun.DB, statusID string) ([]string, error) {
	var statusIDs []string
	err := bundb.NewSelect().
		Table("statuses").
		Column("id").
		Where("? = ?", bun.Ident("in_reply_to_id"), statusID).
		Order("id DESC").
		Scan(ctx, &statusIDs)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		return nil, err
	}
	return statusIDs, nil
}

func (s *statusDB) GetStatusBoosts(ctx context.Context, statusID string) ([]*gtsmodel.Status, error) {
	statusIDs, err := s.GetStatusBoostIDs(ctx, statusID)
	if err != nil {
		return nil, err
	}
	return s.GetStatusesByIDs(ctx, statusIDs)
}

func (s *statusDB) IsStatusBoostedBy(ctx context.Context, statusID string, accountID string) (bool, error) {
	boost, err := s.GetStatusBoost(
		gtscontext.SetBarebones(ctx),
		statusID,
		accountID,
	)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		return false, err
	}
	return (boost != nil), nil
}

func (s *statusDB) CountStatusBoosts(ctx context.Context, statusID string) (int, error) {
	return s.state.Caches.DB.BoostOfIDs.Count(statusID, func() ([]string, error) {
		return getStatusBoostIDs(ctx, s.db, statusID)
	})
}

func (s *statusDB) GetStatusQuotes(ctx context.Context, statusID string, page *paging.Page) ([]*gtsmodel.Status, error) {
	quoteIDs, err := s.state.Caches.DB.QuoteOfIDs.Load(statusID, func() ([]string, error) {
		return getStatusQuoteIDs(ctx, s.db, statusID)
	})
	if err != nil {
		return nil, err
	}
	if page != nil {
		quoteIDs = page.Page(quoteIDs)
	}
	return s.GetStatusesByIDs(ctx, quoteIDs)
}

func (s *statusDB) CountStatusQuotes(ctx context.Context, statusID string) (int, error) {
	return s.state.Caches.DB.QuoteOfIDs.Count(statusID, func() ([]string, error) {
		return getStatusQuoteIDs(ctx, s.db, statusID)
	})
}

func getStatusQuoteIDs(ctx context.Context, bundb *bun.DB, statusID string) ([]string, error) {
	var statusIDs []string
	err := bundb.NewSelect().
		Table("statuses").
		Column("id").
		Where("? = ?", bun.Ident("quote_id"), statusID).
		Order("id DESC").
		Scan(ctx, &statusIDs)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		return nil, err
	}
	return statusIDs, nil
}

func (s *statusDB) GetStatusBoostIDs(ctx context.Context, statusID string) ([]string, error) {
	return s.state.Caches.DB.BoostOfIDs.Load(statusID, func() ([]string, error) {
		return getStatusBoostIDs(ctx, s.db, statusID)
	})
}

func getStatusBoostIDs(ctx context.Context, bundb *bun.DB, statusID string) ([]string, error) {
	var statusIDs []string
	err := bundb.NewSelect().
		Table("statuses").
		Column("id").
		Where("? = ?", bun.Ident("boost_of_id"), statusID).
		Order("id DESC").
		Scan(ctx, &statusIDs)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		return nil, err
	}
	return statusIDs, nil
}

func (s *statusDB) MaxDirectStatusID(ctx context.Context) (string, error) {
	maxID := ""
	if err := s.db.
		NewSelect().
		Model((*gtsmodel.Status)(nil)).
		ColumnExpr("COALESCE(MAX(?), '')", bun.Ident("id")).
		Where("? = ?", bun.Ident("visibility"), gtsmodel.VisibilityDirect).
		Scan(ctx, &maxID); // nocollapse
	err != nil {
		return "", err
	}
	return maxID, nil
}

func (s *statusDB) GetDirectStatusIDsBatch(ctx context.Context, minID string, maxIDInclusive string, count int) ([]string, error) {
	var statusIDs []string
	if err := s.db.
		NewSelect().
		Model((*gtsmodel.Status)(nil)).
		Column("id").
		Where("? = ?", bun.Ident("visibility"), gtsmodel.VisibilityDirect).
		Where("? > ?", bun.Ident("id"), minID).
		Where("? <= ?", bun.Ident("id"), maxIDInclusive).
		Order("id ASC").
		Limit(count).
		Scan(ctx, &statusIDs); // nocollapse
	err != nil {
		return nil, err
	}
	return statusIDs, nil
}

func (s *statusDB) GetStatusInteractions(
	ctx context.Context,
	statusID string,
	localOnly bool,
) ([]gtsmodel.Interaction, error) {
	// Prepare to get interactions.
	interactions := []gtsmodel.Interaction{}

	// Gather faves.
	faves, err := s.state.DB.GetStatusFaves(ctx, statusID)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		return nil, err
	}

	for _, fave := range faves {
		// Get account at least.
		if fave.Account == nil {
			fave.Account, err = s.state.DB.GetAccountByID(ctx, fave.AccountID)
			if err != nil {
				log.Errorf(ctx, "error getting account for fave: %v", err)
				continue
			}
		}

		if localOnly && !fave.Account.IsLocal() {
			// Skip not local.
			continue
		}

		interactions = append(interactions, fave)
	}

	// Gather replies.
	replies, err := s.state.DB.GetStatusReplies(ctx, statusID)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		return nil, err
	}

	for _, reply := range replies {
		// Get account at least.
		if reply.Account == nil {
			reply.Account, err = s.state.DB.GetAccountByID(ctx, reply.AccountID)
			if err != nil {
				log.Errorf(ctx, "error getting account for reply: %v", err)
				continue
			}
		}

		if localOnly && !reply.Account.IsLocal() {
			// Skip not local.
			continue
		}

		interactions = append(interactions, reply)
	}

	// Gather boosts.
	boosts, err := s.state.DB.GetStatusBoosts(ctx, statusID)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		return nil, err
	}

	for _, boost := range boosts {
		// Get account at least.
		if boost.Account == nil {
			boost.Account, err = s.state.DB.GetAccountByID(ctx, boost.AccountID)
			if err != nil {
				log.Errorf(ctx, "error getting account for boost: %v", err)
				continue
			}
		}

		if localOnly && !boost.Account.IsLocal() {
			// Skip not local.
			continue
		}

		interactions = append(interactions, boost)
	}

	if len(interactions) == 0 {
		return nil, db.ErrNoEntries
	}

	return interactions, nil
}

func (s *statusDB) GetStatusByEditID(ctx context.Context, editID string) (*gtsmodel.Status, error) {
	edit, err := s.state.DB.GetStatusEditByID(
		gtscontext.SetBarebones(ctx),
		editID,
	)
	if err != nil {
		return nil, err
	}
	return s.GetStatusByID(ctx, edit.StatusID)
}
