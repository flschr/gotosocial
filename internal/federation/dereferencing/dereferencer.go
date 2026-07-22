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

package dereferencing

import (
	"context"
	"net/url"
	"sync"
	"time"

	"code.superseriousbusiness.org/gopkg/log"
	"code.superseriousbusiness.org/gotosocial/internal/filter/interaction"
	"code.superseriousbusiness.org/gotosocial/internal/filter/relay"
	"code.superseriousbusiness.org/gotosocial/internal/filter/visibility"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/media"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"code.superseriousbusiness.org/gotosocial/internal/transport"
	"code.superseriousbusiness.org/gotosocial/internal/typeutils"
	cache "codeberg.org/gruf/go-cache/v3"
)

// FreshnessWindow represents a duration in which a
// Status or Account is still considered to be "fresh"
// (ie., not in need of a refresh from remote), if its
// last FetchedAt value falls within the window.
//
// For example, if an Account was FetchedAt 09:00, and it
// is now 12:00, then it would be considered "fresh"
// according to DefaultAccountFreshness, but not according
// to Fresh, which would indicate that the Account requires
// refreshing from remote.
type FreshnessWindow = time.Duration

var (
	// 6 hours.
	//
	// Default window for doing a
	// fresh dereference of an Account.
	DefaultAccountFreshness = 6 * time.Hour

	// 2 hours.
	//
	// Default window for doing a
	// fresh dereference of a Status.
	DefaultStatusFreshness = 2 * time.Hour

	// 5 minutes.
	//
	// Fresh is useful when you're wanting
	// a more up-to-date model of something
	// that exceeds default freshness windows.
	//
	// This is tuned to be quite fresh without
	// causing loads of dereferencing calls.
	Fresh = 5 * time.Minute

	// Immediate.
	//
	// This essentially always allows a refresh,
	// and should only be used if a model update
	// was pushed (federated) to the server,
	// i.e. no model dereference is required.
	// Otherwise it could DoS the model's host.
	Freshest = time.Nanosecond

	// ThreadContextFreshness limits synchronous refreshes triggered by
	// authenticated thread-context requests. This keeps remote replies
	// reasonably current without contacting the origin on every open.
	ThreadContextFreshness = 10 * time.Minute

	// ThreadContextRefreshTimeout bounds the extra latency added while
	// importing the first page of replies for a remote thread.
	ThreadContextRefreshTimeout = 3 * time.Second

	// ThreadContextFailureFreshness prevents an unavailable origin from being
	// retried for every context request while recovering much sooner than a
	// successful refresh.
	ThreadContextFailureFreshness = time.Minute
)

// Dereferencer wraps logic and functionality for doing dereferencing
// of remote accounts, statuses, etc, from federated instances.
type Dereferencer struct {
	state               *state.State
	converter           *typeutils.Converter
	transportController transport.Controller
	mediaManager        *media.Manager
	visFilter           *visibility.Filter
	intFilter           *interaction.Filter
	relayFilter         *relay.Filter

	// OnAccountDereference is a hook that gets called on dereference of an account model.
	// It is plumbed-in to the dereferencer but unused. In time it would be nice to add a
	// new websocket API message type "update.account" that sends account model updates.
	OnAccountDereference func(ctx context.Context, account *gtsmodel.Account) error

	// OnStatusDereference is a hook that gets called on dereference of a status
	// model, also indicating whether it was new to us at the time of dereference.
	// This can be used to handle streaming and notifying of status create / update events.
	//
	// see: ./internal/surfacing/surfacing.go
	OnStatusDereference func(ctx context.Context, status *gtsmodel.Status, isNew bool) error

	// OnMediaDereference is a hook that gets called on dereference of a media attachment.
	// This can be used to handle streaming of updated status models when media finishes processing.
	//
	// see: ./internal/surfacing/surfacing.go
	OnMediaDereference func(ctx context.Context, media *gtsmodel.MediaAttachment) error

	// OnEmojiDereference is a hook that gets called on dereference of an emoji attachment.
	// It is plumbed-in to the dereferencer but unused. In time it would be nice to add a
	// new websocket API message type "update.emoji" that sends emoji updates when finished processing.
	OnEmojiDereference func(ctx context.Context, emoji *gtsmodel.Emoji) error

	// in-progress dereferencing media / emoji
	derefMedia    keyedList[*media.ProcessingMedia]
	derefMediaMu  sync.Mutex
	derefEmojis   keyedList[*media.ProcessingEmoji]
	derefEmojisMu sync.Mutex

	// recently attempted remote reply collection refreshes
	threadContextRefreshes cache.Cache[string, threadContextRefresh]

	// handshakes marks current in-progress handshakes
	// occurring, useful to prevent a deadlock between
	// gotosocial instances attempting to dereference
	// accounts for the first time. when a handshake is
	// currently ongoing we know not to block waiting
	// on certain data and instead return an in-progress
	// form of the data as we currently see it.
	handshakes   map[string][]*url.URL
	handshakesMu sync.Mutex
}

// NewDereferencer returns a Dereferencer
// initialized with the given parameters.
func NewDereferencer(
	state *state.State,
	converter *typeutils.Converter,
	transportController transport.Controller,
	visFilter *visibility.Filter,
	intFilter *interaction.Filter,
	relayFilter *relay.Filter,
	mediaManager *media.Manager,
) Dereferencer {
	return Dereferencer{
		state:                  state,
		converter:              converter,
		transportController:    transportController,
		mediaManager:           mediaManager,
		visFilter:              visFilter,
		intFilter:              intFilter,
		relayFilter:            relayFilter,
		threadContextRefreshes: cache.New[string, threadContextRefresh](0, 1000),
		handshakes:             make(map[string][]*url.URL),
	}
}

type threadContextRefresh struct {
	at      time.Time
	success bool
}

func (d *Dereferencer) threadContextRefreshFresh(uri string, now time.Time) bool {
	refresh, ok := d.threadContextRefreshes.Get(uri)
	if !ok {
		return false
	}

	freshness := ThreadContextFailureFreshness
	if refresh.success {
		freshness = ThreadContextFreshness
	}
	if now.Sub(refresh.at) < freshness {
		return true
	}

	d.threadContextRefreshes.Invalidate(uri)
	return false
}

func (d *Dereferencer) onAccountDereference(ctx context.Context, account *gtsmodel.Account) {
	if d.OnAccountDereference != nil {
		if err := d.OnAccountDereference(ctx, account); err != nil {
			log.Errorf(ctx, "error dereferencing account: %v", err)
		}
	}
}

func (d *Dereferencer) onStatusDereference(ctx context.Context, status *gtsmodel.Status, isNew bool) {
	if d.OnStatusDereference != nil {
		if err := d.OnStatusDereference(ctx, status, isNew); err != nil {
			log.Errorf(ctx, "error dereferencing status: %v", err)
		}
	}
}

func (d *Dereferencer) onMediaDereference(ctx context.Context, media *gtsmodel.MediaAttachment) {
	if d.OnMediaDereference != nil {
		if err := d.OnMediaDereference(ctx, media); err != nil {
			log.Errorf(ctx, "error dereferencing media: %v", err)
		}
	}
}

func (d *Dereferencer) onEmojiDereference(ctx context.Context, emoji *gtsmodel.Emoji) {
	if d.OnEmojiDereference != nil {
		if err := d.OnEmojiDereference(ctx, emoji); err != nil {
			log.Errorf(ctx, "error dereferencing emoji: %v", err)
		}
	}
}
