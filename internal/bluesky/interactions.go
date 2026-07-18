// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	stdhtml "html"
	"strings"
	"sync"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/ap"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/messages"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"code.superseriousbusiness.org/gotosocial/internal/uris"
	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

type notificationPage struct {
	Cursor        string                `json:"cursor"`
	Notifications []blueskyNotification `json:"notifications"`
}

type blueskyNotification struct {
	URI       string          `json:"uri"`
	CID       string          `json:"cid"`
	Reason    string          `json:"reason"`
	IndexedAt time.Time       `json:"indexedAt"`
	Author    blueskyAuthor   `json:"author"`
	Record    json.RawMessage `json:"record"`
}

type blueskyAuthor struct {
	DID         string `json:"did"`
	Handle      string `json:"handle"`
	DisplayName string `json:"displayName"`
}

type blueskyPostRecord struct {
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
	Facets    []struct {
		Index    facetIndex `json:"index"`
		Features []struct {
			Type string `json:"$type"`
			URI  string `json:"uri"`
			DID  string `json:"did"`
			Tag  string `json:"tag"`
		} `json:"features"`
	} `json:"facets"`
	Embed *struct {
		Images []struct {
			Alt   string `json:"alt"`
			Image struct {
				Ref struct {
					Link string `json:"$link"`
				} `json:"ref"`
			} `json:"image"`
		} `json:"images"`
	} `json:"embed"`
	Reply *struct {
		Root   blueskyStrongRef `json:"root"`
		Parent blueskyStrongRef `json:"parent"`
	} `json:"reply"`
}

type blueskyStrongRef struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}

type blueskyPostsResponse struct {
	Posts []struct {
		URI    string          `json:"uri"`
		CID    string          `json:"cid"`
		Author blueskyAuthor   `json:"author"`
		Record json.RawMessage `json:"record"`
	} `json:"posts"`
}

// SyncInteractions imports replies and mentions as private, local-only statuses.
func SyncInteractions(ctx context.Context, state *state.State) error {
	connections, err := state.DB.GetBlueskyConnections(ctx)
	if err != nil {
		return err
	}
	errChannel := make(chan error, len(connections))
	semaphore := make(chan struct{}, 4)
	var wait sync.WaitGroup
	for _, connection := range connections {
		wait.Add(1)
		go func(connection *gtsmodel.BlueskyConnection) {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				errChannel <- ctx.Err()
				return
			}
			if err := syncConnection(ctx, state, connection); err != nil {
				errChannel <- fmt.Errorf("%s: %w", connection.Handle, err)
			}
		}(connection)
	}
	wait.Wait()
	close(errChannel)
	var syncErrors []error
	for err := range errChannel {
		syncErrors = append(syncErrors, err)
	}
	return errors.Join(syncErrors...)
}

func syncConnection(ctx context.Context, state *state.State, connection *gtsmodel.BlueskyConnection) (syncErr error) {
	defer lockAccount(connection.AccountID)()
	defer func() {
		connection.LastSyncAt = time.Now()
		if syncErr != nil {
			connection.LastSyncError = truncateUTF8(syncErr.Error(), 1000, 4000)
		} else {
			connection.LastSyncError = ""
		}
		if err := state.DB.UpdateBlueskyConnection(ctx, connection, "last_sync_at", "last_sync_error"); err != nil && syncErr == nil {
			syncErr = err
		}
	}()
	if connection.LastSyncAt.IsZero() || time.Since(connection.LastSyncAt) >= time.Hour {
		if app, _, err := NewOAuthClient(state, connection.AccountID); err == nil {
			if did, err := syntax.ParseDID(connection.DID); err == nil {
				if identity, err := app.Dir.LookupDID(ctx, did); err == nil && identity.Handle.String() != connection.Handle {
					connection.Handle = identity.Handle.String()
					if err := state.DB.UpdateBlueskyConnection(ctx, connection, "handle"); err != nil {
						return err
					}
				}
			}
		}
	}
	client, err := authenticatedClient(ctx, state, connection)
	if err != nil {
		return err
	}
	endpoint, _ := syntax.ParseNSID("app.bsky.notification.listNotifications")
	client = client.WithService("did:web:api.bsky.app#bsky_appview")

	latest := connection.NotificationsSeenAt
	cursor := ""
	seenCursors := make(map[string]struct{})
	for {
		params := map[string]any{"limit": 100}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var page notificationPage
		if err := client.Get(ctx, endpoint, params, &page); err != nil {
			return fmt.Errorf("list notifications: %w", err)
		}
		stop := false
		for _, notification := range page.Notifications {
			// Include equal timestamps again: notification timestamps are not
			// unique, while URI de-duplication makes a small overlap harmless.
			if !connection.NotificationsSeenAt.IsZero() && notification.IndexedAt.Before(connection.NotificationsSeenAt) {
				stop = true
				break
			}
			if notification.IndexedAt.After(latest) {
				latest = notification.IndexedAt
			}
			if notification.Reason == "reply" || notification.Reason == "mention" {
				payload, marshalErr := json.Marshal(notification)
				if marshalErr != nil {
					return fmt.Errorf("encode notification inbox item: %w", marshalErr)
				}
				if err := state.DB.PutBlueskyNotification(ctx, &gtsmodel.BlueskyNotification{
					ID: id.NewULID(), CreatedAt: notification.IndexedAt, AccountID: connection.AccountID,
					URI: notification.URI, Payload: payload, NextAttemptAt: time.Now(),
				}); err != nil {
					return fmt.Errorf("store notification inbox item: %w", err)
				}
			}
		}
		if stop || page.Cursor == "" {
			break
		}
		if _, duplicate := seenCursors[page.Cursor]; duplicate {
			return fmt.Errorf("notification pagination returned repeated cursor")
		}
		seenCursors[page.Cursor] = struct{}{}
		cursor = page.Cursor
	}

	if latest.After(connection.NotificationsSeenAt) {
		connection.NotificationsSeenAt = latest
		if err := state.DB.UpdateBlueskyConnection(ctx, connection, "notifications_seen_at"); err != nil {
			return err
		}
	}
	return errors.Join(
		processNotificationInbox(ctx, state, connection),
		reconcileInteractions(ctx, state, connection, client),
	)
}

func reconcileInteractions(ctx context.Context, state *state.State, connection *gtsmodel.BlueskyConnection, client *atclient.APIClient) error {
	interactions, err := state.DB.GetBlueskyInteractionsForReconcile(ctx, connection.AccountID, 100)
	if err != nil || len(interactions) == 0 {
		return err
	}
	endpoint, _ := syntax.ParseNSID("app.bsky.feed.getPosts")
	client = client.WithService("did:web:api.bsky.app#bsky_appview")
	var reconcileErrors []error
	for start := 0; start < len(interactions); start += 25 {
		end := min(start+25, len(interactions))
		batch := interactions[start:end]
		uris := make([]string, 0, len(batch))
		for _, interaction := range batch {
			uris = append(uris, interaction.URI)
		}
		var response blueskyPostsResponse
		if err := client.Get(ctx, endpoint, map[string]any{"uris": uris}, &response); err != nil {
			return errors.Join(append(reconcileErrors, fmt.Errorf("reconcile Bluesky interactions: %w", err))...)
		}
		posts := make(map[string]struct {
			CID    string
			Author blueskyAuthor
			Record json.RawMessage
		}, len(response.Posts))
		for _, post := range response.Posts {
			posts[post.URI] = struct {
				CID    string
				Author blueskyAuthor
				Record json.RawMessage
			}{post.CID, post.Author, post.Record}
		}
		for _, interaction := range batch {
			post, exists := posts[interaction.URI]
			if !exists {
				status, statusErr := state.DB.GetStatusByID(ctx, interaction.StatusID)
				if statusErr == nil {
					statusErr = state.DB.PopulateStatus(ctx, status)
					if statusErr == nil {
						target, targetErr := state.DB.GetAccountByID(ctx, connection.AccountID)
						if targetErr != nil {
							statusErr = targetErr
						} else {
							state.Workers.Client.Queue.Push(&messages.FromClientAPI{
								APObjectType: ap.ObjectNote, APActivityType: ap.ActivityDelete,
								GTSModel: status, Origin: status.Account, Target: target,
							})
						}
					}
				}
				if statusErr == nil || errors.Is(statusErr, db.ErrNoEntries) {
					statusErr = state.DB.DeleteBlueskyInteraction(ctx, interaction.ID)
				}
				if statusErr != nil {
					reconcileErrors = append(reconcileErrors, statusErr)
				}
				continue
			}
			if post.CID != interaction.CID {
				var record blueskyPostRecord
				if err := json.Unmarshal(post.Record, &record); err != nil {
					reconcileErrors = append(reconcileErrors, err)
					continue
				}
				status, err := state.DB.GetStatusByID(ctx, interaction.StatusID)
				if err != nil {
					reconcileErrors = append(reconcileErrors, err)
					continue
				}
				status.Text = record.Text
				status.Content = renderInteractionContent(record, post.Author, interaction.URL)
				if err := state.DB.UpdateStatus(ctx, status, "text", "content"); err != nil {
					reconcileErrors = append(reconcileErrors, err)
					continue
				}
				if err := state.DB.PopulateStatus(ctx, status); err == nil {
					if target, err := state.DB.GetAccountByID(ctx, connection.AccountID); err == nil {
						state.Workers.Client.Queue.Push(&messages.FromClientAPI{
							APObjectType: ap.ObjectNote, APActivityType: ap.ActivityUpdate,
							GTSModel: status, Origin: status.Account, Target: target,
						})
					}
				}
				interaction.CID = post.CID
				interaction.AuthorHandle = post.Author.Handle
			}
			interaction.LastCheckedAt = time.Now()
			if err := state.DB.UpdateBlueskyInteraction(ctx, interaction, "cid", "author_handle", "last_checked_at"); err != nil {
				reconcileErrors = append(reconcileErrors, err)
			}
		}
	}
	return errors.Join(reconcileErrors...)
}

func processNotificationInbox(ctx context.Context, state *state.State, connection *gtsmodel.BlueskyConnection) error {
	var processingErrors []error
	for {
		items, err := state.DB.GetDueBlueskyNotifications(ctx, connection.AccountID, time.Now(), 100)
		if err != nil {
			return errors.Join(append(processingErrors, err)...)
		}
		if len(items) == 0 {
			return errors.Join(processingErrors...)
		}
		for _, item := range items {
			var notification blueskyNotification
			err := json.Unmarshal(item.Payload, &notification)
			if err == nil {
				err = importNotification(ctx, state, connection, notification)
			}
			if err == nil {
				if deleteErr := state.DB.DeleteBlueskyNotification(ctx, item.ID); deleteErr != nil {
					processingErrors = append(processingErrors, deleteErr)
				}
				continue
			}
			item.Attempts++
			item.LastError = truncateUTF8(err.Error(), 1000, 4000)
			item.DeadLetter = item.Attempts >= 10
			item.NextAttemptAt = time.Now().Add(time.Minute * time.Duration(1<<min(item.Attempts-1, 6)))
			if updateErr := state.DB.UpdateBlueskyNotification(ctx, item, "attempts", "last_error", "dead_letter", "next_attempt_at"); updateErr != nil {
				processingErrors = append(processingErrors, updateErr)
			} else {
				processingErrors = append(processingErrors, fmt.Errorf("notification %s: %w", item.URI, err))
			}
		}
		if len(items) < 100 {
			return errors.Join(processingErrors...)
		}
	}
}

func authenticatedClient(ctx context.Context, state *state.State, connection *gtsmodel.BlueskyConnection) (*atclient.APIClient, error) {
	app, _, err := NewOAuthClient(state, connection.AccountID)
	if err != nil {
		return nil, err
	}
	did, err := syntax.ParseDID(connection.DID)
	if err != nil {
		return nil, err
	}
	session, err := app.ResumeSession(ctx, did, connection.OAuthSessionID)
	if err != nil {
		return nil, fmt.Errorf("resume Bluesky OAuth session: %w", err)
	}
	client := newATClient(state, connection.PDSURL)
	client.Auth = session
	client.AccountDID = &did
	client.Headers.Set("User-Agent", "GoToSocial Plus")
	return client, nil
}

func importNotification(ctx context.Context, state *state.State, connection *gtsmodel.BlueskyConnection, notification blueskyNotification) error {
	if _, err := state.DB.GetBlueskyInteractionByURI(ctx, notification.URI); err == nil {
		return nil
	} else if !errors.Is(err, db.ErrNoEntries) {
		return err
	}
	var record blueskyPostRecord
	if err := json.Unmarshal(notification.Record, &record); err != nil {
		return fmt.Errorf("decode notification record: %w", err)
	}
	root := blueskyStrongRef{URI: notification.URI, CID: notification.CID}
	parent := root
	if record.Reply != nil {
		root, parent = record.Reply.Root, record.Reply.Parent
	}
	parentStatus, err := mappedLocalStatus(ctx, state, connection.AccountID, parent.URI, root.URI)
	if notification.Reason == "reply" && err != nil {
		if errors.Is(err, db.ErrNoEntries) {
			return nil // Unrelated reply in the account's notification stream.
		}
		return err
	}

	target, err := state.DB.GetAccountByID(ctx, connection.AccountID)
	if err != nil {
		return err
	}
	origin, err := state.DB.GetInstanceAccount(ctx, "")
	if err != nil {
		return err
	}
	createdAt := record.CreatedAt
	if createdAt.IsZero() {
		createdAt = notification.IndexedAt
	}
	statusID := id.NewULIDFromTime(createdAt)
	originURIs := uris.GenerateURIsForAccount(origin.Username)
	postURL := blueskyPostURL(notification.Author.Handle, notification.URI)
	content := renderInteractionContent(record, notification.Author, postURL)
	mentionID := id.NewULID()
	status := &gtsmodel.Status{
		ID: statusID, URI: originURIs.StatusesURI + "/" + statusID, URL: originURIs.StatusesURL + "/" + statusID,
		CreatedAt: createdAt, AccountID: origin.ID, AccountURI: origin.URI, Account: origin,
		Content: content, Text: record.Text, ActivityStreamsType: ap.ObjectNote,
		Visibility: gtsmodel.VisibilityDirect, Flags: gtsmodel.StatusFlags(gtsmodel.StatusFlagLocal),
		MentionIDs: []string{mentionID}, InReplyToID: statusIDOf(parentStatus), InReplyToURI: uriOf(parentStatus),
		InReplyToAccountID: connection.AccountID, InReplyTo: parentStatus, InReplyToAccount: target,
	}
	mention := &gtsmodel.Mention{
		ID: mentionID, StatusID: statusID, Status: status, OriginAccountID: origin.ID, OriginAccountURI: origin.URI, OriginAccount: origin,
		TargetAccountID: target.ID, TargetAccount: target, TargetAccountURI: target.URI, TargetAccountURL: target.URL, IsNew: true,
	}
	status.Mentions = []*gtsmodel.Mention{mention}
	interaction := &gtsmodel.BlueskyInteraction{
		ID: id.NewULID(), AccountID: connection.AccountID, StatusID: statusID,
		URI: notification.URI, CID: notification.CID, RootURI: root.URI, RootCID: root.CID,
		ParentURI: parent.URI, ParentCID: parent.CID, AuthorDID: notification.Author.DID,
		AuthorHandle: notification.Author.Handle, URL: postURL,
	}
	if err := state.DB.PutBlueskyInteractionStatus(ctx, status, mention, interaction); err != nil {
		return err
	}
	state.Workers.Client.Queue.Push(&messages.FromClientAPI{
		APObjectType: ap.ObjectNote, APActivityType: ap.ActivityCreate, GTSModel: status, Origin: origin, Target: target,
	})
	return nil
}

func renderInteractionContent(record blueskyPostRecord, author blueskyAuthor, postURL string) string {
	body := renderBlueskyRecord(record, author.DID)
	return fmt.Sprintf(`<p><strong><a href="%s">@%s on Bluesky</a></strong></p><p>%s</p><p><a href="%s">View on Bluesky</a></p>`,
		stdhtml.EscapeString("https://bsky.app/profile/"+author.DID), stdhtml.EscapeString(author.Handle), body, stdhtml.EscapeString(postURL))
}

func renderBlueskyRecord(record blueskyPostRecord, authorDID string) string {
	textBytes := []byte(record.Text)
	var out strings.Builder
	position := 0
	for _, recordFacet := range record.Facets {
		start, end := recordFacet.Index.ByteStart, recordFacet.Index.ByteEnd
		if start < position || end < start || end > len(textBytes) {
			continue
		}
		out.WriteString(strings.ReplaceAll(stdhtml.EscapeString(string(textBytes[position:start])), "\n", "<br>"))
		label := strings.ReplaceAll(stdhtml.EscapeString(string(textBytes[start:end])), "\n", "<br>")
		href := ""
		for _, feature := range recordFacet.Features {
			switch feature.Type {
			case "app.bsky.richtext.facet#link":
				href = feature.URI
			case "app.bsky.richtext.facet#mention":
				href = "https://bsky.app/profile/" + feature.DID
			case "app.bsky.richtext.facet#tag":
				href = "https://bsky.app/hashtag/" + feature.Tag
			}
		}
		if strings.HasPrefix(href, "https://") || strings.HasPrefix(href, "http://") {
			out.WriteString(`<a href="` + stdhtml.EscapeString(href) + `">` + label + `</a>`)
		} else {
			out.WriteString(label)
		}
		position = end
	}
	out.WriteString(strings.ReplaceAll(stdhtml.EscapeString(string(textBytes[position:])), "\n", "<br>"))
	if record.Embed != nil {
		for _, image := range record.Embed.Images {
			if image.Image.Ref.Link == "" {
				continue
			}
			imageURL := "https://cdn.bsky.app/img/feed_fullsize/plain/" + authorDID + "/" + image.Image.Ref.Link + "@jpeg"
			out.WriteString(`<br><a href="` + stdhtml.EscapeString(imageURL) + `">Image: ` + stdhtml.EscapeString(image.Alt) + `</a>`)
		}
	}
	return out.String()
}

func mappedLocalStatus(ctx context.Context, state *state.State, accountID, parentURI, rootURI string) (*gtsmodel.Status, error) {
	for _, uri := range []string{parentURI, rootURI} {
		if post, err := state.DB.GetBlueskyPostByURI(ctx, uri); err == nil {
			if post.AccountID == accountID {
				return state.DB.GetStatusByID(ctx, post.StatusID)
			}
		} else if !errors.Is(err, db.ErrNoEntries) {
			return nil, err
		}
		if interaction, err := state.DB.GetBlueskyInteractionByURI(ctx, uri); err == nil {
			if interaction.AccountID == accountID {
				return state.DB.GetStatusByID(ctx, interaction.StatusID)
			}
		} else if !errors.Is(err, db.ErrNoEntries) {
			return nil, err
		}
	}
	return nil, db.ErrNoEntries
}

func blueskyPostURL(handle, uri string) string {
	rkey := uri[strings.LastIndex(uri, "/")+1:]
	profile := handle
	if strings.HasPrefix(uri, "at://did:") {
		profile = strings.Split(strings.TrimPrefix(uri, "at://"), "/")[0]
	}
	return "https://bsky.app/profile/" + profile + "/post/" + rkey
}

func statusIDOf(status *gtsmodel.Status) string {
	if status == nil {
		return ""
	}
	return status.ID
}
func uriOf(status *gtsmodel.Status) string {
	if status == nil {
		return ""
	}
	return status.URI
}
