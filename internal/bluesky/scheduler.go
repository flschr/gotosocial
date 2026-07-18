// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"code.superseriousbusiness.org/gopkg/log"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"code.superseriousbusiness.org/gotosocial/internal/typeutils"
)

const (
	deliverySchedulerID    = "@bluesky-delivery"
	interactionSchedulerID = "@bluesky-interactions"
)

var deliverySchedulerRunning atomic.Bool
var interactionSchedulerRunning atomic.Bool

func ScheduleJobs(state *state.State, converter *typeutils.Converter) error {
	if !state.Workers.Scheduler.AddRecurring(deliverySchedulerID, time.Now().Add(time.Minute), time.Minute, func(ctx context.Context, _ time.Time) {
		if !deliverySchedulerRunning.CompareAndSwap(false, true) {
			log.Warn(ctx, "skipping overlapping Bluesky delivery run")
			return
		}
		defer deliverySchedulerRunning.Store(false)
		if err := ReconcileOutbox(ctx, state); err != nil {
			log.Errorf(ctx, "error reconciling Bluesky outbox: %v", err)
		}
		processDueDeliveries(ctx, state, converter)
	}) {
		return fmt.Errorf("Bluesky delivery scheduler is already registered")
	}
	if !state.Workers.Scheduler.AddRecurring(interactionSchedulerID, time.Now().Add(30*time.Second), time.Minute, func(ctx context.Context, _ time.Time) {
		if !interactionSchedulerRunning.CompareAndSwap(false, true) {
			log.Warn(ctx, "skipping overlapping Bluesky interaction run")
			return
		}
		defer interactionSchedulerRunning.Store(false)
		if err := SyncInteractions(ctx, state); err != nil {
			log.Errorf(ctx, "error syncing Bluesky interactions: %v", err)
		}
	}) {
		return fmt.Errorf("Bluesky interaction scheduler is already registered")
	}
	return nil
}

func processDueDeliveries(ctx context.Context, state *state.State, converter *typeutils.Converter) {
	now := time.Now()
	// Claim immediately before processing so leases cannot expire while jobs
	// wait behind a large batch of media uploads.
	for processed := 0; processed < 100; processed++ {
		deliveries, err := state.DB.ClaimDueBlueskyDeliveries(ctx, now, time.Now().Add(2*time.Minute), 1)
		if err != nil {
			log.Errorf(ctx, "error loading queued Bluesky deliveries: %v", err)
			break
		}
		if len(deliveries) == 0 {
			break
		}
		delivery := deliveries[0]
		if err := processDeliveryWithLease(ctx, state, converter, delivery); err != nil {
			log.Errorf(ctx, "error retrying Bluesky delivery for status %s: %v", delivery.StatusID, err)
		}
	}
	if err := state.DB.DeleteExpiredBlueskyOAuthStates(ctx, now.Add(-15*time.Minute)); err != nil {
		log.Errorf(ctx, "error deleting expired Bluesky OAuth states: %v", err)
	}
}

func processDeliveryWithLease(ctx context.Context, state *state.State, converter *typeutils.Converter, delivery *gtsmodel.BlueskyDelivery) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	defer func() { <-done }()
	go func() {
		defer close(done)
		expected := delivery.ClaimedUntil
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				next := time.Now().Add(2 * time.Minute)
				renewed, err := state.DB.RenewBlueskyDeliveryClaim(ctx, delivery.ID, expected, next)
				if err != nil || !renewed {
					cancel()
					return
				}
				expected = next
			}
		}
	}()
	err := ProcessDelivery(ctx, state, converter, delivery)
	cancel()
	return err
}
