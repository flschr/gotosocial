// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bundb_test

import (
	"encoding/base64"
	"testing"

	"code.superseriousbusiness.org/gotosocial/internal/bluesky"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/stretchr/testify/suite"
)

type BlueskyTestSuite struct {
	BunDBStandardTestSuite
}

func (suite *BlueskyTestSuite) TestConnectionSettingsAndMappings() {
	ctx := suite.T().Context()
	account := suite.testAccounts["local_account_1"]
	status := suite.testStatuses["local_account_1_status_1"]

	connection := &gtsmodel.BlueskyConnection{
		ID:                id.NewULID(),
		AccountID:         account.ID,
		DID:               "did:plc:testconnection",
		Handle:            "example.test",
		PDSURL:            "https://pds.example.test",
		CrosspostPublic:   false,
		ShowProfileFollow: true,
	}
	suite.Require().NoError(suite.db.PutBlueskyConnection(ctx, connection))

	storedConnection, err := suite.db.GetBlueskyConnectionByAccountID(ctx, account.ID)
	suite.Require().NoError(err)
	suite.False(storedConnection.CrosspostPublic)
	suite.True(storedConnection.ShowProfileFollow)

	oauthState := &gtsmodel.BlueskyOAuthState{
		ID:            id.NewULID(),
		AccountID:     account.ID,
		State:         "oauth-state-test",
		EncryptedData: []byte("encrypted"),
	}
	suite.Require().NoError(suite.db.PutBlueskyOAuthState(ctx, oauthState))
	storedState, err := suite.db.GetBlueskyOAuthState(ctx, oauthState.State)
	suite.Require().NoError(err)
	suite.Equal(oauthState.AccountID, storedState.AccountID)
	suite.Require().NoError(suite.db.DeleteBlueskyOAuthState(ctx, oauthState.State))

	post := &gtsmodel.BlueskyPost{
		ID:           id.NewULID(),
		ConnectionID: connection.ID,
		AccountID:    account.ID,
		StatusID:     status.ID,
		URI:          "at://did:plc:testconnection/app.bsky.feed.post/test",
		CID:          "bafytestpost",
		URL:          "https://bsky.app/profile/example.test/post/test",
	}
	suite.Require().NoError(suite.db.PutBlueskyPost(ctx, post))

	storedPost, err := suite.db.GetBlueskyPostByStatusID(ctx, status.ID)
	suite.Require().NoError(err)
	suite.Equal(post.URI, storedPost.URI)

	interaction := &gtsmodel.BlueskyInteraction{
		ID:           id.NewULID(),
		AccountID:    account.ID,
		StatusID:     suite.testStatuses["local_account_1_status_2"].ID,
		URI:          "at://did:plc:other/app.bsky.feed.post/reply",
		CID:          "bafytestreply",
		RootURI:      post.URI,
		RootCID:      post.CID,
		ParentURI:    post.URI,
		ParentCID:    post.CID,
		AuthorDID:    "did:plc:other",
		AuthorHandle: "other.test",
		URL:          "https://bsky.app/profile/other.test/post/reply",
	}
	suite.Require().NoError(suite.db.PutBlueskyInteraction(ctx, interaction))

	storedInteraction, err := suite.db.GetBlueskyInteractionByURI(ctx, interaction.URI)
	suite.Require().NoError(err)
	suite.Equal(interaction.StatusID, storedInteraction.StatusID)
}

func (suite *BlueskyTestSuite) TestEncryptedOAuthStore() {
	ctx := suite.T().Context()
	account := suite.testAccounts["local_account_1"]
	crypter, err := bluesky.NewCrypter(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	suite.Require().NoError(err)
	store := bluesky.NewOAuthStore(suite.db, crypter, account.ID)

	request := oauth.AuthRequestData{State: "encrypted-state", PKCEVerifier: "pkce-secret"}
	suite.Require().NoError(store.SaveAuthRequestInfo(ctx, request))
	storedRequest, err := store.GetAuthRequestInfo(ctx, request.State)
	suite.Require().NoError(err)
	suite.Equal(request.PKCEVerifier, storedRequest.PKCEVerifier)

	did, err := syntax.ParseDID("did:plc:testoauthstore")
	suite.Require().NoError(err)
	connection := &gtsmodel.BlueskyConnection{
		ID: id.NewULID(), AccountID: account.ID, DID: did.String(),
		Handle: "oauth.test", PDSURL: "https://pds.example.test", ShowProfileFollow: true,
	}
	suite.Require().NoError(suite.db.PutBlueskyConnection(ctx, connection))
	session := oauth.ClientSessionData{
		AccountDID: did, SessionID: request.State, HostURL: connection.PDSURL,
		AccessToken: "access-secret", RefreshToken: "refresh-secret",
	}
	suite.Require().NoError(store.SaveSession(ctx, session))
	storedSession, err := store.GetSession(ctx, did, session.SessionID)
	suite.Require().NoError(err)
	suite.Equal(session.RefreshToken, storedSession.RefreshToken)

	storedConnection, err := suite.db.GetBlueskyConnectionByAccountID(ctx, account.ID)
	suite.Require().NoError(err)
	suite.NotContains(string(storedConnection.OAuthData), session.RefreshToken)
}

func TestBlueskyTestSuite(t *testing.T) {
	suite.Run(t, new(BlueskyTestSuite))
}
