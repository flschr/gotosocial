// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"context"
	"fmt"

	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

type blueskyReplyTarget struct {
	RootURI   string
	RootCID   string
	ParentURI string
	ParentCID string
}

func refreshReplyReferences(ctx context.Context, client *atclient.APIClient, interaction *blueskyReplyTarget) error {
	endpoint, _ := syntax.ParseNSID("app.bsky.feed.getPosts")
	appview := client.WithService("did:web:api.bsky.app#bsky_appview")
	uris := []string{interaction.ParentURI}
	if interaction.RootURI != interaction.ParentURI {
		uris = append(uris, interaction.RootURI)
	}
	var response blueskyPostsResponse
	if err := appview.Get(ctx, endpoint, map[string]any{"uris": uris}, &response); err != nil {
		return fmt.Errorf("refresh Bluesky reply references: %w", err)
	}
	interaction.ParentCID = ""
	interaction.RootCID = ""
	for _, post := range response.Posts {
		switch post.URI {
		case interaction.ParentURI:
			interaction.ParentCID = post.CID
		case interaction.RootURI:
			interaction.RootCID = post.CID
		}
	}
	if interaction.RootURI == interaction.ParentURI {
		interaction.RootCID = interaction.ParentCID
	}
	if interaction.ParentCID == "" || interaction.RootCID == "" {
		return &ConnectionError{Code: ErrorCodeData, Err: fmt.Errorf("Bluesky reply parent or root is unavailable")}
	}
	return nil
}
