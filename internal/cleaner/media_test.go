// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package cleaner_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/admin"
	"code.superseriousbusiness.org/gotosocial/internal/cleaner"
	"code.superseriousbusiness.org/gotosocial/internal/gtscontext"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/media"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"code.superseriousbusiness.org/gotosocial/internal/storage"
	"code.superseriousbusiness.org/gotosocial/internal/transport"
	"code.superseriousbusiness.org/gotosocial/testrig"
	"github.com/stretchr/testify/suite"
)

type MediaTestSuite struct {
	suite.Suite

	state               state.State
	manager             *media.Manager
	cleaner             *cleaner.Cleaner
	transportController transport.Controller
	testAttachments     map[string]*gtsmodel.MediaAttachment
	testAccounts        map[string]*gtsmodel.Account
	testEmojis          map[string]*gtsmodel.Emoji
}

func TestMediaTestSuite(t *testing.T) {
	suite.Run(t, &MediaTestSuite{})
}

func (suite *MediaTestSuite) SetupTest() {
	testrig.InitTestConfig()
	testrig.InitTestLog()

	suite.state.Caches.Init()
	testrig.StartNoopWorkers(&suite.state)

	_ = testrig.NewTestDB(&suite.state)
	suite.state.Storage = testrig.NewInMemoryStorage()
	suite.state.AdminActions = admin.New(suite.state.DB, &suite.state.Workers)

	testrig.StandardStorageSetup(suite.state.Storage, "../../testrig/media")
	testrig.StandardDBSetup(suite.state.DB, nil)

	suite.testAttachments = testrig.NewTestAttachments()
	suite.testAccounts = testrig.NewTestAccounts()
	suite.testEmojis = testrig.NewTestEmojis()
	suite.manager = testrig.NewTestMediaManager(&suite.state)
	suite.cleaner = cleaner.New(&suite.state)
	suite.transportController = testrig.NewTestTransportController(&suite.state, testrig.NewMockHTTPClient(nil, "../../testrig/media"))
}

func (suite *MediaTestSuite) TearDownTest() {
	testrig.StandardDBTeardown(suite.state.DB)
	testrig.StandardStorageTeardown(suite.state.Storage)
	testrig.StopWorkers(&suite.state)
}

func (suite *MediaTestSuite) TestUncacheRemote() {
	ctx := suite.T().Context()

	testStatusAttachment := suite.testAttachments["remote_account_1_status_1_attachment_1"]
	suite.True(testStatusAttachment.Cached())

	testHeader := suite.testAttachments["remote_account_3_header"]
	suite.True(testHeader.Cached())

	after := time.Now().Add(-24 * time.Hour)
	totalUncached, err := suite.cleaner.Media().UncacheRemote(ctx, after)
	suite.NoError(err)
	suite.Equal(3, totalUncached)

	uncachedAttachment, err := suite.state.DB.GetAttachmentByID(ctx, testStatusAttachment.ID)
	suite.NoError(err)
	suite.False(uncachedAttachment.Cached())

	uncachedAttachment, err = suite.state.DB.GetAttachmentByID(ctx, testHeader.ID)
	suite.NoError(err)
	suite.False(uncachedAttachment.Cached())
}

func (suite *MediaTestSuite) TestBlueskyInteractionAvatarIsRetained() {
	ctx := suite.T().Context()
	attachment := suite.testAttachments["remote_account_1_status_1_attachment_1"]
	attachment.StatusID = ""
	suite.Require().NoError(
		suite.state.DB.UpdateAttachment(ctx, attachment, "status_id"),
	)

	account := suite.testAccounts["local_account_1"]
	interaction := &gtsmodel.BlueskyInteraction{
		ID:                    id.NewULID(),
		AccountID:             account.ID,
		StatusID:              testrig.NewTestStatuses()["local_account_1_status_1"].ID,
		URI:                   "at://did:plc:avatar/app.bsky.feed.post/reply",
		CID:                   "bafyavatar",
		RootURI:               "at://did:plc:avatar/app.bsky.feed.post/root",
		RootCID:               "bafyroot",
		ParentURI:             "at://did:plc:avatar/app.bsky.feed.post/root",
		ParentCID:             "bafyroot",
		AuthorDID:             "did:plc:avatar",
		AuthorHandle:          "avatar.test",
		AuthorAvatarURL:       attachment.URL,
		AuthorAvatarStaticURL: attachment.Thumbnail.URL,
		URL:                   "https://bsky.app/profile/avatar.test/post/reply",
	}
	suite.Require().NoError(
		suite.state.DB.PutBlueskyInteraction(ctx, interaction),
	)

	_, err := suite.cleaner.Media().PruneUnused(ctx)
	suite.Require().NoError(err)
	retained, err := suite.state.DB.GetAttachmentByID(ctx, attachment.ID)
	suite.Require().NoError(err)
	suite.True(retained.Cached())

	_, err = suite.cleaner.Media().UncacheRemote(
		ctx,
		time.Now().Add(24*time.Hour),
	)
	suite.Require().NoError(err)
	retained, err = suite.state.DB.GetAttachmentByID(ctx, attachment.ID)
	suite.Require().NoError(err)
	suite.True(retained.Cached())
}

func (suite *MediaTestSuite) TestPurgeRemote() {
	var (
		ctx = suite.T().Context()
	)

	for _, t := range []struct {
		domain   string
		expected int
	}{
		{
			domain:   "fossbros-anonymous.io",
			expected: 1,
		},
		{
			domain:   "example.org",
			expected: 3,
		},
		{
			domain:   "thequeenisstillalive.technology",
			expected: 1,
		},
	} {
		count, err := suite.cleaner.Media().PurgeRemote(ctx, t.domain)
		if err != nil {
			suite.FailNow(err.Error())
		}
		if count != t.expected {
			suite.Fail("",
				"purge %s expected %d, got %d",
				t.domain, t.expected, count,
			)
		}
	}
}

func (suite *MediaTestSuite) TestUncacheRemoteDry() {
	ctx := suite.T().Context()

	testStatusAttachment := suite.testAttachments["remote_account_1_status_1_attachment_1"]
	suite.True(testStatusAttachment.Cached())

	testHeader := suite.testAttachments["remote_account_3_header"]
	suite.True(testHeader.Cached())

	after := time.Now().Add(-24 * time.Hour)
	totalUncached, err := suite.cleaner.Media().UncacheRemote(gtscontext.SetDryRun(ctx), after)
	suite.NoError(err)
	suite.Equal(3, totalUncached)

	uncachedAttachment, err := suite.state.DB.GetAttachmentByID(ctx, testStatusAttachment.ID)
	suite.NoError(err)
	suite.True(uncachedAttachment.Cached())

	uncachedAttachment, err = suite.state.DB.GetAttachmentByID(ctx, testHeader.ID)
	suite.NoError(err)
	suite.True(uncachedAttachment.Cached())
}

func (suite *MediaTestSuite) TestUncacheRemoteTwice() {
	ctx := suite.T().Context()
	after := time.Now().Add(-24 * time.Hour)

	totalUncached, err := suite.cleaner.Media().UncacheRemote(ctx, after)
	suite.NoError(err)
	suite.Equal(3, totalUncached)

	// final uncache should uncache nothing, since the first uncache already happened
	totalUncachedAgain, err := suite.cleaner.Media().UncacheRemote(ctx, after)
	suite.NoError(err)
	suite.Equal(0, totalUncachedAgain)
}

func (suite *MediaTestSuite) TestUncacheAndRecache() {
	ctx := suite.T().Context()
	testStatusAttachment := suite.testAttachments["remote_account_1_status_1_attachment_1"]
	testHeader := suite.testAttachments["remote_account_3_header"]

	after := time.Now().Add(-24 * time.Hour)
	totalUncached, err := suite.cleaner.Media().UncacheRemote(ctx, after)
	suite.NoError(err)
	suite.Equal(3, totalUncached)

	// media should no longer be stored
	_, err = suite.state.Storage.Get(ctx, testStatusAttachment.File.Path)
	suite.True(storage.IsNotFound(err))
	_, err = suite.state.Storage.Get(ctx, testStatusAttachment.Thumbnail.Path)
	suite.True(storage.IsNotFound(err))
	_, err = suite.state.Storage.Get(ctx, testHeader.File.Path)
	suite.True(storage.IsNotFound(err))
	_, err = suite.state.Storage.Get(ctx, testHeader.Thumbnail.Path)
	suite.True(storage.IsNotFound(err))

	// now recache the image....
	data := func(_ context.Context) (io.ReadCloser, error) {
		// load bytes from a test image
		b, err := os.ReadFile("../../testrig/media/thoughtsofdog-original.jpg")
		if err != nil {
			panic(err)
		}
		return io.NopCloser(bytes.NewBuffer(b)), nil
	}

	for _, original := range []*gtsmodel.MediaAttachment{
		testStatusAttachment,
		testHeader,
	} {
		processing := suite.manager.CacheMedia(original, data, media.AdditionalMediaInfo{})

		// synchronously load the recached attachment
		recachedAttachment, err := processing.Load(ctx)
		suite.NoError(err)
		suite.NotNil(recachedAttachment)

		// recachedAttachment should be basically the same as the old attachment
		suite.True(recachedAttachment.Cached())
		suite.Equal(original.ID, recachedAttachment.ID)
		suite.Equal(original.File.Path, recachedAttachment.File.Path)           // file should be stored in the same place
		suite.Equal(original.Thumbnail.Path, recachedAttachment.Thumbnail.Path) // as should the thumbnail
		suite.EqualValues(original.FileMeta, recachedAttachment.FileMeta)       // and the filemeta should be the same

		// recached files should be back in storage
		_, err = suite.state.Storage.Get(ctx, recachedAttachment.File.Path)
		suite.NoError(err)
		_, err = suite.state.Storage.Get(ctx, recachedAttachment.Thumbnail.Path)
		suite.NoError(err)
	}
}

func (suite *MediaTestSuite) TestUncacheOneNonExistent() {
	ctx := suite.T().Context()
	testStatusAttachment := suite.testAttachments["remote_account_1_status_1_attachment_1"]

	// Delete this attachment cached on disk
	media, err := suite.state.DB.GetAttachmentByID(ctx, testStatusAttachment.ID)
	suite.NoError(err)
	suite.True(media.Cached())
	err = suite.state.Storage.Delete(ctx, media.File.Path)
	suite.NoError(err)

	// Now attempt to uncache remote for item with db entry no file
	after := time.Now().Add(-24 * time.Hour)
	totalUncached, err := suite.cleaner.Media().UncacheRemote(ctx, after)
	suite.NoError(err)
	suite.Equal(3, totalUncached)
}
