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
	"slices"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/media"
)

// getEmojiByShortcodeDomain searches input slice
// for emoji with given shortcode and domain.
func getEmojiByShortcodeDomain(
	emojis []*gtsmodel.Emoji,
	shortcode string,
	domain string,
) (
	*gtsmodel.Emoji,
	bool,
) {
	for _, emoji := range emojis {
		if emoji.Shortcode == shortcode &&
			emoji.Domain == domain {
			return emoji, true
		}
	}
	return nil, false
}

// mediaUpToDate returns whether media is up-to-date according
// to additional info, updating media fields where necessary.
func mediaUpToDate(media *gtsmodel.MediaAttachment, info media.AdditionalMediaInfo) bool {
	ok := true

	// Check blurhash up-to-date.
	if info.Blurhash != nil &&
		*info.Blurhash != media.Blurhash {
		media.Blurhash = *info.Blurhash
		ok = false
	}

	// Check description up-to-date.
	if info.Description != nil &&
		*info.Description != media.Description {
		media.Description = *info.Description
		ok = false
	}

	// Check remote URL up-to-date.
	if info.RemoteURL != nil &&
		*info.RemoteURL != media.RemoteURL {
		media.RemoteURL = *info.RemoteURL
		ok = false
	}

	return ok
}

// emojiUpToDate returns whether emoji is up-to-date according
// to additional info, updating emoji fields where necessary.
func emojiUpToDate(emoji *gtsmodel.Emoji, info media.AdditionalEmojiInfo) bool {
	ok := true

	// Recheck uri up-to-date.
	if info.URI != nil &&
		*info.URI != emoji.URI {
		emoji.URI = *info.URI
		ok = false
	}

	// Recheck image remote URL up-to-date.
	if info.ImageRemoteURL != nil &&
		*info.ImageRemoteURL != emoji.ImageRemoteURL {
		emoji.ImageRemoteURL = *info.ImageRemoteURL
		ok = false
	}

	// Recheck image static remote URL up-to-date.
	if info.ImageStaticRemoteURL != nil &&
		*info.ImageStaticRemoteURL != emoji.ImageStaticRemoteURL {
		emoji.ImageStaticRemoteURL = *info.ImageStaticRemoteURL
		ok = false
	}

	return ok
}

// emojiChanged returns whether an emoji has changed in a way
// that indicates that it should be refetched and refreshed.
func emojiChanged(existing, latest *gtsmodel.Emoji) bool {
	return existing.URI != latest.URI ||
		existing.ImageRemoteURL != latest.ImageRemoteURL ||
		existing.ImageStaticRemoteURL != latest.ImageStaticRemoteURL
}

// pollChanged returns whether a poll has changed in way that
// indicates that this should be an entirely new poll. i.e. if
// the available options have changed, or the expiry has changed.
func pollChanged(existing, latest *gtsmodel.Poll) bool {
	return !slices.Equal(existing.Options, latest.Options) ||
		!existing.ExpiresAt.Equal(latest.ExpiresAt)
}

// pollStateUpdated returns whether a poll has updated, i.e. if
// vote counts have changed, or if it has expired / been closed.
func pollStateUpdated(existing, latest *gtsmodel.Poll) bool {
	return *existing.Voters != *latest.Voters ||
		!slices.Equal(existing.Votes, latest.Votes) ||
		!existing.ClosedAt.Equal(latest.ClosedAt)
}

// pollJustClosed returns whether a poll has *just* closed.
func pollJustClosed(existing, latest *gtsmodel.Poll) bool {
	return existing.ClosedAt.IsZero() &&
		!latest.ClosedAt.IsZero() && time.Now().After(latest.ClosedAt)
}

// keyedList is a simple alternative to a hashmap which can
// be used when you expect a (relatively) small number of entries
// and want it to be able to compact when not heavily in use.
// unlike a hashmap which requires enough buckets to handle all
// the possible hashed key permutations of new key values, even
// if it doesn't contain many non-nil entries.
type keyedList[T any] []struct {
	k string
	v T
}

func (l keyedList[T]) get(key string) T {
	for _, kv := range l {
		if kv.k == key {
			return kv.v
		}
	}
	var t T
	return t
}

func (l *keyedList[T]) put(key string, value T) {
	(*l) = append((*l), struct {
		k string
		v T
	}{
		k: key,
		v: value,
	})
}

func (l *keyedList[T]) delete(key string) {
	for i := 0; i < len(*l); {
		// Elem at idx.
		kv := (*l)[i]

		switch {
		case kv.k != key:
			// no match
			i++

		case len(*l) == 1 && cap(*l) > 64:
			// key is last element in slice
			// which has lots extra capacity
			(*l) = nil

		default:
			// Drop element at i'th index.
			_ = copy((*l)[i:], (*l)[i+1:])
			(*l)[len(*l)-1] = struct {
				k string
				v T
			}{}
			(*l) = (*l)[:len(*l)-1]
		}
	}
}
