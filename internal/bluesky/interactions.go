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
	Reply     *struct {
		Root   blueskyStrongRef `json:"root"`
		Parent blueskyStrongRef `json:"parent"`
	} `json:"reply"`
}

type blueskyStrongRef struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}

// SyncInteractions imports replies and mentions as private, local-only statuses.
func SyncInteractions(ctx context.Context, state *state.State) error {
	connections, err := state.DB.GetBlueskyConnections(ctx)
	if err != nil {
		return err
	}
	var syncErrors []error
	for _, connection := range connections {
		if err := syncConnection(ctx, state, connection); err != nil {
			syncErrors = append(syncErrors, fmt.Errorf("%s: %w", connection.Handle, err))
		}
	}
	return errors.Join(syncErrors...)
}

func syncConnection(ctx context.Context, state *state.State, connection *gtsmodel.BlueskyConnection) error {
	defer lockAccount(connection.AccountID)()
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
	return processNotificationInbox(ctx, state, connection)
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
	content := fmt.Sprintf(`<p><strong><a href="%s">@%s on Bluesky</a></strong></p><p>%s</p><p><a href="%s">View on Bluesky</a></p>`,
		stdhtml.EscapeString("https://bsky.app/profile/"+notification.Author.Handle), stdhtml.EscapeString(notification.Author.Handle),
		strings.ReplaceAll(stdhtml.EscapeString(record.Text), "\n", "<br>"), stdhtml.EscapeString(postURL))
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
