// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package account_test

import (
	"testing"

	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"github.com/stretchr/testify/suite"
)

type VirtualBlueskyGetTestSuite struct {
	AccountStandardTestSuite
}

func (suite *VirtualBlueskyGetTestSuite) TestGetVirtualBlueskyAuthorIsPrivateAndResolvable() {
	ctx := suite.T().Context()
	requestingAccount := suite.testAccounts["local_account_1"]
	virtualID := id.ULIDFromString("bluesky-author", "did:plc:virtual-author")
	interaction := &gtsmodel.BlueskyInteraction{
		ID:                    id.NewULID(),
		AccountID:             requestingAccount.ID,
		StatusID:              suite.testStatuses["local_account_1_status_1"].ID,
		URI:                   "at://did:plc:virtual-author/app.bsky.feed.post/one",
		CID:                   "bafyvirtual",
		RootURI:               "at://did:plc:virtual-author/app.bsky.feed.post/one",
		RootCID:               "bafyvirtual",
		ParentURI:             "at://did:plc:virtual-author/app.bsky.feed.post/one",
		ParentCID:             "bafyvirtual",
		AuthorDID:             "did:plc:virtual-author",
		AuthorAccountID:       virtualID,
		AuthorHandle:          "author.example",
		AuthorDisplayName:     "Virtual Author",
		AuthorAvatarURL:       "https://social.example.org/files/avatar.jpeg",
		AuthorAvatarStaticURL: "https://social.example.org/files/avatar-small.jpeg",
		URL:                   "https://bsky.app/profile/did:plc:virtual-author/post/one",
	}
	suite.Require().NoError(suite.db.PutBlueskyInteraction(ctx, interaction))

	apiAccount, errWithCode := suite.accountProcessor.Get(ctx, requestingAccount, virtualID)
	suite.Require().Nil(errWithCode)
	suite.Equal(virtualID, apiAccount.ID)
	suite.Equal("author.example", apiAccount.Acct)
	suite.Equal("Virtual Author :bluesky:", apiAccount.DisplayName)

	_, errWithCode = suite.accountProcessor.Get(ctx, suite.testAccounts["admin_account"], virtualID)
	suite.Require().NotNil(errWithCode)
	suite.Equal(404, errWithCode.Code())
}

func TestVirtualBlueskyGetTestSuite(t *testing.T) {
	suite.Run(t, new(VirtualBlueskyGetTestSuite))
}
