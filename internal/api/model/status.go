// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package model

import (
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/language"
)

// Status models a status or post.
//
// swagger:model status
type Status struct {
	// ID of the status.
	// example: 01FBVD42CQ3ZEEVMW180SBX03B
	ID string `json:"id"`
	// The date when this status was created (ISO 8601 Datetime).
	// example: 2021-07-30T09:20:25+00:00
	CreatedAt string `json:"created_at"`
	// Timestamp of when the status was last edited (ISO 8601 Datetime).
	// example: 2021-07-30T09:20:25+00:00
	// nullable: true
	EditedAt *string `json:"edited_at"`
	// ID of the status being replied to.
	// example: 01FBVD42CQ3ZEEVMW180SBX03B
	// nullable: true
	InReplyToID *string `json:"in_reply_to_id"`
	// ID of the account being replied to.
	// example: 01FBVD42CQ3ZEEVMW180SBX03B
	// nullable: true
	InReplyToAccountID *string `json:"in_reply_to_account_id"`
	// Status contains sensitive content.
	// example: false
	Sensitive bool `json:"sensitive"`
	// Subject, summary, or content warning for the status.
	// example: warning nsfw
	SpoilerText string `json:"spoiler_text"`
	// Visibility of this status.
	// example: unlisted
	Visibility Visibility `json:"visibility"`
	// Set to "true" if status is not federated, ie., a "local only" status; omitted from response otherwise.
	LocalOnly bool `json:"local_only,omitempty"`
	// Primary language of this status (ISO 639 Part 1 two-letter language code).
	// Will be null if language is not known.
	// example: en
	Language *string `json:"language"`
	// ActivityPub URI of the status. Equivalent to the status's activitypub ID.
	// example: https://example.org/users/some_user/statuses/01FBVD42CQ3ZEEVMW180SBX03B
	URI string `json:"uri"`
	// The status's publicly available web URL. This link will only work if the visibility of the status is 'public'.
	// example: https://example.org/@some_user/statuses/01FBVD42CQ3ZEEVMW180SBX03B
	URL string `json:"url"`
	// Number of replies to this status, according to our instance.
	RepliesCount int `json:"replies_count"`
	// Number of times this status has been boosted/reblogged, according to our instance.
	ReblogsCount int `json:"reblogs_count"`
	// Number of favourites/likes this status has received, according to our instance.
	FavouritesCount int `json:"favourites_count"`
	// Number of times this status has been quoted, according to our instance.
	// Omitted when zero, so statuses that aren't quoted keep their existing shape.
	QuotesCount int `json:"quotes_count,omitempty"`
	// This status has been favourited by the account viewing it.
	Favourited bool `json:"favourited"`
	// This status has been boosted/reblogged by the account viewing it.
	Reblogged bool `json:"reblogged"`
	// Replies to this status have been muted by the account viewing it.
	Muted bool `json:"muted"`
	// This status has been bookmarked by the account viewing it.
	Bookmarked bool `json:"bookmarked"`
	// This status has been pinned by the account viewing it (only relevant for your own statuses).
	Pinned bool `json:"pinned"`
	// The content of this status. Should be HTML, but might also be plaintext in some cases.
	// example: <p>Hey this is a status!</p>
	Content string `json:"content"`
	// The status that this status reblogs/boosts.
	// nullable: true
	Reblog *StatusReblogged `json:"reblog"`
	// Information about the status being quoted, if any (Mastodon 4.4+ quote posts).
	// Omitted for non-quote statuses; clients treat absent and null identically.
	// nullable: true
	Quote *Quote `json:"quote,omitempty"`
	// The application used to post this status, if visible.
	Application *Application `json:"application,omitempty"`
	// The account that authored this status.
	Account *Account `json:"account"`
	// Media that is attached to this status.
	MediaAttachments []*Attachment `json:"media_attachments"`
	// Mentions of users within the status content.
	Mentions []Mention `json:"mentions"`
	// Hashtags used within the status content.
	Tags []Tag `json:"tags"`
	// Custom emoji to be used when rendering status content.
	Emojis []Emoji `json:"emojis"`
	// Preview card for links included within status content.
	// nullable: true
	Card *Card `json:"card"`
	// The poll attached to the status.
	// nullable: true
	Poll *Poll `json:"poll"`
	// Plain-text source of a status. Returned instead of content when status is deleted,
	// so the user may redraft from the source text without the client having to reverse-engineer
	// the original text from the HTML content.
	Text string `json:"text,omitempty"`
	// Content type that was used to parse the status's text. Returned when
	// status is deleted, so if the user is redrafting the message the client
	// can default to the same content type.
	ContentType StatusContentType `json:"content_type,omitempty"`
	// A list of filters that matched this status and why they matched, if there are any such filters.
	Filtered []FilterResult `json:"filtered,omitempty"`
	// The interaction policy for this status, as set by the status author.
	InteractionPolicy InteractionPolicy `json:"interaction_policy"`
	// Quote approval info: whom this status may be quoted by, and whether the
	// requesting account may quote it (Mastodon 4.5+). Omitted when quoting
	// doesn't apply to this status.
	QuoteApproval *QuoteApproval `json:"quote_approval,omitempty"`
}

// QuoteApproval describes who may quote a status and whether the requesting
// account's quote would be approved (Mastodon 4.5+). Clients use current_user
// to decide whether to offer a "quote" action.
//
// swagger:model quoteApproval
type QuoteApproval struct {
	// Policy values for whom quotes are automatically approved, e.g. ["public"].
	Automatic []string `json:"automatic"`
	// Policy values for whom quotes require manual approval by the author.
	Manual []string `json:"manual"`
	// The requesting account's quote ability for this status.
	// One of: "automatic", "manual", "denied", "unsupported".
	CurrentUser string `json:"current_user"`
}

// Quote models the relationship between a status and a status
// that it quotes (Mastodon 4.4+ quote posts).
//
// See https://docs.joinmastodon.org/entities/Quote/
//
// swagger:model quote
type Quote struct {
	// The state of the quote.
	// One of: "pending", "accepted", "rejected", "revoked", "deleted", "unauthorized".
	// example: accepted
	State string `json:"state"`
	// The quoted status. Only set (non-null) when state is "accepted"
	// and the quote is not nested too deeply to inline.
	// nullable: true
	QuotedStatus *Status `json:"quoted_status"`
	// The ID of the quoted status. Set instead of quoted_status when the
	// quote is nested too deeply to inline the full status (shallow quote).
	// nullable: true
	QuotedStatusID *string `json:"quoted_status_id,omitempty"`
}

// Quote state values. See
// https://docs.joinmastodon.org/entities/Quote/#state
const (
	QuoteStatePending      = "pending"
	QuoteStateAccepted     = "accepted"
	QuoteStateRejected     = "rejected"
	QuoteStateRevoked      = "revoked"
	QuoteStateDeleted      = "deleted"
	QuoteStateUnauthorized = "unauthorized"
)

// StatusVisibilityDebugResponse provides detailed
// visibility information for a requested status.
// The information layout largerly matches how we
// cache and store visibility, mute and filtering
// results internally, and is more for GoToSocial
// developer debug information rather than knowing
// exactly which mention etc was muted or filtered.
//
// swagger:model statusVisibilityDebugResponse
type StatusVisibilityDebugResponse struct {

	// URI is the URI of the status in question.
	URI string `json:"uri"`

	// Mute contains any details about a mute currently
	// acting on status. An empty result indicates
	// no mute (active or not) is matching on status.
	Mute *StatusMuteResult `json:"mute,omitempty"`

	// MuteNotifications contains any details about a
	// notification mute currently acting on status. An empty
	// result indicates no mute (active or not) is matching on status.
	MuteNotifications *StatusMuteResult `json:"mute_notifications,omitempty"`

	// Contains any filters matching on status across any contexts. An
	// empty result indicates no filters in any contexts match on status.
	Filters *StatusFiltersResult `json:"filters,omitempty"`

	// Visibility contains visibility information for status
	// across all contexts we check in, these indicate whether
	// status should be shown in the given context based on
	// core status attributes (e.g. is a boost) or account blocks.
	Visibility StatusVisibilityResult `json:"visibility"`
}

type StatusVisibilityResult struct {
	// General indicates whether status
	// should appear AT ALL to user.
	General bool `json:"general"`

	// Public indicates whether status
	// should appear on public timelines.
	Public bool `json:"public"`

	// Home indicates whether status
	// should appear on home timelines.
	Home bool `json:"home"`

	// Tag indicates whether status
	// should appear on tag timelines.
	Tag bool `json:"tag"`
}

type StatusMuteResult struct {
	// Active indicates whether this
	// mute is active on the status.
	Active bool `json:"active"`

	// Expires indicates mute expiry time string (if any).
	Expires *string `json:"expires,omitempty"`
}

type StatusFiltersResult struct {
	// Public contains filter results active
	// on status in a public timeline context.
	Public []StatusFilterResult `json:"public"`

	// Home contains filter results active
	// on status in a home timeline context.
	Home []StatusFilterResult `json:"home"`

	// Notifications contains filter results
	// active on status in a notification context.
	Notifications []StatusFilterResult `json:"notifications"`

	// Account contains filter results
	// active on status in an account context.
	Account []StatusFilterResult `json:"account"`

	// Thread contains filter results
	// active on status in a thread context.
	Thread []StatusFilterResult `json:"thread"`
}

type StatusFilterResult struct {
	// Result contains a filter result currently
	// active on the status, only present if the
	// applying filter policy is not "HIDE".
	Result *FilterResult `json:"result,omitempty"`

	// Active indicates whether this
	// filter is active on the status.
	Active bool `json:"active"`

	// Expires indicates filter expiry time string (if any).
	Expires *string `json:"expires,omitempty"`
}

// StatusReblogged represents a reblogged status.
//
// swagger:model statusReblogged
type StatusReblogged struct {
	*Status
}

// WebStatus is like *model.Status, but contains
// additional fields used only for HTML templating.
//
// swagger:ignore
type WebStatus struct {
	*Status

	// HTML version of spoiler content
	// (ie., not converted to plaintext).
	SpoilerContent string `json:"-"`

	// Override API account with web account.
	Account *WebAccount `json:"account"`

	// Web version of media
	// attached to this status.
	MediaAttachments []*WebAttachment `json:"media_attachments"`

	// Template-ready language tag and
	// string, based on *status.Language.
	LanguageTag *language.Language

	// Template-ready poll options with vote shares
	// calculated as a percentage of total votes.
	PollOptions []WebPollOption

	// Status is from a local account.
	Local bool

	// Level of indentation at which to
	// display this status in the web view.
	Indent int

	// This status is the last visible status
	// in the main thread, so everything below
	// can be considered "replies".
	ThreadLastMain bool

	// This status is the one around which
	// the thread context was constructed.
	ThreadContextStatus bool

	// This status is the first visibile status
	// after the "main" thread, so it and everything
	// below it can be considered "replies".
	ThreadFirstReply bool

	// Sorted slice of StatusEdit times for
	// this status, from latest to oldest.
	// Only set if status has been edited.
	// Last entry is always creation time.
	EditTimeline []string `json:"-"`

	// Set if this is a boost.
	Reblog *WebStatusReblogged `json:"-"`
}

// WebStatusReblogged represents
// a reblogged status.
type WebStatusReblogged struct {
	*WebStatus
}

/*
** The below functions are added onto the API model status so that it satisfies
** the Preparable interface in internal/timeline.
 */

func (s *Status) GetID() string {
	return s.ID
}

func (s *Status) GetAccountID() string {
	if s.Account != nil {
		return s.Account.ID
	}
	return ""
}

func (s *Status) GetBoostOfID() string {
	if s.Reblog != nil {
		return s.Reblog.ID
	}
	return ""
}

func (s *Status) GetBoostOfAccountID() string {
	if s.Reblog != nil && s.Reblog.Account != nil {
		return s.Reblog.Account.ID
	}
	return ""
}

// StatusCreateRequest models status creation parameters.
//
// swagger:ignore
type StatusCreateRequest struct {
	// IdempotencyKey is supplied through the Idempotency-Key HTTP header.
	// It is not accepted as part of the request body.
	IdempotencyKey string `form:"-" json:"-"`

	// Text content of the status.
	// If media_ids is provided, this becomes optional.
	// Attaching a poll is optional while status is provided.
	Status string `form:"status" json:"status"`

	// Array of Attachment ids to be attached as media.
	// If provided, status becomes optional, and poll cannot be used.
	MediaIDs []string `form:"media_ids[]" json:"media_ids"`

	// Poll to include with this status.
	Poll *PollRequest `form:"poll" json:"poll"`

	// ID of the status being replied to, if status is a reply.
	InReplyToID string `form:"in_reply_to_id" json:"in_reply_to_id"`

	// ID of the status being quoted, if status is a quote (Mastodon 4.4+).
	QuotedStatusID string `form:"quoted_status_id" json:"quoted_status_id"`

	// Status and attached media should be marked as sensitive.
	Sensitive bool `form:"sensitive" json:"sensitive"`

	// Text to be shown as a warning or subject before the actual content.
	// Statuses are generally collapsed behind this field.
	SpoilerText string `form:"spoiler_text" json:"spoiler_text"`

	// Visibility of the posted status.
	Visibility Visibility `form:"visibility" json:"visibility"`

	// Set to "true" if this status should not be
	// federated,ie. it should be a "local only" status.
	LocalOnly *bool `form:"local_only" json:"local_only"`

	// Deprecated: Only used if LocalOnly is not set.
	Federated *bool `form:"federated" json:"federated"`

	// ISO8601 datetime string at which to schedule a status.
	//
	// Providing this parameter with a *future* time will cause ScheduledStatus
	// to be returned instead of Status (must be at least 5 minutes in the future).
	//
	// Providing this parameter with a *past* time will cause the status to be backdated,
	// and will not push it to the user's followers. This is intended for importing old statuses.
	ScheduledAtRaw string `form:"scheduled_at" json:"scheduled_at"`

	// ScheduledAtParsed is the parsed version
	// of scheduled_at, if scheduled_at was set.
	ScheduledAt *time.Time `form:"-" json:"-"`

	// ISO 639 language code for this status.
	Language string `form:"language" json:"language"`

	// Content type to use when parsing this status.
	ContentType StatusContentType `form:"content_type" json:"content_type"`

	// Interaction policy to use for this status.
	InteractionPolicy *InteractionPolicy `form:"-" json:"interaction_policy"`
}

// Separate form for parsing interaction
// policy on status create requests.
//
// swagger:ignore
type StatusInteractionPolicyForm struct {

	// Interaction policy to use for this status.
	InteractionPolicy *InteractionPolicy `form:"interaction_policy" json:"-"`
}

// Visibility models the visibility of a status.
//
// swagger:enum statusVisibility
// swagger:type string
type Visibility string

const (
	// VisibilityNone is visible to nobody. This is only used for the visibility of web statuses.
	VisibilityNone Visibility = "none"
	// VisibilityPublic is visible to everyone, and will be available via the web even for nonauthenticated users.

	VisibilityPublic Visibility = "public"

	// VisibilityUnlisted is visible to everyone, but only on home timelines, lists, etc.
	VisibilityUnlisted Visibility = "unlisted"

	// VisibilityPrivate is visible only to followers of the account that posted the status.
	VisibilityPrivate Visibility = "private"

	// VisibilityMutualsOnly is visible only to mutual followers of the account that posted the status.
	VisibilityMutualsOnly Visibility = "mutuals_only"

	// VisibilityDirect is visible only to accounts tagged in the status. It is equivalent to a direct message.
	VisibilityDirect Visibility = "direct"
)

// StatusContentType is the content type with which to parse the submitted status.
// Can be either text/plain or text/markdown. Empty will default to text/plain.
//
// swagger:enum statusContentType
// swagger:type string
type StatusContentType string

// Content type to use when parsing submitted
// status into an html-formatted status.
const (
	StatusContentTypePlain    StatusContentType = "text/plain"
	StatusContentTypeMarkdown StatusContentType = "text/markdown"
	StatusContentTypeDefault                    = StatusContentTypePlain
)

// StatusSource represents the source text of a
// status as submitted to the API when it was created.
//
// swagger:model statusSource
type StatusSource struct {

	// ID of the status.
	// example: 01FBVD42CQ3ZEEVMW180SBX03B
	ID string `json:"id"`

	// Plain-text source of a status.
	Text string `json:"text"`

	// Plain-text version of spoiler text.
	SpoilerText string `json:"spoiler_text"`

	// Content type that was used to parse the text.
	ContentType StatusContentType `json:"content_type,omitempty"`
}

// StatusEdit represents one historical revision of a status, containing
// partial information about the state of the status at that revision.
//
// swagger:model statusEdit
type StatusEdit struct {

	// The content of this status at this revision.
	// Should be HTML, but might also be plaintext in some cases.
	// example: <p>Hey this is a status!</p>
	Content string `json:"content"`

	// Subject, summary, or content warning for the status at this revision.
	// example: warning nsfw
	SpoilerText string `json:"spoiler_text"`

	// Status marked sensitive at this revision.
	// example: false
	Sensitive bool `json:"sensitive"`

	// The date when this revision was created (ISO 8601 Datetime).
	// example: 2021-07-30T09:20:25+00:00
	CreatedAt string `json:"created_at"`

	// The account that authored this status.
	Account *Account `json:"account"`

	// The poll attached to the status at this revision.
	// Note that edits changing the poll options will be collapsed together into one edit, since this action resets the poll.
	// nullable: true
	Poll *Poll `json:"poll"`

	// Media that is attached to this status.
	MediaAttachments []*Attachment `json:"media_attachments"`

	// Custom emoji to be used when rendering status content.
	Emojis []Emoji `json:"emojis"`
}

// StatusEditRequest models status edit parameters.
//
// swagger:ignore
type StatusEditRequest struct {

	// Text content of the status.
	// If media_ids is provided, this becomes optional.
	// Attaching a poll is optional while status is provided.
	Status string `form:"status" json:"status"`

	// Text to be shown as a warning or subject before the actual content.
	// Statuses are generally collapsed behind this field.
	SpoilerText string `form:"spoiler_text" json:"spoiler_text"`

	// Content type to use when parsing this status.
	ContentType StatusContentType `form:"content_type" json:"content_type"`

	// Status and attached media should be marked as sensitive.
	Sensitive bool `form:"sensitive" json:"sensitive"`

	// ISO 639 language code for this status.
	Language string `form:"language" json:"language"`

	// Array of Attachment ids to be attached as media.
	// If provided, status becomes optional, and poll cannot be used.
	MediaIDs []string `form:"media_ids[]" json:"media_ids"`

	// Array of Attachment attributes to be updated in attached media.
	MediaAttributes []AttachmentAttributesRequest `form:"media_attributes[]" json:"media_attributes"`

	// Poll to include with this status.
	Poll *PollRequest `form:"poll" json:"poll"`
}
