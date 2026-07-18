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
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"code.superseriousbusiness.org/gotosocial/internal/typeutils"
)

const schedulerID = "@bluesky"

var schedulerRunning atomic.Bool

func ScheduleJobs(state *state.State, converter *typeutils.Converter) error {
	if !state.Workers.Scheduler.AddRecurring(schedulerID, time.Now().Add(time.Minute), time.Minute, func(ctx context.Context, _ time.Time) {
		if !schedulerRunning.CompareAndSwap(false, true) {
			log.Warn(ctx, "skipping overlapping Bluesky background run")
			return
		}
		defer schedulerRunning.Store(false)
		processDueDeliveries(ctx, state, converter)
		if err := SyncInteractions(ctx, state); err != nil {
			log.Errorf(ctx, "error syncing Bluesky interactions: %v", err)
		}
	}) {
		return fmt.Errorf("Bluesky scheduler is already registered")
	}
	return nil
}

func processDueDeliveries(ctx context.Context, state *state.State, converter *typeutils.Converter) {
	now := time.Now()
	// Claim immediately before processing so leases cannot expire while jobs
	// wait behind a large batch of media uploads.
	for processed := 0; processed < 100; processed++ {
		deliveries, err := state.DB.ClaimDueBlueskyDeliveries(ctx, now, time.Now().Add(5*time.Minute), 1)
		if err != nil {
			log.Errorf(ctx, "error loading queued Bluesky deliveries: %v", err)
			break
		}
		if len(deliveries) == 0 {
			break
		}
		delivery := deliveries[0]
		if err := ProcessDelivery(ctx, state, converter, delivery); err != nil {
			log.Errorf(ctx, "error retrying Bluesky delivery for status %s: %v", delivery.StatusID, err)
		}
	}
	if err := state.DB.DeleteExpiredBlueskyOAuthStates(ctx, now.Add(-15*time.Minute)); err != nil {
		log.Errorf(ctx, "error deleting expired Bluesky OAuth states: %v", err)
	}
}
