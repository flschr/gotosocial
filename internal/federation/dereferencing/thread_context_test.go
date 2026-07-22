// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package dereferencing

import (
	"testing"
	"time"

	cache "codeberg.org/gruf/go-cache/v3"
	"github.com/stretchr/testify/require"
)

func TestThreadContextRefreshFresh(t *testing.T) {
	now := time.Now()
	d := Dereferencer{
		threadContextRefreshes: cache.New[string, threadContextRefresh](0, 10),
	}

	d.threadContextRefreshes.Set("recent-success", threadContextRefresh{
		at:      now.Add(-ThreadContextFreshness + time.Second),
		success: true,
	})
	d.threadContextRefreshes.Set("expired-success", threadContextRefresh{
		at:      now.Add(-ThreadContextFreshness),
		success: true,
	})
	d.threadContextRefreshes.Set("recent-failure", threadContextRefresh{
		at:      now.Add(-ThreadContextFailureFreshness + time.Second),
		success: false,
	})
	d.threadContextRefreshes.Set("expired-failure", threadContextRefresh{
		at:      now.Add(-ThreadContextFailureFreshness),
		success: false,
	})

	require.True(t, d.threadContextRefreshFresh("recent-success", now))
	require.False(t, d.threadContextRefreshFresh("expired-success", now))
	require.True(t, d.threadContextRefreshFresh("recent-failure", now))
	require.False(t, d.threadContextRefreshFresh("expired-failure", now))
	require.False(t, d.threadContextRefreshFresh("missing", now))
}
