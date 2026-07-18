// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"errors"
	"fmt"
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
	return ErrorCodeRemote
}
