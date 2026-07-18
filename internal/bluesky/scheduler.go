// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"context"
	"fmt"
	"time"

	"code.superseriousbusiness.org/gopkg/log"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"code.superseriousbusiness.org/gotosocial/internal/typeutils"
)

const schedulerID = "@bluesky"

func ScheduleJobs(state *state.State, converter *typeutils.Converter) error {
	if !state.Workers.Scheduler.AddRecurring(schedulerID, time.Now(), time.Minute, func(ctx context.Context, _ time.Time) {
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
	deliveries, err := state.DB.GetDueBlueskyDeliveries(ctx, time.Now(), 100)
	if err != nil {
		log.Errorf(ctx, "error loading queued Bluesky deliveries: %v", err)
		return
	}
	for _, delivery := range deliveries {
		if err := ProcessDelivery(ctx, state, converter, delivery); err != nil {
			log.Errorf(ctx, "error retrying Bluesky delivery for status %s: %v", delivery.StatusID, err)
		}
	}
}
