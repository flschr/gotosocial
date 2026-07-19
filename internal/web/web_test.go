// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import "testing"

func TestFrontendScriptWaitsForParsedMarkup(t *testing.T) {
	script := frontendJavascript()
	if script.Async {
		t.Fatal("frontend script must not run asynchronously before profile markup exists")
	}
	if !script.Defer {
		t.Fatal("frontend script must wait for profile markup to be parsed")
	}
	if script.Src != jsFrontend {
		t.Fatalf("unexpected frontend script %q", script.Src)
	}
}
