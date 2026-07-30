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

package bundb_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/paging"
	"code.superseriousbusiness.org/gotosocial/internal/util"
	"github.com/stretchr/testify/suite"
)

type InteractionTestSuite struct {
	BunDBStandardTestSuite
}

func (suite *InteractionTestSuite) markInteractionsPending(
	ctx context.Context,
	statusID string,
) (pendingCount int) {
	// Get replies of given status.
	replies, err := suite.state.DB.GetStatusReplies(ctx, statusID)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		suite.FailNow(err.Error())
	}

	// Mark each reply as pending approval.
	for _, reply := range replies {
		reply.Flags.SetPendingApproval(true)
		if err := suite.state.DB.UpdateStatus(ctx,
			reply,
			"flags",
		); err != nil {
			suite.FailNow(err.Error())
		}

		// Put an interaction request in the DB for this reply.
		intReqID := id.NewULIDFromTime(reply.CreatedAt)
		intReq := &gtsmodel.InteractionRequest{
			ID:                    intReqID,
			TargetStatusID:        reply.InReplyToID,
			TargetStatus:          reply.InReplyTo,
			TargetAccountID:       reply.InReplyToAccountID,
			TargetAccount:         reply.InReplyToAccount,
			InteractingAccountID:  reply.AccountID,
			InteractingAccount:    reply.Account,
			InteractionRequestURI: reply.URI + gtsmodel.ImpoliteReplyRequestSuffix,
			InteractionURI:        reply.URI,
			InteractionType:       gtsmodel.InteractionReply,
			Polite:                util.Ptr(false),
			Reply:                 reply,
		}
		if err := suite.state.DB.PutInteractionRequest(ctx, intReq); err != nil {
			suite.FailNow(err.Error())
		}

		pendingCount++
	}

	// Get boosts of given status.
	boosts, err := suite.state.DB.GetStatusBoosts(ctx, statusID)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		suite.FailNow(err.Error())
	}

	// Mark each boost as pending approval.
	for _, boost := range boosts {
		boost.Flags.SetPendingApproval(true)
		if err := suite.state.DB.UpdateStatus(ctx,
			boost,
			"flags",
		); err != nil {
			suite.FailNow(err.Error())
		}

		// Put an interaction request in the DB for this boost.
		intReqID := id.NewULIDFromTime(boost.CreatedAt)
		intReq := &gtsmodel.InteractionRequest{
			ID:                    intReqID,
			TargetStatusID:        boost.BoostOfID,
			TargetStatus:          boost.BoostOf,
			TargetAccountID:       boost.BoostOfAccountID,
			TargetAccount:         boost.BoostOfAccount,
			InteractingAccountID:  boost.AccountID,
			InteractingAccount:    boost.Account,
			InteractionRequestURI: boost.URI + gtsmodel.ImpoliteAnnounceRequestSuffix,
			InteractionURI:        boost.URI,
			InteractionType:       gtsmodel.InteractionAnnounce,
			Polite:                util.Ptr(false),
			Announce:              boost,
		}
		if err := suite.state.DB.PutInteractionRequest(ctx, intReq); err != nil {
			suite.FailNow(err.Error())
		}

		pendingCount++
	}

	// Get faves of given status.
	faves, err := suite.state.DB.GetStatusFaves(ctx, statusID)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		suite.FailNow(err.Error())
	}

	// Mark each fave as pending approval.
	for _, fave := range faves {
		fave.PendingApproval = util.Ptr(true)
		if err := suite.state.DB.UpdateStatusFave(
			ctx,
			fave,
			"pending_approval",
		); err != nil {
			suite.FailNow(err.Error())
		}

		// Put an impolite interaction request in the DB for this fave.
		intReqID := id.NewULIDFromTime(fave.CreatedAt)
		intReq := &gtsmodel.InteractionRequest{
			ID:                    intReqID,
			TargetStatusID:        fave.StatusID,
			TargetStatus:          fave.Status,
			TargetAccountID:       fave.TargetAccountID,
			TargetAccount:         fave.TargetAccount,
			InteractingAccountID:  fave.AccountID,
			InteractingAccount:    fave.Account,
			InteractionRequestURI: fave.URI + gtsmodel.ImpoliteLikeRequestSuffix,
			InteractionURI:        fave.URI,
			InteractionType:       gtsmodel.InteractionLike,
			Polite:                util.Ptr(false),
			Like:                  fave,
		}
		if err := suite.state.DB.PutInteractionRequest(ctx, intReq); err != nil {
			suite.FailNow(err.Error())
		}

		pendingCount++
	}

	return pendingCount
}

func (suite *InteractionTestSuite) TestGetPending() {
	var (
		testStatus = suite.testStatuses["local_account_1_status_1"]
		ctx        = suite.T().Context()
		acctID     = suite.testAccounts["local_account_1"].ID
		statusID   = ""
		likes      = true
		replies    = true
		boosts     = true
		page       = &paging.Page{
			Max:   paging.MaxID(id.Highest),
			Limit: 20,
		}
	)

	// Update target test status to mark
	// all interactions with it pending.
	pendingCount := suite.markInteractionsPending(ctx, testStatus.ID)

	// Get pendingInts interactions.
	pendingInts, err := suite.state.DB.GetInteractionsRequestsForAcct(
		ctx,
		acctID,
		statusID,
		likes,
		replies,
		boosts,
		true,
		page,
	)
	suite.NoError(err)
	suite.Len(pendingInts, pendingCount)

	// Ensure relevant model populated.
	for _, pendingInt := range pendingInts {
		switch pendingInt.InteractionType {

		case gtsmodel.InteractionLike:
			suite.NotNil(pendingInt.Like)

		case gtsmodel.InteractionReply:
			suite.NotNil(pendingInt.Reply)

		case gtsmodel.InteractionAnnounce:
			suite.NotNil(pendingInt.Announce)
		}
	}
}

func (suite *InteractionTestSuite) TestGetPendingRepliesOnly() {
	var (
		testStatus = suite.testStatuses["local_account_1_status_1"]
		ctx        = suite.T().Context()
		acctID     = suite.testAccounts["local_account_1"].ID
		statusID   = ""
		likes      = false
		replies    = true
		boosts     = false
		page       = &paging.Page{
			Max:   paging.MaxID(id.Highest),
			Limit: 20,
		}
	)

	// Update target test status to mark
	// all interactions with it pending.
	suite.markInteractionsPending(ctx, testStatus.ID)

	// Get pendingInts interactions.
	pendingInts, err := suite.state.DB.GetInteractionsRequestsForAcct(
		ctx,
		acctID,
		statusID,
		likes,
		replies,
		boosts,
		false,
		page,
	)
	suite.NoError(err)

	// Ensure only replies returned.
	for _, pendingInt := range pendingInts {
		suite.Equal(gtsmodel.InteractionReply, pendingInt.InteractionType)
	}
}

func (suite *InteractionTestSuite) TestGetPendingQuotesOnlyForStatus() {
	var (
		ctx          = suite.T().Context()
		targetStatus = suite.testStatuses["local_account_1_status_1"]
		quoteStatus  = suite.testStatuses["local_account_2_status_1"]
	)

	request := &gtsmodel.InteractionRequest{
		ID:                    id.NewULID(),
		TargetStatusID:        targetStatus.ID,
		TargetStatus:          targetStatus,
		TargetAccountID:       targetStatus.AccountID,
		TargetAccount:         targetStatus.Account,
		InteractingAccountID:  quoteStatus.AccountID,
		InteractingAccount:    quoteStatus.Account,
		InteractionRequestURI: quoteStatus.URI + "#quote-request",
		InteractionURI:        quoteStatus.URI,
		InteractionType:       gtsmodel.InteractionQuote,
		Polite:                util.Ptr(true),
		Quote:                 quoteStatus,
	}
	suite.NoError(suite.state.DB.PutInteractionRequest(ctx, request))

	pending, err := suite.state.DB.GetInteractionsRequestsForAcct(
		ctx,
		"",
		targetStatus.ID,
		false,
		false,
		false,
		true,
		&paging.Page{Max: paging.MaxID(id.Highest), Limit: 20},
	)
	suite.NoError(err)
	suite.Len(pending, 1)
	suite.Equal(gtsmodel.InteractionQuote, pending[0].InteractionType)
	suite.NotNil(pending[0].Quote)
	suite.Equal(quoteStatus.ID, pending[0].Quote.ID)
}

func (suite *InteractionTestSuite) TestInteractionRejected() {
	var (
		ctx = suite.T().Context()
		req = new(gtsmodel.InteractionRequest)
	)

	// Make a copy of the request we'll modify.
	*req = *suite.testInteractionRequests["admin_account_reply_turtle"]

	// No rejection in the db for this interaction URI so it should be OK.
	rejected, err := suite.state.DB.IsInteractionRejected(ctx, req.InteractionURI)
	if err != nil {
		suite.FailNow(err.Error())
	}
	if rejected {
		suite.FailNow("wanted rejected = false, got true")
	}

	// Update the interaction request to mark it rejected.
	req.RejectedAt = time.Now()
	req.ResponseURI = "https://some.reject.uri"
	if err := suite.state.DB.UpdateInteractionRequest(ctx, req, "response_uri", "rejected_at"); err != nil {
		suite.FailNow(err.Error())
	}

	// Rejection in the db for this interaction URI now so it should be très mauvais.
	rejected, err = suite.state.DB.IsInteractionRejected(ctx, req.InteractionURI)
	if err != nil {
		suite.FailNow(err.Error())
	}
	if !rejected {
		suite.FailNow("wanted rejected = true, got false")
	}
}

func TestInteractionTestSuite(t *testing.T) {
	suite.Run(t, new(InteractionTestSuite))
}
