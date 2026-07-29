// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package federatingdb_test

import (
	"testing"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/ap"
	"code.superseriousbusiness.org/gotosocial/internal/gtscontext"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/uris"
	"code.superseriousbusiness.org/gotosocial/internal/util"
	"code.superseriousbusiness.org/gotosocial/testrig"
	"github.com/stretchr/testify/suite"
)

type DeleteTestSuite struct {
	FederatingDBTestSuite
}

func (suite *DeleteTestSuite) TestDeleteQuoteAuthorization() {
	quotingAccount := suite.testAccounts["local_account_1"]
	targetAccount := suite.testAccounts["remote_account_1"]
	ctx := createTestContext(suite.T(), quotingAccount, targetAccount)
	ctx = gtscontext.SetActivityID(
		ctx,
		testrig.URLMustParse(targetAccount.URI+"/deletes/quote-1"),
	)

	quote := suite.testStatuses["local_account_1_status_1"]
	target := suite.testStatuses["remote_account_1_status_1"]
	authURI := targetAccount.URI + "/authorizations/quote-1"
	quote.QuoteID = target.ID
	quote.Quote = target
	quote.QuoteURI = target.URI
	quote.QuoteAccountID = target.AccountID
	quote.QuoteAccount = targetAccount
	quote.QuoteApprovalURI = authURI
	suite.NoError(suite.state.DB.UpdateStatus(
		ctx,
		quote,
		"quote_id",
		"quote_uri",
		"quote_account_id",
		"quote_approval_uri",
	))

	req := &gtsmodel.InteractionRequest{
		ID:                    "01K4STEH5NWAXBZ4TFNGQQQ987",
		TargetStatusID:        target.ID,
		TargetStatus:          target,
		TargetAccountID:       target.AccountID,
		TargetAccount:         targetAccount,
		InteractingAccountID:  quote.AccountID,
		InteractingAccount:    quotingAccount,
		InteractionRequestURI: uris.GenerateURIForQuoteRequest(quotingAccount.Username, "01K4STEH5NWAXBZ4TFNGQQQ987"),
		InteractionURI:        quote.URI,
		InteractionType:       gtsmodel.InteractionQuote,
		Polite:                util.Ptr(true),
		Quote:                 quote,
		AcceptedAt:            time.Now(),
		ResponseURI:           targetAccount.URI + "/accepts/quote-1",
		AuthorizationURI:      authURI,
	}
	suite.NoError(suite.state.DB.PutInteractionRequest(ctx, req))

	suite.NoError(suite.federatingDB.Delete(ctx, testrig.URLMustParse(authURI)))

	storedReq, err := suite.state.DB.GetInteractionRequestByID(ctx, req.ID)
	suite.NoError(err)
	suite.True(storedReq.IsAccepted())
	suite.Empty(storedReq.AuthorizationURI)

	storedQuote, err := suite.state.DB.GetStatusByID(ctx, quote.ID)
	suite.NoError(err)
	suite.Empty(storedQuote.QuoteApprovalURI)

	msg, ok := suite.state.Workers.Federator.Queue.PopCtx(ctx)
	suite.True(ok)
	suite.Equal(ap.ObjectQuoteAuthorization, msg.APObjectType)
	suite.Equal(ap.ActivityDelete, msg.APActivityType)
	suite.Equal(authURI, msg.APIRI.String())
}

func (suite *DeleteTestSuite) TestDeleteThirdPartyQuoteAuthorization() {
	receivingAccount := suite.testAccounts["local_account_1"]
	targetAccount := suite.testAccounts["remote_account_1"]
	ctx := createTestContext(suite.T(), receivingAccount, targetAccount)

	quote := suite.testStatuses["remote_account_2_status_1"]
	target := suite.testStatuses["remote_account_1_status_1"]
	authURI := targetAccount.URI + "/authorizations/third-party-quote-1"
	quote.QuoteID = target.ID
	quote.Quote = target
	quote.QuoteURI = target.URI
	quote.QuoteAccountID = target.AccountID
	quote.QuoteAccount = targetAccount
	quote.QuoteApprovalURI = authURI
	suite.NoError(suite.state.DB.UpdateStatus(
		ctx,
		quote,
		"quote_id",
		"quote_uri",
		"quote_account_id",
		"quote_approval_uri",
	))

	suite.NoError(suite.federatingDB.Delete(ctx, testrig.URLMustParse(authURI)))

	storedQuote, err := suite.state.DB.GetStatusByID(ctx, quote.ID)
	suite.NoError(err)
	suite.Empty(storedQuote.QuoteApprovalURI)

	_, ok := suite.state.Workers.Federator.Queue.Pop()
	suite.False(ok)
}

func TestDeleteTestSuite(t *testing.T) {
	suite.Run(t, new(DeleteTestSuite))
}
