// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package user_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"code.superseriousbusiness.org/gotosocial/testrig"
	"github.com/stretchr/testify/suite"
)

type BlueskyHandlerTestSuite struct {
	UserStandardTestSuite
}

func (suite *BlueskyHandlerTestSuite) TestMetadataIsPublic() {
	recorder := httptest.NewRecorder()
	ctx, _ := testrig.CreateGinTestContext(recorder, nil)
	ctx.Request = httptest.NewRequest(http.MethodGet, "http://localhost:8080/api/v1/user/bluesky/client-metadata.json", nil)
	suite.userModule.BlueskyMetadataGETHandler(ctx)
	suite.Equal(http.StatusOK, recorder.Code)
	suite.Contains(recorder.Body.String(), `"dpop_bound_access_tokens":true`)
}

func (suite *BlueskyHandlerTestSuite) TestInvalidCallbackRedirectsSafely() {
	recorder := httptest.NewRecorder()
	ctx, _ := testrig.CreateGinTestContext(recorder, nil)
	ctx.Request = httptest.NewRequest(http.MethodGet, "http://localhost:8080/api/v1/user/bluesky/callback?state=missing", nil)
	suite.userModule.BlueskyCallbackGETHandler(ctx)
	suite.Equal(http.StatusFound, recorder.Code)
	suite.Equal("/settings/user/bluesky?error=connection_failed", recorder.Header().Get("Location"))
}

func TestBlueskyHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(BlueskyHandlerTestSuite))
}
