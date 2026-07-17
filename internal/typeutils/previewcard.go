// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package typeutils

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/gtscontext"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"golang.org/x/net/html"
)

const (
	previewCardMaxHTML = 1 << 20
	previewCardTimeout = 4 * time.Second
	previewCardTTL     = 24 * time.Hour
	previewCardMissTTL = time.Hour
	previewCardMaxText = 2000
)

type previewCardCacheEntry struct {
	card      *model.Card
	expiresAt time.Time
	loading   bool
}

func (c *Converter) previewCardForStatus(ctx context.Context, status *gtsmodel.Status, sensitive bool) *model.Card {
	if !config.GetStatusesPreviewCards() || c.state.HTTPClient == nil || status.Content == "" || sensitive {
		return nil
	}

	cardURL := firstPreviewCardURL(status.Content)
	if cardURL == nil {
		return nil
	}

	key := cardURL.String()
	if cached, ok := c.previewCards.Load(key); ok {
		entry := cached.(previewCardCacheEntry)
		if entry.loading || time.Now().Before(entry.expiresAt) {
			return entry.card
		}
		c.previewCards.Delete(key)
	}

	// Reserve this URL so concurrent status conversions do not all fetch it.
	placeholder := previewCardCacheEntry{
		expiresAt: time.Now().Add(previewCardTimeout),
		loading:   true,
	}
	if cached, loaded := c.previewCards.LoadOrStore(key, placeholder); loaded {
		return cached.(previewCardCacheEntry).card
	}

	// New local posts are fetched synchronously so the create response already
	// contains its card. Older and remote timeline entries warm the cache in the
	// background, preventing a page of uncached links from blocking serially.
	recentLocal := status.Account != nil &&
		status.Account.IsLocal() &&
		time.Since(status.CreatedAt) < 5*time.Minute
	if recentLocal {
		return c.fetchAndCachePreviewCard(ctx, key, cardURL)
	}

	go c.fetchAndCachePreviewCard(context.Background(), key, cardURL)
	return nil
}

func (c *Converter) fetchAndCachePreviewCard(ctx context.Context, key string, cardURL *url.URL) *model.Card {
	fetchCtx, cancel := context.WithTimeout(gtscontext.SetFastFail(ctx), previewCardTimeout)
	defer cancel()

	card, err := c.fetchPreviewCard(fetchCtx, cardURL)
	ttl := previewCardTTL
	if err != nil || card == nil {
		card = nil
		ttl = previewCardMissTTL
	}

	c.previewCards.Store(key, previewCardCacheEntry{
		card:      card,
		expiresAt: time.Now().Add(ttl),
	})
	return card
}

func firstPreviewCardURL(content string) *url.URL {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return nil
	}

	var visit func(*html.Node) *url.URL
	visit = func(node *html.Node) *url.URL {
		if node.Type == html.ElementNode && node.Data == "a" && !hasSocialLinkRel(node) {
			if href := attr(node, "href"); href != "" {
				parsed, err := url.Parse(href)
				if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" {
					return parsed
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if found := visit(child); found != nil {
				return found
			}
		}
		return nil
	}
	return visit(doc)
}

func hasSocialLinkRel(node *html.Node) bool {
	rels := strings.Fields(attr(node, "rel"))
	for _, rel := range rels {
		if rel == "tag" {
			return true
		}
	}
	class := strings.Fields(attr(node, "class"))
	for _, item := range class {
		if item == "mention" || item == "hashtag" {
			return true
		}
	}
	return false
}

func (c *Converter) fetchPreviewCard(ctx context.Context, target *url.URL) (*model.Card, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9")
	req.Header.Set("Accept-Charset", "utf-8")

	rsp, err := c.state.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()

	if rsp.StatusCode < 200 || rsp.StatusCode >= 300 {
		return nil, fmt.Errorf("preview response status %s", rsp.Status)
	}
	contentType := rsp.Header.Get("Content-Type")
	if contentType != "" && !strings.Contains(strings.ToLower(contentType), "html") {
		return nil, fmt.Errorf("preview response is not HTML: %s", contentType)
	}
	if rsp.ContentLength > previewCardMaxHTML {
		return nil, fmt.Errorf("preview response exceeds %d bytes", previewCardMaxHTML)
	}

	limited := io.LimitReader(rsp.Body, previewCardMaxHTML+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(body) > previewCardMaxHTML {
		return nil, fmt.Errorf("preview response exceeds %d bytes", previewCardMaxHTML)
	}

	return parsePreviewCard(target, body)
}

func attr(node *html.Node, key string) string {
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, key) {
			return attribute.Val
		}
	}
	return ""
}
