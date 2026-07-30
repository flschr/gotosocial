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

package gtsmodel

import (
	"net/url"
	"time"

	"code.superseriousbusiness.org/gopkg/log"
	"code.superseriousbusiness.org/gopkg/xslices"
)

// Status represents a user-created 'post' or
// 'status' in the database, either remote or local
type Status struct {

	// Primary ID of this item in the database.
	ID string `bun:"type:CHAR(26),pk,nullzero,notnull,unique"`

	// When was the status created.
	CreatedAt time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`

	// When this status was last edited (if set).
	EditedAt time.Time `bun:"type:timestamptz,nullzero"`

	// When was item last fetched, also used (a little bit confusingly)
	// to store time of deletion for stubbed status, as it would be unused.
	FetchedAt time.Time `bun:"type:timestamptz,nullzero"`

	// Activitypub URI of this status.
	URI string `bun:",unique,nullzero,notnull"`

	// Web url for viewing this status.
	URL string `bun:",nullzero"`

	// Content HTML for this status.
	Content string `bun:""`

	// Database IDs of any media attachments associated with this
	// status, and the attachments corresponding to attachmentIDs.
	AttachmentIDs []string           `bun:"attachments,array"`
	Attachments   []*MediaAttachment `bun:"attached_media,rel:has-many"`

	// Database IDs of any tags used in this status, and the
	// tags corresponding to tagIDs. https://bun.uptrace.dev/guide/relations.html#many-to-many-relation
	TagIDs []string `bun:"tags,array"`
	Tags   []*Tag   `bun:"attached_tags,m2m:status_to_tags"`

	// Database IDs of any mentions in this status,
	// and the mentions corresponding to mentionIDs.
	MentionIDs []string   `bun:"mentions,array"`
	Mentions   []*Mention `bun:"attached_mentions,rel:has-many"`

	// Database IDs of any emojis used in this status, and the
	// emojis corresponding to emojiIDs. https://bun.uptrace.dev/guide/relations.html#many-to-many-relation
	EmojiIDs []string `bun:"emojis,array"`
	Emojis   []*Emoji `bun:"attached_emojis,m2m:status_to_emojis"`

	// ID of the account that posted this status, the
	// activitypub URI of the account that posted this status,
	// and the Account model corresponding to the accountID.
	AccountID  string   `bun:"type:CHAR(26),nullzero,notnull"`
	AccountURI string   `bun:",nullzero,notnull"`
	Account    *Account `bun:"rel:belongs-to"`

	// ID of the status this is in reply to (or NULL), the
	// activitypub URI of the status this is in reply to (or NULL),
	// and the Status model corresponding to the statusID (if set).
	InReplyToID  string  `bun:"type:CHAR(26),nullzero"`
	InReplyToURI string  `bun:",nullzero"`
	InReplyTo    *Status `bun:"-"`

	// ID of the account that this status replies to,
	// account corresponding to inReplyToAccountID.
	InReplyToAccountID string   `bun:"type:CHAR(26),nullzero"`
	InReplyToAccount   *Account `bun:"rel:belongs-to"`

	// ID of the status this status is a boost of,
	// the URI of the status this status is a boost of
	// (not inserted in the db, just for dereferencing purposes),
	// and the status that corresponds to boostOfID.
	BoostOfID     string   `bun:"type:CHAR(26),nullzero"`
	BoostOfURI    *url.URL `bun:"-"`
	BoostOfURIStr string   `bun:"-"`
	BoostOf       *Status  `bun:"-"`

	// ID of the account that owns the boosted status,
	// and account that corresponds to boostOfAccountID
	BoostOfAccountID string   `bun:"type:CHAR(26),nullzero"`
	BoostOfAccount   *Account `bun:"rel:belongs-to"`

	// ID of the status this status quotes (or NULL), the
	// activitypub URI of the quoted status (or NULL), and
	// the Status model corresponding to quoteID (if set/dereferenced).
	QuoteID  string  `bun:"type:CHAR(26),nullzero"`
	QuoteURI string  `bun:",nullzero"`
	Quote    *Status `bun:"-"`

	// ID of the account whose status is quoted,
	// and account that corresponds to quoteAccountID.
	QuoteAccountID string   `bun:"type:CHAR(26),nullzero"`
	QuoteAccount   *Account `bun:"rel:belongs-to"`

	// URI of the QuoteAuthorization (FEP-044f) proving this quote
	// was approved by the quoted account (or NULL). Analogous to
	// ApprovedByURI for replies/boosts; empty until approved.
	QuoteApprovalURI string `bun:",nullzero"`

	// ID of the thread to which this status belongs.
	ThreadID string `bun:"type:CHAR(26),nullzero,notnull,default:'00000000000000000000000000'"`

	// IDs of status edits for this status, ordered from
	// smallest (oldest) -> largest (newest) ID. Edits of
	// this status, ordered from oldest -> newest edit.
	//
	// Edited in particular is an ephemeral field
	// only set on statuses that were just edited.
	EditIDs []string      `bun:"edits,array"`
	Edits   []*StatusEdit `bun:"-"`
	Edited  bool          `bun:"-"`

	// ID of the poll attached to this status,
	// and the Poll that corresponds to pollID.
	PollID string `bun:"type:CHAR(26),nullzero"`
	Poll   *Poll  `bun:"-"`

	// Content warning HTML for this status, and the
	// original text of the content warning without formatting
	ContentWarning     string `bun:",nullzero"`
	ContentWarningText string `bun:""`

	// Flags contains numerous status boolean flags.
	Flags StatusFlags `bun:",notnull,default:0"`

	// Visibility entry for this status.
	Visibility Visibility `bun:",nullzero,notnull"`

	// What language is this status written in?
	Language string `bun:",nullzero"`

	// Which application was used to create this status? And
	// the application corresponding to createdWithApplicationID.
	CreatedWithApplicationID string       `bun:"type:CHAR(26),nullzero"`
	CreatedWithApplication   *Application `bun:"rel:belongs-to"`

	// Idempotency key supplied by the client that created this local status.
	// Scoped to account and application; remote statuses leave this empty.
	IdempotencyKey string `bun:",nullzero"`

	// What is the activitystreams type of this status?
	// See: https://www.w3.org/TR/activitystreams-vocabulary/#object-types.
	//
	// Will probably almost always be Note but who knows!.
	ActivityStreamsType string `bun:",nullzero,notnull"`

	// Original text of the status without formatting.
	Text string `bun:""`

	// Content type used to process the original text of the status.
	ContentType StatusContentType `bun:",nullzero"`

	// InteractionPolicy for this status. If null then the default InteractionPolicy
	// should be assumed for this status's Visibility. Always null for boost wrappers.
	InteractionPolicy *InteractionPolicy `bun:""`

	// If true, then status is a reply to or boost wrapper of a status on
	// our instance, has permission to do the interaction, and an Accept
	// should be sent out for it immediately. Field not stored in the DB.
	PreApproved bool `bun:"-"`

	// URI of *either* an Accept Activity, or a ReplyAuthorization or
	// AnnounceAuthorization, which approves the Announce, Create or
	// interaction request Activity that this status was/will be attached to.
	ApprovedByURI string `bun:",nullzero"`

	// BlueskyInteractionID marks a private local proxy status imported from
	// Bluesky. It deliberately does not represent an ActivityPub actor.
	BlueskyInteractionID string `bun:"type:CHAR(26),nullzero"`
}

// GetAccount returns the account that owns
// this status. May be nil if status not populated.
// Fulfils Interaction interface.
func (s *Status) GetAccount() *Account {
	return s.Account
}

// AttachmentsPopulated returns whether media attachments
// are populated according to current AttachmentIDs.
func (s *Status) AttachmentsPopulated() bool {
	if len(s.AttachmentIDs) != len(s.Attachments) {
		// this is the quickest indicator.
		return false
	}
	for i, id := range s.AttachmentIDs {
		if s.Attachments[i].ID != id {
			return false
		}
	}
	return true
}

// TagsPopulated returns whether tags are
// populated according to current TagIDs.
func (s *Status) TagsPopulated() bool {
	if len(s.TagIDs) != len(s.Tags) {
		// this is the quickest indicator.
		return false
	}
	for i, id := range s.TagIDs {
		if s.Tags[i].ID != id {
			return false
		}
	}
	return true
}

// MentionsPopulated returns whether mentions are
// populated according to current MentionIDs.
func (s *Status) MentionsPopulated() bool {
	if len(s.MentionIDs) != len(s.Mentions) {
		// this is the quickest indicator.
		return false
	}
	for i, id := range s.MentionIDs {
		if s.Mentions[i].ID != id {
			return false
		}
	}
	return true
}

// EmojisPopulated returns whether emojis are
// populated according to current EmojiIDs.
func (s *Status) EmojisPopulated() bool {
	if len(s.EmojiIDs) != len(s.Emojis) {
		// this is the quickest indicator.
		return false
	}
	for i, id := range s.EmojiIDs {
		if s.Emojis[i].ID != id {
			return false
		}
	}
	return true
}

// EditsPopulated returns whether edits are
// populated according to current EditIDs.
func (s *Status) EditsPopulated() bool {
	if len(s.EditIDs) != len(s.Edits) {
		// this is quickest indicator.
		return false
	}
	for i, id := range s.EditIDs {
		if s.Edits[i].ID != id {
			return false
		}
	}
	return true
}

// EmojisUpToDate returns whether status emoji attachments of receiving status are up-to-date
// according to emoji attachments of the passed status, by comparing their emoji URIs. We don't
// use IDs as this is used to determine whether there are new emojis to fetch.
func (s *Status) EmojisUpToDate(other *Status) bool {
	if len(s.Emojis) != len(other.Emojis) {
		// this is the quickest indicator.
		return false
	}
	for i := range s.Emojis {
		if s.Emojis[i].URI != other.Emojis[i].URI {
			return false
		}
	}
	return true
}

// GetAttachmentByRemoteURL searches status for MediaAttachment{} with remote URL.
func (s *Status) GetAttachmentByRemoteURL(url string) (*MediaAttachment, bool) {
	for _, media := range s.Attachments {
		if media.RemoteURL == url {
			return media, true
		}
	}
	return nil, false
}

// GetMentionByTargetURI searches status for Mention{} with target URI.
func (s *Status) GetMentionByTargetURI(uri string) (*Mention, bool) {
	for _, mention := range s.Mentions {
		if mention.TargetAccountURI == uri {
			return mention, true
		}
	}
	return nil, false
}

// GetMentionByTargetID searches status for Mention{} with target ID.
func (s *Status) GetMentionByTargetID(id string) (*Mention, bool) {
	for _, mention := range s.Mentions {
		if mention.TargetAccountID == id {
			return mention, true
		}
	}
	return nil, false
}

// GetMentionByUsernameDomain fetches the Mention associated with given
// username and domains, typically extracted from a mention Namestring.
func (s *Status) GetMentionByUsernameDomain(username, domain string) (*Mention, bool) {
	for _, mention := range s.Mentions {

		// We can only check if target
		// account is set on the mention.
		account := mention.TargetAccount
		if account == nil {
			continue
		}

		// Usernames must always match.
		if account.Username != username {
			continue
		}

		// Finally, either domains must
		// match or an empty domain may
		// be permitted if account local.
		if account.Domain == domain ||
			(domain == "" && account.IsLocal()) {
			return mention, true
		}
	}

	return nil, false
}

// GetTagByName searches status for Tag{} with name.
func (s *Status) GetTagByName(name string) (*Tag, bool) {
	for _, tag := range s.Tags {
		if tag.Name == name {
			return tag, true
		}
	}
	return nil, false
}

// MentionsAccount returns whether status mentions the given account ID.
func (s *Status) MentionsAccount(accountID string) bool {
	for _, mention := range s.Mentions {
		if mention.TargetAccountID == accountID {
			return true
		}
	}
	return false
}

// BelongsToAccount returns whether status belongs to the given account ID.
func (s *Status) BelongsToAccount(accountID string) bool {
	return s.AccountID == accountID
}

// LocalOnly returns true if this status
// is "local-only" ie., unfederated.
func (s *Status) LocalOnly() bool {
	return !s.Flags.Federated()
}

// AllAttachmentIDs gathers ALL media attachment IDs from both
// the receiving Status{}, and any historical Status{}.Edits.
func (s *Status) AllAttachmentIDs() []string {
	var total int

	// Check if this is being correctly
	// called on fully populated status.
	if !s.EditsPopulated() {
		log.Warnf(nil, "status edits not populated for %s", s.URI)
	}

	// Get count of attachment IDs.
	total += len(s.AttachmentIDs)
	for _, edit := range s.Edits {
		total += len(edit.AttachmentIDs)
	}

	// Start gathering of all IDs with *current* attachment IDs.
	attachmentIDs := make([]string, len(s.AttachmentIDs), total)
	copy(attachmentIDs, s.AttachmentIDs)

	// Append IDs of historical edits.
	for _, edit := range s.Edits {
		attachmentIDs = append(attachmentIDs, edit.AttachmentIDs...)
	}

	// Deduplicate these IDs in case of shared media.
	return xslices.Deduplicate(attachmentIDs)
}

// UpdatedAt returns latest time this status
// was updated, either EditedAt or CreatedAt.
func (s *Status) UpdatedAt() time.Time {
	if s.EditedAt.IsZero() {
		return s.CreatedAt
	}
	return s.EditedAt
}

// ToOrCcPublic returns true if the visibility
// of this status is either to or cc public
// (ie., public or unlisted/unlocked status).
func (s *Status) ToOrCcPublic() bool {
	return s.Visibility == VisibilityPublic ||
		s.Visibility == VisibilityUnlocked
}

// Stub will return a status with unset fields,
// maintaining only those necessary for it to be
// dealt with as a placeholder status in a thread.
func (s *Status) Stub() (stub Status) {

	// Maintain base
	// status ID-ables.
	stub.ID = s.ID
	stub.URL = s.URL
	stub.URI = s.URI

	// ActivityStreams type is not nullable field.
	stub.ActivityStreamsType = s.ActivityStreamsType

	// Maintain threading information
	// including direct status parent.
	stub.InReplyToID = s.InReplyToID
	stub.InReplyToURI = s.InReplyToURI
	stub.InReplyToAccountID = s.InReplyToAccountID
	stub.ThreadID = s.ThreadID

	// Maintain author for proper
	// visibility calculations, though
	// we won't expose author anymore.
	stub.AccountID = s.AccountID
	stub.AccountURI = s.AccountURI
	stub.Account = s.Account

	// Maintain mentions and visibility for
	// proper visibility calculations, though
	// we won't expect mentions anymore.
	stub.MentionIDs = s.MentionIDs
	stub.Visibility = s.Visibility
	stub.Mentions = s.Mentions

	// Maintain the approved by URI for proper
	// handling of existing interaction approvals.
	stub.ApprovedByURI = s.ApprovedByURI

	// Maintain status creation
	// update fetched-at to store
	// date at which status deleted.
	stub.CreatedAt = s.CreatedAt
	stub.FetchedAt = time.Now()

	// Finally, copy over
	// flags and mark deleted.
	stub.Flags = s.Flags
	stub.Flags.SetDeleted(true)

	// Unset flags that can no longer
	// apply to a deleted staetus model.
	stub.Flags.SetPendingApproval(false)
	stub.Flags.SetSensitive(false)

	return
}

// StatusToTag is an intermediate struct to facilitate the
// many2many relationship between a status and one or more tags.
type StatusToTag struct {
	StatusID string  `bun:"type:CHAR(26),unique:statustag,nullzero,notnull"`
	Status   *Status `bun:"rel:belongs-to"`
	TagID    string  `bun:"type:CHAR(26),unique:statustag,nullzero,notnull"`
	Tag      *Tag    `bun:"rel:belongs-to"`
}

// StatusToEmoji is an intermediate struct to facilitate the
// many2many relationship between a status and one or more emojis.
type StatusToEmoji struct {
	StatusID string  `bun:"type:CHAR(26),unique:statusemoji,nullzero,notnull"`
	Status   *Status `bun:"rel:belongs-to"`
	EmojiID  string  `bun:"type:CHAR(26),unique:statusemoji,nullzero,notnull"`
	Emoji    *Emoji  `bun:"rel:belongs-to"`
}

// Visibility represents the
// visibility granularity of a status.
type Visibility enumType

const (
	// VisibilityNone means nobody can see this.
	// It's only used for web status visibility.
	VisibilityNone Visibility = 1

	// VisibilityPublic means this status will
	// be visible to everyone on all timelines.
	VisibilityPublic Visibility = 2

	// VisibilityUnlocked means this status will be visible to everyone,
	// but will only show on home timeline to followers, and in lists.
	VisibilityUnlocked Visibility = 3

	// VisibilityFollowersOnly means this status is viewable to followers only.
	VisibilityFollowersOnly Visibility = 4

	// VisibilityMutualsOnly means this status
	// is visible to mutual followers only.
	VisibilityMutualsOnly Visibility = 5

	// VisibilityDirect means this status is
	// visible only to mentioned recipients.
	VisibilityDirect Visibility = 6

	// VisibilityDefault is used when no other setting can be found.
	VisibilityDefault Visibility = VisibilityUnlocked
)

// String returns a stringified, frontend API compatible form of Visibility.
func (v Visibility) String() string {
	switch v {
	case VisibilityNone:
		return "none"
	case VisibilityPublic:
		return "public"
	case VisibilityUnlocked:
		return "unlocked"
	case VisibilityFollowersOnly:
		return "followers_only"
	case VisibilityMutualsOnly:
		return "mutuals_only"
	case VisibilityDirect:
		return "direct"
	default:
		panic("invalid visibility")
	}
}

// StatusContentType is the content type with which a status's text is
// parsed. Can be either plain or markdown. Empty will default to plain.
type StatusContentType enumType

const (
	StatusContentTypePlain    StatusContentType = 1
	StatusContentTypeMarkdown StatusContentType = 2
	StatusContentTypeDefault                    = StatusContentTypePlain
)

// Content models the simple string content
// of a status along with its ContentMap,
// which contains content entries keyed by
// BCP47 language tag.
//
// Content and/or ContentMap may be zero/nil.
type Content struct {
	Content    string
	ContentMap map[string]string
}
