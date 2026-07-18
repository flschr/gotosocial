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

// ShouldUpsertMappedStatus decides how an existing remote mapping should
// follow a local edit. A temporarily disconnected account keeps eligible
// upserts queued; disconnecting must never turn an edit into a deletion.
func ShouldUpsertMappedStatus(status *gtsmodel.Status, connection *gtsmodel.BlueskyConnection, isReply bool) bool {
	if isReply {
		return true
	}
	if connection == nil {
		return EligibleForExistingMapping(status, false)
	}
	if connection.Active() {
		return EligibleForCrosspost(status, connection)
	}
	return connection.CrosspostPublic && EligibleForExistingMapping(status, false)
}

func QueueStatus(ctx context.Context, state *state.State, status *gtsmodel.Status) (*gtsmodel.BlueskyDelivery, error) {
	return queueDelivery(ctx, state, status.AccountID, status.ID, deliveryUpsert)
}

func QueueDelete(ctx context.Context, state *state.State, accountID, statusID string) (*gtsmodel.BlueskyDelivery, error) {
	return queueDelivery(ctx, state, accountID, statusID, deliveryDelete)
}

func queueDelivery(ctx context.Context, state *state.State, accountID, statusID, action string) (*gtsmodel.BlueskyDelivery, error) {
	now := time.Now()
	delivery := &gtsmodel.BlueskyDelivery{
		ID: id.NewULID(), AccountID: accountID, StatusID: statusID,
		Action: action, Generation: 1, UpdatedAt: now, NextAttemptAt: now,
	}
	if err := state.DB.PutBlueskyDelivery(ctx, delivery); err != nil {
		return nil, err
	}
	return state.DB.GetBlueskyDeliveryByStatusID(ctx, statusID)
}

func ProcessDelivery(ctx context.Context, state *state.State, converter *typeutils.Converter, delivery *gtsmodel.BlueskyDelivery) error {
	if delivery.Action == deliveryDelete {
		defer lockAccount(delivery.AccountID)()
		if err := deleteStatus(ctx, state, delivery.StatusID, false, false); err != nil {
			return recordDeliveryFailure(ctx, state, delivery, err)
		}
		return completeDelivery(ctx, state, delivery)
	}
	status, err := state.DB.GetStatusByID(ctx, delivery.StatusID)
	if errors.Is(err, db.ErrNoEntries) {
		if _, mappingErr := state.DB.GetBlueskyPostByStatusID(ctx, delivery.StatusID); mappingErr == nil {
			if err := deleteStatus(ctx, state, delivery.StatusID, false, true); err != nil {
				return recordDeliveryFailure(ctx, state, delivery, err)
			}
			return completeDelivery(ctx, state, delivery)
		}
		return completeDelivery(ctx, state, delivery)
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
		if _, mappingErr := state.DB.GetBlueskyPostByStatusID(ctx, status.ID); mappingErr == nil {
			return recordDeliveryFailure(ctx, state, delivery, &ConnectionError{Code: ErrorCodeData, Err: fmt.Errorf("saved Bluesky identity is unavailable: %w", err)})
		}
		return completeDelivery(ctx, state, delivery)
	}
	if err != nil {
		return recordDeliveryFailure(ctx, state, delivery, err)
	}
	replyTarget, replyErr := replyTargetForStatus(ctx, state, status)
	if errors.Is(replyErr, db.ErrNoEntries) {
		if mapping, mappingErr := state.DB.GetBlueskyPostByStatusID(ctx, status.ID); mappingErr == nil && !ShouldUpsertMappedStatus(status, connection, false) {
			if err := deleteStatus(ctx, state, mapping.StatusID, false, false); err != nil {
				return recordDeliveryFailure(ctx, state, delivery, err)
			}
			return completeDelivery(ctx, state, delivery)
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
	return completeDelivery(ctx, state, delivery)
}

func completeDelivery(ctx context.Context, state *state.State, delivery *gtsmodel.BlueskyDelivery) error {
	_, err := state.DB.CompleteBlueskyDelivery(ctx, delivery.ID, delivery.Generation, delivery.ClaimID)
	return err
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
	delivery.LastErrorCode = errorCode(cause)
	delivery.DeadLetter = delivery.Attempts >= 10
	if _, err := state.DB.UpdateClaimedBlueskyDelivery(ctx, delivery, "attempts", "next_attempt_at", "last_error", "last_error_code", "updated_at", "claimed_until", "dead_letter"); err != nil {
		return fmt.Errorf("record Bluesky delivery failure after %v: %w", cause, err)
	}
	return cause
}

func retryDelay(attempts int) time.Duration {
	return time.Minute * time.Duration(1<<min(attempts-1, 6))
}
