// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"github.com/bluesky-social/indigo/atproto/auth/oauth"
)

func OAuthURLs() (baseURL, metadataURL, callbackURL string) {
	baseURL = config.GetProtocol() + "://" + config.GetHost()
	metadataURL = baseURL + "/api/v1/user/bluesky/client-metadata.json"
	callbackURL = baseURL + "/api/v1/user/bluesky/callback"
	return
}

func NewOAuthClient(database db.DB, accountID string) (*oauth.ClientApp, *OAuthStore, error) {
	crypter, err := NewCrypter(config.GetBlueskyOAuthEncryptionKey())
	if err != nil {
		return nil, nil, err
	}
	_, metadataURL, callbackURL := OAuthURLs()
	clientConfig := oauth.NewPublicConfig(metadataURL, callbackURL, []string{"atproto", "transition:generic"})
	clientConfig.UserAgent = "GoToSocial Plus"
	store := NewOAuthStore(database, crypter, accountID)
	return oauth.NewClientApp(&clientConfig, store), store, nil
}
