// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package dereferencing

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMicroblogInReplyTo(t *testing.T) {
	const page = `
		<div class="post h-entry">
			<a class="u-in-reply-to" href="https://example.org/not-selected">Other</a>
		</div>
		<div class="post h-entry post_selected" id="post_div_94613323">
			<a href="https://www.manton.org/original" class="u-in-reply-to replyto_hidden">
				In reply to
			</a>
		</div>`

	parentURL, err := parseMicroblogInReplyTo(strings.NewReader(page))
	require.NoError(t, err)
	assert.Equal(t, "https://www.manton.org/original", parentURL)
}

func TestParseMicroblogInReplyToNoSelectedReply(t *testing.T) {
	const page = `
		<div class="post h-entry post_selected">
			<a href="https://example.org/unrelated">Unrelated link</a>
		</div>`

	parentURL, err := parseMicroblogInReplyTo(strings.NewReader(page))
	require.NoError(t, err)
	assert.Empty(t, parentURL)
}
