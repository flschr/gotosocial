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

package status_test

import (
	"testing"

	"code.superseriousbusiness.org/gotosocial/internal/ap"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/processing/status"
	"github.com/stretchr/testify/suite"
)

// Test topo sorting of thread replies.
type topoSortTestSuite struct {
	suite.Suite
}

func statusIDs(apiStatuses []*gtsmodel.Status) []string {
	ids := make([]string, 0, len(apiStatuses))
	for _, apiStatus := range apiStatuses {
		ids = append(ids, apiStatus.ID)
	}
	return ids
}

func (suite *topoSortTestSuite) TestBranched() {
	// https://commons.wikimedia.org/wiki/File:Sorted_binary_tree_ALL_RGB.svg
	f := &gtsmodel.Status{ID: "F"}
	b := &gtsmodel.Status{ID: "B", InReplyToID: f.ID}
	a := &gtsmodel.Status{ID: "A", InReplyToID: b.ID}
	d := &gtsmodel.Status{ID: "D", InReplyToID: b.ID}
	c := &gtsmodel.Status{ID: "C", InReplyToID: d.ID}
	e := &gtsmodel.Status{ID: "E", InReplyToID: d.ID}
	g := &gtsmodel.Status{ID: "G", InReplyToID: f.ID}
	i := &gtsmodel.Status{ID: "I", InReplyToID: g.ID}
	h := &gtsmodel.Status{ID: "H", InReplyToID: i.ID}

	expected := statusIDs([]*gtsmodel.Status{f, b, a, d, c, e, g, i, h})
	list := []*gtsmodel.Status{a, b, c, d, e, f, g, h, i}
	status.TopoSort(list, "")
	actual := statusIDs(list)

	suite.Equal(expected, actual)
}

func (suite *topoSortTestSuite) TestBranchedWithSelfReplyChain() {
	targetAccount := &gtsmodel.Account{ID: "1"}
	otherAccount := &gtsmodel.Account{ID: "2"}

	f := &gtsmodel.Status{
		ID:      "F",
		Account: targetAccount,
	}
	b := &gtsmodel.Status{
		ID:                 "B",
		Account:            targetAccount,
		AccountID:          targetAccount.ID,
		InReplyToID:        f.ID,
		InReplyToAccountID: f.Account.ID,
	}
	d := &gtsmodel.Status{
		ID:                 "D",
		Account:            targetAccount,
		AccountID:          targetAccount.ID,
		InReplyToID:        b.ID,
		InReplyToAccountID: b.Account.ID,
	}
	e := &gtsmodel.Status{
		ID:                 "E",
		Account:            targetAccount,
		AccountID:          targetAccount.ID,
		InReplyToID:        d.ID,
		InReplyToAccountID: d.Account.ID,
	}
	c := &gtsmodel.Status{
		ID:                 "C",
		Account:            otherAccount,
		AccountID:          otherAccount.ID,
		InReplyToID:        d.ID,
		InReplyToAccountID: d.Account.ID,
	}
	a := &gtsmodel.Status{
		ID:                 "A",
		Account:            otherAccount,
		AccountID:          otherAccount.ID,
		InReplyToID:        b.ID,
		InReplyToAccountID: b.Account.ID,
	}
	g := &gtsmodel.Status{
		ID:                 "G",
		Account:            otherAccount,
		AccountID:          otherAccount.ID,
		InReplyToID:        f.ID,
		InReplyToAccountID: f.Account.ID,
	}
	i := &gtsmodel.Status{
		ID:                 "I",
		Account:            targetAccount,
		AccountID:          targetAccount.ID,
		InReplyToID:        g.ID,
		InReplyToAccountID: g.Account.ID,
	}
	h := &gtsmodel.Status{
		ID:                 "H",
		Account:            otherAccount,
		AccountID:          otherAccount.ID,
		InReplyToID:        i.ID,
		InReplyToAccountID: i.Account.ID,
	}

	expected := statusIDs([]*gtsmodel.Status{f, b, d, e, c, a, g, i, h})
	list := []*gtsmodel.Status{a, b, c, d, e, f, g, h, i}
	status.TopoSort(list, targetAccount.ID)
	actual := statusIDs(list)

	suite.Equal(expected, actual)
}

func (suite *topoSortTestSuite) TestDisconnected() {
	f := &gtsmodel.Status{ID: "F"}
	b := &gtsmodel.Status{ID: "B", InReplyToID: f.ID}
	dID := "D"
	e := &gtsmodel.Status{ID: "E", InReplyToID: dID}

	expected := statusIDs([]*gtsmodel.Status{e, f, b})
	list := []*gtsmodel.Status{b, e, f}
	status.TopoSort(list, "")
	actual := statusIDs(list)

	suite.Equal(expected, actual)
}

func (suite *topoSortTestSuite) TestTrivialCycle() {
	xID := "X"
	x := &gtsmodel.Status{ID: xID, InReplyToID: xID}

	expected := statusIDs([]*gtsmodel.Status{x})
	list := []*gtsmodel.Status{x}
	status.TopoSort(list, "")
	actual := statusIDs(list)

	suite.ElementsMatch(expected, actual)
}

func (suite *topoSortTestSuite) TestCycle() {
	yID := "Y"
	x := &gtsmodel.Status{ID: "X", InReplyToID: yID}
	y := &gtsmodel.Status{ID: yID, InReplyToID: x.ID}

	expected := statusIDs([]*gtsmodel.Status{x, y})
	list := []*gtsmodel.Status{x, y}
	status.TopoSort(list, "")
	actual := statusIDs(list)

	suite.ElementsMatch(expected, actual)
}

func (suite *topoSortTestSuite) TestMixedCycle() {
	yID := "Y"
	x := &gtsmodel.Status{ID: "X", InReplyToID: yID}
	y := &gtsmodel.Status{ID: yID, InReplyToID: x.ID}
	z := &gtsmodel.Status{ID: "Z"}

	expected := statusIDs([]*gtsmodel.Status{x, y, z})
	list := []*gtsmodel.Status{x, y, z}
	status.TopoSort(list, "")
	actual := statusIDs(list)

	suite.ElementsMatch(expected, actual)
}

func (suite *topoSortTestSuite) TestEmpty() {
	expected := statusIDs([]*gtsmodel.Status{})
	list := []*gtsmodel.Status{}
	status.TopoSort(list, "")
	actual := statusIDs(list)

	suite.Equal(expected, actual)
}

func (suite *topoSortTestSuite) TestNil() {
	expected := statusIDs(nil)
	var list []*gtsmodel.Status
	status.TopoSort(list, "")
	actual := statusIDs(list)

	suite.Equal(expected, actual)
}

func TestTopoSortTestSuite(t *testing.T) {
	suite.Run(t, &topoSortTestSuite{})
}

// Test properties of web contexts.
type webContextGetTestSuite struct {
	StatusStandardTestSuite

	indexableAccount1    *gtsmodel.Account
	indexableAccount2    *gtsmodel.Account
	nonindexableAccount1 *gtsmodel.Account
}

// Find a test account by name with a given indexable setting, or fail if we can't.
func (suite *webContextGetTestSuite) requireIndexableAccount(name string, indexable bool) *gtsmodel.Account {
	account := suite.testAccounts[name]
	if account == nil || account.Indexable == nil || *account.Indexable != indexable {
		suite.FailNowf("expected test account %s's indexable setting to be %v", name, indexable)
	}
	return account
}

func (suite *webContextGetTestSuite) SetupSuite() {
	suite.StatusStandardTestSuite.SetupSuite()

	suite.indexableAccount1 = suite.requireIndexableAccount("local_account_1", true)
	suite.indexableAccount2 = suite.requireIndexableAccount("admin_account", true)
	suite.nonindexableAccount1 = suite.requireIndexableAccount("local_account_2", false)
}

// Create a status in the test DB, or fail if we can't.
func (suite *webContextGetTestSuite) createStatus(account *gtsmodel.Account, visibility gtsmodel.Visibility, inReplyTo *gtsmodel.Status) *gtsmodel.Status {
	ctx := suite.T().Context()
	statusID := id.NewULID()
	newStatus := &gtsmodel.Status{
		ID:                  statusID,
		URI:                 "https://status-id.test/" + statusID,
		Flags:               gtsmodel.StatusFlags(gtsmodel.StatusFlagLocal | gtsmodel.StatusFlagFederated),
		AccountID:           account.ID,
		AccountURI:          account.URI,
		Visibility:          visibility,
		ActivityStreamsType: ap.ObjectNote,
	}
	if inReplyTo != nil {
		newStatus.InReplyToID = inReplyTo.ID
		newStatus.InReplyToAccountID = inReplyTo.AccountID
	}
	if err := suite.db.PutStatus(ctx, newStatus); err != nil {
		suite.FailNow(err.Error())
	}
	return newStatus
}

// Create a local-only direct reply that mentions targetAccount.
func (suite *webContextGetTestSuite) createPrivateLocalReply(
	account *gtsmodel.Account,
	targetAccount *gtsmodel.Account,
	inReplyTo *gtsmodel.Status,
) *gtsmodel.Status {
	ctx := suite.T().Context()
	statusID := id.NewULID()
	mentionID := id.NewULID()
	mention := &gtsmodel.Mention{
		ID:               mentionID,
		StatusID:         statusID,
		OriginAccountID:  account.ID,
		OriginAccountURI: account.URI,
		TargetAccountID:  targetAccount.ID,
	}
	newStatus := &gtsmodel.Status{
		ID:                  statusID,
		URI:                 "https://status-id.test/" + statusID,
		Flags:               gtsmodel.StatusFlags(gtsmodel.StatusFlagLocal),
		AccountID:           account.ID,
		AccountURI:          account.URI,
		InReplyToID:         inReplyTo.ID,
		InReplyToURI:        inReplyTo.URI,
		InReplyToAccountID:  inReplyTo.AccountID,
		MentionIDs:          []string{mentionID},
		Visibility:          gtsmodel.VisibilityDirect,
		ActivityStreamsType: ap.ObjectNote,
	}
	if err := suite.db.PutStatus(ctx, newStatus); err != nil {
		suite.FailNow(err.Error())
	}
	if err := suite.db.PutMention(ctx, mention); err != nil {
		suite.FailNow(err.Error())
	}
	return newStatus
}

// A private local-only reply can share the public post's thread while staying
// visible only to the mentioned local account through the standard client API.
func (suite *webContextGetTestSuite) TestPrivateLocalReplyUnderPublicStatus() {
	ctx := suite.T().Context()

	op := suite.createStatus(suite.indexableAccount1, gtsmodel.VisibilityPublic, nil)
	privateReply := suite.createPrivateLocalReply(
		suite.indexableAccount2,
		suite.indexableAccount1,
		op,
	)

	ownerContext, errWithCode := suite.status.ContextGet(ctx, suite.indexableAccount1, op.ID)
	if errWithCode != nil {
		suite.FailNow(errWithCode.Error())
	}
	suite.Require().Len(ownerContext.Descendants, 1)
	suite.Equal(privateReply.ID, ownerContext.Descendants[0].ID)
	suite.True(ownerContext.Descendants[0].LocalOnly)
	suite.Equal("direct", string(ownerContext.Descendants[0].Visibility))

	otherContext, errWithCode := suite.status.ContextGet(ctx, suite.nonindexableAccount1, op.ID)
	if errWithCode != nil {
		suite.FailNow(errWithCode.Error())
	}
	suite.Empty(otherContext.Descendants)

	webContext, errWithCode := suite.status.WebContextGet(ctx, op.ID)
	if errWithCode != nil {
		suite.FailNow(errWithCode.Error())
	}
	suite.Len(webContext.Statuses, 1)
	suite.Equal(1, webContext.ThreadRepliesHidden)
}

// If all visible statuses in a thread are indexable, so is the thread.
func (suite *webContextGetTestSuite) TestAllVisibleIndexable() {
	ctx := suite.T().Context()

	op := suite.createStatus(suite.indexableAccount1, gtsmodel.VisibilityPublic, nil)
	_ = suite.createStatus(suite.indexableAccount2, gtsmodel.VisibilityPublic, op)
	webContext, err := suite.status.WebContextGet(ctx, op.ID)
	if err != nil {
		suite.FailNow(err.Error())
		return
	}
	suite.Len(webContext.Statuses, 2, "precondition failed")
	suite.Equal(0, webContext.ThreadRepliesHidden, "precondition failed")

	suite.True(webContext.Indexable)
}

func (suite *webContextGetTestSuite) TestUnauthenticatedRemoteContextDoesNotRefresh() {
	ctx := suite.T().Context()
	remote := suite.testStatuses["remote_account_1_status_1"]
	suite.Require().NotNil(remote)

	before := suite.client.RequestCount(remote.URI)
	_, err := suite.status.WebContextGet(ctx, remote.ID)
	suite.Require().Nil(err)
	suite.Equal(before, suite.client.RequestCount(remote.URI))
}

// If any visible statuses in a thread are not indexable, so is the thread.
func (suite *webContextGetTestSuite) TestOneVisibleNonindexable() {
	ctx := suite.T().Context()

	op := suite.createStatus(suite.indexableAccount1, gtsmodel.VisibilityPublic, nil)
	_ = suite.createStatus(suite.indexableAccount2, gtsmodel.VisibilityPublic, op)
	_ = suite.createStatus(suite.nonindexableAccount1, gtsmodel.VisibilityPublic, op)
	webContext, err := suite.status.WebContextGet(ctx, op.ID)
	if err != nil {
		suite.FailNow(err.Error())
		return
	}
	suite.Len(webContext.Statuses, 3, "precondition failed")
	suite.Equal(0, webContext.ThreadRepliesHidden, "precondition failed")

	suite.False(webContext.Indexable)
}

// If the only nonindexable statuses in a thread are also invisible, then the thread is still indexable.
func (suite *webContextGetTestSuite) TestOneInvisibleNonindexable() {
	ctx := suite.T().Context()

	op := suite.createStatus(suite.indexableAccount1, gtsmodel.VisibilityPublic, nil)
	_ = suite.createStatus(suite.indexableAccount2, gtsmodel.VisibilityPublic, op)
	_ = suite.createStatus(suite.nonindexableAccount1, gtsmodel.VisibilityUnlocked, op)
	webContext, err := suite.status.WebContextGet(ctx, op.ID)
	if err != nil {
		suite.FailNow(err.Error())
		return
	}
	suite.Len(webContext.Statuses, 2, "precondition failed")
	suite.Equal(1, webContext.ThreadRepliesHidden, "precondition failed")

	suite.True(webContext.Indexable)
}

func TestWebContextGetTestSuite(t *testing.T) {
	suite.Run(t, &webContextGetTestSuite{})
}
