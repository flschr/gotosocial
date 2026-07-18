// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"net/http"

	"code.superseriousbusiness.org/gotosocial/internal/gtscontext"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"github.com/bluesky-social/indigo/atproto/atclient"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func newATClient(state *state.State, host string) *atclient.APIClient {
	client := atclient.NewAPIClient(host)
	client.Client = protectedHTTPClient(state)
	return client
}

func protectedHTTPClient(state *state.State) *http.Client {
	return &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		return state.HTTPClient.Do(request.WithContext(gtscontext.SetFastFail(request.Context())))
	})}
}
