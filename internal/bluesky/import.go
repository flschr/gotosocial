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
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"code.superseriousbusiness.org/gopkg/log"
	"code.superseriousbusiness.org/gotosocial/internal/ap"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	gtsmedia "code.superseriousbusiness.org/gotosocial/internal/media"
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
	if skip, err := classifyParentLookup(notification.Reason, err); err != nil {
		return err
	} else if skip {
		return nil // Unrelated reply in the account's notification stream.
	}

	target, err := state.DB.GetAccountByID(ctx, connection.AccountID)
	if err != nil {
		return err
	}
	origin, err := state.DB.GetInstanceAccount(ctx, "")
	if err != nil {
		return err
	}
	// AppView's indexedAt is server-observed. Prefer it over the author-supplied
	// record timestamp so a malformed remote post cannot distort local ordering.
	createdAt := interactionCreatedAt(notification.IndexedAt, record.CreatedAt)
	statusID := id.NewULIDFromTime(createdAt)
	interactionID := id.NewULID()
	authorAccountID := id.ULIDFromString("bluesky-author", notification.Author.DID)
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
		BlueskyInteractionID: interactionID,
	}
	mention := &gtsmodel.Mention{
		ID: mentionID, StatusID: statusID, Status: status, OriginAccountID: origin.ID, OriginAccountURI: origin.URI, OriginAccount: origin,
		TargetAccountID: target.ID, TargetAccount: target, TargetAccountURI: target.URI, TargetAccountURL: target.URL, IsNew: true,
	}
	status.Mentions = []*gtsmodel.Mention{mention}
	interaction := &gtsmodel.BlueskyInteraction{
		ID: interactionID, AccountID: connection.AccountID, StatusID: statusID,
		URI: notification.URI, CID: notification.CID, RootURI: root.URI, RootCID: root.CID,
		ParentURI: parent.URI, ParentCID: parent.CID, AuthorDID: notification.Author.DID,
		AuthorAccountID: authorAccountID, AuthorHandle: notification.Author.Handle, AuthorDisplayName: notification.Author.DisplayName,
		AuthorAvatar: notification.Author.Avatar, URL: postURL,
	}
	if err := cacheBlueskyAuthorAvatar(ctx, state, origin.ID, interaction); err != nil {
		log.Warnf(ctx, "error caching Bluesky author avatar for %s: %v", interaction.AuthorDID, err)
	}
	if err := state.DB.PutBlueskyInteractionStatus(ctx, status, mention, interaction); err != nil {
		return err
	}
	if err := attachBlueskyMedia(ctx, state, status, record, notification.Author.DID); err != nil {
		// The reply itself is already safely stored. Deliver it without media and
		// let reconciliation retry the attachment instead of losing the notice.
		log.Errorf(ctx, "error attaching Bluesky media to interaction %s: %v", interaction.ID, err)
	}
	state.Workers.Client.Queue.Push(&messages.FromClientAPI{
		APObjectType: ap.ObjectNote, APActivityType: ap.ActivityCreate, GTSModel: status, Origin: origin, Target: target,
	})
	return nil
}

func cacheBlueskyAuthorAvatar(
	ctx context.Context,
	state *state.State,
	ownerAccountID string,
	interaction *gtsmodel.BlueskyInteraction,
) error {
	if interaction.AuthorAvatar == "" {
		interaction.AuthorAvatarURL = ""
		interaction.AuthorAvatarStaticURL = ""
		return nil
	}
	if existing, err := state.DB.GetBlueskyInteractionByAuthorAccountID(ctx, interaction.AuthorAccountID, interaction.AccountID); err == nil &&
		existing.AuthorAvatar == interaction.AuthorAvatar &&
		existing.AuthorAvatarURL != "" {
		cached, err := blueskyAuthorAvatarCached(ctx, state, existing.AuthorAvatarURL)
		if err != nil {
			return err
		}
		if cached {
			interaction.AuthorAvatarURL = existing.AuthorAvatarURL
			interaction.AuthorAvatarStaticURL = existing.AuthorAvatarStaticURL
			return nil
		}
	} else if err != nil && !errors.Is(err, db.ErrNoEntries) {
		return err
	}

	remoteURL := interaction.AuthorAvatar
	isAvatar := true
	manager := gtsmedia.NewManager(state)
	processing, err := manager.CreateMedia(ctx, ownerAccountID, func(ctx context.Context) (io.ReadCloser, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL, nil)
		if err != nil {
			return nil, err
		}
		response, err := state.HTTPClient.Do(request)
		if err != nil {
			return nil, err
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			response.Body.Close()
			return nil, fmt.Errorf("Bluesky avatar returned %s", response.Status)
		}
		return response.Body, nil
	}, gtsmedia.AdditionalMediaInfo{RemoteURL: &remoteURL, Avatar: &isAvatar})
	if err != nil {
		return err
	}
	attachment, err := processing.Load(ctx)
	if err != nil {
		return err
	}
	if attachment.URL == "" || attachment.Error != 0 {
		return fmt.Errorf("Bluesky avatar could not be processed")
	}
	interaction.AuthorAvatarURL = attachment.URL
	interaction.AuthorAvatarStaticURL = attachment.Thumbnail.URL
	if interaction.AuthorAvatarStaticURL == "" {
		interaction.AuthorAvatarStaticURL = attachment.URL
	}
	return nil
}

func blueskyAuthorAvatarCached(ctx context.Context, state *state.State, avatarURL string) (bool, error) {
	attachment, err := state.DB.GetAttachmentByURL(ctx, avatarURL)
	if errors.Is(err, db.ErrNoEntries) {
		return false, nil
	} else if err != nil {
		return false, err
	} else if !attachment.Cached() {
		return false, nil
	}

	hasOriginal, err := state.Storage.Has(ctx, attachment.File.Path)
	if err != nil || !hasOriginal {
		return false, err
	}
	if !attachment.HasThumbnail() {
		return true, nil
	}
	return state.Storage.Has(ctx, attachment.Thumbnail.Path)
}

type interactionMedia struct {
	URL         string
	Description string
	BlobCID     string
}

func attachBlueskyMedia(
	ctx context.Context,
	state *state.State,
	status *gtsmodel.Status,
	record blueskyPostRecord,
	authorDID string,
) error {
	mediaItems := interactionMediaItems(record, authorDID)
	if len(mediaItems) == 0 {
		return nil
	}
	if len(mediaItems) > 4 {
		mediaItems = mediaItems[:4]
	}

	manager := gtsmedia.NewManager(state)
	for _, item := range mediaItems {
		item := item
		if item.URL == "" && item.BlobCID != "" {
			pds, err := resolveBlueskyPDS(ctx, state, authorDID)
			if err != nil {
				return err
			}
			item.URL = strings.TrimRight(pds, "/") + "/xrpc/com.atproto.sync.getBlob?did=" + url.QueryEscape(authorDID) + "&cid=" + url.QueryEscape(item.BlobCID)
		}
		processing, err := manager.CreateMedia(ctx, status.AccountID, func(ctx context.Context) (io.ReadCloser, error) {
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, item.URL, nil)
			if err != nil {
				return nil, err
			}
			response, err := state.HTTPClient.Do(request)
			if err != nil {
				return nil, err
			}
			if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
				response.Body.Close()
				return nil, fmt.Errorf("Bluesky media returned %s", response.Status)
			}
			return response.Body, nil
		}, gtsmedia.AdditionalMediaInfo{
			StatusID:    &status.ID,
			RemoteURL:   &item.URL,
			Description: &item.Description,
		})
		if err != nil {
			return err
		}
		attachment, err := processing.Load(ctx)
		if err != nil {
			return err
		}
		if attachment.URL == "" || attachment.Error != 0 {
			continue
		}
		status.AttachmentIDs = append(status.AttachmentIDs, attachment.ID)
		status.Attachments = append(status.Attachments, attachment)
	}
	if len(status.AttachmentIDs) == 0 {
		return nil
	}
	return state.DB.UpdateStatus(ctx, status, "attachments")
}

func interactionMediaItems(record blueskyPostRecord, authorDID string) []interactionMedia {
	if record.Embed == nil {
		return nil
	}
	embed := record.Embed
	if embed.Media != nil {
		embed = embed.Media
	}
	items := make([]interactionMedia, 0, len(embed.Images)+1)
	for _, image := range embed.Images {
		if image.Image.Ref.Link == "" {
			continue
		}
		items = append(items, interactionMedia{
			URL:         "https://cdn.bsky.app/img/feed_fullsize/plain/" + authorDID + "/" + image.Image.Ref.Link + "@jpeg",
			Description: image.Alt,
		})
	}
	if external := embed.External; external != nil && isDirectMediaURL(external.URI) {
		description := strings.TrimSpace(strings.TrimPrefix(external.Description, "ALT:"))
		if description == "" {
			description = strings.TrimSpace(external.Title)
		}
		items = append(items, interactionMedia{URL: external.URI, Description: description})
	}
	if embed.Video != nil && embed.Video.Ref.Link != "" {
		items = append(items, interactionMedia{BlobCID: embed.Video.Ref.Link, Description: embed.Alt})
	}
	return items
}

func resolveBlueskyPDS(ctx context.Context, state *state.State, did string) (string, error) {
	var documentURL string
	switch {
	case strings.HasPrefix(did, "did:plc:"):
		documentURL = "https://plc.directory/" + url.PathEscape(did)
	case strings.HasPrefix(did, "did:web:"):
		parts := strings.Split(strings.TrimPrefix(did, "did:web:"), ":")
		host, err := url.PathUnescape(parts[0])
		if err != nil || host == "" {
			return "", fmt.Errorf("invalid did:web identifier")
		}
		if len(parts) == 1 {
			documentURL = "https://" + host + "/.well-known/did.json"
		} else {
			segments := make([]string, 0, len(parts)-1)
			for _, part := range parts[1:] {
				segment, err := url.PathUnescape(part)
				if err != nil || segment == "" || strings.Contains(segment, "/") {
					return "", fmt.Errorf("invalid did:web identifier")
				}
				segments = append(segments, url.PathEscape(segment))
			}
			documentURL = "https://" + host + "/" + strings.Join(segments, "/") + "/did.json"
		}
	default:
		return "", fmt.Errorf("unsupported Bluesky DID method")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, documentURL, nil)
	if err != nil {
		return "", err
	}
	response, err := state.HTTPClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("Bluesky DID document returned %s", response.Status)
	}
	var document struct {
		Service []struct {
			ID              string `json:"id"`
			Type            string `json:"type"`
			ServiceEndpoint string `json:"serviceEndpoint"`
		} `json:"service"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&document); err != nil {
		return "", err
	}
	for _, service := range document.Service {
		if (service.ID == "#atproto_pds" || service.Type == "AtprotoPersonalDataServer") && strings.HasPrefix(service.ServiceEndpoint, "https://") {
			return service.ServiceEndpoint, nil
		}
	}
	return "", fmt.Errorf("Bluesky DID document has no PDS endpoint")
}

func isDirectMediaURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return false
	}
	switch strings.ToLower(path.Ext(parsed.Path)) {
	case ".gif", ".mp4", ".webm", ".mov", ".m4v":
		return true
	default:
		return false
	}
}

func classifyParentLookup(reason string, err error) (bool, error) {
	if err == nil {
		return false, nil
	}
	if errors.Is(err, db.ErrNoEntries) {
		return reason == "reply", nil
	}
	return false, err
}

func interactionCreatedAt(indexedAt, recordCreatedAt time.Time) time.Time {
	if !indexedAt.IsZero() {
		return indexedAt
	}
	if !recordCreatedAt.IsZero() {
		return recordCreatedAt
	}
	return time.Now()
}

func renderInteractionContent(record blueskyPostRecord, author blueskyAuthor, postURL string) string {
	body := renderBlueskyRecord(record, author.DID)
	if body == "" {
		return fmt.Sprintf(`<p><a href="%s">View reply on Bluesky</a></p>`, stdhtml.EscapeString(postURL))
	}
	return fmt.Sprintf(`<p>%s</p><p><a href="%s">View reply on Bluesky</a></p>`,
		body, stdhtml.EscapeString(postURL))
}

func renderBlueskyRecord(record blueskyPostRecord, _ string) string {
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
	if record.Embed != nil && record.Embed.External != nil {
		external := record.Embed.External
		if strings.HasPrefix(external.URI, "https://") || strings.HasPrefix(external.URI, "http://") {
			if out.Len() > 0 {
				out.WriteString("<br>")
			}
			label := strings.TrimSpace(external.Title)
			if label == "" {
				label = strings.TrimSpace(strings.TrimPrefix(external.Description, "ALT:"))
			}
			if label == "" {
				label = "Open embedded media"
			}
			out.WriteString(`<a href="` + stdhtml.EscapeString(external.URI) + `">` + stdhtml.EscapeString(label) + `</a>`)
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
