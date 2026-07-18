// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"crypto/sha256"

	"code.superseriousbusiness.org/gotosocial/internal/id"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

// recordKeyForStatusID maps a GoToSocial ULID to a stable ATProto TID.
// Preserve the ULID millisecond for useful ordering and use 20 hash bits to
// distinguish records created within the same millisecond.
func recordKeyForStatusID(statusID string) (string, error) {
	createdAt, err := id.TimeFromULID(statusID)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(statusID))
	microOffset := int64((uint16(digest[0])<<8 | uint16(digest[1])) % 1000)
	clockID := uint(uint16(digest[2])<<8|uint16(digest[3])) & 0x3ff
	return syntax.NewTID(createdAt.UnixMilli()*1000+microOffset, clockID).String(), nil
}
