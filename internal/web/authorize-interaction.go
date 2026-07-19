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
	"net/http"
	"net/url"

	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	apiutil "code.superseriousbusiness.org/gotosocial/internal/api/util"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"github.com/gin-gonic/gin"
)

// authorizeInteractionGETHandler handles redirects from remote
// (usually Mastodon) instances when a user tries to do a
// "remote interaction" and gives their GoToSocial account/domain.
// We use this handler instead of serving a generic 404 page.
func (m *Module) authorizeInteractionGETHandler(c *gin.Context) {
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

	// Redirects to the "authorize_interaction"
	// endpoint should contain the URI of the
	// object that the user is trying to interact
	// with in the 'uri' query param.
	uriStr := c.Query("uri")
	if uriStr == "" {
		const text = "no uri query parameter found in string"
		errWithCode := gtserror.NewWithCode(http.StatusNotFound, text)
		apiutil.WebErrorHandler(c, errWithCode, instanceGet)
	}

	// Try to parse the object URI.
	interactionURI, err := url.Parse(uriStr)
	if err != nil {
		err := gtserror.Newf("interaction URI could not be parsed: %w", err)
		errWithCode := gtserror.NewErrorBadRequest(err, err.Error())
		apiutil.WebErrorHandler(c, errWithCode, instanceGet)
	}

	page := apiutil.WebPage{
		Template:    "authorize-interaction.tmpl",
		Instance:    instance,
		OGMeta:      apiutil.OGBase(instance),
		Stylesheets: []string{cssAbout},
		Javascript: []apiutil.JavascriptEntry{
			frontendJavascript(),
		},
		Extra: map[string]any{
			"interactionURI": interactionURI,
		},
	}

	apiutil.TemplateWebPage(c, page)
}
