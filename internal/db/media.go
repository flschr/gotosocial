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

package db

import (
	"context"

	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/paging"
)

// Media contains functions related to creating/getting/removing media attachments.
type Media interface {
	// GetAttachmentByID gets a single attachment by its ID.
	GetAttachmentByID(ctx context.Context, id string) (*gtsmodel.MediaAttachment, error)

	// GetAttachmentByURL gets a single attachment by its local public URL.
	GetAttachmentByURL(ctx context.Context, url string) (*gtsmodel.MediaAttachment, error)

	// GetAttachmentsByIDs fetches a list of media attachments for given IDs.
	GetAttachmentsByIDs(ctx context.Context, ids []string) ([]*gtsmodel.MediaAttachment, error)

	// PutAttachment inserts the given attachment into the database.
	PutAttachment(ctx context.Context, media *gtsmodel.MediaAttachment) error

	// UpdateAttachment will update the given attachment in the database.
	UpdateAttachment(ctx context.Context, media *gtsmodel.MediaAttachment, columns ...string) error

	// UnattachAttachments will unattach given media attachments from any status.
	UnattachAttachments(ctx context.Context, ids ...string) error

	// DeleteAttachment will delete the single given media attachment from the database.
	DeleteAttachment(ctx context.Context, media *gtsmodel.MediaAttachment) error

	// DeleteAttachments will delete the given media attachments from the database.
	DeleteAttachments(ctx context.Context, ids ...string) error

	// GetAttachments fetches media attachments, with given paging parameters.
	GetAttachments(ctx context.Context, page *paging.Page) ([]*gtsmodel.MediaAttachment, error)

	// GetAttachmentsByAccountID fetches media attachments by account ID, with given paging parameters.
	GetAttachmentsByAccountID(ctx context.Context, accountID string, page *paging.Page) ([]*gtsmodel.MediaAttachment, error)

	// GetRemoteAttachments fetches media attachments with a non-empty domain, with given paging parameters.
	GetRemoteAttachments(ctx context.Context, page *paging.Page) ([]*gtsmodel.MediaAttachment, error)

	// GetCachedAttachments fetches cached media attachments with a non-empty domain, with given paging parameters.
	GetCachedAttachments(ctx context.Context, page *paging.Page) ([]*gtsmodel.MediaAttachment, error)
}
