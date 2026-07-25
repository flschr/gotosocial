// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package dereferencing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"golang.org/x/net/html"
)

const microblogReplyPageMaxSize = 2 << 20

func (d *Dereferencer) enrichMicroblogInReplyTo(
	ctx context.Context,
	requestUser string,
	status *gtsmodel.Status,
) error {
	statusURL, err := url.Parse(status.URL)
	if err != nil || !strings.EqualFold(statusURL.Hostname(), "micro.blog") {
		return nil
	}

	tsport, err := d.transportController.NewTransportForUsername(ctx, requestUser)
	if err != nil {
		return fmt.Errorf("create transport: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL.String(), nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "text/html")

	rsp, err := tsport.GET(req)
	if err != nil {
		return fmt.Errorf("fetch status page: %w", err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode < http.StatusOK || rsp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("fetch status page: unexpected HTTP status %s", rsp.Status)
	}

	parentURL, err := parseMicroblogInReplyTo(
		io.LimitReader(rsp.Body, microblogReplyPageMaxSize),
	)
	if err != nil || parentURL == "" {
		return err
	}

	parent, err := d.state.DB.GetStatusByURI(ctx, parentURL)
	if errors.Is(err, db.ErrNoEntries) {
		parent, err = d.state.DB.GetStatusByURL(ctx, parentURL)
	}
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		return fmt.Errorf("lookup reply parent %s: %w", parentURL, err)
	}

	if parent == nil {
		status.InReplyToURI = parentURL
		return nil
	}

	status.InReplyToURI = parent.URI
	status.InReplyToID = parent.ID
	status.InReplyTo = parent
	status.InReplyToAccountID = parent.AccountID
	status.InReplyToAccount = parent.Account
	return nil
}

func parseMicroblogInReplyTo(r io.Reader) (string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return "", fmt.Errorf("parse status page: %w", err)
	}

	var walk func(*html.Node, bool) string
	walk = func(node *html.Node, selectedPost bool) string {
		if node.Type == html.ElementNode {
			if node.Data == "div" && hasHTMLClass(node, "post_selected") {
				selectedPost = true
			}
			if selectedPost &&
				node.Data == "a" &&
				hasHTMLClass(node, "u-in-reply-to") {
				return htmlAttribute(node, "href")
			}
		}

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if href := walk(child, selectedPost); href != "" {
				return href
			}
		}
		return ""
	}

	return walk(doc, false), nil
}

func hasHTMLClass(node *html.Node, class string) bool {
	for _, attr := range node.Attr {
		if attr.Key == "class" {
			for _, value := range strings.Fields(attr.Val) {
				if value == class {
					return true
				}
			}
		}
	}
	return false
}

func htmlAttribute(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}
