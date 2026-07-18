// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package user

import (
	"net/http"

	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	apiutil "code.superseriousbusiness.org/gotosocial/internal/api/util"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"github.com/gin-gonic/gin"
)

func (m *Module) BlueskyGETHandler(c *gin.Context) {
	authed, errWithCode := apiutil.TokenAuth(c, true, true, true, true, apiutil.ScopeReadAccounts)
	if errWithCode != nil {
		apiutil.ErrorHandler(c, errWithCode, m.processor.InstanceGetV1)
		return
	}

	connection, errWithCode := m.processor.BlueskyConnectionGet(c.Request.Context(), authed.Account.ID)
	if errWithCode != nil {
		apiutil.ErrorHandler(c, errWithCode, m.processor.InstanceGetV1)
		return
	}
	apiutil.JSON(c, http.StatusOK, connection)
}

func (m *Module) BlueskyPATCHHandler(c *gin.Context) {
	authed, errWithCode := apiutil.TokenAuth(c, true, true, true, true, apiutil.ScopeWriteAccounts)
	if errWithCode != nil {
		apiutil.ErrorHandler(c, errWithCode, m.processor.InstanceGetV1)
		return
	}

	form := new(apimodel.BlueskySettingsUpdateRequest)
	if err := c.ShouldBind(form); err != nil {
		apiutil.ErrorHandler(c, gtserror.NewErrorBadRequest(err, err.Error()), m.processor.InstanceGetV1)
		return
	}
	connection, errWithCode := m.processor.BlueskySettingsUpdate(c.Request.Context(), authed.Account.ID, form)
	if errWithCode != nil {
		apiutil.ErrorHandler(c, errWithCode, m.processor.InstanceGetV1)
		return
	}
	apiutil.JSON(c, http.StatusOK, connection)
}

func (m *Module) BlueskyConnectPOSTHandler(c *gin.Context) {
	authed, errWithCode := apiutil.TokenAuth(c, true, true, true, true, apiutil.ScopeWriteAccounts)
	if errWithCode != nil {
		apiutil.ErrorHandler(c, errWithCode, m.processor.InstanceGetV1)
		return
	}
	form := new(apimodel.BlueskyConnectRequest)
	if err := c.ShouldBind(form); err != nil {
		apiutil.ErrorHandler(c, gtserror.NewErrorBadRequest(err, err.Error()), m.processor.InstanceGetV1)
		return
	}
	response, errWithCode := m.processor.BlueskyConnectStart(c.Request.Context(), authed.Account.ID, form.Identifier)
	if errWithCode != nil {
		apiutil.ErrorHandler(c, errWithCode, m.processor.InstanceGetV1)
		return
	}
	apiutil.JSON(c, http.StatusOK, response)
}

func (m *Module) BlueskyCallbackGETHandler(c *gin.Context) {
	redirectURL, errWithCode := m.processor.BlueskyConnectCallback(c.Request.Context(), c.Request.URL.Query())
	if errWithCode != nil {
		apiutil.ErrorHandler(c, errWithCode, m.processor.InstanceGetV1)
		return
	}
	c.Redirect(http.StatusFound, redirectURL)
}

func (m *Module) BlueskyMetadataGETHandler(c *gin.Context) {
	metadata, err := m.processor.BlueskyClientMetadata()
	if err != nil {
		apiutil.ErrorHandler(c, gtserror.NewErrorInternalError(err), m.processor.InstanceGetV1)
		return
	}
	apiutil.JSON(c, http.StatusOK, metadata)
}

func (m *Module) BlueskyDELETEHandler(c *gin.Context) {
	authed, errWithCode := apiutil.TokenAuth(c, true, true, true, true, apiutil.ScopeWriteAccounts)
	if errWithCode != nil {
		apiutil.ErrorHandler(c, errWithCode, m.processor.InstanceGetV1)
		return
	}
	if errWithCode := m.processor.BlueskyDisconnect(c.Request.Context(), authed.Account.ID); errWithCode != nil {
		apiutil.ErrorHandler(c, errWithCode, m.processor.InstanceGetV1)
		return
	}
	c.Status(http.StatusNoContent)
}
