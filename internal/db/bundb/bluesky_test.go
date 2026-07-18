// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bundb_test

import (
	"testing"

	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
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

func TestBlueskyTestSuite(t *testing.T) {
	suite.Run(t, new(BlueskyTestSuite))
}
