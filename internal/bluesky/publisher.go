// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtscontext"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"code.superseriousbusiness.org/gotosocial/internal/typeutils"
	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

const (
	maxPostGraphemes = 300
	maxPostBytes     = 3000
	maxImages        = 4
	maxBlobBytes     = 1_000_000
)

type facet struct {
	Index    facetIndex     `json:"index"`
	Features []facetFeature `json:"features"`
}

type facetIndex struct {
	ByteStart int `json:"byteStart"`
	ByteEnd   int `json:"byteEnd"`
}

type facetFeature struct {
	Type string `json:"$type"`
	URI  string `json:"uri"`
}

type createRecordResponse struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}

type uploadBlobResponse struct {
	Blob any `json:"blob"`
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func newATClient(state *state.State, host string) *atclient.APIClient {
	client := atclient.NewAPIClient(host)
	// Route every PDS/AppView request, including redirects, through GTS's
	// size limits and SSRF-protected network client.
	client.Client = protectedHTTPClient(state)
	return client
}

func protectedHTTPClient(state *state.State) *http.Client {
	return &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		// ATProto request bodies are not always rewindable. Disable the GTS
		// client's transparent retry loop and let the durable delivery queue
		// retry by constructing a completely fresh request instead.
		return state.HTTPClient.Do(request.WithContext(gtscontext.SetFastFail(request.Context())))
	})}
}

func QueueStatus(ctx context.Context, state *state.State, status *gtsmodel.Status) (*gtsmodel.BlueskyDelivery, error) {
	if existing, err := state.DB.GetBlueskyDeliveryByStatusID(ctx, status.ID); err == nil {
		return existing, nil
	} else if !errors.Is(err, db.ErrNoEntries) {
		return nil, err
	}
	delivery := &gtsmodel.BlueskyDelivery{
		ID: id.NewULID(), AccountID: status.AccountID, StatusID: status.ID,
		NextAttemptAt: time.Now(),
	}
	if err := state.DB.PutBlueskyDelivery(ctx, delivery); err != nil {
		return nil, err
	}
	return delivery, nil
}

func ProcessDelivery(ctx context.Context, state *state.State, converter *typeutils.Converter, delivery *gtsmodel.BlueskyDelivery) error {
	status, err := state.DB.GetStatusByID(ctx, delivery.StatusID)
	if errors.Is(err, db.ErrNoEntries) {
		if _, mappingErr := state.DB.GetBlueskyPostByStatusID(ctx, delivery.StatusID); mappingErr == nil {
			if deleteErr := DeleteStatus(ctx, state, delivery.StatusID); deleteErr != nil {
				return recordDeliveryFailure(ctx, state, delivery, deleteErr)
			}
			return nil
		}
		return state.DB.DeleteBlueskyDeliveryByStatusID(ctx, delivery.StatusID)
	}
	if err != nil {
		return recordDeliveryFailure(ctx, state, delivery, err)
	}
	defer lockAccount(status.AccountID)()
	if err := state.DB.PopulateStatus(ctx, status); err != nil {
		return recordDeliveryFailure(ctx, state, delivery, err)
	}
	if _, err := state.DB.GetBlueskyConnectionByAccountID(ctx, status.AccountID); errors.Is(err, db.ErrNoEntries) {
		return state.DB.DeleteBlueskyDeliveryByStatusID(ctx, delivery.StatusID)
	} else if err != nil {
		return recordDeliveryFailure(ctx, state, delivery, err)
	}
	if _, interactionErr := state.DB.GetBlueskyInteractionByStatusID(ctx, status.InReplyToID); errors.Is(interactionErr, db.ErrNoEntries) {
		if mapping, mappingErr := state.DB.GetBlueskyPostByStatusID(ctx, status.ID); mappingErr == nil {
			connection, _ := state.DB.GetBlueskyConnectionByAccountID(ctx, status.AccountID)
			if !EligibleForCrosspost(status, connection) {
				if err := DeleteStatus(ctx, state, mapping.StatusID); err != nil {
					return recordDeliveryFailure(ctx, state, delivery, err)
				}
				return nil
			}
		}
	} else if interactionErr != nil {
		return recordDeliveryFailure(ctx, state, delivery, interactionErr)
	}
	apiStatus, err := converter.StatusToAPIStatus(ctx, status, status.Account)
	if err == nil {
		interaction, interactionErr := state.DB.GetBlueskyInteractionByStatusID(ctx, status.InReplyToID)
		switch {
		case interactionErr == nil:
			err = PublishReply(ctx, state, status, apiStatus.Card, interaction)
		case errors.Is(interactionErr, db.ErrNoEntries):
			err = PublishStatus(ctx, state, status, apiStatus.Card)
		default:
			err = interactionErr
		}
	}
	if err != nil {
		return recordDeliveryFailure(ctx, state, delivery, err)
	}
	return state.DB.DeleteBlueskyDeliveryByStatusID(ctx, delivery.StatusID)
}

func recordDeliveryFailure(ctx context.Context, state *state.State, delivery *gtsmodel.BlueskyDelivery, cause error) error {
	delivery.Attempts++
	delay := time.Minute * time.Duration(1<<min(delivery.Attempts-1, 6))
	delivery.NextAttemptAt = time.Now().Add(delay)
	delivery.LastError = truncateUTF8(cause.Error(), 1000, 4000)
	delivery.UpdatedAt = time.Now()
	delivery.ClaimedUntil = time.Time{}
	delivery.DeadLetter = delivery.Attempts >= 10
	if err := state.DB.UpdateBlueskyDelivery(ctx, delivery, "attempts", "next_attempt_at", "last_error", "updated_at", "claimed_until", "dead_letter"); err != nil {
		return fmt.Errorf("record Bluesky delivery failure after %v: %w", cause, err)
	}
	return cause
}

func EligibleForCrosspost(status *gtsmodel.Status, connection *gtsmodel.BlueskyConnection) bool {
	return connection != nil &&
		connection.CrosspostPublic &&
		status.Visibility == gtsmodel.VisibilityPublic &&
		!status.LocalOnly() &&
		status.InReplyToID == "" &&
		status.BoostOfID == "" &&
		status.PollID == "" &&
		len(status.MentionIDs) == 0
}

func PublishStatus(ctx context.Context, state *state.State, status *gtsmodel.Status, card *apimodel.Card) error {
	return publishStatus(ctx, state, status, card, nil)
}

// PublishReply publishes a local-only reply to an imported Bluesky interaction.
func PublishReply(ctx context.Context, state *state.State, status *gtsmodel.Status, card *apimodel.Card, interaction *gtsmodel.BlueskyInteraction) error {
	return publishStatus(ctx, state, status, card, interaction)
}

// DeleteStatus removes the Bluesky counterpart of status, if one exists. It is
// deliberately idempotent so callers can safely invoke it for every local
// status deletion or privacy downgrade.
func DeleteStatus(ctx context.Context, state *state.State, statusID string) error {
	post, err := state.DB.GetBlueskyPostByStatusID(ctx, statusID)
	if errors.Is(err, db.ErrNoEntries) {
		return state.DB.DeleteBlueskyDeliveryByStatusID(ctx, statusID)
	}
	if err != nil {
		return err
	}
	connection, err := state.DB.GetBlueskyConnectionByAccountID(ctx, post.AccountID)
	if errors.Is(err, db.ErrNoEntries) {
		return state.DB.DeleteBlueskyPost(ctx, post.ID)
	}
	if err != nil {
		return err
	}
	client, err := authenticatedClient(ctx, state, connection)
	if err != nil {
		return err
	}
	rkey := post.URI[strings.LastIndex(post.URI, "/")+1:]
	if err := deleteATRecord(ctx, client, connection.DID, "app.bsky.feed.threadgate", rkey, true); err != nil {
		return fmt.Errorf("delete Bluesky threadgate: %w", err)
	}
	if err := deleteATRecord(ctx, client, connection.DID, "app.bsky.feed.post", rkey, false); err != nil {
		return fmt.Errorf("delete Bluesky post: %w", err)
	}
	if err := state.DB.DeleteBlueskyPost(ctx, post.ID); err != nil {
		return err
	}
	return state.DB.DeleteBlueskyDeliveryByStatusID(ctx, statusID)
}

func publishStatus(ctx context.Context, state *state.State, status *gtsmodel.Status, card *apimodel.Card, interaction *gtsmodel.BlueskyInteraction) error {
	connection, err := state.DB.GetBlueskyConnectionByAccountID(ctx, status.AccountID)
	if err != nil {
		return err
	}
	if interaction == nil && !EligibleForCrosspost(status, connection) {
		return nil
	}
	if interaction != nil && interaction.AccountID != status.AccountID {
		return fmt.Errorf("Bluesky interaction belongs to a different account")
	}
	existingPost, existingErr := state.DB.GetBlueskyPostByStatusID(ctx, status.ID)
	if existingErr != nil && !errors.Is(existingErr, db.ErrNoEntries) {
		return existingErr
	}
	if err := state.DB.PopulateStatus(ctx, status); err != nil {
		return fmt.Errorf("populate status for Bluesky: %w", err)
	}

	app, _, err := NewOAuthClient(state, status.AccountID)
	if err != nil {
		return err
	}
	did, err := syntax.ParseDID(connection.DID)
	if err != nil {
		return err
	}
	session, err := app.ResumeSession(ctx, did, connection.OAuthSessionID)
	if err != nil {
		return fmt.Errorf("resume Bluesky OAuth session: %w", err)
	}
	client := newATClient(state, connection.PDSURL)
	client.Auth = session
	client.AccountDID = &did
	client.Headers.Set("User-Agent", "GoToSocial Plus")

	postText, facets := blueskyTextForStatus(status, interaction != nil)
	record := map[string]any{
		"$type":     "app.bsky.feed.post",
		"text":      postText,
		"createdAt": status.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if len(facets) != 0 {
		record["facets"] = facets
	}
	if status.Language != "" {
		record["langs"] = []string{status.Language}
	}
	if interaction != nil {
		record["reply"] = map[string]any{
			"root":   map[string]string{"uri": interaction.RootURI, "cid": interaction.RootCID},
			"parent": map[string]string{"uri": interaction.URI, "cid": interaction.CID},
		}
	}

	images := make([]map[string]any, 0, maxImages)
	for _, attachment := range status.Attachments {
		if len(images) == maxImages || attachment.Type != gtsmodel.FileTypeImage {
			continue
		}
		blob, err := uploadStoredBlob(ctx, state, client, attachment.File.Path, attachment.File.ContentType)
		if err != nil {
			return fmt.Errorf("upload Bluesky image: %w", err)
		}
		images = append(images, map[string]any{
			"image": blob,
			"alt":   truncateUTF8(attachment.Description, 1000, 10_000),
		})
	}
	if len(images) != 0 {
		record["embed"] = map[string]any{"$type": "app.bsky.embed.images", "images": images}
	} else if card != nil && card.URL != "" {
		external := map[string]any{
			"uri":         card.URL,
			"title":       truncateUTF8(card.Title, 300, 3000),
			"description": truncateUTF8(card.Description, 300, 3000),
		}
		if card.Image != "" {
			if blob, err := uploadRemoteImage(ctx, state, client, card.Image); err == nil {
				external["thumb"] = blob
			}
		}
		record["embed"] = map[string]any{"$type": "app.bsky.embed.external", "external": external}
	}

	// A deterministic record key makes retries idempotent even if a network
	// failure happens after Bluesky accepted the write but before we saw it.
	endpoint, _ := syntax.ParseNSID("com.atproto.repo.putRecord")
	var response createRecordResponse
	if err := client.Post(ctx, endpoint, map[string]any{
		"repo":       connection.DID,
		"collection": "app.bsky.feed.post",
		"rkey":       status.ID,
		"record":     record,
	}, &response); err != nil {
		return fmt.Errorf("create Bluesky post: %w", err)
	}
	rkey := response.URI[strings.LastIndex(response.URI, "/")+1:]
	if err := syncThreadgate(ctx, client, connection.DID, status, response.URI, existingPost != nil); err != nil {
		return err
	}
	if existingPost != nil {
		existingPost.URI = response.URI
		existingPost.CID = response.CID
		existingPost.URL = "https://bsky.app/profile/" + connection.DID + "/post/" + rkey
		return state.DB.UpdateBlueskyPost(ctx, existingPost, "uri", "cid", "url")
	}
	return state.DB.PutBlueskyPost(ctx, &gtsmodel.BlueskyPost{
		ID: id.NewULID(), ConnectionID: connection.ID, AccountID: status.AccountID,
		StatusID: status.ID, URI: response.URI, CID: response.CID,
		URL: "https://bsky.app/profile/" + connection.DID + "/post/" + rkey,
	})
}

func syncThreadgate(ctx context.Context, client *atclient.APIClient, repo string, status *gtsmodel.Status, postURI string, updating bool) error {
	publicReplies := status.InteractionPolicy == nil || policyAllowsPublic(status.InteractionPolicy.CanReply)
	if publicReplies {
		if !updating {
			return nil
		}
		if err := deleteATRecord(ctx, client, repo, "app.bsky.feed.threadgate", status.ID, true); err != nil {
			return fmt.Errorf("remove Bluesky reply restriction: %w", err)
		}
		return nil
	}
	// Mastodon manual approval and follower collections have no equivalent in
	// Bluesky. An empty allow list is the conservative mapping: it never opens
	// replies more widely than the source status intended.
	endpoint, _ := syntax.ParseNSID("com.atproto.repo.putRecord")
	if err := client.Post(ctx, endpoint, map[string]any{
		"repo": repo, "collection": "app.bsky.feed.threadgate", "rkey": status.ID,
		"record": map[string]any{
			"$type": "app.bsky.feed.threadgate", "post": postURI,
			"createdAt": status.CreatedAt.UTC().Format(time.RFC3339Nano), "allow": []any{},
		},
	}, nil); err != nil {
		return fmt.Errorf("apply Bluesky reply restriction: %w", err)
	}
	return nil
}

func deleteATRecord(ctx context.Context, client *atclient.APIClient, repo, collection, rkey string, ignoreMissing bool) error {
	endpoint, _ := syntax.ParseNSID("com.atproto.repo.deleteRecord")
	err := client.Post(ctx, endpoint, map[string]any{
		"repo": repo, "collection": collection, "rkey": rkey,
	}, nil)
	if ignoreMissing && err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "recordnotfound") || strings.Contains(message, "record not found") {
			return nil
		}
	}
	return err
}

func policyAllowsPublic(rules *gtsmodel.PolicyRules) bool {
	if rules == nil {
		return false
	}
	for _, value := range rules.AutomaticApproval {
		if value == gtsmodel.PolicyValuePublic {
			return true
		}
	}
	return false
}
