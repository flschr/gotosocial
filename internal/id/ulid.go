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

package id

import (
	"crypto/rand"
	"crypto/sha256"
	"math/big"
	"time"

	"code.superseriousbusiness.org/gopkg/log"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"codeberg.org/gruf/go-kv/v2"
	"github.com/oklog/ulid/v2"
)

// ULIDFromString returns a stable, valid ULID for the given namespace and
// value. It is intended for virtual records which need Mastodon-compatible
// identifiers but must not be persisted as real local accounts.
func ULIDFromString(namespace, value string) string {
	sum := sha256.Sum256([]byte(namespace + "\x00" + value))
	var derived ulid.ULID
	copy(derived[:], sum[:len(derived)])
	// Clear the two overflow bits so String always produces a canonical ULID.
	derived[0] &= 0x3f
	return derived.String()
}

const (
	// Highest is the highest possible ULID
	Highest = "ZZZZZZZZZZZZZZZZZZZZZZZZZZ"

	// Lowest is the lowest possible ULID
	Lowest = "00000000000000000000000000"

	// ~20 years in seconds
	randomRange = 631152381
)

// bigRandomRange contains randomRange as big.Int.
var bigRandomRange = big.NewInt(randomRange)

// ULID represents the actual ULID binary type, which
// itself can easily be converted to string or have its
// timestamp value extracted from it.
type ULID = ulid.ULID

// newAt returns a new ulid.ULID from timestamp,
// else panics with caller's caller information.
func newAt(ts uint64) string {
	ulid, err := ulid.New(ts, rand.Reader)
	if err != nil {
		panic(gtserror.NewfAt(4, "error generating ulid: %w", err))
	}
	return ulid.String()
}

// NewULID returns a new ULID
// string using the current time.
func NewULID() string {
	return newAt(ulid.Now())
}

// NewULIDFromTime returns a new ULID string using
// given time, or from current time on any error.
func NewULIDFromTime(t time.Time) string {
	ts := ulid.Timestamp(t)
	if ts > ulid.MaxTime() {
		log.WarnKVs(nil, kv.Fields{
			{K: "caller", V: log.Caller(3)},
			{K: "value", V: t},
			{K: "msg", V: "invalid ulid time"},
		}...)
		ts = ulid.Now()
	}
	return newAt(ts)
}

// NewRandomULID returns a new ULID string using a random
// time in an ~80 year range around the current datetime.
func NewRandomULID() string {
	n1, err := rand.Int(rand.Reader, bigRandomRange)
	if err != nil {
		panic(gtserror.NewfAt(3, "error reading rand: %w", err))
	}

	n2, err := rand.Int(rand.Reader, bigRandomRange)
	if err != nil {
		panic(gtserror.NewfAt(3, "error reading rand: %w", err))
	}

	// Random addition and decrement durations.
	add := time.Duration(n1.Int64()) * time.Second
	dec := -time.Duration(n2.Int64()) * time.Second

	// Return new ULID string from now.
	t := time.Now().Add(add).Add(dec)
	return newAt(ulid.Timestamp(t))
}

// TimeFromULID parses a ULID string and returns the
// encoded time.Time{}, or error with caller prefix.
func TimeFromULID(id string) (time.Time, error) {
	parsed, err := ulid.ParseStrict(id)
	if err != nil {
		return time.Time{}, gtserror.NewfAt(3, "could not extract time (malformed ulid): %w", err)
	}
	return ulid.Time(parsed.Time()), nil
}

// ZeroULIDForTime returns the zero-value ULID for given time.
func ZeroULIDForTime(t time.Time) string {
	ts := ulid.Timestamp(t)
	var ulid ulid.ULID
	if err := ulid.SetTime(ts); err != nil {
		panic(err)
	}
	return ulid.String()
}

// ZeroBinaryULIDForTime is the same as ZeroULIDForTime()
// except that it returns the binary ULID representation.
func ZeroBinaryULIDForTime(t time.Time) ULID {
	ts := ulid.Timestamp(t)
	var ulid ulid.ULID
	if err := ulid.SetTime(ts); err != nil {
		panic(err)
	}
	return ulid
}
