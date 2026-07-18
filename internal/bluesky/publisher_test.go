// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"strings"
	"testing"

	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"github.com/rivo/uniseg"
	"github.com/stretchr/testify/require"
)

func TestHTMLTextAndFacetsPreservesLinkedLabel(t *testing.T) {
	plain, facets := htmlTextAndFacets(`<p>Read <a href="https://example.org/post">the article</a>.</p>`)
	require.Equal(t, "Read the article.\n", plain)
	require.Len(t, facets, 1)
	require.Equal(t, len("Read "), facets[0].Index.ByteStart)
	require.Equal(t, len("Read the article"), facets[0].Index.ByteEnd)
	require.Equal(t, "https://example.org/post", facets[0].Features[0].URI)
}

func TestBlueskyTextTruncatesWithCanonicalLink(t *testing.T) {
	status := &gtsmodel.Status{
		Content: strings.Repeat("word ", 100),
		URL:     "https://example.org/@author/status/1",
	}
	plain, facets := blueskyText(status)
	require.LessOrEqual(t, uniseg.GraphemeClusterCount(plain), maxPostGraphemes)
	require.LessOrEqual(t, len(plain), maxPostBytes)
	require.True(t, strings.HasSuffix(plain, status.URL))
	require.Len(t, facets, 1)
}

func TestEligibleForCrosspost(t *testing.T) {
	connection := &gtsmodel.BlueskyConnection{CrosspostPublic: true}
	status := &gtsmodel.Status{Visibility: gtsmodel.VisibilityPublic}
	status.Flags.SetFederated(true)
	require.True(t, EligibleForCrosspost(status, connection))

	status.InReplyToID = "reply"
	require.False(t, EligibleForCrosspost(status, connection))
	status.InReplyToID = ""
	status.MentionIDs = []string{"mention"}
	require.False(t, EligibleForCrosspost(status, connection))
}
