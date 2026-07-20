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

	"code.superseriousbusiness.org/gopkg/xslices"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/paging"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"github.com/uptrace/bun"
)

type mediaDB struct {
	db    *bun.DB
	state *state.State
}

func (m *mediaDB) GetAttachmentByID(ctx context.Context, id string) (*gtsmodel.MediaAttachment, error) {
	return m.getAttachment(
		ctx,
		"ID",
		func(attachment *gtsmodel.MediaAttachment) error {
			return m.db.NewSelect().
				Model(attachment).
				Where("? = ?", bun.Ident("media_attachment.id"), id).
				Scan(ctx)
		},
		id,
	)
}

func (m *mediaDB) GetAttachmentsByIDs(ctx context.Context, ids []string) ([]*gtsmodel.MediaAttachment, error) {
	// Load all media IDs via cache loader callbacks.
	media, err := m.state.Caches.DB.Media.LoadIDs("ID",
		ids,
		func(uncached []string) ([]*gtsmodel.MediaAttachment, error) {
			// Preallocate expected length of uncached media attachments.
			media := make([]*gtsmodel.MediaAttachment, 0, len(uncached))

			// Perform database query scanning
			// the remaining (uncached) IDs.
			if err := m.db.NewSelect().
				Model(&media).
				Where("? IN (?)", bun.Ident("id"), bun.List(uncached)).
				Scan(ctx); err != nil {
				return nil, err
			}

			return media, nil
		},
	)
	if err != nil {
		return nil, err
	}

	// Reorder the media by their
	// IDs to ensure in correct order.
	getID := func(m *gtsmodel.MediaAttachment) string { return m.ID }
	xslices.OrderBy(media, ids, getID)

	return media, nil
}

func (m *mediaDB) getAttachment(ctx context.Context, lookup string, dbQuery func(*gtsmodel.MediaAttachment) error, keyParts ...any) (*gtsmodel.MediaAttachment, error) {
	return m.state.Caches.DB.Media.LoadOne(lookup, func() (*gtsmodel.MediaAttachment, error) {
		var attachment gtsmodel.MediaAttachment

		// Not cached! Perform database query
		if err := dbQuery(&attachment); err != nil {
			return nil, err
		}

		return &attachment, nil
	}, keyParts...)
}

func (m *mediaDB) PutAttachment(ctx context.Context, media *gtsmodel.MediaAttachment) error {
	return m.state.Caches.DB.Media.Store(media, func() error {
		_, err := m.db.NewInsert().Model(media).Exec(ctx)
		return err
	})
}

func (m *mediaDB) UpdateAttachment(ctx context.Context, media *gtsmodel.MediaAttachment, columns ...string) error {
	return m.state.Caches.DB.Media.Store(media, func() error {
		_, err := m.db.NewUpdate().
			Model(media).
			Where("? = ?", bun.Ident("id"), media.ID).
			Column(columns...).
			Exec(ctx)
		return err
	})
}

func (m *mediaDB) UnattachAttachments(ctx context.Context, ids ...string) error {
	// Update media attachments with
	// given IDs in the database,
	// clearing their `status_id` col.
	if _, err := m.db.NewUpdate().
		Table("media_attachments").
		Where("? IN (?)", bun.Ident("id"), bun.List(ids)).
		Set("? = NULL", bun.Ident("status_id")).
		Exec(ctx); err != nil && !errors.Is(err, db.ErrNoEntries) {
		return err
	}

	// Invalidate all updated models with given IDs.
	m.state.Caches.DB.Media.InvalidateIDs("ID", ids)

	return nil
}

func (m *mediaDB) DeleteAttachment(ctx context.Context, media *gtsmodel.MediaAttachment) error {
	// Delete media attachments with
	// given IDs from the database.
	if _, err := m.db.NewDelete().
		Table("media_attachments").
		Where("? = ?", bun.Ident("id"), media.ID).
		Exec(ctx); err != nil && !errors.Is(err, db.ErrNoEntries) {
		return err
	}

	// Invalidate deleted media model with its ID.
	m.state.Caches.DB.Media.Invalidate("ID", media.ID)
	m.state.Caches.OnInvalidateMedia(media)

	return nil
}

func (m *mediaDB) DeleteAttachments(ctx context.Context, ids ...string) error {
	deleted := make([]*gtsmodel.MediaAttachment, 0, len(ids))

	// Delete media attachments with
	// given IDs from the database.
	if _, err := m.db.NewDelete().
		Model(&deleted).
		Where("? IN (?)", bun.Ident("id"), bun.List(ids)).
		Returning("?, ?, ?", bun.Ident("id"), bun.Ident("account_id"), bun.Ident("status_id")).
		Exec(ctx); err != nil && !errors.Is(err, db.ErrNoEntries) {
		return err
	}

	// Invalidate all deleted models with given IDs,
	// calling hooks manually in case not in cache.
	m.state.Caches.DB.Media.InvalidateIDs("ID", ids)
	for _, deleted := range deleted {
		m.state.Caches.OnInvalidateMedia(deleted)
	}

	return nil
}

func (m *mediaDB) GetAttachments(ctx context.Context, page *paging.Page) ([]*gtsmodel.MediaAttachment, error) {
	return m.getAttachmentsPagedByID(ctx, nil, page)
}

func (m *mediaDB) GetAttachmentsByAccountID(ctx context.Context, accountID string, page *paging.Page) ([]*gtsmodel.MediaAttachment, error) {
	return m.getAttachmentsPagedByID(ctx, func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("? = ?", bun.Ident("account_id"), accountID)
	}, page)
}

func (m *mediaDB) GetRemoteAttachments(ctx context.Context, page *paging.Page) ([]*gtsmodel.MediaAttachment, error) {
	return m.getAttachmentsPagedByID(ctx, func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("? IS NOT NULL", bun.Ident("remote_url"))
	}, page)
}

func (m *mediaDB) GetCachedAttachments(ctx context.Context, page *paging.Page) ([]*gtsmodel.MediaAttachment, error) {
	return m.getAttachmentsPagedByID(ctx, func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.
			Where("? IS NOT NULL", bun.Ident("remote_url")).
			Where("? != ?", bun.Ident("file_path"), "").
			Where("? != ?", bun.Ident("thumbnail_path"), "")
	}, page)
}

func (m *mediaDB) getAttachmentsPagedByID(ctx context.Context, query func(*bun.SelectQuery) *bun.SelectQuery, page *paging.Page) ([]*gtsmodel.MediaAttachment, error) {
	maxID := page.GetMax()
	minID := page.GetMin()
	limit := page.GetLimit()
	order := page.GetOrder()

	// Pre-allocate slice of dest IDs.
	ids := make([]string, 0, limit)

	// Start building query.
	q := m.db.NewSelect().
		Table("media_attachments").
		Column("id")

	if query != nil {
		// Append caller
		// query details.
		q = query(q)
	}

	if maxID != "" {
		// Set a maximum ID boundary if was given.
		q = q.Where("? < ?", bun.Ident("id"), maxID)
	}

	if minID != "" {
		// Set a minimum ID boundary if was given.
		q = q.Where("? > ?", bun.Ident("id"), minID)
	}

	// Set query ordering.
	if order.Ascending() {
		q = q.OrderExpr("? ASC", bun.Ident("id"))
	} else /* i.e. descending */ {
		q = q.OrderExpr("? DESC", bun.Ident("id"))
	}

	// A limit should always
	// be supplied for this.
	q = q.Limit(limit)

	// Finally, perform query into IDs slice.
	if err := q.Scan(ctx, &ids); err != nil {
		return nil, err
	}

	// Fetch media from DB with given IDs.
	return m.GetAttachmentsByIDs(ctx, ids)
}
