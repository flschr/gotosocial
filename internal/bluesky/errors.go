// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bluesky-social/indigo/atproto/atclient"
)

const (
	ErrorCodeAuth          = "auth"
	ErrorCodeConfiguration = "configuration"
	ErrorCodeRemote        = "remote"
	ErrorCodeData          = "data"
)

type ConnectionError struct {
	Code string
	Err  error
}

func (e *ConnectionError) Error() string { return fmt.Sprintf("Bluesky %s error: %v", e.Code, e.Err) }
func (e *ConnectionError) Unwrap() error { return e.Err }

func errorCode(err error) string {
	var connectionErr *ConnectionError
	if errors.As(err, &connectionErr) {
		return connectionErr.Code
	}
	var apiErr *atclient.APIError
	if errors.As(err, &apiErr) && (apiErr.Name == "ExpiredToken" || apiErr.Name == "InvalidToken" || apiErr.Name == "InvalidLogin") {
		return ErrorCodeAuth
	}
	// Indigo currently returns OAuth token endpoint failures as formatted
	// errors rather than a typed error. Treat terminal refresh failures as
	// authentication errors so operators and users are told to reconnect
	// instead of waiting for a retry that cannot succeed.
	if isTerminalOAuthError(err) {
		return ErrorCodeAuth
	}
	return ErrorCodeRemote
}

func isTerminalOAuthError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "invalid_grant") ||
		strings.Contains(message, "session expired") ||
		strings.Contains(message, "invalid refresh token")
}
