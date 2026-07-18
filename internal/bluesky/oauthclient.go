// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/identity"
)

func OAuthURLs() (baseURL, metadataURL, callbackURL string) {
	baseURL = config.GetProtocol() + "://" + config.GetHost()
	metadataURL = baseURL + "/api/v1/user/bluesky/client-metadata.json"
	callbackURL = baseURL + "/api/v1/user/bluesky/callback"
	return
}

func OAuthClientConfig() oauth.ClientConfig {
	_, metadataURL, callbackURL := OAuthURLs()
	clientConfig := oauth.NewPublicConfig(metadataURL, callbackURL, []string{"atproto", "transition:generic"})
	clientConfig.UserAgent = "GoToSocial Plus"
	return clientConfig
}

func NewOAuthClient(state *state.State, accountID string) (*oauth.ClientApp, *OAuthStore, error) {
	crypter, err := NewCrypter(config.GetBlueskyOAuthEncryptionKey())
	if err != nil {
		return nil, nil, err
	}
	clientConfig := OAuthClientConfig()
	store := NewOAuthStore(state.DB, crypter, accountID)
	app := oauth.NewClientApp(&clientConfig, store)
	client := protectedHTTPClient(state)
	app.Client = client
	app.Resolver.Client = client
	if cache, ok := app.Dir.(*identity.CacheDirectory); ok {
		if base, ok := cache.Inner.(*identity.BaseDirectory); ok {
			base.HTTPClient = *client
		}
	}
	return app, store, nil
}
