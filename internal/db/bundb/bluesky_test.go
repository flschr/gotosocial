// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bundb_test

import (
	"encoding/base64"
	"testing"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/bluesky"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/typeutils"
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
		ID:                    id.NewULID(),
		AccountID:             account.ID,
		StatusID:              suite.testStatuses["local_account_1_status_2"].ID,
		URI:                   "at://did:plc:other/app.bsky.feed.post/reply",
		CID:                   "bafytestreply",
		RootURI:               post.URI,
		RootCID:               post.CID,
		ParentURI:             post.URI,
		ParentCID:             post.CID,
		AuthorDID:             "did:plc:other",
		AuthorHandle:          "other.test",
		AuthorAvatarURL:       "https://example.org/files/avatar.webp",
		AuthorAvatarStaticURL: "https://example.org/files/avatar-small.jpeg",
		URL:                   "https://bsky.app/profile/other.test/post/reply",
	}
	suite.Require().NoError(suite.db.PutBlueskyInteraction(ctx, interaction))

	storedInteraction, err := suite.db.GetBlueskyInteractionByURI(ctx, interaction.URI)
	suite.Require().NoError(err)
	suite.Equal(interaction.StatusID, storedInteraction.StatusID)
	suite.NotEmpty(storedInteraction.AuthorAccountID)
	storedByVirtualAccount, err := suite.db.GetBlueskyInteractionByAuthorAccountID(ctx, storedInteraction.AuthorAccountID, storedInteraction.AccountID)
	suite.Require().NoError(err)
	suite.Equal(interaction.ID, storedByVirtualAccount.ID)
	inUse, err := suite.db.IsBlueskyInteractionAvatar(
		ctx,
		interaction.AuthorAvatarURL,
		interaction.AuthorAvatarStaticURL,
	)
	suite.Require().NoError(err)
	suite.True(inUse)
	inUse, err = suite.db.IsBlueskyInteractionAvatar(
		ctx,
		"https://example.org/files/unrelated.webp",
		"",
	)
	suite.Require().NoError(err)
	suite.False(inUse)

	queued := &gtsmodel.BlueskyDelivery{
		ID: id.NewULID(), AccountID: account.ID, StatusID: suite.testStatuses["local_account_1_status_3"].ID,
		NextAttemptAt: time.Now(),
	}
	suite.Require().NoError(suite.db.PutBlueskyDelivery(ctx, queued))
	inbox := &gtsmodel.BlueskyNotification{
		ID: id.NewULID(), AccountID: account.ID, URI: "at://did:plc:other/app.bsky.feed.post/inbox",
		Payload: []byte(`{"reason":"reply"}`), NextAttemptAt: time.Now(),
	}
	suite.Require().NoError(suite.db.PutBlueskyNotification(ctx, inbox))
	suite.Require().NoError(suite.db.DeleteBlueskyDataByAccountID(ctx, account.ID))
	_, err = suite.db.GetBlueskyConnectionByAccountID(ctx, account.ID)
	suite.Error(err)
	_, err = suite.db.GetBlueskyPostByStatusID(ctx, status.ID)
	suite.Error(err)
	_, err = suite.db.GetBlueskyInteractionByURI(ctx, interaction.URI)
	suite.Error(err)
	dueInbox, err := suite.db.GetDueBlueskyNotifications(ctx, account.ID, time.Now().Add(time.Minute), 10)
	suite.Require().NoError(err)
	suite.Empty(dueInbox)
}

func (suite *BlueskyTestSuite) TestDurableDeliveryQueue() {
	ctx := suite.T().Context()
	status := suite.testStatuses["local_account_1_status_1"]
	delivery := &gtsmodel.BlueskyDelivery{
		ID: id.NewULID(), AccountID: status.AccountID, StatusID: status.ID,
		NextAttemptAt: time.Now().Add(-time.Minute),
	}
	suite.Require().NoError(suite.db.PutBlueskyDelivery(ctx, delivery))
	replacement := &gtsmodel.BlueskyDelivery{
		ID: id.NewULID(), AccountID: status.AccountID, StatusID: status.ID,
		Action: "delete", Attempts: 5, NextAttemptAt: time.Now(),
	}
	suite.Require().NoError(suite.db.PutBlueskyDelivery(ctx, replacement))
	replaced, err := suite.db.GetBlueskyDeliveryByStatusID(ctx, status.ID)
	suite.Require().NoError(err)
	suite.Equal("delete", replaced.Action)
	suite.EqualValues(2, replaced.Generation)
	suite.Zero(replaced.Attempts)

	now := time.Now()
	due, err := suite.db.ClaimDueBlueskyDeliveries(ctx, now, now.Add(5*time.Minute), 10)
	suite.Require().NoError(err)
	suite.Require().Len(due, 1)
	suite.Equal(status.ID, due[0].StatusID)
	renewedUntil := now.Add(10 * time.Minute)
	renewed, err := suite.db.RenewBlueskyDeliveryClaim(ctx, due[0].ID, due[0].ClaimID, renewedUntil)
	suite.Require().NoError(err)
	suite.True(renewed)
	renewed, err = suite.db.RenewBlueskyDeliveryClaim(ctx, due[0].ID, "wrong-claim", renewedUntil.Add(time.Minute))
	suite.Require().NoError(err)
	suite.False(renewed)
	claimedAgain, err := suite.db.ClaimDueBlueskyDeliveries(ctx, now, now.Add(5*time.Minute), 10)
	suite.Require().NoError(err)
	suite.Empty(claimedAgain)

	// A new edit arriving while this generation is claimed must survive the
	// older worker's eventual completion or failure update.
	newer := &gtsmodel.BlueskyDelivery{
		ID: id.NewULID(), AccountID: status.AccountID, StatusID: status.ID,
		Action: "upsert", Generation: 1, NextAttemptAt: time.Now(),
	}
	suite.Require().NoError(suite.db.PutBlueskyDelivery(ctx, newer))
	completed, err := suite.db.CompleteBlueskyDelivery(ctx, due[0].ID, due[0].Generation, due[0].ClaimID)
	suite.Require().NoError(err)
	suite.False(completed)
	due[0].Attempts = 9
	updated, err := suite.db.UpdateClaimedBlueskyDelivery(ctx, due[0], "attempts")
	suite.Require().NoError(err)
	suite.False(updated)
	current, err := suite.db.GetBlueskyDeliveryByStatusID(ctx, status.ID)
	suite.Require().NoError(err)
	suite.EqualValues(3, current.Generation)
	suite.Equal("upsert", current.Action)
	suite.Zero(current.Attempts)

	delivery.Attempts = 2
	delivery.NextAttemptAt = time.Now().Add(time.Hour)
	delivery.ClaimedUntil = time.Time{}
	suite.Require().NoError(suite.db.UpdateBlueskyDelivery(ctx, delivery, "attempts", "next_attempt_at", "claimed_until"))
	due, err = suite.db.ClaimDueBlueskyDeliveries(ctx, time.Now(), time.Now().Add(5*time.Minute), 10)
	suite.Require().NoError(err)
	suite.Empty(due)
	suite.Require().NoError(suite.db.DeleteBlueskyDeliveryByStatusID(ctx, status.ID))
}

func (suite *BlueskyTestSuite) TestOutboxReconciliationRestoresMissingJob() {
	ctx := suite.T().Context()
	account := suite.testAccounts["local_account_1"]
	status := suite.testStatuses["local_account_1_status_1"]
	connection := &gtsmodel.BlueskyConnection{
		ID: id.NewULID(), AccountID: account.ID, DID: "did:plc:outbox",
		Handle: "outbox.test", PDSURL: "https://pds.example.test",
		OAuthSessionID: "session", OAuthData: []byte("encrypted"), CrosspostPublic: true,
		OutboxCheckedAt: status.CreatedAt.Add(-time.Second),
	}
	suite.Require().NoError(suite.db.PutBlueskyConnection(ctx, connection))
	suite.Require().NoError(bluesky.ReconcileOutbox(ctx, &suite.state))
	delivery, err := suite.db.GetBlueskyDeliveryByStatusID(ctx, status.ID)
	suite.Require().NoError(err)
	suite.Equal("upsert", delivery.Action)
}

func (suite *BlueskyTestSuite) TestOutboxDoesNotBackfillBeforeCrosspostWasEnabled() {
	ctx := suite.T().Context()
	account := suite.testAccounts["local_account_1"]
	status := suite.testStatuses["local_account_1_status_1"]
	enabledAt := time.Now().Add(-time.Second)
	connection := &gtsmodel.BlueskyConnection{
		ID: id.NewULID(), AccountID: account.ID, DID: "did:plc:nooutboxbackfill",
		Handle: "no-backfill.test", PDSURL: "https://pds.example.test",
		OAuthSessionID: "session", OAuthData: []byte("encrypted"), CrosspostPublic: true,
		OutboxCheckedAt: enabledAt, CrosspostEnabledAt: enabledAt,
	}
	suite.Require().NoError(suite.db.PutBlueskyConnection(ctx, connection))
	suite.Require().NoError(bluesky.ReconcileOutbox(ctx, &suite.state))
	_, err := suite.db.GetBlueskyDeliveryByStatusID(ctx, status.ID)
	suite.Error(err)
}

func (suite *BlueskyTestSuite) TestMappedEditWaitsWhileDisconnected() {
	ctx := suite.T().Context()
	account := suite.testAccounts["local_account_1"]
	status := suite.testStatuses["local_account_1_status_1"]
	connection := &gtsmodel.BlueskyConnection{
		ID: id.NewULID(), AccountID: account.ID, DID: "did:plc:disconnectededit",
		Handle: "disconnected.test", PDSURL: "https://pds.example.test", CrosspostPublic: true,
	}
	suite.Require().NoError(suite.db.PutBlueskyConnection(ctx, connection))
	post := &gtsmodel.BlueskyPost{
		ID: id.NewULID(), ConnectionID: connection.ID, AccountID: account.ID, StatusID: status.ID,
		URI: "at://did:plc:disconnectededit/app.bsky.feed.post/root", CID: "root-cid",
		RootURI: "at://did:plc:disconnectededit/app.bsky.feed.post/root", RootCID: "root-cid",
		URL: "https://bsky.app/profile/did:plc:disconnectededit/post/root",
	}
	suite.Require().NoError(suite.db.PutBlueskyPost(ctx, post))
	delivery := &gtsmodel.BlueskyDelivery{
		ID: id.NewULID(), AccountID: account.ID, StatusID: status.ID,
		Action: "upsert", NextAttemptAt: time.Now(),
	}
	suite.Require().NoError(suite.db.PutBlueskyDelivery(ctx, delivery))
	suite.Error(bluesky.ProcessDelivery(ctx, &suite.state, typeutils.NewConverter(&suite.state), delivery))
	storedDelivery, err := suite.db.GetBlueskyDeliveryByStatusID(ctx, status.ID)
	suite.Require().NoError(err)
	suite.Equal("upsert", storedDelivery.Action)
	_, err = suite.db.GetBlueskyPostByStatusID(ctx, status.ID)
	suite.Require().NoError(err)
}

func (suite *BlueskyTestSuite) TestOutboxReconciliationDeletesUnchangedQuoteMapping() {
	ctx := suite.T().Context()
	account := suite.testAccounts["local_account_1"]
	status := new(gtsmodel.Status)
	*status = *suite.testStatuses["local_account_1_status_1"]
	status.QuoteURI = "https://remote.example/users/alice/statuses/quoted"
	suite.Require().NoError(suite.db.UpdateStatus(ctx, status, "quote_uri"))

	connection := &gtsmodel.BlueskyConnection{
		ID: id.NewULID(), AccountID: account.ID, DID: "did:plc:quote-cleanup",
		Handle: "quote-cleanup.test", PDSURL: "https://pds.example.test",
		OAuthSessionID: "session", OAuthData: []byte("encrypted"), CrosspostPublic: true,
		OutboxCheckedAt: time.Now(),
	}
	suite.Require().NoError(suite.db.PutBlueskyConnection(ctx, connection))
	post := &gtsmodel.BlueskyPost{
		ID: id.NewULID(), ConnectionID: connection.ID, AccountID: account.ID, StatusID: status.ID,
		URI: "at://did:plc:quote-cleanup/app.bsky.feed.post/root", CID: "quote-cid",
		RootURI: "at://did:plc:quote-cleanup/app.bsky.feed.post/root", RootCID: "quote-cid",
		URL: "https://bsky.app/profile/did:plc:quote-cleanup/post/root", UpdatedAt: time.Now(),
	}
	suite.Require().NoError(suite.db.PutBlueskyPost(ctx, post))
	_, err := bluesky.QueueStatus(ctx, &suite.state, status)
	suite.Require().NoError(err)

	suite.Require().NoError(bluesky.ReconcileOutbox(ctx, &suite.state))
	delivery, err := suite.db.GetBlueskyDeliveryByStatusID(ctx, status.ID)
	suite.Require().NoError(err)
	suite.Equal("delete", delivery.Action)
	suite.EqualValues(2, delivery.Generation)

	suite.Require().NoError(bluesky.ReconcileOutbox(ctx, &suite.state))
	preserved, err := suite.db.GetBlueskyDeliveryByStatusID(ctx, status.ID)
	suite.Require().NoError(err)
	suite.Equal(delivery.ID, preserved.ID)
	suite.Equal(delivery.Generation, preserved.Generation)
}

func (suite *BlueskyTestSuite) TestPersistedQuoteUpsertWithoutMappingIsDiscarded() {
	ctx := suite.T().Context()
	account := suite.testAccounts["local_account_1"]
	status := new(gtsmodel.Status)
	*status = *suite.testStatuses["local_account_1_status_1"]
	status.QuoteURI = "https://remote.example/users/alice/statuses/quoted"
	suite.Require().NoError(suite.db.UpdateStatus(ctx, status, "quote_uri"))

	connection := &gtsmodel.BlueskyConnection{
		ID: id.NewULID(), AccountID: account.ID, DID: "did:plc:stale-quote",
		Handle: "stale-quote.test", PDSURL: "https://pds.example.test",
		OAuthSessionID: "session", OAuthData: []byte("encrypted"), CrosspostPublic: true,
	}
	suite.Require().NoError(suite.db.PutBlueskyConnection(ctx, connection))
	_, err := bluesky.QueueStatus(ctx, &suite.state, status)
	suite.Require().NoError(err)
	claimed, err := suite.db.ClaimDueBlueskyDeliveries(ctx, time.Now().Add(time.Second), time.Now().Add(time.Minute), 1)
	suite.Require().NoError(err)
	suite.Require().Len(claimed, 1)

	suite.Require().NoError(bluesky.ProcessDelivery(ctx, &suite.state, typeutils.NewConverter(&suite.state), claimed[0]))
	_, err = suite.db.GetBlueskyDeliveryByStatusID(ctx, status.ID)
	suite.Error(err)
	_, err = suite.db.GetBlueskyPostByStatusID(ctx, status.ID)
	suite.Error(err)
}

func (suite *BlueskyTestSuite) TestOutboxReconciliationRestoresMissingEditJob() {
	ctx := suite.T().Context()
	account := suite.testAccounts["local_account_1"]
	status := suite.testStatuses["local_account_1_status_1"]
	checkedAt := time.Now().Add(-time.Minute)
	connection := &gtsmodel.BlueskyConnection{
		ID: id.NewULID(), AccountID: account.ID, DID: "did:plc:outboxedit",
		Handle: "outbox-edit.test", PDSURL: "https://pds.example.test",
		OAuthSessionID: "session", OAuthData: []byte("encrypted"), CrosspostPublic: true,
		OutboxCheckedAt: checkedAt,
	}
	suite.Require().NoError(suite.db.PutBlueskyConnection(ctx, connection))
	post := &gtsmodel.BlueskyPost{
		ID: id.NewULID(), ConnectionID: connection.ID, AccountID: account.ID, StatusID: status.ID,
		URI: "at://did:plc:outboxedit/app.bsky.feed.post/root", CID: "old-cid",
		RootURI: "at://did:plc:outboxedit/app.bsky.feed.post/root", RootCID: "old-cid",
		URL: "https://bsky.app/profile/did:plc:outboxedit/post/root",
	}
	suite.Require().NoError(suite.db.PutBlueskyPost(ctx, post))
	post.UpdatedAt = checkedAt.Add(-time.Second)
	suite.Require().NoError(suite.db.UpdateBlueskyPost(ctx, post, "updated_at"))
	status.EditedAt = time.Now().Add(-time.Second)
	suite.Require().NoError(suite.db.UpdateStatus(ctx, status, "edited_at"))
	suite.Require().NoError(bluesky.ReconcileOutbox(ctx, &suite.state))
	delivery, err := suite.db.GetBlueskyDeliveryByStatusID(ctx, status.ID)
	suite.Require().NoError(err)
	suite.Equal("upsert", delivery.Action)
}

func (suite *BlueskyTestSuite) TestOutboxDoesNotRepeatDeliveredEdit() {
	ctx := suite.T().Context()
	account := suite.testAccounts["local_account_1"]
	status := suite.testStatuses["local_account_1_status_1"]
	checkedAt := time.Now().Add(-time.Minute)
	connection := &gtsmodel.BlueskyConnection{
		ID: id.NewULID(), AccountID: account.ID, DID: "did:plc:outboxdelivered",
		Handle: "outbox-delivered.test", PDSURL: "https://pds.example.test",
		OAuthSessionID: "session", OAuthData: []byte("encrypted"), CrosspostPublic: true,
		OutboxCheckedAt: checkedAt,
	}
	suite.Require().NoError(suite.db.PutBlueskyConnection(ctx, connection))
	status.EditedAt = time.Now().Add(-2 * time.Second)
	suite.Require().NoError(suite.db.UpdateStatus(ctx, status, "edited_at"))
	post := &gtsmodel.BlueskyPost{
		ID: id.NewULID(), ConnectionID: connection.ID, AccountID: account.ID, StatusID: status.ID,
		URI: "at://did:plc:outboxdelivered/app.bsky.feed.post/root", CID: "new-cid",
		RootURI: "at://did:plc:outboxdelivered/app.bsky.feed.post/root", RootCID: "new-cid",
		URL:       "https://bsky.app/profile/did:plc:outboxdelivered/post/root",
		UpdatedAt: time.Now().Add(-time.Second),
	}
	suite.Require().NoError(suite.db.PutBlueskyPost(ctx, post))
	suite.Require().NoError(bluesky.ReconcileOutbox(ctx, &suite.state))
	_, err := suite.db.GetBlueskyDeliveryByStatusID(ctx, status.ID)
	suite.Error(err)
}

func (suite *BlueskyTestSuite) TestDisconnectCleanupPreservesPostMappingsAndResetsRetries() {
	ctx := suite.T().Context()
	account := suite.testAccounts["local_account_1"]
	status := suite.testStatuses["local_account_1_status_1"]
	connection := &gtsmodel.BlueskyConnection{
		ID: id.NewULID(), AccountID: account.ID, DID: "did:plc:preserve",
		Handle: "preserve.test", PDSURL: "https://pds.example.test",
		OAuthSessionID: "session", OAuthData: []byte("encrypted"),
		NotificationsSeenAt: time.Now().Add(-time.Hour), CrosspostPublic: true, ShowProfileFollow: false,
	}
	suite.Require().NoError(suite.db.PutBlueskyConnection(ctx, connection))
	connection.ShowProfileFollow = false
	suite.Require().NoError(suite.db.UpdateBlueskyConnection(ctx, connection, "show_profile_follow"))
	claimUntil := time.Now().Add(2 * time.Minute)
	claimed, err := suite.db.ClaimBlueskyConnection(ctx, connection.ID, time.Now(), claimUntil)
	suite.Require().NoError(err)
	suite.True(claimed)
	claimed, err = suite.db.ClaimBlueskyConnection(ctx, connection.ID, time.Now(), claimUntil)
	suite.Require().NoError(err)
	suite.False(claimed)
	renewedClaim := claimUntil.Add(time.Minute)
	renewed, err := suite.db.RenewBlueskyConnectionClaim(ctx, connection.ID, claimUntil, renewedClaim)
	suite.Require().NoError(err)
	suite.True(renewed)
	suite.Require().NoError(suite.db.ReleaseBlueskyConnectionClaim(ctx, connection.ID, renewedClaim))
	post := &gtsmodel.BlueskyPost{
		ID: id.NewULID(), ConnectionID: connection.ID, AccountID: account.ID, StatusID: status.ID,
		URI: "at://did:plc:preserve/app.bsky.feed.post/root", CID: "root-cid",
		RootURI: "at://did:plc:preserve/app.bsky.feed.post/root", RootCID: "root-cid",
		ParentURI: "at://did:plc:other/app.bsky.feed.post/parent", ParentCID: "parent-cid",
		URL: "https://bsky.app/profile/did:plc:preserve/post/root",
	}
	suite.Require().NoError(suite.db.PutBlueskyPost(ctx, post))
	isReply, err := bluesky.IsReplyTarget(ctx, &suite.state, &gtsmodel.Status{InReplyToID: status.ID})
	suite.Require().NoError(err)
	suite.True(isReply)
	delivery := &gtsmodel.BlueskyDelivery{
		ID: id.NewULID(), AccountID: account.ID, StatusID: suite.testStatuses["local_account_1_status_2"].ID,
		Action: "delete", Attempts: 10, DeadLetter: true, LastError: "failed", NextAttemptAt: time.Now(),
	}
	suite.Require().NoError(suite.db.PutBlueskyDelivery(ctx, delivery))
	suite.Require().NoError(suite.db.RetryBlueskyFailures(ctx, account.ID, time.Now()))
	reset, err := suite.db.GetBlueskyDeliveryByStatusID(ctx, delivery.StatusID)
	suite.Require().NoError(err)
	suite.Zero(reset.Attempts)
	suite.False(reset.DeadLetter)
	suite.Empty(reset.LastError)
	queued, err := bluesky.QueueStatus(ctx, &suite.state, suite.testStatuses["local_account_1_status_2"])
	suite.Require().NoError(err)
	suite.Equal("upsert", queued.Action)
	queued, err = bluesky.QueueDelete(ctx, &suite.state, account.ID, delivery.StatusID)
	suite.Require().NoError(err)
	suite.Equal("delete", queued.Action)
	mappedDelivery := &gtsmodel.BlueskyDelivery{
		ID: id.NewULID(), AccountID: account.ID, StatusID: status.ID,
		Action: "upsert", NextAttemptAt: time.Now(),
	}
	suite.Require().NoError(suite.db.PutBlueskyDelivery(ctx, mappedDelivery))

	suite.Require().NoError(suite.db.DeleteBlueskyConnectionDataByAccountID(ctx, account.ID))
	preservedConnection, err := suite.db.GetBlueskyConnectionByAccountID(ctx, account.ID)
	suite.Require().NoError(err)
	suite.False(preservedConnection.Active())
	suite.Equal(connection.NotificationsSeenAt.Unix(), preservedConnection.NotificationsSeenAt.Unix())
	suite.True(preservedConnection.CrosspostPublic)
	suite.False(preservedConnection.ShowProfileFollow)
	preserved, err := suite.db.GetBlueskyPostByStatusID(ctx, status.ID)
	suite.Require().NoError(err)
	suite.Equal(post.URI, preserved.URI)
	suite.Equal(post.ParentURI, preserved.ParentURI)
	_, err = suite.db.GetBlueskyDeliveryByStatusID(ctx, status.ID)
	suite.Require().NoError(err)
	_, err = suite.db.GetBlueskyDeliveryByStatusID(ctx, delivery.StatusID)
	suite.Error(err)
}

func (suite *BlueskyTestSuite) TestDisconnectDeletesProxyBeforeItsMapping() {
	ctx := suite.T().Context()
	account := suite.testAccounts["local_account_1"]
	proxyStatus := suite.testStatuses["local_account_1_status_2"]
	connection := &gtsmodel.BlueskyConnection{
		ID: id.NewULID(), AccountID: account.ID, DID: "did:plc:proxycleanup",
		Handle: "proxy-cleanup.test", PDSURL: "https://pds.example.test",
	}
	suite.Require().NoError(suite.db.PutBlueskyConnection(ctx, connection))
	interaction := &gtsmodel.BlueskyInteraction{
		ID: id.NewULID(), AccountID: account.ID, StatusID: proxyStatus.ID,
		URI: "at://did:plc:proxycleanup/app.bsky.feed.post/reply", CID: "reply-cid",
		RootURI: "at://did:plc:proxycleanup/app.bsky.feed.post/root", RootCID: "root-cid",
		ParentURI: "at://did:plc:proxycleanup/app.bsky.feed.post/root", ParentCID: "root-cid",
		AuthorDID: "did:plc:other", AuthorHandle: "other.test",
		URL: "https://bsky.app/profile/did:plc:other/post/reply",
	}
	suite.Require().NoError(suite.db.PutBlueskyInteraction(ctx, interaction))
	suite.Require().NoError(bluesky.Disconnect(ctx, &suite.state, account.ID))
	deletedProxy, err := suite.db.GetStatusByID(ctx, proxyStatus.ID)
	suite.Require().NoError(err)
	suite.True(deletedProxy.Flags.Deleted())
	_, err = suite.db.GetBlueskyInteractionByURI(ctx, interaction.URI)
	suite.Error(err)
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
	suite.Require().NoError(store.DeleteSession(ctx, did, session.SessionID))
	deferredSession := session
	deferredSession.SessionID = "deferred-session"
	store.DeferSessionPersistence()
	suite.Require().NoError(store.SaveSession(ctx, deferredSession))
	_, err = store.GetSession(ctx, did, deferredSession.SessionID)
	suite.Error(err)
	suite.Require().NoError(store.PersistSession(ctx, deferredSession))
	storedSession, err = store.GetSession(ctx, did, deferredSession.SessionID)
	suite.Require().NoError(err)
	suite.Equal(deferredSession.RefreshToken, storedSession.RefreshToken)
	otherDID, err := syntax.ParseDID("did:plc:differentoauthidentity")
	suite.Require().NoError(err)
	wrongSession := session
	wrongSession.AccountDID = otherDID
	wrongSession.SessionID = "wrong-session"
	suite.ErrorIs(store.SaveSession(ctx, wrongSession), bluesky.ErrIdentityMismatch)
	storedSession, err = store.GetSession(ctx, did, deferredSession.SessionID)
	suite.Require().NoError(err)
	suite.Equal(session.RefreshToken, storedSession.RefreshToken)

	storedConnection, err := suite.db.GetBlueskyConnectionByAccountID(ctx, account.ID)
	suite.Require().NoError(err)
	suite.NotContains(string(storedConnection.OAuthData), session.RefreshToken)
}

func TestBlueskyTestSuite(t *testing.T) {
	suite.Run(t, new(BlueskyTestSuite))
}
