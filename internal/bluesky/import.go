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

	"code.superseriousbusiness.org/gotosocial/internal/ap"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/messages"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"code.superseriousbusiness.org/gotosocial/internal/uris"
)

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
