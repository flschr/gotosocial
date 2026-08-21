// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"context"
	"errors"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/state"
)

const outboxOverlap = time.Minute

// ReconcileOutbox reconstructs delivery jobs that could have been missed if a
// process stopped between committing a local status change and queueing its
// asynchronous Bluesky work.
func ReconcileOutbox(ctx context.Context, state *state.State) error {
	connections, err := state.DB.GetBlueskyConnections(ctx)
	if err != nil {
		return err
	}
	var reconcileErrors []error
	for _, connection := range connections {
		if err := reconcileConnectionOutbox(ctx, state, connection); err != nil {
			reconcileErrors = append(reconcileErrors, err)
		}
	}
	return errors.Join(reconcileErrors...)
}

func reconcileConnectionOutbox(ctx context.Context, state *state.State, connection *gtsmodel.BlueskyConnection) error {
	if err := reconcileIneligibleMappings(ctx, state, connection); err != nil {
		return err
	}
	checkedAt := time.Now()
	if connection.OutboxCheckedAt.IsZero() {
		connection.OutboxCheckedAt = checkedAt
		if connection.CrosspostPublic && connection.CrosspostEnabledAt.IsZero() {
			connection.CrosspostEnabledAt = checkedAt
			return state.DB.UpdateBlueskyConnection(ctx, connection, "outbox_checked_at", "crosspost_enabled_at")
		}
		return state.DB.UpdateBlueskyConnection(ctx, connection, "outbox_checked_at")
	}
	since := connection.OutboxCheckedAt.Add(-outboxOverlap)
	if !connection.CrosspostEnabledAt.IsZero() && since.Before(connection.CrosspostEnabledAt) {
		since = connection.CrosspostEnabledAt
	}
	statuses, err := state.DB.GetBlueskyStatusesChangedBetween(ctx, connection.AccountID, since, checkedAt)
	if err != nil {
		return err
	}
	for _, status := range statuses {
		if _, err := state.DB.GetBlueskyDeliveryByStatusID(ctx, status.ID); err == nil {
			continue
		} else if !errors.Is(err, db.ErrNoEntries) {
			return err
		}
		mapping, mappingErr := state.DB.GetBlueskyPostByStatusID(ctx, status.ID)
		if mappingErr != nil && !errors.Is(mappingErr, db.ErrNoEntries) {
			return mappingErr
		}
		if mappingErr == nil {
			if !status.Flags.Deleted() && !status.UpdatedAt().After(mapping.UpdatedAt) {
				continue
			}
			if status.Flags.Deleted() {
				if _, err := QueueDelete(ctx, state, connection.AccountID, status.ID); err != nil {
					return err
				}
				continue
			}
			isReply, err := IsReplyTarget(ctx, state, status)
			if err != nil {
				return err
			}
			if ShouldUpsertMappedStatus(status, connection, isReply) {
				_, err = QueueStatus(ctx, state, status)
			} else {
				_, err = QueueDelete(ctx, state, mapping.AccountID, status.ID)
			}
			if err != nil {
				return err
			}
			continue
		}
		if status.Flags.Deleted() {
			continue
		}
		isReply, err := IsReplyTarget(ctx, state, status)
		if err != nil {
			return err
		}
		if ShouldQueueNewStatus(status, connection, isReply) {
			if _, err := QueueStatus(ctx, state, status); err != nil {
				return err
			}
		}
	}
	connection.OutboxCheckedAt = checkedAt
	return state.DB.UpdateBlueskyConnection(ctx, connection, "outbox_checked_at")
}

func reconcileIneligibleMappings(ctx context.Context, state *state.State, connection *gtsmodel.BlueskyConnection) error {
	posts, err := state.DB.GetIneligibleBlueskyPostsByAccountID(ctx, connection.AccountID)
	if err != nil {
		return err
	}
	for _, post := range posts {
		delivery, err := state.DB.GetBlueskyDeliveryByStatusID(ctx, post.StatusID)
		if err == nil && delivery.Action == deliveryDelete {
			continue
		}
		if err != nil && !errors.Is(err, db.ErrNoEntries) {
			return err
		}
		if _, err := QueueDelete(ctx, state, post.AccountID, post.StatusID); err != nil {
			return err
		}
	}
	return nil
}
