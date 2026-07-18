// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"context"
	"errors"
	"fmt"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"code.superseriousbusiness.org/gotosocial/internal/typeutils"
)

const (
	deliveryUpsert = "upsert"
	deliveryDelete = "delete"
)

func QueueStatus(ctx context.Context, state *state.State, status *gtsmodel.Status) (*gtsmodel.BlueskyDelivery, error) {
	return queueDelivery(ctx, state, status.AccountID, status.ID, deliveryUpsert)
}

func QueueDelete(ctx context.Context, state *state.State, accountID, statusID string) (*gtsmodel.BlueskyDelivery, error) {
	return queueDelivery(ctx, state, accountID, statusID, deliveryDelete)
}

func queueDelivery(ctx context.Context, state *state.State, accountID, statusID, action string) (*gtsmodel.BlueskyDelivery, error) {
	if existing, err := state.DB.GetBlueskyDeliveryByStatusID(ctx, statusID); err == nil {
		existing.Action, existing.Attempts, existing.NextAttemptAt = action, 0, time.Now()
		existing.ClaimedUntil, existing.LastError, existing.DeadLetter, existing.UpdatedAt = time.Time{}, "", false, time.Now()
		if err := state.DB.UpdateBlueskyDelivery(ctx, existing, "action", "attempts", "next_attempt_at", "claimed_until", "last_error", "dead_letter", "updated_at"); err != nil {
			return nil, err
		}
		return existing, nil
	} else if !errors.Is(err, db.ErrNoEntries) {
		return nil, err
	}
	delivery := &gtsmodel.BlueskyDelivery{ID: id.NewULID(), AccountID: accountID, StatusID: statusID, Action: action, NextAttemptAt: time.Now()}
	if err := state.DB.PutBlueskyDelivery(ctx, delivery); err != nil {
		return nil, err
	}
	return delivery, nil
}

func ProcessDelivery(ctx context.Context, state *state.State, converter *typeutils.Converter, delivery *gtsmodel.BlueskyDelivery) error {
	if delivery.Action == deliveryDelete {
		defer lockAccount(delivery.AccountID)()
		if err := DeleteStatus(ctx, state, delivery.StatusID); err != nil {
			return recordDeliveryFailure(ctx, state, delivery, err)
		}
		return nil
	}
	status, err := state.DB.GetStatusByID(ctx, delivery.StatusID)
	if errors.Is(err, db.ErrNoEntries) {
		if _, mappingErr := state.DB.GetBlueskyPostByStatusID(ctx, delivery.StatusID); mappingErr == nil {
			if err := DeleteStatus(ctx, state, delivery.StatusID); err != nil {
				return recordDeliveryFailure(ctx, state, delivery, err)
			}
			return nil
		}
		return state.DB.DeleteBlueskyDeliveryByStatusID(ctx, delivery.StatusID)
	}
	if err != nil {
		return recordDeliveryFailure(ctx, state, delivery, err)
	}
	defer lockAccount(status.AccountID)()
	if err := state.DB.PopulateStatus(ctx, status); err != nil {
		return recordDeliveryFailure(ctx, state, delivery, err)
	}
	connection, err := state.DB.GetBlueskyConnectionByAccountID(ctx, status.AccountID)
	if errors.Is(err, db.ErrNoEntries) {
		return state.DB.DeleteBlueskyDeliveryByStatusID(ctx, delivery.StatusID)
	}
	if err != nil {
		return recordDeliveryFailure(ctx, state, delivery, err)
	}
	replyTarget, replyErr := replyTargetForStatus(ctx, state, status)
	if errors.Is(replyErr, db.ErrNoEntries) {
		if mapping, mappingErr := state.DB.GetBlueskyPostByStatusID(ctx, status.ID); mappingErr == nil && !EligibleForCrosspost(status, connection) {
			if err := DeleteStatus(ctx, state, mapping.StatusID); err != nil {
				return recordDeliveryFailure(ctx, state, delivery, err)
			}
			return nil
		}
	} else if replyErr != nil {
		return recordDeliveryFailure(ctx, state, delivery, replyErr)
	}
	apiStatus, err := converter.StatusToAPIStatus(ctx, status, status.Account)
	if err == nil {
		if replyTarget != nil {
			err = PublishReply(ctx, state, status, apiStatus.Card, replyTarget)
		} else {
			err = PublishStatus(ctx, state, status, apiStatus.Card)
		}
	}
	if err != nil {
		return recordDeliveryFailure(ctx, state, delivery, err)
	}
	return state.DB.DeleteBlueskyDeliveryByStatusID(ctx, delivery.StatusID)
}

func replyTargetForStatus(ctx context.Context, state *state.State, status *gtsmodel.Status) (*gtsmodel.BlueskyInteraction, error) {
	if ownPost, err := state.DB.GetBlueskyPostByStatusID(ctx, status.ID); err == nil && ownPost.ParentURI != "" {
		return &gtsmodel.BlueskyInteraction{
			AccountID: ownPost.AccountID, URI: ownPost.ParentURI, CID: ownPost.ParentCID,
			RootURI: ownPost.RootURI, RootCID: ownPost.RootCID,
		}, nil
	} else if err != nil && !errors.Is(err, db.ErrNoEntries) {
		return nil, err
	}
	if status.InReplyToID == "" {
		return nil, db.ErrNoEntries
	}
	interaction, err := state.DB.GetBlueskyInteractionByStatusID(ctx, status.InReplyToID)
	if err == nil {
		return interaction, nil
	}
	if !errors.Is(err, db.ErrNoEntries) {
		return nil, err
	}
	post, err := state.DB.GetBlueskyPostByStatusID(ctx, status.InReplyToID)
	if err != nil {
		return nil, err
	}
	rootURI, rootCID := post.RootURI, post.RootCID
	if rootURI == "" {
		rootURI, rootCID = post.URI, post.CID
	}
	return &gtsmodel.BlueskyInteraction{AccountID: post.AccountID, URI: post.URI, CID: post.CID, RootURI: rootURI, RootCID: rootCID}, nil
}

func EligibleForExistingMapping(status *gtsmodel.Status, isReply bool) bool {
	if isReply {
		return true
	}
	return status.Visibility == gtsmodel.VisibilityPublic && !status.LocalOnly() &&
		status.InReplyToID == "" && status.BoostOfID == "" && status.PollID == "" && len(status.MentionIDs) == 0
}

func IsReplyTarget(ctx context.Context, state *state.State, status *gtsmodel.Status) (bool, error) {
	_, err := replyTargetForStatus(ctx, state, status)
	if errors.Is(err, db.ErrNoEntries) {
		return false, nil
	}
	return err == nil, err
}

func recordDeliveryFailure(ctx context.Context, state *state.State, delivery *gtsmodel.BlueskyDelivery, cause error) error {
	delivery.Attempts++
	delivery.NextAttemptAt = time.Now().Add(retryDelay(delivery.Attempts))
	delivery.LastError, delivery.UpdatedAt, delivery.ClaimedUntil = truncateUTF8(cause.Error(), 1000, 4000), time.Now(), time.Time{}
	delivery.DeadLetter = delivery.Attempts >= 10
	if err := state.DB.UpdateBlueskyDelivery(ctx, delivery, "attempts", "next_attempt_at", "last_error", "updated_at", "claimed_until", "dead_letter"); err != nil {
		return fmt.Errorf("record Bluesky delivery failure after %v: %w", cause, err)
	}
	return cause
}

func retryDelay(attempts int) time.Duration {
	return time.Minute * time.Duration(1<<min(attempts-1, 6))
}
