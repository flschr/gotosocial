// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/stretchr/testify/require"
)

func TestRecordKeyForStatusIDIsStableValidTID(t *testing.T) {
	const statusID = "01KXVBTPFH5E5RPVE09CV1F91V"

	first, err := recordKeyForStatusID(statusID)
	require.NoError(t, err)
	second, err := recordKeyForStatusID(statusID)
	require.NoError(t, err)
	require.Equal(t, first, second)
	_, err = syntax.ParseTID(first)
	require.NoError(t, err)
}

func TestRecordKeyForStatusIDRejectsMalformedULID(t *testing.T) {
	_, err := recordKeyForStatusID("not-a-ulid")
	require.Error(t, err)
}
