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

package web

import (
	"context"
	"errors"

	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	apiutil "code.superseriousbusiness.org/gotosocial/internal/api/util"
	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/paging"
	"github.com/gin-gonic/gin"
)

const (
	directoryPath = "/directory"
)

func (m *Module) directoryGETHandler(c *gin.Context) {
	instance, errWithCode := m.processor.InstanceGetV1(c.Request.Context())
	if errWithCode != nil {
		apiutil.WebErrorHandler(c, errWithCode, m.processor.InstanceGetV1)
		return
	}

	// Return instance we already got from the db,
	// don't try to fetch it again when erroring.
	instanceGet := func(ctx context.Context) (*apimodel.InstanceV1, gtserror.WithCode) {
		return instance, nil
	}

	// We only serve text/html at this endpoint.
	if _, errWithCode := apiutil.NegotiateAccept(c, apiutil.TextHTML); errWithCode != nil {
		apiutil.WebErrorHandler(c, errWithCode, instanceGet)
		return
	}

	// Only serve the directory if permitted to do so.
	if !config.GetInstanceDirectoryWebEnabled() {
		const errText = "directory not exposed"
		const errTextHelpful = "this instance does not currently expose an account directory"
		errWithCode := gtserror.NewErrorNotFound(errors.New(errText), errTextHelpful)
		apiutil.WebErrorHandler(c, errWithCode, instanceGet)
		return
	}

	// Parse paging params, forcing limit of 40.
	page, errWithCode := paging.ParseIDPage(c,
		40, // min limit
		40, // max limit
		40, // default limit
	)
	if errWithCode != nil {
		apiutil.WebErrorHandler(c, errWithCode, instanceGet)
		return
	}

	// Parse order (default "active").
	orderBy, errWithCode := apiutil.ParseDirectoryOrder(
		c.Query(apiutil.OrderKey),
		gtsmodel.DirectoryOrderByActive,
	)
	if errWithCode != nil {
		apiutil.WebErrorHandler(c, errWithCode, instanceGet)
		return
	}

	// Get web model accounts.
	resp, errWithCode := m.processor.Account().WebDirectoryGet(
		c.Request.Context(),
		page,
		orderBy,
	)
	if errWithCode != nil {
		apiutil.WebErrorHandler(c, errWithCode, instanceGet)
		return
	}

	// If we're not on the first page
	// of results, show previous link.
	var accountsPrev string
	paging := page.GetMax() != ""
	if paging {
		accountsPrev = resp.PrevLink
	}

	// If indexing is allowed, set robots
	// meta to more permissive setting.
	var robotsMeta string
	if config.GetInstanceRobotsAllowIndexing() {
		robotsMeta = apiutil.RobotsDirectivesAllowSome
	}

	apiutil.TemplateWebPage(c, apiutil.WebPage{
		Template:    "directory.tmpl",
		Instance:    instance,
		OGMeta:      apiutil.OGBase(instance),
		Stylesheets: []string{cssFA, cssDirectory},
		Extra: map[string]any{
			"accounts":      resp.Items,
			"accounts_next": resp.NextLink,
			"accounts_prev": accountsPrev,
			"robotsMeta":    robotsMeta,
		},
	})
}
