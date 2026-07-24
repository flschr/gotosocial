// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package id

import (
	"testing"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

func TestULIDFromStringIsStableAndNamespaced(t *testing.T) {
	first := ULIDFromString("bluesky-author", "did:plc:example")
	require.Equal(t, first, ULIDFromString("bluesky-author", "did:plc:example"))
	require.NotEqual(t, first, ULIDFromString("other", "did:plc:example"))
	_, err := ulid.ParseStrict(first)
	require.NoError(t, err)
}
