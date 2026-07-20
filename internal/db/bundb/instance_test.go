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
	"strconv"
	"testing"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/paging"
	"code.superseriousbusiness.org/gotosocial/internal/util"
	"github.com/stretchr/testify/suite"
)

type InstanceTestSuite struct {
	BunDBStandardTestSuite
}

func (suite *InstanceTestSuite) TestCountInstanceUsers() {
	count, err := suite.db.CountInstanceAccounts(suite.T().Context())
	suite.NoError(err)
	suite.Equal(5, count)
}

func (suite *InstanceTestSuite) TestCountInstanceStatuses() {
	count, err := suite.db.CountInstanceStatuses(suite.T().Context())
	suite.NoError(err)
	suite.Equal(24, count)
}

func (suite *InstanceTestSuite) TestCountInstancePeers() {
	count, err := suite.db.CountInstancePeers(suite.T().Context())
	suite.NoError(err)
	suite.Equal(4, count)
}

func (suite *InstanceTestSuite) TestGetInstanceNonexistent() {
	instance, err := suite.db.GetInstance(suite.T().Context(), "doesnt.exist.com")
	suite.ErrorIs(err, db.ErrNoEntries)
	suite.Nil(instance)
}

func (suite *InstanceTestSuite) TestGetInstancePeers() {
	peers, err := suite.db.GetInstancePeers(suite.T().Context(), false)
	suite.NoError(err)
	suite.Len(peers, 4)
}

func (suite *InstanceTestSuite) TestGetInstancePeersIncludeSuspended() {
	peers, err := suite.db.GetInstancePeers(suite.T().Context(), true)
	suite.NoError(err)
	suite.Len(peers, 5)
}

func (suite *InstanceTestSuite) TestGetInstanceAccounts() {
	accounts, err := suite.db.GetInstanceAccounts(suite.T().Context(), "fossbros-anonymous.io", "", 10)
	suite.NoError(err)
	suite.Len(accounts, 1)
}

func (suite *InstanceTestSuite) TestGetInstanceModeratorAddressesOK() {
	// We have one admin user by default.
	addresses, err := suite.db.GetInstanceModeratorAddresses(suite.T().Context())
	suite.NoError(err)
	suite.EqualValues([]string{"admin@example.org"}, addresses)
}

func (suite *InstanceTestSuite) TestGetInstanceModeratorAddressesZorkAsModerator() {
	// Promote zork to moderator role.
	testUser := &gtsmodel.User{}
	*testUser = *suite.testUsers["local_account_1"]
	testUser.Moderator = util.Ptr(true)
	if err := suite.db.UpdateUser(suite.T().Context(), testUser, "moderator"); err != nil {
		suite.FailNow(err.Error())
	}

	addresses, err := suite.db.GetInstanceModeratorAddresses(suite.T().Context())
	suite.NoError(err)
	suite.EqualValues([]string{"admin@example.org", "zork@example.org"}, addresses)
}

func (suite *InstanceTestSuite) TestGetInstanceModeratorAddressesNoAdmin() {
	// Demote admin from admin + moderator roles.
	testUser := &gtsmodel.User{}
	*testUser = *suite.testUsers["admin_account"]
	testUser.Admin = util.Ptr(false)
	testUser.Moderator = util.Ptr(false)
	if err := suite.db.UpdateUser(suite.T().Context(), testUser, "admin", "moderator"); err != nil {
		suite.FailNow(err.Error())
	}

	addresses, err := suite.db.GetInstanceModeratorAddresses(suite.T().Context())
	suite.ErrorIs(err, db.ErrNoEntries)
	suite.Empty(addresses)
}

func (suite *InstanceTestSuite) TestInstanceDeliveryTracking() {
	ctx := suite.T().Context()
	testInstance := suite.testInstances["thequeenisstillalive.technology"]

	for i := 0; i <= 25; i++ {
		if err := suite.state.DB.AddInstanceDeliveryError(ctx,
			testInstance.Domain,
			"error "+strconv.Itoa(i),
		); err != nil {
			suite.FailNow(err.Error())
		}

		instance, err := suite.state.DB.GetInstanceByID(ctx, testInstance.ID)
		if err != nil {
			suite.FailNow(err.Error())
		}

		if l := len(instance.DeliveryErrors); l > 20 {
			suite.FailNow("", "instance delivery errors length was %d, wanted < 20", l)
		}
	}

	// Clear all the errors we just added by setting successful delivery to now.
	if err := suite.state.DB.SetInstanceSuccessfulDelivery(ctx, testInstance.Domain); err != nil {
		suite.FailNow(err.Error())
	}

	instance, err := suite.state.DB.GetInstanceByID(ctx, testInstance.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}

	suite.Empty(instance.DeliveryErrors)
	suite.WithinDuration(time.Now(), instance.LatestSuccessfulDelivery, 1*time.Minute)
}

func (suite *InstanceTestSuite) TestInstanceWithErrors() {
	ctx := suite.T().Context()

	// Set paging.
	page := new(paging.Page)
	page.Max.Value = suite.testInstances["example.org"].ID
	page.Max.Order = paging.OrderDescending

	// Get alphabetical.
	instances, err := suite.state.DB.GetInstancesPage(
		ctx,
		page,
		"", // domain
		gtsmodel.InstanceOrderByAlphabetical,
		true, // withErrorsOnly
	)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.Len(instances, 1)

	// Get by first seen.
	instances, err = suite.state.DB.GetInstancesPage(
		ctx,
		page,
		"", // domain
		gtsmodel.InstanceOrderByFirstSeen,
		true, // withErrorsOnly
	)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.Len(instances, 1)
}

func TestInstanceTestSuite(t *testing.T) {
	suite.Run(t, new(InstanceTestSuite))
}
