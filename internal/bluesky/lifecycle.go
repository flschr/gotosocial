// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"context"
	"errors"
	"sync"

	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

var accountLocks sync.Map

func lockAccount(accountID string) func() {
	value, _ := accountLocks.LoadOrStore(accountID, new(sync.Mutex))
	mutex := value.(*sync.Mutex)
	mutex.Lock()
	return mutex.Unlock
}

// Disconnect revokes the active OAuth session when possible and always
// removes every locally stored Bluesky credential and mapping for accountID.
func Disconnect(ctx context.Context, state *state.State, accountID string) error {
	defer lockAccount(accountID)()
	connection, err := state.DB.GetBlueskyConnectionByAccountID(ctx, accountID)
	if errors.Is(err, db.ErrNoEntries) {
		return state.DB.DeleteBlueskyDataByAccountID(ctx, accountID)
	}
	if err != nil {
		return err
	}
	if app, _, appErr := NewOAuthClient(state, accountID); appErr == nil && connection.OAuthSessionID != "" {
		if did, parseErr := syntax.ParseDID(connection.DID); parseErr == nil {
			_ = app.Logout(ctx, did, connection.OAuthSessionID)
		}
	}
	return state.DB.DeleteBlueskyDataByAccountID(ctx, accountID)
}
