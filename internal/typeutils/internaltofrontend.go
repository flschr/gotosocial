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

package typeutils

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"code.superseriousbusiness.org/gopkg/log"
	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/language"
	"code.superseriousbusiness.org/gotosocial/internal/media"
	"code.superseriousbusiness.org/gotosocial/internal/text"
	"code.superseriousbusiness.org/gotosocial/internal/uris"
	"code.superseriousbusiness.org/gotosocial/internal/util"
	"codeberg.org/gruf/go-debug"
)

const (
	instanceStatusesCharactersReservedPerURL = 25
	instancePollsMinExpiration               = 300     // seconds
	instancePollsMaxExpiration               = 2629746 // seconds
	instanceAccountsMaxFeaturedTags          = 10
	instanceSourceURL                        = "https://codeberg.org/superseriousbusiness/gotosocial"
	instanceMastodonVersion                  = "3.5.3"
)

var instanceStatusesSupportedMimeTypes = []string{
	string(apimodel.StatusContentTypePlain),
	string(apimodel.StatusContentTypeMarkdown),
}

func toMastodonVersion(in string) string {
	return instanceMastodonVersion + "+" + strings.ReplaceAll(in, " ", "-")
}

// UserToAPIUser converts a *gtsmodel.User to an API
// representation suitable for serving to that user.
//
// Contains sensitive info so should only
// ever be served to the user themself.
func (c *Converter) UserToAPIUser(ctx context.Context, u *gtsmodel.User) *apimodel.User {
	user := &apimodel.User{
		ID:               u.ID,
		CreatedAt:        util.FormatISO8601(u.CreatedAt),
		Email:            u.Email,
		UnconfirmedEmail: u.UnconfirmedEmail,
		Reason:           u.Reason,
		Moderator:        *u.Moderator,
		Admin:            *u.Admin,
		Disabled:         *u.Disabled,
		Approved:         *u.Approved,
	}

	// Zero-able dates.
	if !u.LastEmailedAt.IsZero() {
		user.LastEmailedAt = util.FormatISO8601(u.LastEmailedAt)
	}

	if !u.ConfirmedAt.IsZero() {
		user.ConfirmedAt = util.FormatISO8601(u.ConfirmedAt)
	}

	if !u.ConfirmationSentAt.IsZero() {
		user.ConfirmationSentAt = util.FormatISO8601(u.ConfirmationSentAt)
	}

	if !u.ResetPasswordSentAt.IsZero() {
		user.ResetPasswordSentAt = util.FormatISO8601(u.ResetPasswordSentAt)
	}

	if !u.TwoFactorEnabledAt.IsZero() {
		user.TwoFactorEnabledAt = util.FormatISO8601(u.TwoFactorEnabledAt)
	}

	return user
}

// accountWebVisibility returns web visibility for a local account's statuses.
func accountWebVisibility(a *gtsmodel.Account) apimodel.Visibility {
	switch {
	case *a.HidesToPublicFromUnauthedWeb:
		// Hides all.
		return apimodel.VisibilityNone

	case !*a.HidesCcPublicFromUnauthedWeb:
		// Shows unlisted + public (Masto default).
		return apimodel.VisibilityUnlisted

	default:
		// Shows public only (GtS default).
		return apimodel.VisibilityPublic
	}
}

// AccountToAPIAccountSensitive takes a db model account as a param, and returns a populated apitype account, or an error
// if something goes wrong. The returned account should be ready to serialize on an API level, and may have sensitive fields
// (such as user settings and follow requests count), so serve it only to an authorized user who should have permission to see it.
func (c *Converter) AccountToAPIAccountSensitive(ctx context.Context, a *gtsmodel.Account) (*apimodel.Account, error) {
	// We can build this sensitive account model
	// by first getting the public account, and
	// then adding the Source object and role permissions bitmap to it.
	apiAccount, err := c.AccountToAPIAccountPublic(ctx, a)
	if err != nil {
		return nil, err
	}

	// Ensure account stats populated.
	if err := c.state.DB.PopulateAccountStats(ctx, a); err != nil {
		return nil, gtserror.Newf(
			"error getting stats for account %s: %w",
			a.ID, err,
		)
	}

	// Populate the authenticated account's sensitive role directly. Public
	// account representations intentionally omit local role information.
	user, err := c.state.DB.GetUserByAccountID(ctx, a.ID)
	if err != nil {
		return nil, gtserror.Newf(
			"error getting user for account %s: %w",
			a.ID, err,
		)
	}
	apiAccount.Role = c.APIAccountDisplayRoleToAPIAccountRoleSensitive(
		c.UserToAPIAccountDisplayRole(user),
	)

	statusContentType := string(apimodel.StatusContentTypeDefault)
	if a.Settings.StatusContentType != "" {
		statusContentType = a.Settings.StatusContentType
	}

	apiAccount.Source = &apimodel.Source{
		Privacy:             VisToAPIVis(a.Settings.Privacy),
		WebVisibility:       accountWebVisibility(a),
		WebLayout:           a.Settings.WebLayout.String(),
		WebIncludeBoosts:    *a.Settings.WebIncludeBoosts,
		Sensitive:           *a.Settings.Sensitive,
		Language:            a.Settings.Language,
		StatusContentType:   statusContentType,
		Note:                a.NoteRaw,
		Fields:              c.fieldsToAPIFields(a.FieldsRaw),
		FollowRequestsCount: *a.Stats.FollowRequestsCount,
		AlsoKnownAsURIs:     a.AlsoKnownAsURIs,
	}

	return apiAccount, nil
}

// AccountToAPIAccountPublic takes a db model account as a param, and returns a populated apitype account, or an error
// if something goes wrong. The returned account should be ready to serialize on an API level, and may NOT have sensitive fields.
// In other words, this is the public record that the server has of an account.
func (c *Converter) AccountToAPIAccountPublic(ctx context.Context, a *gtsmodel.Account) (*apimodel.Account, error) {
	account, err := c.accountToAPIAccountPublic(ctx, a)
	if err != nil {
		return nil, err
	}

	if a.MovedTo != nil {
		account.Moved, err = c.accountToAPIAccountPublic(ctx, a.MovedTo)
		if err != nil {
			log.Errorf(ctx, "error converting account movedTo: %v", err)
		}
	}

	return account, nil
}

// AccountToWebAccount converts a gts model account into an
// api representation suitable for serving into a web template.
//
// Should only be used when preparing to template an account,
// callers looking to serialize an account into a model for
// serving over the client API should always use one of the
// AccountToAPIAccount functions instead.
func (c *Converter) AccountToWebAccount(
	ctx context.Context,
	account *gtsmodel.Account,
	apiAccount *apimodel.Account,
) (*apimodel.WebAccount, error) {
	if apiAccount == nil {
		var err error

		apiAccount, err = c.AccountToAPIAccountPublic(ctx, account)
		if err != nil {
			return nil, err
		}
	}

	webAccount := &apimodel.WebAccount{
		Account: apiAccount,
	}

	// Set additional avatar information for
	// serving the avatar in a nice <picture>.
	if avatar := account.AvatarMediaAttachment; avatar != nil {
		apiAttachment := AttachmentToAPIAttachment(avatar)
		webAccount.AvatarAttachment = &apimodel.WebAttachment{
			Attachment:      &apiAttachment,
			MIMEType:        avatar.File.ContentType,
			PreviewMIMEType: avatar.Thumbnail.ContentType,
		}
	}

	// Set additional header information for
	// serving the header in a nice <picture>.
	if header := account.HeaderMediaAttachment; header != nil {
		apiAttachment := AttachmentToAPIAttachment(header)
		webAccount.HeaderAttachment = &apimodel.WebAttachment{
			Attachment:      &apiAttachment,
			MIMEType:        header.File.ContentType,
			PreviewMIMEType: header.Thumbnail.ContentType,
		}
	}

	// Check for presence of settings before
	// populating settings-specific thingies,
	// as instance account doesn't store a
	// settings struct.
	if account.Settings != nil {
		webAccount.WebLayout = account.Settings.WebLayout.String()
		webAccount.WebVisibility = accountWebVisibility(account)
	}

	return webAccount, nil
}

// accountToAPIAccountPublic provides all the logic for AccountToAPIAccount, MINUS fetching moved account, to prevent possible recursion.
func (c *Converter) accountToAPIAccountPublic(ctx context.Context, a *gtsmodel.Account) (*apimodel.Account, error) {

	// Populate account struct fields.
	err := c.state.DB.PopulateAccount(ctx, a)

	switch {
	case err == nil:
		// No problem.

	case a.Stats != nil:
		// We have stats so that's
		// *maybe* OK, try to continue.
		log.Errorf(ctx, "error(s) populating account, will continue: %s", err)

	default:
		// There was an error and we don't
		// have stats, we can't continue.
		return nil, gtserror.Newf("account stats not populated, could not continue: %w", err)
	}

	// Basic account stats:
	//   - Followers count
	//   - Following count
	//   - Statuses count
	//   - Last status time

	var (
		followersCount = *a.Stats.FollowersCount
		followingCount = *a.Stats.FollowingCount
		statusesCount  = *a.Stats.StatusesCount
		lastStatusAt   = func() *string {
			if a.Stats.LastStatusAt.IsZero() {
				return nil
			}
			return util.Ptr(util.FormatISO8601Date(a.Stats.LastStatusAt))
		}()
	)

	// Profile media + nice extras:
	//   - Avatar
	//   - Header
	//   - Fields
	//   - Emojis

	var (
		aviID           string
		aviURL          string
		aviURLStatic    string
		aviDesc         string
		headerID        string
		headerURL       string
		headerURLStatic string
		headerDesc      string
	)

	if a.AvatarMediaAttachment != nil {
		aviID = a.AvatarMediaAttachmentID
		aviURL = a.AvatarMediaAttachment.URL
		aviURLStatic = a.AvatarMediaAttachment.Thumbnail.URL
		aviDesc = a.AvatarMediaAttachment.Description
	}

	if a.HeaderMediaAttachment != nil {
		headerID = a.HeaderMediaAttachmentID
		headerURL = a.HeaderMediaAttachment.URL
		headerURLStatic = a.HeaderMediaAttachment.Thumbnail.URL
		headerDesc = a.HeaderMediaAttachment.Description
	}

	// convert account gts model fields to front api model fields
	fields := c.fieldsToAPIFields(a.Fields)

	// GTS model emojis -> frontend.
	apiEmojis := c.emojisToAPI(ctx,
		a.Emojis,
		a.EmojiIDs,
	)

	// Bits that vary between remote + local accounts:
	//   - Account (acct) string.
	//   - Role.
	//   - Settings things (enableRSS, theme, customCSS, hideCollections).

	var (
		acct            string
		enableRSS       bool
		theme           string
		customCSS       string
		hideCollections bool
	)

	if a.IsRemote() {
		// Domain may be in Punycode,
		// de-punify it just in case.
		d, err := util.DePunify(a.Domain)
		if err != nil {
			return nil, gtserror.Newf("error de-punifying domain %s for account id %s: %w", a.Domain, a.ID, err)
		}

		acct = a.Username + "@" + d
	} else {
		// Populate local user settings. Skip instance accounts since they have
		// no settings and should retain their bare username.
		if !a.IsInstance() {
			enableRSS = *a.Settings.EnableRSS
			theme = a.Settings.Theme
			customCSS = a.Settings.CustomCSS
			hideCollections = *a.Settings.HideCollections
		}

		if a.IsInstance() {
			acct = a.Username
		} else {
			// Include account-domain for local users so split-domain
			// deployments display the public handle in Mastodon clients.
			acct = a.Username + "@" + config.GetAccountDomain()
		}
	}

	var (
		locked       = util.PtrOrValue(a.Locked, true)
		discoverable = util.PtrOrValue(a.Discoverable, false)
		indexable    = util.PtrOrValue(a.Indexable, false)
	)

	// Remaining properties are simple and
	// can be populated directly below.

	accountFrontend := &apimodel.Account{
		ID:                a.ID,
		Username:          a.Username,
		Acct:              acct,
		DisplayName:       a.DisplayName,
		Locked:            locked,
		Discoverable:      discoverable,
		Indexable:         indexable,
		NoIndex:           !indexable,
		Bot:               a.ActorType.IsBot(),
		CreatedAt:         util.FormatISO8601(a.CreatedAt),
		Note:              a.Note,
		URL:               a.URL,
		Avatar:            aviURL,
		AvatarStatic:      aviURLStatic,
		AvatarDescription: aviDesc,
		AvatarMediaID:     aviID,
		Header:            headerURL,
		HeaderStatic:      headerURLStatic,
		HeaderDescription: headerDesc,
		HeaderMediaID:     headerID,
		FollowersCount:    followersCount,
		FollowingCount:    followingCount,
		StatusesCount:     statusesCount,
		LastStatusAt:      lastStatusAt,
		Emojis:            apiEmojis,
		Fields:            fields,
		Suspended:         !a.SuspendedAt.IsZero(),
		Theme:             theme,
		CustomCSS:         customCSS,
		EnableRSS:         enableRSS,
		HideCollections:   hideCollections,
		Group:             false,
	}

	// Bodge default avatar + header in,
	// if we didn't have one already.
	c.ensureAvatar(accountFrontend)
	c.ensureHeader(accountFrontend)

	return accountFrontend, nil
}

// UserToAPIAccountDisplayRole returns the API representation of a user's display role.
// This will accept a nil user but does not always return a value:
// the default "user" role is considered uninteresting and not returned.
func (c *Converter) UserToAPIAccountDisplayRole(user *gtsmodel.User) *apimodel.AccountDisplayRole {
	switch {
	case user == nil:
		return nil
	case *user.Admin:
		return &apimodel.AccountDisplayRole{
			ID:   string(apimodel.AccountRoleAdmin),
			Name: apimodel.AccountRoleAdmin,
		}
	case *user.Moderator:
		return &apimodel.AccountDisplayRole{
			ID:   string(apimodel.AccountRoleModerator),
			Name: apimodel.AccountRoleModerator,
		}
	default:
		return nil
	}
}

// APIAccountDisplayRoleToAPIAccountRoleSensitive returns the API representation of a user's role,
// with permission bitmap. This will accept a nil display role and always returns a value.
func (c *Converter) APIAccountDisplayRoleToAPIAccountRoleSensitive(display *apimodel.AccountDisplayRole) *apimodel.AccountRole {
	// Default to user role.
	role := &apimodel.AccountRole{
		AccountDisplayRole: apimodel.AccountDisplayRole{
			ID:   string(apimodel.AccountRoleUser),
			Name: apimodel.AccountRoleUser,
		},
		Permissions: apimodel.AccountRolePermissionsNone,
		Highlighted: false,
	}

	// If there's a display role, use that instead.
	if display != nil {
		role.AccountDisplayRole = *display
		role.Highlighted = true
		switch display.Name {
		case apimodel.AccountRoleAdmin:
			role.Permissions = apimodel.AccountRolePermissionsForAdminRole
		case apimodel.AccountRoleModerator:
			role.Permissions = apimodel.AccountRolePermissionsForModeratorRole
		}
	}

	return role
}

func (c *Converter) fieldsToAPIFields(f []*gtsmodel.Field) []apimodel.Field {
	fields := make([]apimodel.Field, len(f))

	for i, field := range f {
		mField := apimodel.Field{
			Name:  field.Name,
			Value: field.Value,
		}

		if !field.VerifiedAt.IsZero() {
			verified := util.FormatISO8601(field.VerifiedAt)
			mField.VerifiedAt = util.Ptr(verified)
		}

		fields[i] = mField
	}

	return fields
}

// AccountToAPIAccountBlocked takes a db model account as a param, and returns a apitype account, or an error if
// something goes wrong. The returned account will be a bare minimum representation of the account. This function should be used
// when someone wants to view an account they've blocked.
func (c *Converter) AccountToAPIAccountBlocked(ctx context.Context, a *gtsmodel.Account) (*apimodel.Account, error) {
	var acct string

	if a.IsRemote() {
		// Domain may be in Punycode,
		// de-punify it just in case.
		d, err := util.DePunify(a.Domain)
		if err != nil {
			return nil, gtserror.Newf("error de-punifying domain %s for account id %s: %w", a.Domain, a.ID, err)
		}

		acct = a.Username + "@" + d
	} else {
		if a.IsInstance() {
			acct = a.Username
		} else {
			acct = a.Username + "@" + config.GetAccountDomain()
		}
	}

	account := &apimodel.Account{
		ID:        a.ID,
		Username:  a.Username,
		Acct:      acct,
		NoIndex:   true,
		Bot:       a.ActorType.IsBot(),
		CreatedAt: util.FormatISO8601(a.CreatedAt),
		URL:       a.URL,
		// Empty array (not nillable).
		Emojis: make([]apimodel.Emoji, 0),
		// Empty array (not nillable).
		Fields:    make([]apimodel.Field, 0),
		Suspended: !a.SuspendedAt.IsZero(),
	}

	// Don't show the account's actual
	// avatar+header since it may be
	// upsetting to the blocker. Just
	// show generic avatar+header instead.
	c.ensureAvatar(account)
	c.ensureHeader(account)

	return account, nil
}

func (c *Converter) AccountToAdminAPIAccount(ctx context.Context, a *gtsmodel.Account) (*apimodel.AdminAccountInfo, error) {
	var (
		email                  string
		ip                     *string
		domain                 *string
		locale                 string
		confirmed              bool
		inviteRequest          *string
		approved               bool
		disabled               bool
		role                   = *c.APIAccountDisplayRoleToAPIAccountRoleSensitive(nil)
		createdByApplicationID string
	)

	if err := c.state.DB.PopulateAccount(ctx, a); err != nil {
		log.Errorf(ctx, "error(s) populating account, will continue: %s", err)
	}

	if a.IsRemote() {
		// Domain may be in Punycode,
		// de-punify it just in case.
		d, err := util.DePunify(a.Domain)
		if err != nil {
			return nil, fmt.Errorf("AccountToAdminAPIAccount: error de-punifying domain %s for account id %s: %w", a.Domain, a.ID, err)
		}

		domain = &d
	} else if !a.IsInstance() {
		// This is a local, non-instance
		// acct; we can fetch more info.
		user, err := c.state.DB.GetUserByAccountID(ctx, a.ID)
		if err != nil {
			return nil, fmt.Errorf("AccountToAdminAPIAccount: error getting user from database for account id %s: %w", a.ID, err)
		}

		if user.Email != "" {
			email = user.Email
		} else {
			email = user.UnconfirmedEmail
		}

		if i := user.SignUpIP.String(); i != "<nil>" {
			ip = &i
		}

		locale = user.Locale
		if user.Reason != "" {
			inviteRequest = &user.Reason
		}

		role = *c.APIAccountDisplayRoleToAPIAccountRoleSensitive(
			c.UserToAPIAccountDisplayRole(user),
		)

		confirmed = !user.ConfirmedAt.IsZero()
		approved = *user.Approved
		disabled = *user.Disabled
		createdByApplicationID = user.CreatedByApplicationID
	}

	apiAccount, err := c.AccountToAPIAccountPublic(ctx, a)
	if err != nil {
		return nil, fmt.Errorf("AccountToAdminAPIAccount: error converting account to api account for account id %s: %w", a.ID, err)
	}

	return &apimodel.AdminAccountInfo{
		ID:                     a.ID,
		Username:               a.Username,
		Domain:                 domain,
		CreatedAt:              util.FormatISO8601(a.CreatedAt),
		Email:                  email,
		IP:                     ip,
		IPs:                    []interface{}{}, // not implemented,
		Locale:                 locale,
		InviteRequest:          inviteRequest,
		Role:                   role,
		Confirmed:              confirmed,
		Approved:               approved,
		Disabled:               disabled,
		Silenced:               !a.SilencedAt.IsZero(),
		Suspended:              !a.SuspendedAt.IsZero(),
		Account:                apiAccount,
		CreatedByApplicationID: createdByApplicationID,
		InvitedByAccountID:     "", // not implemented (yet)
	}, nil
}

func (c *Converter) AppToAPIAppSensitive(ctx context.Context, a *gtsmodel.Application) (*apimodel.Application, error) {
	vapidKeyPair, err := c.state.DB.GetVAPIDKeyPair(ctx)
	if err != nil {
		return nil, gtserror.Newf("error getting VAPID public key: %w", err)
	}

	createdAt, err := id.TimeFromULID(a.ID)
	if err != nil {
		return nil, gtserror.Newf("error converting id to time: %w", err)
	}

	return &apimodel.Application{
		ID:           a.ID,
		CreatedAt:    util.FormatISO8601(createdAt),
		Name:         a.Name,
		Website:      a.Website,
		RedirectURI:  strings.Join(a.RedirectURIs, "\n"),
		RedirectURIs: a.RedirectURIs,
		ClientID:     a.ClientID,
		ClientSecret: a.ClientSecret,
		VapidKey:     vapidKeyPair.Public,
		Scopes:       strings.Split(a.Scopes, " "),
	}, nil
}

// AppToAPIAppPublic takes a db model application as a param, and returns a populated apitype application, or an error
// if something goes wrong. The returned application should be ready to serialize on an API level, and has sensitive
// fields sanitized so that it can be served to non-authorized accounts without revealing any private information.
func AppToAPIAppPublic(app *gtsmodel.Application) *apimodel.Application {
	return &apimodel.Application{
		Name:    app.Name,
		Website: app.Website,
	}
}

// zero value media filemeta.
var zeroSmall gtsmodel.Small
var zeroOriginal gtsmodel.Original

// AttachmentToAPIAttachment converts a gts model media attacahment into its api representation.
func AttachmentToAPIAttachment(media *gtsmodel.MediaAttachment) apimodel.Attachment {
	var api apimodel.Attachment
	api.Type = media.Type.String()
	api.ID = media.ID

	// Set initial API model attachment fields.
	api.Blurhash = util.PtrIf(media.Blurhash)
	api.RemoteURL = util.PtrIf(media.RemoteURL)
	api.PreviewRemoteURL = util.PtrIf(media.Thumbnail.RemoteURL)
	api.Description = util.PtrIf(media.Description)

	if media.Error != 0 {
		// Set media error string.
		api.Error = new(string)
		*api.Error = media.Error.String()
		return api
	}

	if media.URL == "" {
		// If the URL isn't set yet (which it *should* be),
		// this likely indicates a bug with placeholder being
		// returned for still-processing media. Set an error
		// so it gets filtered out with placeholder text.
		const errText = "still processing"
		api.Error = new(string)
		*api.Error = errText
		return api
	}

	// Allocate media metadata object.
	api.Meta = new(apimodel.MediaMeta)

	// Set file focus details.
	// (this doesn't make much sense if media
	// has no image, but the API doesn't yet
	// distinguish between zero values vs. none).
	api.Meta.Focus = new(apimodel.MediaFocus)
	api.Meta.Focus.X = media.FileMeta.Focus.X
	api.Meta.Focus.Y = media.FileMeta.Focus.Y

	// If the URL is set, either the file is currently
	// processing, or is successfully stored locally.
	api.TextURL = util.Ptr(media.URL)
	api.URL = api.TextURL

	// Only add file details if we have any stored.
	if media.FileMeta.Original != zeroOriginal {
		api.Meta.Original = apimodel.MediaDimensions{
			Width:     media.FileMeta.Original.Width,
			Height:    media.FileMeta.Original.Height,
			Aspect:    media.FileMeta.Original.Aspect,
			Size:      toAPISize(media.FileMeta.Original.Width, media.FileMeta.Original.Height),
			FrameRate: toAPIFrameRate(media.FileMeta.Original.Framerate),
			Duration:  util.PtrOrZero(media.FileMeta.Original.Duration),
			Bitrate:   util.PtrOrZero(media.FileMeta.Original.Bitrate),
		}
	}

	if media.Thumbnail.URL != "" {
		if api.Meta == nil {
			// Allocate media metadata object.
			api.Meta = new(apimodel.MediaMeta)

			// Set file focus details.
			// (this doesn't make much sense if media
			// has no image, but the API doesn't yet
			// distinguish between zero values vs. none).
			api.Meta.Focus = new(apimodel.MediaFocus)
			api.Meta.Focus.X = media.FileMeta.Focus.X
			api.Meta.Focus.Y = media.FileMeta.Focus.Y
		}

		// If thumbnail URL is set, either the file is
		// currently processing, or is stored locally.
		api.PreviewURL = util.Ptr(media.Thumbnail.URL)

		// Only add details if we have any stored.
		if media.FileMeta.Small != zeroSmall {
			api.Meta.Small = apimodel.MediaDimensions{
				Width:  media.FileMeta.Small.Width,
				Height: media.FileMeta.Small.Height,
				Aspect: media.FileMeta.Small.Aspect,
				Size:   toAPISize(media.FileMeta.Small.Width, media.FileMeta.Small.Height),
			}
		}
	}

	return api
}

// MentionToAPIMention converts a gts model mention into its api (frontend) representation for serialization on the API.
func (c *Converter) MentionToAPIMention(ctx context.Context, mention *gtsmodel.Mention) (apimodel.Mention, error) {
	if mention.TargetAccount == nil {
		var err error

		mention.TargetAccount, err = c.state.DB.GetAccountByID(ctx, mention.TargetAccountID)
		if err != nil {
			return apimodel.Mention{}, gtserror.Newf("db error getting mention target: %w", err)
		}
	}

	var acct string
	if mention.TargetAccount.IsLocal() {
		if mention.TargetAccount.IsInstance() {
			acct = mention.TargetAccount.Username
		} else {
			acct = mention.TargetAccount.Username + "@" + config.GetAccountDomain()
		}
	} else {
		// Domain may be in Punycode, de-punify it just in case.
		d, err := util.DePunify(mention.TargetAccount.Domain)
		if err != nil {
			return apimodel.Mention{}, gtserror.Newf("error de-punifying mention target %s domain: %w", mention.TargetAccount.UsernameDomain(), err)
		}

		// Set the de-punified username@domain combo.
		acct = mention.TargetAccount.Username + "@" + d
	}

	return apimodel.Mention{
		ID:       mention.TargetAccount.ID,
		Username: mention.TargetAccount.Username,
		URL:      mention.TargetAccount.URL,
		Acct:     acct,
	}, nil
}

// EmojiToAPIEmoji converts a gts model emoji into its api (frontend) representation for serialization on the API.
func (c *Converter) EmojiToAPIEmoji(ctx context.Context, emoji *gtsmodel.Emoji) (apimodel.Emoji, error) {
	var category string

	if emoji.CategoryID != "" {
		var err error

		emoji.Category, err = c.state.DB.GetEmojiCategory(ctx, emoji.CategoryID)
		if err != nil {
			return apimodel.Emoji{}, gtserror.Newf("db error getting emoji %s category: %w", emoji.ShortcodeDomain(), err)
		}

		category = emoji.Category.Name
	}

	return apimodel.Emoji{
		Shortcode:       emoji.Shortcode,
		URL:             emoji.ImageURL,
		StaticURL:       emoji.ImageStaticURL,
		VisibleInPicker: *emoji.VisibleInPicker,
		Category:        category,
	}, nil
}

// EmojiToAdminAPIEmoji converts a gts model emoji into an API representation with extra admin information.
func (c *Converter) EmojiToAdminAPIEmoji(ctx context.Context, emoji *gtsmodel.Emoji) (*apimodel.AdminEmoji, error) {
	apiEmoji, err := c.EmojiToAPIEmoji(ctx, emoji)
	if err != nil {
		return nil, err
	}

	if !emoji.IsLocal() {
		// Domain may be in Punycode,
		// de-punify it just in case.
		var err error
		emoji.Domain, err = util.DePunify(emoji.Domain)
		if err != nil {
			return nil, gtserror.Newf("error de-punifying emoji %s domain: %w", emoji.ShortcodeDomain(), err)
		}
	}

	return &apimodel.AdminEmoji{
		Emoji:         apiEmoji,
		ID:            emoji.ID,
		Disabled:      *emoji.Disabled,
		Domain:        emoji.Domain,
		UpdatedAt:     util.FormatISO8601(emoji.UpdatedAt),
		TotalFileSize: emoji.ImageFileSize + emoji.ImageStaticFileSize,
		ContentType:   emoji.ImageContentType,
		URI:           emoji.URI,
	}, nil
}

// EmojiCategoryToAPIEmojiCategory converts a gts model emoji category into its api (frontend) representation.
func EmojiCategoryToAPIEmojiCategory(category *gtsmodel.EmojiCategory) *apimodel.EmojiCategory {
	return &apimodel.EmojiCategory{
		ID:   category.ID,
		Name: category.Name,
	}
}

// TagToAPITag converts a gts model tag into its api (frontend) representation for serialization on the API.
// If stubHistory is set to 'true', then the 'history' field of the tag will be populated with a pointer to an empty slice, for API compatibility reasons.
// following is an optional flag marking whether the currently authenticated user (if there is one) is following the tag.
func TagToAPITag(tag *gtsmodel.Tag, stubHistory bool, following *bool) apimodel.Tag {
	return apimodel.Tag{
		Name: strings.ToLower(tag.Name),
		URL:  uris.URIForTag(tag.Name),
		History: func() *[]any {
			if !stubHistory {
				return nil
			}

			h := make([]any, 0)
			return &h
		}(),
		Following: following,
	}
}

// StatusToAPIStatus converts a gts model
// status into its api (frontend) representation
// for serialization on the API.
//
// Requesting account can be nil.
func (c *Converter) StatusToAPIStatus(
	ctx context.Context,
	status *gtsmodel.Status,
	requestingAccount *gtsmodel.Account,
) (*apimodel.Status, error) {
	return c.statusToAPIStatus(
		ctx,
		status,
		requestingAccount,
		true, // addPendingNote
	)
}

// statusToAPIStatus is the package-internal implementation
// of StatusToAPIStatus that lets the caller customize whether
// to placehold unknown attachment types, and/or add a note
// about the status being pending and requiring approval.
func (c *Converter) statusToAPIStatus(
	ctx context.Context,
	status *gtsmodel.Status,
	requestingAccount *gtsmodel.Account,
	addPendingNote bool,
) (*apimodel.Status, error) {

	// previously used to be a function
	// argument but we only ever set true.
	const placeholdAttachments = true

	// Pass status to core conversion function.
	apiStatus, err := c.statusToFrontend(ctx,
		status,
		requestingAccount, // Can be nil.
	)
	if err != nil {
		return nil, err
	}

	// Convert status author account to frontend API model.
	apiStatus.Account, err = c.AccountToAPIAccountPublic(ctx,
		status.Account)
	if err != nil {
		return nil, gtserror.Newf("error converting status author: %w", err)
	}

	// Convert author of boosted
	// status (if set) to API model.
	if apiStatus.Reblog != nil {
		apiStatus.Reblog.Account, err = c.AccountToAPIAccountPublic(ctx, status.BoostOfAccount)
		if err != nil {
			return nil, gtserror.Newf("error converting boost author: %w", err)
		}
	}

	if placeholdAttachments {
		var attachNote string

		// Normalize status for API by pruning attachments
		// that were not able to be locally stored, and replacing
		// them with a helpful message + links to remote.
		attachNote, apiStatus.MediaAttachments = placeholderAttachments(apiStatus.MediaAttachments)
		apiStatus.Content += attachNote

		// Do the same for the reblogged status.
		if apiStatus.Reblog != nil {
			attachNote, apiStatus.Reblog.MediaAttachments = placeholderAttachments(apiStatus.Reblog.MediaAttachments)
			apiStatus.Reblog.Content += attachNote
		}
	}

	if addPendingNote {
		// If this status is pending approval and
		// replies to the requester, add a note
		// about how to approve or reject the reply.
		if status.Flags.PendingApproval() &&
			requestingAccount != nil &&
			requestingAccount.ID == status.InReplyToAccountID {
			pendingNote, err := c.pendingReplyNote(ctx, status)
			if err != nil {
				return nil, gtserror.Newf("error deriving 'pending reply' note: %w", err)
			}
			apiStatus.Content += pendingNote
		}
	}

	return apiStatus, nil
}

// StatusToWebStatus converts a gts model status into an
// api representation suitable for serving into a web template.
//
// Requesting account can be nil.
func (c *Converter) StatusToWebStatus(
	ctx context.Context,
	s *gtsmodel.Status,
) (*apimodel.WebStatus, error) {
	apiStatus, err := c.statusToFrontend(ctx, s,
		nil, // No authed requester.
	)
	if err != nil {
		return nil, err
	}

	// Convert status author to web model.
	acct, err := c.AccountToWebAccount(ctx, s.Account, nil)
	if err != nil {
		return nil, err
	}

	webStatus := &apimodel.WebStatus{
		Status:         apiStatus,
		SpoilerContent: s.ContentWarning,
		Account:        acct,
	}

	// If this is a boost, set Reblog on it
	// and return early, we only care about
	// the boost + boosting account.
	if s.BoostOf != nil {
		reblog, err := c.StatusToWebStatus(ctx, s.BoostOf)
		if err != nil {
			return nil, err
		}
		webStatus.Reblog = &apimodel.WebStatusReblogged{reblog}
		return webStatus, nil
	}

	// Whack a newline before and after each "pre" to make it easier to outdent it.
	webStatus.Content = strings.ReplaceAll(webStatus.Content, "<pre>", "\n<pre>")
	webStatus.Content = strings.ReplaceAll(webStatus.Content, "</pre>", "</pre>\n")
	webStatus.SpoilerContent = strings.ReplaceAll(webStatus.SpoilerContent, "<pre>", "\n<pre>")
	webStatus.SpoilerContent = strings.ReplaceAll(webStatus.SpoilerContent, "</pre>", "</pre>\n")

	// Add additional information for template.
	// Assume empty langs, hope for not empty language.
	webStatus.LanguageTag = new(language.Language)
	if lang := webStatus.Language; lang != nil {
		langTag, err := language.Parse(*lang)
		if err != nil {
			log.Warnf(
				ctx,
				"error parsing %s as language tag: %v",
				*lang, err,
			)
		} else {
			webStatus.LanguageTag = langTag
		}
	}

	if poll := webStatus.Poll; poll != nil {
		// Calculate vote share of each poll option and
		// format them for easier template consumption.
		totalVotes := poll.VotesCount

		PollOptions := make([]apimodel.WebPollOption, len(poll.Options))
		for i, option := range poll.Options {
			var voteShare float32

			if totalVotes != 0 && option.VotesCount != nil {
				voteShare = float32(*option.VotesCount) / float32(totalVotes) * 100
			}

			// Format to two decimal points and ditch any
			// trailing zeroes.
			//
			// We want to be precise enough that eg., "1.54%"
			// is distinct from "1.68%" in polls with loads
			// of votes.
			//
			// However, if we've got eg., a two-option poll
			// in which each option has half the votes, then
			// "50%" looks better than "50.00%".
			//
			// By the same token, it's pointless to show
			// "0.00%" or "100.00%".
			voteShareStr := fmt.Sprintf("%.2f", voteShare)
			voteShareStr = strings.TrimSuffix(voteShareStr, ".00")

			webPollOption := apimodel.WebPollOption{
				PollOption:   option,
				PollID:       poll.ID,
				Emojis:       webStatus.Emojis,
				LanguageTag:  webStatus.LanguageTag,
				VoteShare:    voteShare,
				VoteShareStr: voteShareStr,
			}
			PollOptions[i] = webPollOption
		}

		webStatus.PollOptions = PollOptions
	}

	// Mark local.
	webStatus.Local = s.Flags.Local()

	// Get edit history for this
	// status, if it's been edited.
	if webStatus.EditedAt != nil {
		// Make sure edits are populated.
		if len(s.Edits) != len(s.EditIDs) {
			s.Edits, err = c.state.DB.GetStatusEditsByIDs(ctx, s.EditIDs)
			if err != nil && !errors.Is(err, db.ErrNoEntries) {
				err := gtserror.Newf("db error getting status edits: %w", err)
				return nil, err
			}
		}

		// Include each historical entry
		// (this includes the created date).
		for _, edit := range s.Edits {
			webStatus.EditTimeline = append(
				webStatus.EditTimeline,
				util.FormatISO8601(edit.CreatedAt),
			)
		}

		// End with latest revision.
		webStatus.EditTimeline = append(
			webStatus.EditTimeline,
			*webStatus.EditedAt,
		)

		// Reverse the slice so that instead of going
		// from oldest (original status) to newest
		// (latest revision), it goes from newest
		// to oldest, like a timeline, to make
		// things easier when web templating.
		//
		// It'll look something like:
		//
		//	- edit3 date (ie., latest version)
		//	- edit2 date (if we have it)
		//	- edit1 date (if we have it)
		//	- created date
		slices.Reverse(webStatus.EditTimeline)
	}

	// Set additional templating
	// variables on media attachments.

	// Get gtsmodel attachments
	// into a convenient map.
	ogAttachments := make(
		map[string]*gtsmodel.MediaAttachment,
		len(s.Attachments),
	)
	for _, a := range s.Attachments {
		ogAttachments[a.ID] = a
	}

	// Convert each API attachment
	// into a web attachment.
	webStatus.MediaAttachments = make(
		[]*apimodel.WebAttachment,
		len(apiStatus.MediaAttachments),
	)
	for i, apiAttachment := range apiStatus.MediaAttachments {
		ogAttachment := ogAttachments[apiAttachment.ID]
		webStatus.MediaAttachments[i] = &apimodel.WebAttachment{
			Attachment:       apiAttachment,
			Sensitive:        apiStatus.Sensitive,
			MIMEType:         ogAttachment.File.ContentType,
			PreviewMIMEType:  ogAttachment.Thumbnail.ContentType,
			ParentStatusLink: apiStatus.URL,
		}
	}

	return webStatus, nil
}

// statusToFrontend is a package internal function for
// parsing a status into its initial frontend representation.
//
// Requesting account can be nil.
//
// This function also doesn't handle converting the
// account to api/web model -- the caller must do that.
func (c *Converter) statusToFrontend(
	ctx context.Context,
	status *gtsmodel.Status,
	requestingAccount *gtsmodel.Account,
) (
	*apimodel.Status,
	error,
) {
	apiStatus, err := c.baseStatusToFrontend(ctx,
		status,
		requestingAccount,
	)
	if err != nil {
		return nil, err
	}

	if status.BoostOf != nil {
		reblog, err := c.baseStatusToFrontend(ctx,
			status.BoostOf,
			requestingAccount,
		)
		if err != nil {
			return nil, gtserror.Newf("error converting boosted status: %w", err)
		}

		// Set boosted status and set interactions and filter results from original.
		apiStatus.Reblog = &apimodel.StatusReblogged{reblog}
		apiStatus.Favourited = apiStatus.Reblog.Favourited
		apiStatus.Bookmarked = apiStatus.Reblog.Bookmarked
		apiStatus.Muted = apiStatus.Reblog.Muted
		apiStatus.Reblogged = apiStatus.Reblog.Reblogged
		apiStatus.Pinned = apiStatus.Reblog.Pinned
		apiStatus.Filtered = apiStatus.Reblog.Filtered
	}

	return apiStatus, nil
}

// baseStatusToFrontend performs the main logic
// of statusToFrontend() without handling of boost
// logic, to prevent *possible* recursion issues.
//
// This function also doesn't handle converting the
// account to api/web model -- the caller must do that.
func (c *Converter) baseStatusToFrontend(
	ctx context.Context,
	status *gtsmodel.Status,
	requester *gtsmodel.Account,
) (
	*apimodel.Status,
	error,
) {
	// Try to populate status struct pointer fields.
	// We can continue in many cases of partial failure,
	// but there are some fields we actually need.
	if err := c.state.DB.PopulateStatus(ctx, status); err != nil {
		switch {
		case status.Account == nil:
			return nil, gtserror.Newf("error(s) populating status, required account not set: %w", err)

		case status.BoostOfID != "" && status.BoostOf == nil:
			return nil, gtserror.Newf("error(s) populating status, required boost not set: %w", err)

		default:
			log.Errorf(ctx, "error(s) populating status, will continue: %v", err)
		}
	}

	// Check whether status is deleted,
	// if so return a placeholder model.
	//
	// It's up to callers outside the typeutils
	// package to decide if they want to return
	// deleted statuses, or instead return 404.
	if status.Flags.Deleted() {

		// Note that the account will get populated as
		// normal for this status. We only show deleted
		// statuses as part of threads, which would reveal
		// the original author through mentions so there's
		// no point in stubbing out the deleted's author.
		return toDeletedStatusPlaceholder(status), nil
	}

	// Check if domain is limited, as this
	// will affect how we have to serialize it.
	limit, err := c.state.DB.MatchDomainLimit(ctx,
		status.Account.Domain)
	if err != nil {
		return nil, gtserror.Newf("error matching domain limit: %w", err)
	}

	// Build content warning for this post.
	contentWarning := status.ContentWarning
	if limit != nil && limit.ContentWarning != "" {
		if contentWarning == "" {
			// Set content warning
			// to limit content warning.
			contentWarning = limit.ContentWarning
		} else {
			// Prepend content warning
			// with limit content warning.
			contentWarning = limit.ContentWarning + "; " + contentWarning
		}
	}

	// Post is sensitive if there's a content
	// warning, or it's explicitly marked as
	// sensitive, or a domain limit says so.
	sensitive := contentWarning != "" ||
		status.Flags.Sensitive() ||
		limit.MediaMarkSensitive()

	repliesCount, err := c.state.DB.CountStatusReplies(ctx, status.ID)
	if err != nil {
		return nil, gtserror.Newf("error counting replies: %w", err)
	}

	reblogsCount, err := c.state.DB.CountStatusBoosts(ctx, status.ID)
	if err != nil {
		return nil, gtserror.Newf("error counting reblogs: %w", err)
	}

	favesCount, err := c.state.DB.CountStatusFaves(ctx, status.ID)
	if err != nil {
		return nil, gtserror.Newf("error counting faves: %w", err)
	}

	apiAttachments := c.attachmentsToAPI(ctx, status.Attachments, status.AttachmentIDs)

	apiEmojis := c.emojisToAPI(ctx, status.Emojis, status.EmojiIDs)

	apiMentions := c.mentionsToAPI(ctx, status.Mentions, status.MentionIDs)

	apiTags := c.tagsToAPI(ctx, status.Tags, status.TagIDs)

	// Take status's interaction policy, or
	// fall back to default for its visibility.
	var p *gtsmodel.InteractionPolicy
	if p = status.InteractionPolicy; p == nil {
		p = gtsmodel.DefaultInteractionPolicyFor(status.Visibility)
	}

	apiInteractionPolicy, err := c.InteractionPolicyToAPIInteractionPolicy(ctx, p, status, requester)
	if err != nil {
		return nil, gtserror.Newf("error converting interaction policy: %w", err)
	}

	apiStatus := &apimodel.Status{
		ID:                 status.ID,
		CreatedAt:          util.FormatISO8601(status.CreatedAt),
		InReplyToID:        nil, // Set below.
		InReplyToAccountID: nil, // Set below.
		Sensitive:          sensitive,
		Visibility:         VisToAPIVis(status.Visibility),
		LocalOnly:          status.LocalOnly(),
		Language:           nil, // Set below.
		URI:                status.URI,
		URL:                status.URL,
		RepliesCount:       repliesCount,
		ReblogsCount:       reblogsCount,
		FavouritesCount:    favesCount,
		Content:            status.Content,
		Reblog:             nil, // Set below.
		Application:        nil, // Set below.
		Account:            nil, // Caller must do this.
		MediaAttachments:   apiAttachments,
		Mentions:           apiMentions,
		Tags:               apiTags,
		Emojis:             apiEmojis,
		Card:               c.previewCardForStatus(ctx, status, sensitive),
		Text:               status.Text,
		ContentType:        ContentTypeToAPIContentType(status.ContentType),
		InteractionPolicy:  apiInteractionPolicy,

		// Mastodon API says spoiler_text should be *text*, not HTML, so
		// parse any HTML back to plaintext when serializing via the API,
		// attempting to preserve semantic intent to keep it readable.
		SpoilerText: text.ParseHTMLToPlain(contentWarning),
	}

	if at := status.EditedAt; !at.IsZero() {
		timestamp := util.FormatISO8601(at)
		apiStatus.EditedAt = util.Ptr(timestamp)
	}

	apiStatus.InReplyToID = util.PtrIf(status.InReplyToID)
	apiStatus.InReplyToAccountID = util.PtrIf(status.InReplyToAccountID)
	apiStatus.Language = util.PtrIf(status.Language)

	switch {
	case status.CreatedWithApplication != nil:
		// App exists for this status and is set.
		apiStatus.Application = AppToAPIAppPublic(status.CreatedWithApplication)

	case status.CreatedWithApplicationID != "":
		// App existed for this status but not
		// anymore, it's probably been cleaned up.
		// Set a dummy application.
		apiStatus.Application = &apimodel.Application{
			Name: "unknown application",
		}

	default:
		// No app stored for this (probably remote)
		// status, so nothing to do (app is optional).
	}

	if status.Poll != nil {
		// Set originating
		// status on the poll.
		poll := status.Poll
		poll.Status = status

		apiStatus.Poll, err = c.PollToAPIPoll(ctx, requester, poll)
		if err != nil {
			return nil, gtserror.Newf("error converting poll: %w", err)
		}
	}

	// Status interactions.
	//
	if status.BoostOf != nil { //nolint
		// populated *outside* this
		// function to prevent recursion.
	} else {
		interacts, err := c.interactionsWithStatusForAccount(ctx, status, requester)
		if err != nil {
			log.Errorf(ctx,
				"error getting interactions for status %s for account %s: %v",
				status.URI, requester.URI, err,
			)
		}
		apiStatus.Favourited = interacts.Favourited
		apiStatus.Bookmarked = interacts.Bookmarked
		apiStatus.Muted = interacts.Muted
		apiStatus.Reblogged = interacts.Reblogged
		apiStatus.Pinned = interacts.Pinned
	}

	// If web URL is empty for whatever
	// reason, provide AP URI as fallback.
	if apiStatus.URL == "" {
		apiStatus.URL = apiStatus.URI
	}

	return apiStatus, nil
}

// StatusToEditHistory converts a status and its historical edits
// (if any) to a slice of API model status edits, ordered from original
// status at index 0 to latest revision at index len(slice)-1.
func (c *Converter) StatusToEditHistory(
	ctx context.Context,
	status *gtsmodel.Status,
) ([]*apimodel.StatusEdit, error) {
	var media map[string]*gtsmodel.MediaAttachment

	// Gather attachments of status AND edits.
	attachmentIDs := status.AllAttachmentIDs()
	if len(attachmentIDs) > 0 {

		// Fetch all of the gathered status attachments from the database.
		attachments, err := c.state.DB.GetAttachmentsByIDs(ctx, attachmentIDs)
		if err != nil {
			return nil, gtserror.Newf("error getting attachments from db: %w", err)
		}

		// Generate a lookup map in 'media' of status attachments by their IDs.
		media = util.KeyBy(attachments, func(m *gtsmodel.MediaAttachment) string {
			return m.ID
		})
	}

	// Convert the status author account to API model.
	apiAccount, err := c.AccountToAPIAccountPublic(ctx,
		status.Account)
	if err != nil {
		return nil, gtserror.Newf("error converting account: %w", err)
	}

	// Convert status emojis to their API models, this includes all
	// emojis both current and historic, so it gets passed to each edit.
	apiEmojis := c.emojisToAPI(ctx, status.Emojis, status.EmojiIDs)

	var votes []int
	var options []string

	if status.Poll != nil {
		// Extract status poll options.
		options = status.Poll.Options

		// Show votes only if closed / allowed.
		if !status.Poll.ClosedAt.IsZero() ||
			!*status.Poll.HideCounts {
			votes = status.Poll.Votes
		}
	}

	// Append *current* version of the status to last slot
	// in the edits so we can add it at the bottom as latest
	// revision using the below loop. Note: a new slice is
	// created here with the append, to avoid modifying Edits on status.
	edits := append(status.Edits, &gtsmodel.StatusEdit{ //nolint:gocritic
		Content:                status.Content,
		ContentWarning:         status.ContentWarning,
		Sensitive:              util.Ptr(status.Flags.Sensitive()),
		PollOptions:            options,
		PollVotes:              votes,
		AttachmentIDs:          status.AttachmentIDs,
		AttachmentDescriptions: nil,                // no change from current
		CreatedAt:              status.UpdatedAt(), // falls back to creation
	})

	// Iterate through status revisions, starting at original
	// status when it was created (ie., oldest revision).
	//
	// This creates a slice of revisions that goes from
	// oldest (original status) to newest (latest revision).
	apiEdits := make([]*apimodel.StatusEdit, len(edits))
	if len(apiEdits) != len(edits) {
		panic(gtserror.New("bound check elimination"))
	}
	for i, edit := range edits {

		// Iterate through edit attachment IDs, getting model from 'media' lookup.
		apiAttachments := make([]*apimodel.Attachment, 0, len(edit.AttachmentIDs))
		for _, id := range edit.AttachmentIDs {
			attachment, ok := media[id]
			if !ok {
				continue
			}

			// Convert and append each media attachment to slice.
			apiAttachment := AttachmentToAPIAttachment(attachment)
			if apiAttachment.Error != nil {

				// Only include attachment if it's loadable and usable.
				apiAttachments = append(apiAttachments, &apiAttachment)
			}
		}

		// If media descriptions are set, update API model descriptions.
		if len(edit.AttachmentIDs) == len(edit.AttachmentDescriptions) {
			var j int
			for i, id := range edit.AttachmentIDs {
				descr := edit.AttachmentDescriptions[i]
				for ; j < len(apiAttachments); j++ {
					if apiAttachments[j].ID == id {
						apiAttachments[j].Description = &descr
						break
					}
				}
			}
		}

		// Attach status poll if set.
		var apiPoll *apimodel.Poll
		if len(edit.PollOptions) > 0 {
			apiPoll = new(apimodel.Poll)

			// Iterate through poll options and attach each to API poll model.
			apiPoll.Options = make([]apimodel.PollOption, len(edit.PollOptions))
			if len(apiPoll.Options) != len(edit.PollOptions) {
				panic(gtserror.New("bound check elimination"))
			}
			for i, option := range edit.PollOptions {
				apiPoll.Options[i] = apimodel.PollOption{
					Title: option,
				}
			}

			// If poll votes are attached, set vote counts.
			if len(edit.PollVotes) == len(apiPoll.Options) {
				for i, votes := range edit.PollVotes {
					apiPoll.Options[i].VotesCount = &votes
				}
			}
		}

		// Set status edit on return slice.
		apiEdits[i] = &apimodel.StatusEdit{
			CreatedAt:        util.FormatISO8601(edit.CreatedAt),
			Content:          edit.Content,
			SpoilerText:      edit.ContentWarning,
			Sensitive:        util.PtrOrZero(edit.Sensitive),
			Account:          apiAccount,
			Poll:             apiPoll,
			MediaAttachments: apiAttachments,
			Emojis:           apiEmojis, // same models used for whole status + all edits
		}
	}

	return apiEdits, nil
}

// VisToAPIVis converts a gts visibility into its api equivalent
func VisToAPIVis(m gtsmodel.Visibility) apimodel.Visibility {
	switch m {
	case gtsmodel.VisibilityPublic:
		return apimodel.VisibilityPublic
	case gtsmodel.VisibilityUnlocked:
		return apimodel.VisibilityUnlisted
	case gtsmodel.VisibilityFollowersOnly, gtsmodel.VisibilityMutualsOnly:
		return apimodel.VisibilityPrivate
	case gtsmodel.VisibilityDirect:
		return apimodel.VisibilityDirect
	case gtsmodel.VisibilityNone:
		return apimodel.VisibilityNone
	}
	return ""
}

// Converts a gts status content type into its api equivalent
func ContentTypeToAPIContentType(m gtsmodel.StatusContentType) apimodel.StatusContentType {
	switch m {
	case gtsmodel.StatusContentTypePlain:
		return apimodel.StatusContentTypePlain
	case gtsmodel.StatusContentTypeMarkdown:
		return apimodel.StatusContentTypeMarkdown
	}
	return ""
}

// InstanceRuleToAdminAPIRule converts a local instance rule into its api equivalent for serving at /api/v1/admin/instance/rules/:id
func InstanceRuleToAPIRule(r gtsmodel.Rule) apimodel.InstanceRule {
	return apimodel.InstanceRule{
		ID:   r.ID,
		Text: r.Text,
	}
}

// InstanceRulesToAPIRules converts all local instance rules into their api equivalent for serving at /api/v1/instance/rules
func InstanceRulesToAPIRules(r []gtsmodel.Rule) []apimodel.InstanceRule {
	rules := make([]apimodel.InstanceRule, len(r))
	for i, v := range r {
		rules[i] = InstanceRuleToAPIRule(v)
	}
	return rules
}

// InstanceRuleToAdminAPIRule converts a local instance rule into its api equivalent for serving at /api/v1/admin/instance/rules/:id
func InstanceRuleToAdminAPIRule(r *gtsmodel.Rule) *apimodel.AdminInstanceRule {
	return &apimodel.AdminInstanceRule{
		ID:        r.ID,
		CreatedAt: util.FormatISO8601(r.CreatedAt),
		UpdatedAt: util.FormatISO8601(r.UpdatedAt),
		Text:      r.Text,
	}
}

// InstanceSettingsToAPIV1Instance converts our instance settings
// into its api equivalent for serving at /api/v1/instance.
func (c *Converter) InstanceSettingsToAPIV1Instance(
	ctx context.Context,
	settings *gtsmodel.InstanceSettings,
) (*apimodel.InstanceV1, error) {
	var (
		host       = config.GetHost()
		acctDomain = cmp.Or(config.GetAccountDomain(), host)
	)

	// Depunify the account domain.
	if dp, err := util.DePunify(acctDomain); err == nil {
		acctDomain = dp
	}

	instance := &apimodel.InstanceV1{
		URI:                  host,
		AccountDomain:        acctDomain,
		Title:                settings.Title,
		Description:          settings.Description,
		DescriptionText:      settings.DescriptionText,
		CustomCSS:            settings.CustomCSS,
		ShortDescription:     settings.ShortDescription,
		ShortDescriptionText: settings.ShortDescriptionText,
		Email:                settings.ContactEmail,
		Version:              config.GetSoftwareVersion(),
		Languages:            config.GetInstanceLanguages().TagStrs(),
		Registrations:        config.GetAccountsRegistrationOpen(),
		ApprovalRequired:     true,                               // approval always required
		InvitesEnabled:       false,                              // todo: not supported yet
		MaxTootChars:         uint(config.GetStatusesMaxChars()), // #nosec G115 -- Already validated.
		Rules:                InstanceRulesToAPIRules(settings.Rules),
		Terms:                settings.Terms,
		TermsRaw:             settings.TermsText,
	}

	if config.GetInstanceInjectMastodonVersion() {
		instance.Version = toMastodonVersion(instance.Version)
	}

	if debug.DEBUG {
		instance.Debug = util.Ptr(true)
	}

	// Instance configuration.
	instance.Configuration.Statuses.MaxCharacters = config.GetStatusesMaxChars()
	instance.Configuration.Statuses.MaxMediaAttachments = config.GetStatusesMediaMaxFiles()
	instance.Configuration.Statuses.CharactersReservedPerURL = instanceStatusesCharactersReservedPerURL
	instance.Configuration.Statuses.SupportedMimeTypes = instanceStatusesSupportedMimeTypes
	instance.Configuration.MediaAttachments.SupportedMimeTypes = media.SupportedMIMETypes

	// NOTE: we use the local max sizes here
	// as it hints to apps like Tusky for image
	// compression of locally uploaded media.
	//
	// TODO: return local / remote depending
	// on authorized endpoint user (if any)?
	localMax := config.GetMediaLocalMaxSize()
	imageSz := cmp.Or(config.GetMediaImageSizeHint(), localMax)
	videoSz := cmp.Or(config.GetMediaVideoSizeHint(), localMax)
	instance.Configuration.MediaAttachments.ImageSizeLimit = int(imageSz) // #nosec G115 -- Already validated.
	instance.Configuration.MediaAttachments.VideoSizeLimit = int(videoSz) // #nosec G115 -- Already validated.

	// We don't actually set any limits on these. Set to max possible.
	instance.Configuration.MediaAttachments.ImageMatrixLimit = math.MaxInt32
	instance.Configuration.MediaAttachments.VideoFrameRateLimit = math.MaxInt32
	instance.Configuration.MediaAttachments.VideoMatrixLimit = math.MaxInt32

	instance.Configuration.Polls.MaxOptions = config.GetStatusesPollMaxOptions()
	instance.Configuration.Polls.MaxCharactersPerOption = config.GetStatusesPollOptionMaxChars()
	instance.Configuration.Polls.MinExpiration = instancePollsMinExpiration
	instance.Configuration.Polls.MaxExpiration = instancePollsMaxExpiration
	instance.Configuration.Accounts.AllowCustomCSS = config.GetAccountsAllowCustomCSS()
	instance.Configuration.Accounts.MaxFeaturedTags = instanceAccountsMaxFeaturedTags
	instance.Configuration.Accounts.MaxProfileFields = config.GetAccountsMaxProfileFields()
	instance.Configuration.Emojis.EmojiSizeLimit = int(config.GetMediaEmojiLocalMaxSize()) // #nosec G115 -- Already validated.
	instance.Configuration.OIDCEnabled = config.GetOIDCEnabled()

	// URLs
	instance.URLs.StreamingAPI = "wss://" + host

	// Populate instance statistics.
	stats := make(map[string]*int, 3)
	userCount, err := c.state.DB.CountInstanceAccounts(ctx)
	if err != nil {
		return nil, gtserror.Newf("db error getting counting instance users: %w", err)
	}
	stats["user_count"] = util.Ptr(userCount)

	statusCount, err := c.state.DB.CountInstanceStatuses(ctx)
	if err != nil {
		return nil, gtserror.Newf("db error getting counting instance statuses: %w", err)
	}
	stats["status_count"] = util.Ptr(statusCount)

	domainCount, err := c.state.DB.CountInstancePeers(ctx)
	if err != nil {
		return nil, gtserror.Newf("db error getting counting instance domains: %w", err)
	}
	stats["domain_count"] = util.Ptr(domainCount)
	instance.Stats = stats

	if config.GetInstanceStatsMode() == config.InstanceStatsModeBaffle {
		// Whack random stats on the instance to be used
		// by handlers in internal/api/client/instance.
		instance.RandomStats = c.RandomStats()
	}

	// Instance thumbnail.
	iAccount, err := c.state.DB.GetInstanceAccount(ctx, "")
	if err != nil {
		return nil, gtserror.Newf("db error getting instance account: %w", err)
	}

	if iAccount.AvatarMediaAttachmentID != "" {
		// Use instance account's
		// avatar as thumbnail, if set.
		if iAccount.AvatarMediaAttachment == nil {
			avi, err := c.state.DB.GetAttachmentByID(ctx, iAccount.AvatarMediaAttachmentID)
			if err != nil {
				return nil, gtserror.Newf("error getting instance avatar attachment: %w", err)
			}
			iAccount.AvatarMediaAttachment = avi
		}

		instance.Thumbnail = iAccount.AvatarMediaAttachment.URL
		instance.ThumbnailType = iAccount.AvatarMediaAttachment.File.ContentType
		instance.ThumbnailStatic = iAccount.AvatarMediaAttachment.Thumbnail.URL
		instance.ThumbnailStaticType = iAccount.AvatarMediaAttachment.Thumbnail.ContentType
		instance.ThumbnailDescription = iAccount.AvatarMediaAttachment.Description
	} else {
		// Fall back to default thumbnail.
		instance.Thumbnail = config.GetProtocol() + "://" + host + "/assets/logo.webp"
	}

	// Contact account, if set.
	if settings.ContactAccountID != "" {
		if settings.ContactAccount == nil {
			contactAccount, err := c.state.DB.GetAccountByID(ctx, settings.ContactAccountID)
			if err != nil {
				return nil, gtserror.Newf("db error getting instance contact account: %w", err)
			}
			settings.ContactAccount = contactAccount
		}

		account, err := c.AccountToAPIAccountPublic(ctx, settings.ContactAccount)
		if err != nil {
			return nil, gtserror.Newf("error converting instance contact account: %w", err)
		}
		instance.ContactAccount = account
	}

	return instance, nil
}

// InstanceSettingsToAPIV2Instance converts our instance settings
// into its api equivalent for serving at /api/v2/instance.
func (c *Converter) InstanceSettingsToAPIV2Instance(
	ctx context.Context,
	settings *gtsmodel.InstanceSettings,
) (*apimodel.InstanceV2, error) {
	var (
		host       = config.GetHost()
		acctDomain = cmp.Or(config.GetAccountDomain(), host)
	)

	// Depunify the account domain.
	if dp, err := util.DePunify(acctDomain); err == nil {
		acctDomain = dp
	}

	instance := &apimodel.InstanceV2{
		Domain:          host,
		AccountDomain:   acctDomain,
		Title:           settings.Title,
		Version:         config.GetSoftwareVersion(),
		SourceURL:       instanceSourceURL,
		Description:     settings.Description,
		DescriptionText: settings.DescriptionText,
		CustomCSS:       settings.CustomCSS,
		Usage:           apimodel.InstanceV2Usage{}, // todo: not implemented
		Languages:       config.GetInstanceLanguages().TagStrs(),
		Rules:           InstanceRulesToAPIRules(settings.Rules),
		Terms:           settings.Terms,
		TermsText:       settings.TermsText,
	}

	if config.GetInstanceInjectMastodonVersion() {
		instance.Version = toMastodonVersion(instance.Version)
	}

	if debug.DEBUG {
		instance.Debug = util.Ptr(true)
	}

	if config.GetInstanceStatsMode() == config.InstanceStatsModeBaffle {
		// Whack random stats on the instance to be used
		// by handlers in internal/api/client/instance.
		instance.RandomStats = c.RandomStats()
	}

	// Instance thumbnail.
	iAccount, err := c.state.DB.GetInstanceAccount(ctx, "")
	if err != nil {
		return nil, gtserror.Newf("db error getting instance account: %w", err)
	}

	if iAccount.AvatarMediaAttachmentID != "" {
		// Use instance account's
		// avatar as thumbnail, if set.
		if iAccount.AvatarMediaAttachment == nil {
			avi, err := c.state.DB.GetAttachmentByID(ctx, iAccount.AvatarMediaAttachmentID)
			if err != nil {
				return nil, gtserror.Newf("error getting instance avatar attachment: %w", err)
			}
			iAccount.AvatarMediaAttachment = avi
		}

		instance.Thumbnail.URL = iAccount.AvatarMediaAttachment.URL
		instance.Thumbnail.Type = iAccount.AvatarMediaAttachment.File.ContentType
		instance.Thumbnail.StaticURL = iAccount.AvatarMediaAttachment.Thumbnail.URL
		instance.Thumbnail.StaticType = iAccount.AvatarMediaAttachment.Thumbnail.ContentType
		instance.Thumbnail.Description = iAccount.AvatarMediaAttachment.Description
		instance.Thumbnail.Blurhash = iAccount.AvatarMediaAttachment.Blurhash
	} else {
		// Fall back to default thumbnail.
		instance.Thumbnail.URL = config.GetProtocol() + "://" + host + "/assets/logo.webp"
	}

	termsOfService := config.GetProtocol() + "://" + host + "/about#rules"

	// Instance configuration.
	instance.Configuration.URLs.Streaming = "wss://" + host
	instance.Configuration.URLs.About = config.GetProtocol() + "://" + host + "/about"
	instance.Configuration.URLs.TermsOfService = &termsOfService
	instance.Configuration.Statuses.MaxCharacters = config.GetStatusesMaxChars()
	instance.Configuration.Statuses.MaxMediaAttachments = config.GetStatusesMediaMaxFiles()
	instance.Configuration.Statuses.CharactersReservedPerURL = instanceStatusesCharactersReservedPerURL
	instance.Configuration.Statuses.SupportedMimeTypes = instanceStatusesSupportedMimeTypes
	instance.Configuration.MediaAttachments.SupportedMimeTypes = media.SupportedMIMETypes
	instance.Configuration.MediaAttachments.DescriptionLimit = config.GetMediaDescriptionMaxChars()
	instance.Configuration.MediaAttachments.DescriptionMinimum = config.GetMediaDescriptionMinChars()

	// NOTE: we use the local max sizes here
	// as it hints to apps like Tusky for image
	// compression of locally uploaded media.
	//
	// TODO: return local / remote depending
	// on authorized endpoint user (if any)?
	localMax := config.GetMediaLocalMaxSize()
	imageSz := cmp.Or(config.GetMediaImageSizeHint(), localMax)
	videoSz := cmp.Or(config.GetMediaVideoSizeHint(), localMax)
	instance.Configuration.MediaAttachments.ImageSizeLimit = int(imageSz) // #nosec G115 -- Already validated.
	instance.Configuration.MediaAttachments.VideoSizeLimit = int(videoSz) // #nosec G115 -- Already validated.

	// We don't actually set any limits on these. Set to max possible.
	instance.Configuration.MediaAttachments.ImageMatrixLimit = math.MaxInt32
	instance.Configuration.MediaAttachments.VideoFrameRateLimit = math.MaxInt32
	instance.Configuration.MediaAttachments.VideoMatrixLimit = math.MaxInt32

	instance.Configuration.Polls.MaxOptions = config.GetStatusesPollMaxOptions()
	instance.Configuration.Polls.MaxCharactersPerOption = config.GetStatusesPollOptionMaxChars()
	instance.Configuration.Polls.MinExpiration = instancePollsMinExpiration
	instance.Configuration.Polls.MaxExpiration = instancePollsMaxExpiration
	instance.Configuration.Accounts.AllowCustomCSS = config.GetAccountsAllowCustomCSS()
	instance.Configuration.Accounts.MaxFeaturedTags = instanceAccountsMaxFeaturedTags
	instance.Configuration.Accounts.MaxProfileFields = config.GetAccountsMaxProfileFields()
	instance.Configuration.Emojis.EmojiSizeLimit = int(config.GetMediaEmojiLocalMaxSize()) // #nosec G115 -- Already validated.
	instance.Configuration.OIDCEnabled = config.GetOIDCEnabled()

	vapidKeyPair, err := c.state.DB.GetVAPIDKeyPair(ctx)
	if err != nil {
		return nil, gtserror.Newf("error getting VAPID public key: %w", err)
	}
	instance.Configuration.VAPID.PublicKey = vapidKeyPair.Public

	// registrations
	instance.Registrations.Enabled = config.GetAccountsRegistrationOpen()
	instance.Registrations.ApprovalRequired = true // always required
	instance.Registrations.Message = nil           // todo: not implemented
	instance.Registrations.MinAge = nil            // not implemented
	instance.Registrations.ReasonRequired = config.GetAccountsReasonRequired()

	// Contact email.
	instance.Contact.Email = settings.ContactEmail

	// Contact account, if set.
	if settings.ContactAccountID != "" {
		if settings.ContactAccount == nil {
			contactAccount, err := c.state.DB.GetAccountByID(ctx, settings.ContactAccountID)
			if err != nil {
				return nil, gtserror.Newf("db error getting instance contact account: %w", err)
			}
			settings.ContactAccount = contactAccount
		}

		account, err := c.AccountToAPIAccountPublic(ctx, settings.ContactAccount)
		if err != nil {
			return nil, gtserror.Newf("error converting instance contact account: %w", err)
		}
		instance.Contact.Account = account
	}

	return instance, nil
}

// RelationshipToAPIRelationship converts a gts relationship into its api equivalent for serving in various places
func (c *Converter) RelationshipToAPIRelationship(ctx context.Context, r *gtsmodel.Relationship) (*apimodel.Relationship, error) {
	return &apimodel.Relationship{
		ID:                  r.ID,
		Following:           r.Following,
		ShowingReblogs:      r.ShowingReblogs,
		Notifying:           r.Notifying,
		FollowedBy:          r.FollowedBy,
		Blocking:            r.Blocking,
		BlockedBy:           r.BlockedBy,
		Muting:              r.Muting,
		MutingNotifications: r.MutingNotifications,
		Requested:           r.Requested,
		RequestedBy:         r.RequestedBy,
		DomainBlocking:      r.DomainBlocking,
		Endorsed:            r.Endorsed,
		Note:                r.Note,
	}, nil
}

// NotificationToAPINotification converts a gts notification into a api notification
func (c *Converter) NotificationToAPINotification(
	ctx context.Context,
	notif *gtsmodel.Notification,
) (*apimodel.Notification, error) {
	// Ensure notif populated.
	if err := c.state.DB.PopulateNotification(ctx, notif); err != nil {
		return nil, gtserror.Newf("error populating notification: %w", err)
	}

	// Get account that triggered this notif.
	apiAccount, err := c.AccountToAPIAccountPublic(ctx, notif.OriginAccount)
	if err != nil {
		return nil, gtserror.Newf("error converting account to api: %w", err)
	}

	// Get status that triggered this notif, if set.
	var apiStatus *apimodel.Status
	if notif.Status != nil {
		apiStatus, err = c.StatusToAPIStatus(ctx,
			notif.Status,
			notif.TargetAccount,
		)
		if err != nil {
			return nil, gtserror.Newf("error converting status to api: %w", err)
		}

		if apiStatus.Reblog != nil {
			// Use the actual reblog status
			// for the notifications endpoint.
			apiStatus = apiStatus.Reblog.Status
		}
	}

	return &apimodel.Notification{
		ID:        notif.ID,
		Type:      notif.NotificationType.String(),
		CreatedAt: util.FormatISO8601(notif.CreatedAt),
		Account:   apiAccount,
		Status:    apiStatus,
	}, nil
}

// ConversationToAPIConversation converts
// a conversation into its API representation.
func (c *Converter) ConversationToAPIConversation(
	ctx context.Context,
	conversation *gtsmodel.Conversation,
	requester *gtsmodel.Account,
) (*apimodel.Conversation, error) {
	apiConversation := &apimodel.Conversation{
		ID:     conversation.ID,
		Unread: !*conversation.Read,
	}

	// Populate most recent status in convo;
	// can be nil if this status is filtered.
	if conversation.LastStatus != nil {
		var err error
		apiConversation.LastStatus, err = c.StatusToAPIStatus(
			ctx,
			conversation.LastStatus,
			requester,
		)
		if err != nil {
			return nil, gtserror.Newf(
				"error converting status %s to API representation: %w",
				conversation.LastStatus.ID,
				err,
			)
		}
	}

	// If no other accounts are involved in this convo,
	// just include the requesting account and return.
	//
	// See: https://codeberg.org/superseriousbusiness/gotosocial/issues/3385#issuecomment-2394033477
	otherAcctsLen := len(conversation.OtherAccounts)
	if otherAcctsLen == 0 {
		apiAcct, err := c.AccountToAPIAccountPublic(ctx, requester)
		if err != nil {
			err := gtserror.Newf(
				"error converting account %s to API representation: %w",
				requester.ID, err,
			)
			return nil, err
		}

		apiConversation.Accounts = []apimodel.Account{*apiAcct}
		return apiConversation, nil
	}

	// Other accounts are involved in the
	// convo. Convert each to API model.
	apiConversation.Accounts = make([]apimodel.Account, otherAcctsLen)
	for i, account := range conversation.OtherAccounts {
		blocked, err := c.state.DB.IsEitherBlocked(ctx,
			requester.ID, account.ID,
		)
		if err != nil {
			err := gtserror.Newf(
				"db error checking blocks between accounts %s and %s: %w",
				requester.ID, account.ID, err,
			)
			return nil, err
		}

		// API account model varies depending
		// on status of conversation participant.
		var apiAcct *apimodel.Account
		if blocked || account.IsSuspended() {
			apiAcct, err = c.AccountToAPIAccountBlocked(ctx, account)
		} else {
			apiAcct, err = c.AccountToAPIAccountPublic(ctx, account)
		}

		if err != nil {
			err := gtserror.Newf(
				"error converting account %s to API representation: %w",
				account.ID, err,
			)
			return nil, err
		}

		apiConversation.Accounts[i] = *apiAcct
	}

	return apiConversation, nil
}

// DomainPermToAPIDomainPerm converts a gtsmodel domain block,
// allow, draft, or ignore into an api domain permission.
func (c *Converter) DomainPermToAPIDomainPerm(
	ctx context.Context,
	d gtsmodel.DomainPermission,
	export bool,
) (*apimodel.DomainPermission, error) {
	// Domain may be in Punycode,
	// de-punify it just in case.
	domain, err := util.DePunify(d.GetDomain())
	if err != nil {
		return nil, gtserror.Newf("error de-punifying domain %s: %w", d.GetDomain(), err)
	}

	domainPerm := &apimodel.DomainPermission{
		Domain: apimodel.Domain{
			Domain:        domain,
			PublicComment: util.Ptr(d.GetPublicComment()),
		},
	}

	// If we're exporting, provide
	// only bare minimum detail.
	if export {
		return domainPerm, nil
	}

	domainPerm.ID = d.GetID()
	domainPerm.Obfuscate = d.GetObfuscate()
	domainPerm.PrivateComment = util.Ptr(d.GetPrivateComment())
	domainPerm.SubscriptionID = d.GetSubscriptionID()
	domainPerm.CreatedBy = d.GetCreatedByAccountID()
	if createdAt := d.GetCreatedAt(); !createdAt.IsZero() {
		domainPerm.CreatedAt = util.FormatISO8601(createdAt)
	}

	// If this is a draft, also add the permission type.
	if _, ok := d.(*gtsmodel.DomainPermissionDraft); ok {
		domainPerm.PermissionType = d.GetType().String()
	}

	return domainPerm, nil
}

func (c *Converter) DomainPermSubToAPIDomainPermSub(
	ctx context.Context,
	d *gtsmodel.DomainPermissionSubscription,
) (*apimodel.DomainPermissionSubscription, error) {
	createdAt, err := id.TimeFromULID(d.ID)
	if err != nil {
		return nil, gtserror.Newf("error converting id to time: %w", err)
	}

	// URI may be in Punycode,
	// de-punify it just in case.
	uri, err := util.DePunify(d.URI)
	if err != nil {
		return nil, gtserror.Newf("error de-punifying URI %s: %w", d.URI, err)
	}

	var (
		fetchedAt             string
		successfullyFetchedAt string
	)

	if !d.FetchedAt.IsZero() {
		fetchedAt = util.FormatISO8601(d.FetchedAt)
	}

	if !d.SuccessfullyFetchedAt.IsZero() {
		successfullyFetchedAt = util.FormatISO8601(d.SuccessfullyFetchedAt)
	}

	count, err := c.state.DB.CountDomainPermissionSubscriptionPerms(ctx, d.ID)
	if err != nil {
		return nil, gtserror.Newf("error counting perm sub perms: %w", err)
	}

	return &apimodel.DomainPermissionSubscription{
		ID:                    d.ID,
		Priority:              d.Priority,
		Title:                 d.Title,
		PermissionType:        d.PermissionType.String(),
		AsDraft:               *d.AsDraft,
		AdoptOrphans:          *d.AdoptOrphans,
		RemoveRetracted:       *d.RemoveRetracted,
		CreatedBy:             d.CreatedByAccountID,
		CreatedAt:             util.FormatISO8601(createdAt),
		URI:                   uri,
		ContentType:           d.ContentType.String(),
		FetchUsername:         d.FetchUsername,
		FetchPassword:         d.FetchPassword,
		FetchedAt:             fetchedAt,
		SuccessfullyFetchedAt: successfullyFetchedAt,
		Error:                 d.Error,
		Count:                 uint64(count), // #nosec G115 -- Don't care about overflow here.
	}, nil
}

// ReportToAPIReport converts a gts model report into an api model report, for serving at /api/v1/reports
func (c *Converter) ReportToAPIReport(ctx context.Context, r *gtsmodel.Report) (*apimodel.Report, error) {
	report := &apimodel.Report{
		ID:          r.ID,
		CreatedAt:   util.FormatISO8601(r.CreatedAt),
		ActionTaken: !r.ActionTakenAt.IsZero(),
		Category:    "other", // todo: only support default 'other' category right now
		Comment:     r.Comment,
		Forwarded:   *r.Forwarded,
		StatusIDs:   r.StatusIDs,
		RuleIDs:     r.RuleIDs,
	}

	if !r.ActionTakenAt.IsZero() {
		actionTakenAt := util.FormatISO8601(r.ActionTakenAt)
		report.ActionTakenAt = &actionTakenAt
	}

	if actionComment := r.ActionTaken; actionComment != "" {
		report.ActionTakenComment = &actionComment
	}

	if r.TargetAccount == nil {
		tAccount, err := c.state.DB.GetAccountByID(ctx, r.TargetAccountID)
		if err != nil {
			return nil, fmt.Errorf("ReportToAPIReport: error getting target account with id %s from the db: %s", r.TargetAccountID, err)
		}
		r.TargetAccount = tAccount
	}

	apiAccount, err := c.AccountToAPIAccountPublic(ctx, r.TargetAccount)
	if err != nil {
		return nil, fmt.Errorf("ReportToAPIReport: error converting target account to api: %s", err)
	}
	report.TargetAccount = apiAccount

	return report, nil
}

// ReportToAdminAPIReport converts a gts model report into an admin view report, for serving at /api/v1/admin/reports
func (c *Converter) ReportToAdminAPIReport(ctx context.Context, r *gtsmodel.Report, requestingAccount *gtsmodel.Account) (*apimodel.AdminReport, error) {
	var (
		err                  error
		actionTakenAt        *string
		actionTakenComment   *string
		actionTakenByAccount *apimodel.AdminAccountInfo
	)

	if !r.ActionTakenAt.IsZero() {
		ata := util.FormatISO8601(r.ActionTakenAt)
		actionTakenAt = &ata
	}

	if r.Account == nil {
		r.Account, err = c.state.DB.GetAccountByID(ctx, r.AccountID)
		if err != nil {
			return nil, fmt.Errorf("ReportToAdminAPIReport: error getting account with id %s from the db: %w", r.AccountID, err)
		}
	}
	account, err := c.AccountToAdminAPIAccount(ctx, r.Account)
	if err != nil {
		return nil, fmt.Errorf("ReportToAdminAPIReport: error converting account with id %s to adminAPIAccount: %w", r.AccountID, err)
	}

	if r.TargetAccount == nil {
		r.TargetAccount, err = c.state.DB.GetAccountByID(ctx, r.TargetAccountID)
		if err != nil {
			return nil, fmt.Errorf("ReportToAdminAPIReport: error getting target account with id %s from the db: %w", r.TargetAccountID, err)
		}
	}
	targetAccount, err := c.AccountToAdminAPIAccount(ctx, r.TargetAccount)
	if err != nil {
		return nil, fmt.Errorf("ReportToAdminAPIReport: error converting target account with id %s to adminAPIAccount: %w", r.TargetAccountID, err)
	}

	if r.ActionTakenByAccountID != "" {
		if r.ActionTakenByAccount == nil {
			r.ActionTakenByAccount, err = c.state.DB.GetAccountByID(ctx, r.ActionTakenByAccountID)
			if err != nil {
				return nil, fmt.Errorf("ReportToAdminAPIReport: error getting action taken by account with id %s from the db: %w", r.ActionTakenByAccountID, err)
			}
		}

		actionTakenByAccount, err = c.AccountToAdminAPIAccount(ctx, r.ActionTakenByAccount)
		if err != nil {
			return nil, fmt.Errorf("ReportToAdminAPIReport: error converting action taken by account with id %s to adminAPIAccount: %w", r.ActionTakenByAccountID, err)
		}
	}

	statuses := make([]*apimodel.Status, 0, len(r.StatusIDs))
	if len(r.StatusIDs) != 0 && len(r.Statuses) == 0 {
		r.Statuses, err = c.state.DB.GetStatusesByIDs(ctx, r.StatusIDs)
		if err != nil {
			return nil, fmt.Errorf("ReportToAdminAPIReport: error getting statuses from the db: %w", err)
		}
	}
	for _, s := range r.Statuses {
		status, err := c.statusToAPIStatus(ctx,
			s,
			requestingAccount,

			// Don't add note about
			// pending, it's not
			// relevant here.
			false,
		)
		if err != nil {
			return nil, fmt.Errorf("ReportToAdminAPIReport: error converting status with id %s to api status: %w", s.ID, err)
		}
		statuses = append(statuses, status)
	}

	rules := make([]*apimodel.InstanceRule, 0, len(r.RuleIDs))
	if len(r.RuleIDs) != 0 && len(r.Rules) == 0 {
		r.Rules, err = c.state.DB.GetRulesByIDs(ctx, r.RuleIDs)
		if err != nil {
			return nil, fmt.Errorf("ReportToAdminAPIReport: error getting rules from the db: %w", err)
		}
	}
	for _, v := range r.Rules {
		rules = append(rules, &apimodel.InstanceRule{
			ID:   v.ID,
			Text: v.Text,
		})
	}

	if ac := r.ActionTaken; ac != "" {
		actionTakenComment = &ac
	}

	return &apimodel.AdminReport{
		ID:                   r.ID,
		ActionTaken:          !r.ActionTakenAt.IsZero(),
		ActionTakenAt:        actionTakenAt,
		Category:             "other", // todo: only support default 'other' category right now
		Comment:              r.Comment,
		Forwarded:            *r.Forwarded,
		CreatedAt:            util.FormatISO8601(r.CreatedAt),
		UpdatedAt:            util.FormatISO8601(r.UpdatedAt),
		Account:              account,
		TargetAccount:        targetAccount,
		AssignedAccount:      actionTakenByAccount,
		ActionTakenByAccount: actionTakenByAccount,
		ActionTakenComment:   actionTakenComment,
		Statuses:             statuses,
		Rules:                rules,
	}, nil
}

// ListToAPIList converts one gts model list into an api model list, for serving at /api/v1/lists/{id}
func (c *Converter) ListToAPIList(ctx context.Context, l *gtsmodel.List) (*apimodel.List, error) {
	return &apimodel.List{
		ID:            l.ID,
		Title:         l.Title,
		RepliesPolicy: string(l.RepliesPolicy),
		Exclusive:     *l.Exclusive,
	}, nil
}

// MarkersToAPIMarker converts several gts model markers into an api marker, for serving at /api/v1/markers
func (c *Converter) MarkersToAPIMarker(ctx context.Context, markers []*gtsmodel.Marker) (*apimodel.Marker, error) {
	apiMarker := &apimodel.Marker{}
	for _, marker := range markers {
		apiTimelineMarker := &apimodel.TimelineMarker{
			LastReadID: marker.LastReadID,
			UpdatedAt:  util.FormatISO8601(marker.UpdatedAt),
			Version:    marker.Version,
		}
		switch apimodel.MarkerName(marker.Name) {
		case apimodel.MarkerNameHome:
			apiMarker.Home = apiTimelineMarker
		case apimodel.MarkerNameNotifications:
			apiMarker.Notifications = apiTimelineMarker
		default:
			return nil, fmt.Errorf("unknown marker timeline name: %s", marker.Name)
		}
	}
	return apiMarker, nil
}

// PollToAPIPoll converts a database (gtsmodel) Poll into an API model representation appropriate for the given requesting account.
func (c *Converter) PollToAPIPoll(ctx context.Context, requester *gtsmodel.Account, poll *gtsmodel.Poll) (*apimodel.Poll, error) {

	// Ensure the poll model is fully populated for src status.
	if err := c.state.DB.PopulatePoll(ctx, poll); err != nil {
		return nil, gtserror.Newf("error populating poll: %w", err)
	}

	var (
		options     []apimodel.PollOption
		totalVotes  int
		totalVoters *int
		hasVoted    *bool
		ownChoices  *[]int
		isAuthor    bool
		expiresAt   *string
	)

	// Preallocate a slice of frontend model poll choices.
	options = make([]apimodel.PollOption, len(poll.Options))
	if len(options) != len(poll.Options) {
		panic(gtserror.New("bound check elimination"))
	}

	// Add the titles to all of the options.
	for i, title := range poll.Options {
		options[i].Title = title
	}

	if requester != nil {
		// Get vote by requester in poll (if any).
		vote, err := c.state.DB.GetPollVoteBy(ctx,
			poll.ID,
			requester.ID,
		)
		if err != nil && !errors.Is(err, db.ErrNoEntries) {
			return nil, gtserror.Newf("error getting vote for poll %s: %w", poll.ID, err)
		}

		if vote != nil {
			// Set choices by requester.
			ownChoices = &vote.Choices

			// Update default total in the
			// case that counts are hidden
			// (so we just show our own).
			totalVotes = len(vote.Choices)
		} else {
			// Requester hasn't yet voted, use
			// empty slice to serialize as `[]`.
			ownChoices = &[]int{}
		}

		// Check if requester is author of source status.
		isAuthor = (requester.ID == poll.Status.AccountID)

		// Set whether requester has voted in poll (or = author).
		hasVoted = util.Ptr((isAuthor || len(*ownChoices) > 0))
	}

	if isAuthor || !*poll.HideCounts {
		// Only in the case that hide counts is
		// disabled, or the requester is the author
		// do we actually populate the vote counts.

		// If we voted in this poll, we'll have set totalVotes
		// earlier. Reset here to avoid double counting.
		totalVotes = 0
		if *poll.Multiple {
			// The total number of voters are only
			// provided in the case of a multiple
			// choice poll. All else leaves it nil.
			totalVoters = poll.Voters
		}

		// Populate per-vote counts
		// and overall total vote count.
		for i, count := range poll.Votes {
			if options[i].VotesCount == nil {
				options[i].VotesCount = new(int)
			}
			(*options[i].VotesCount) += count
			totalVotes += count
		}
	}

	if !poll.ExpiresAt.IsZero() {
		// Calculate poll expiry string (if set).
		str := util.FormatISO8601(poll.ExpiresAt)
		expiresAt = &str
	}

	// Get emojis from parent status.
	apiEmojis := c.emojisToAPI(ctx,
		poll.Status.Emojis,
		poll.Status.EmojiIDs,
	)

	return &apimodel.Poll{
		ID:          poll.ID,
		ExpiresAt:   expiresAt,
		Expired:     poll.Closed(),
		Multiple:    (*poll.Multiple),
		VotesCount:  totalVotes,
		VotersCount: totalVoters,
		Voted:       hasVoted,
		OwnVotes:    ownChoices,
		Options:     options,
		Emojis:      apiEmojis,
	}, nil
}

// FilterToAPIFiltersV1 converts one GTS model filter into an API v1 filter list
func FilterToAPIFiltersV1(filter *gtsmodel.Filter) []*apimodel.FilterV1 {
	apiFilters := make([]*apimodel.FilterV1, 0, len(filter.Keywords))
	for _, filterKeyword := range filter.Keywords {
		apiFilter := FilterKeywordToAPIFilterV1(filter, filterKeyword)
		apiFilters = append(apiFilters, apiFilter)
	}
	return apiFilters
}

// FilterKeywordToAPIFilterV1 converts one GTS model filter and filter keyword into an API v1 filter
func FilterKeywordToAPIFilterV1(filter *gtsmodel.Filter, keyword *gtsmodel.FilterKeyword) *apimodel.FilterV1 {
	return &apimodel.FilterV1{
		// v1 filters have a single keyword each, so we use the filter keyword ID as the v1 filter ID.
		ID:           keyword.ID,
		Phrase:       keyword.Keyword,
		Context:      filterToAPIFilterContexts(filter),
		WholeWord:    util.PtrOrValue(keyword.WholeWord, false),
		ExpiresAt:    filterExpiresAtToAPIFilterExpiresAt(filter.ExpiresAt),
		Irreversible: filter.Action == gtsmodel.FilterActionHide,
	}
}

// FilterToAPIFilterV2 converts one GTS model filter into an API v2 filter.
func FilterToAPIFilterV2(filter *gtsmodel.Filter) *apimodel.FilterV2 {
	apiFilterKeywords := make([]apimodel.FilterKeyword, len(filter.Keywords))
	if len(apiFilterKeywords) != len(filter.Keywords) {
		// bound check eliminiation compiler-hint
		panic(gtserror.New("BCE"))
	}
	for i, filterKeyword := range filter.Keywords {
		apiFilterKeywords[i] = apimodel.FilterKeyword{
			ID:        filterKeyword.ID,
			Keyword:   filterKeyword.Keyword,
			WholeWord: util.PtrOrValue(filterKeyword.WholeWord, false),
		}
	}
	apiFilterStatuses := make([]apimodel.FilterStatus, len(filter.Statuses))
	if len(apiFilterStatuses) != len(filter.Statuses) {
		// bound check eliminiation compiler-hint
		panic(gtserror.New("BCE"))
	}
	for i, filterStatus := range filter.Statuses {
		apiFilterStatuses[i] = apimodel.FilterStatus{
			ID:       filterStatus.ID,
			StatusID: filterStatus.StatusID,
		}
	}
	return &apimodel.FilterV2{
		ID:           filter.ID,
		Title:        filter.Title,
		Context:      filterToAPIFilterContexts(filter),
		ExpiresAt:    filterExpiresAtToAPIFilterExpiresAt(filter.ExpiresAt),
		FilterAction: filterActionToAPIFilterAction(filter.Action),
		Keywords:     apiFilterKeywords,
		Statuses:     apiFilterStatuses,
	}
}

func filterExpiresAtToAPIFilterExpiresAt(expiresAt time.Time) *string {
	if expiresAt.IsZero() {
		return nil
	}
	return util.Ptr(util.FormatISO8601(expiresAt))
}

func filterToAPIFilterContexts(filter *gtsmodel.Filter) []apimodel.FilterContext {
	apiContexts := make([]apimodel.FilterContext, 0, apimodel.FilterContextNumValues)
	if filter.Contexts.Home() {
		apiContexts = append(apiContexts, apimodel.FilterContextHome)
	}
	if filter.Contexts.Notifications() {
		apiContexts = append(apiContexts, apimodel.FilterContextNotifications)
	}
	if filter.Contexts.Public() {
		apiContexts = append(apiContexts, apimodel.FilterContextPublic)
	}
	if filter.Contexts.Thread() {
		apiContexts = append(apiContexts, apimodel.FilterContextThread)
	}
	if filter.Contexts.Account() {
		apiContexts = append(apiContexts, apimodel.FilterContextAccount)
	}
	return apiContexts
}

func filterActionToAPIFilterAction(m gtsmodel.FilterAction) apimodel.FilterAction {
	switch m {
	case gtsmodel.FilterActionWarn:
		return apimodel.FilterActionWarn
	case gtsmodel.FilterActionHide:
		return apimodel.FilterActionHide
	case gtsmodel.FilterActionBlur:
		return apimodel.FilterActionBlur
	}
	return apimodel.FilterActionNone
}

// FilterKeywordToAPIFilterKeyword converts a GTS model filter status into an API filter status.
func FilterKeywordToAPIFilterKeyword(filterKeyword *gtsmodel.FilterKeyword) *apimodel.FilterKeyword {
	return &apimodel.FilterKeyword{
		ID:        filterKeyword.ID,
		Keyword:   filterKeyword.Keyword,
		WholeWord: util.PtrOrValue(filterKeyword.WholeWord, false),
	}
}

// FilterStatusToAPIFilterStatus converts a GTS model filter status into an API filter status.
func FilterStatusToAPIFilterStatus(filterStatus *gtsmodel.FilterStatus) *apimodel.FilterStatus {
	return &apimodel.FilterStatus{
		ID:       filterStatus.ID,
		StatusID: filterStatus.StatusID,
	}
}

// ThemesToAPIThemes converts a slice of gtsmodel Themes into apimodel Themes.
func (c *Converter) ThemesToAPIThemes(themes []*gtsmodel.Theme) []apimodel.Theme {
	apiThemes := make([]apimodel.Theme, len(themes))
	for i, theme := range themes {
		apiThemes[i] = apimodel.Theme{
			Title:       theme.Title,
			Description: theme.Description,
			FileName:    theme.FileName,
		}
	}
	return apiThemes
}

// Convert the given gtsmodel policy
// into an apimodel interaction policy.
//
// Provided status can be nil to convert a
// policy without a particular status in mind,
// but ***if status is nil then sub-policies
// CanLike, CanReply, and CanAnnounce on
// the given policy must *not* be nil.***
//
// RequestingAccount can also be nil for
// unauthorized requests (web, public api etc).
func (c *Converter) InteractionPolicyToAPIInteractionPolicy(
	ctx context.Context,
	policy *gtsmodel.InteractionPolicy,
	status *gtsmodel.Status,
	requester *gtsmodel.Account,
) (
	apiPolicy apimodel.InteractionPolicy,
	err error,
) {
	// gtsmodel CanLike -> apimodel CanFavourite
	if policy.CanLike != nil {
		// Use the set CanLike value.
		apiPolicy.CanFavourite = apimodel.PolicyRules{
			AutomaticApproval: policyValsToAPIPolicyVals(policy.CanLike.AutomaticApproval),
			ManualApproval:    policyValsToAPIPolicyVals(policy.CanLike.ManualApproval),
		}
	} else {
		// Use default CanLike value for this vis.
		pCanLike := gtsmodel.DefaultCanLikeFor(status.Visibility)
		apiPolicy.CanFavourite = apimodel.PolicyRules{
			AutomaticApproval: policyValsToAPIPolicyVals(pCanLike.AutomaticApproval),
			ManualApproval:    policyValsToAPIPolicyVals(pCanLike.ManualApproval),
		}
	}

	// gtsmodel CanReply -> apimodel CanReply
	if policy.CanReply != nil {
		// Use the set CanReply value.
		apiPolicy.CanReply = apimodel.PolicyRules{
			AutomaticApproval: policyValsToAPIPolicyVals(policy.CanReply.AutomaticApproval),
			ManualApproval:    policyValsToAPIPolicyVals(policy.CanReply.ManualApproval),
		}
	} else {
		// Use default CanReply value for this vis.
		pCanReply := gtsmodel.DefaultCanReplyFor(status.Visibility)
		apiPolicy.CanReply = apimodel.PolicyRules{
			AutomaticApproval: policyValsToAPIPolicyVals(pCanReply.AutomaticApproval),
			ManualApproval:    policyValsToAPIPolicyVals(pCanReply.ManualApproval),
		}
	}

	// gtsmodel CanAnnounce -> apimodel CanReblog
	if policy.CanAnnounce != nil {
		// Use the set CanAnnounce value.
		apiPolicy.CanReblog = apimodel.PolicyRules{
			AutomaticApproval: policyValsToAPIPolicyVals(policy.CanAnnounce.AutomaticApproval),
			ManualApproval:    policyValsToAPIPolicyVals(policy.CanAnnounce.ManualApproval),
		}
	} else {
		// Use default CanAnnounce value for this vis.
		pCanAnnounce := gtsmodel.DefaultCanAnnounceFor(status.Visibility)
		apiPolicy.CanReblog = apimodel.PolicyRules{
			AutomaticApproval: policyValsToAPIPolicyVals(pCanAnnounce.AutomaticApproval),
			ManualApproval:    policyValsToAPIPolicyVals(pCanAnnounce.ManualApproval),
		}
	}

	if status == nil || requester == nil {
		// We're done here!
		return apiPolicy, nil
	}

	// Status and requester are both defined,
	// so we can add the "me" Value to the policy
	// for each interaction type, if applicable.

	likeable, err := c.intFilter.StatusLikeable(ctx, requester, status)
	if err != nil {
		return apiPolicy, gtserror.Newf("error checking status likeable by requester: %w", err)
	}

	if likeable.Permission == gtsmodel.PolicyPermissionAutomaticApproval {
		// We can do this!
		apiPolicy.CanFavourite.AutomaticApproval = append(
			apiPolicy.CanFavourite.AutomaticApproval,
			apimodel.PolicyValueMe,
		)
	} else if likeable.Permission == gtsmodel.PolicyPermissionManualApproval {
		// We can do this with approval.
		apiPolicy.CanFavourite.ManualApproval = append(
			apiPolicy.CanFavourite.ManualApproval,
			apimodel.PolicyValueMe,
		)
	}

	replyable, err := c.intFilter.StatusReplyable(ctx, requester, status)
	if err != nil {
		return apiPolicy, gtserror.Newf("error checking status replyable by requester: %w", err)
	}

	if replyable.Permission == gtsmodel.PolicyPermissionAutomaticApproval {
		// We can do this!
		apiPolicy.CanReply.AutomaticApproval = append(
			apiPolicy.CanReply.AutomaticApproval,
			apimodel.PolicyValueMe,
		)
	} else if replyable.Permission == gtsmodel.PolicyPermissionManualApproval {
		// We can do this with approval.
		apiPolicy.CanReply.ManualApproval = append(
			apiPolicy.CanReply.ManualApproval,
			apimodel.PolicyValueMe,
		)
	}

	boostable, err := c.intFilter.StatusBoostable(ctx, requester, status)
	if err != nil {
		return apiPolicy, gtserror.Newf("error checking status boostable by requester: %w", err)
	}

	if boostable.Permission == gtsmodel.PolicyPermissionAutomaticApproval {
		// We can do this!
		apiPolicy.CanReblog.AutomaticApproval = append(
			apiPolicy.CanReblog.AutomaticApproval,
			apimodel.PolicyValueMe,
		)
	} else if boostable.Permission == gtsmodel.PolicyPermissionManualApproval {
		// We can do this with approval.
		apiPolicy.CanReblog.ManualApproval = append(
			apiPolicy.CanReblog.ManualApproval,
			apimodel.PolicyValueMe,
		)
	}

	return apiPolicy, nil
}

func policyValsToAPIPolicyVals(vals gtsmodel.PolicyValues) []apimodel.PolicyValue {

	var (
		valsLen = len(vals)

		// Use a map to deduplicate added vals as we go.
		addedVals = make(map[apimodel.PolicyValue]struct{}, valsLen)

		// Vals we'll be returning.
		apiVals = make([]apimodel.PolicyValue, 0, valsLen)
	)

	for _, policyVal := range vals {
		switch policyVal {

		case gtsmodel.PolicyValueAuthor:
			// Author can do this.
			newVal := apimodel.PolicyValueAuthor
			if _, added := addedVals[newVal]; !added {
				apiVals = append(apiVals, newVal)
				addedVals[newVal] = struct{}{}
			}

		case gtsmodel.PolicyValueMentioned:
			// Mentioned can do this.
			newVal := apimodel.PolicyValueMentioned
			if _, added := addedVals[newVal]; !added {
				apiVals = append(apiVals, newVal)
				addedVals[newVal] = struct{}{}
			}

		case gtsmodel.PolicyValueMutuals:
			// Mutuals can do this.
			newVal := apimodel.PolicyValueMutuals
			if _, added := addedVals[newVal]; !added {
				apiVals = append(apiVals, newVal)
				addedVals[newVal] = struct{}{}
			}

		case gtsmodel.PolicyValueFollowing:
			// Following can do this.
			newVal := apimodel.PolicyValueFollowing
			if _, added := addedVals[newVal]; !added {
				apiVals = append(apiVals, newVal)
				addedVals[newVal] = struct{}{}
			}

		case gtsmodel.PolicyValueFollowers:
			// Followers can do this.
			newVal := apimodel.PolicyValueFollowers
			if _, added := addedVals[newVal]; !added {
				apiVals = append(apiVals, newVal)
				addedVals[newVal] = struct{}{}
			}

		case gtsmodel.PolicyValuePublic:
			// Public can do this.
			newVal := apimodel.PolicyValuePublic
			if _, added := addedVals[newVal]; !added {
				apiVals = append(apiVals, newVal)
				addedVals[newVal] = struct{}{}
			}

		default:
			// Specific URI of ActivityPub Actor.
			newVal := apimodel.PolicyValue(policyVal)
			if _, added := addedVals[newVal]; !added {
				apiVals = append(apiVals, newVal)
				addedVals[newVal] = struct{}{}
			}
		}
	}

	return apiVals
}

// InteractionReqToAPIInteractionReq converts the given *gtsmodel.InteractionRequest
// to an *apimodel.InteractionRequest, from the perspective of requestingAcct.
func (c *Converter) InteractionReqToAPIInteractionReq(
	ctx context.Context,
	req *gtsmodel.InteractionRequest,
	requestingAcct *gtsmodel.Account,
) (*apimodel.InteractionRequest, error) {
	// Ensure interaction request is populated.
	if err := c.state.DB.PopulateInteractionRequest(ctx, req); err != nil {
		err := gtserror.Newf("error populating: %w", err)
		return nil, err
	}

	interactingAcct, err := c.AccountToAPIAccountPublic(ctx, req.InteractingAccount)
	if err != nil {
		err := gtserror.Newf("error converting interacting acct: %w", err)
		return nil, err
	}

	interactedStatus, err := c.StatusToAPIStatus(ctx,
		req.TargetStatus,
		requestingAcct,
	)
	if err != nil {
		err := gtserror.Newf("error converting interacted status: %w", err)
		return nil, err
	}

	var reply *apimodel.Status
	if req.InteractionType == gtsmodel.InteractionReply && req.Reply != nil {
		reply, err = c.statusToAPIStatus(ctx,
			req.Reply,
			requestingAcct,

			// Don't add note about pending;
			// requester already knows it's
			// pending because they're looking
			// at the request right now.
			false,
		)
		if err != nil {
			err := gtserror.Newf("error converting reply: %w", err)
			return nil, err
		}
	}

	var acceptedAt string
	if req.IsAccepted() {
		acceptedAt = util.FormatISO8601(req.AcceptedAt)
	}

	var rejectedAt string
	if req.IsRejected() {
		rejectedAt = util.FormatISO8601(req.RejectedAt)
	}

	createdAt, err := id.TimeFromULID(req.ID)
	if err != nil {
		// Error already
		// nicely wrapped.
		return nil, err
	}

	return &apimodel.InteractionRequest{
		ID:         req.ID,
		Type:       req.InteractionType.String(),
		CreatedAt:  util.FormatISO8601(createdAt),
		Account:    interactingAcct,
		Status:     interactedStatus,
		Reply:      reply,
		AcceptedAt: acceptedAt,
		RejectedAt: rejectedAt,
	}, nil
}

func webPushNotificationPolicyToAPIWebPushNotificationPolicy(policy gtsmodel.WebPushNotificationPolicy) apimodel.WebPushNotificationPolicy {
	switch policy {
	case gtsmodel.WebPushNotificationPolicyAll:
		return apimodel.WebPushNotificationPolicyAll
	case gtsmodel.WebPushNotificationPolicyFollowed:
		return apimodel.WebPushNotificationPolicyFollowed
	case gtsmodel.WebPushNotificationPolicyFollower:
		return apimodel.WebPushNotificationPolicyFollower
	case gtsmodel.WebPushNotificationPolicyNone:
		return apimodel.WebPushNotificationPolicyNone
	}
	return ""
}

func (c *Converter) WebPushSubscriptionToAPIWebPushSubscription(
	ctx context.Context,
	subscription *gtsmodel.WebPushSubscription,
) (*apimodel.WebPushSubscription, error) {
	vapidKeyPair, err := c.state.DB.GetVAPIDKeyPair(ctx)
	if err != nil {
		return nil, gtserror.Newf("error getting VAPID key pair: %w", err)
	}

	return &apimodel.WebPushSubscription{
		ID:        subscription.ID,
		Endpoint:  subscription.Endpoint,
		ServerKey: vapidKeyPair.Public,
		Alerts: apimodel.WebPushSubscriptionAlerts{
			Follow:           subscription.NotificationFlags.Get(gtsmodel.NotificationFollow),
			FollowRequest:    subscription.NotificationFlags.Get(gtsmodel.NotificationFollowRequest),
			Favourite:        subscription.NotificationFlags.Get(gtsmodel.NotificationFavourite),
			Mention:          subscription.NotificationFlags.Get(gtsmodel.NotificationMention),
			Reblog:           subscription.NotificationFlags.Get(gtsmodel.NotificationReblog),
			Poll:             subscription.NotificationFlags.Get(gtsmodel.NotificationPoll),
			Status:           subscription.NotificationFlags.Get(gtsmodel.NotificationStatus),
			Update:           subscription.NotificationFlags.Get(gtsmodel.NotificationUpdate),
			AdminSignup:      subscription.NotificationFlags.Get(gtsmodel.NotificationAdminSignup),
			AdminReport:      subscription.NotificationFlags.Get(gtsmodel.NotificationAdminReport),
			PendingFavourite: subscription.NotificationFlags.Get(gtsmodel.NotificationPendingFave),
			PendingReply:     subscription.NotificationFlags.Get(gtsmodel.NotificationPendingReply),
			PendingReblog:    subscription.NotificationFlags.Get(gtsmodel.NotificationPendingReblog),
		},
		Policy:   webPushNotificationPolicyToAPIWebPushNotificationPolicy(subscription.Policy),
		Standard: true,
	}, nil
}

func (c *Converter) TokenToAPITokenInfo(ctx context.Context, token *gtsmodel.Token) (*apimodel.TokenInfo, error) {
	createdAt, err := id.TimeFromULID(token.ID)
	if err != nil {
		err := gtserror.Newf("error parsing time from token id: %w", err)
		return nil, err
	}

	var lastUsed string
	if !token.LastUsed.IsZero() {
		lastUsed = util.FormatISO8601(token.LastUsed)
	}

	application, err := c.state.DB.GetApplicationByClientID(ctx, token.ClientID)
	if err != nil {
		err := gtserror.Newf("db error getting application with client id %s: %w", token.ClientID, err)
		return nil, err
	}

	apiApplication := AppToAPIAppPublic(application)

	return &apimodel.TokenInfo{
		ID:          token.ID,
		Name:        token.Name,
		CreatedAt:   util.FormatISO8601(createdAt),
		LastUsed:    lastUsed,
		Scope:       token.Scope,
		Application: apiApplication,
	}, nil
}

func (c *Converter) ScheduledStatusToAPIScheduledStatus(ctx context.Context, status *gtsmodel.ScheduledStatus) (*apimodel.ScheduledStatus, error) {
	scheduledAt := util.FormatISO8601(status.ScheduledAt)

	apiScheduledStatus := &apimodel.ScheduledStatus{
		ID:          status.ID,
		ScheduledAt: scheduledAt,
		Params: &apimodel.ScheduledStatusParams{
			Text:          status.Text,
			MediaIDs:      status.MediaIDs,
			Sensitive:     *status.Sensitive,
			SpoilerText:   status.SpoilerText,
			Visibility:    VisToAPIVis(status.Visibility),
			InReplyToID:   status.InReplyToID,
			Language:      status.Language,
			ApplicationID: status.ApplicationID,
			LocalOnly:     *status.LocalOnly,
			ContentType:   apimodel.StatusContentType(status.ContentType),
			ScheduledAt:   nil,
		},
		MediaAttachments: c.attachmentsToAPI(ctx, status.MediaAttachments, status.MediaIDs),
	}

	if len(status.Poll.Options) > 1 {
		apiScheduledStatus.Params.Poll = &apimodel.ScheduledStatusParamsPoll{
			Options:    status.Poll.Options,
			ExpiresIn:  status.Poll.ExpiresIn,
			Multiple:   *status.Poll.Multiple,
			HideTotals: *status.Poll.HideTotals,
		}
	}

	if status.InteractionPolicy != nil {
		apiInteractionPolicy, err := c.InteractionPolicyToAPIInteractionPolicy(ctx, status.InteractionPolicy, nil, nil)
		if err != nil {
			return nil, gtserror.Newf("error converting interaction policy: %w", err)
		}
		apiScheduledStatus.Params.InteractionPolicy = &apiInteractionPolicy
	}

	return apiScheduledStatus, nil
}

func (c *Converter) DomainLimitToAPIDomainLimit(
	ctx context.Context,
	domainLimit *gtsmodel.DomainLimit,
) (*apimodel.DomainLimit, error) {

	// Domain may be in Punycode,
	// de-punify it just in case.
	domain, err := util.DePunify(domainLimit.Domain)
	if err != nil {
		return nil, gtserror.Newf("error de-punifying %s: %w", domainLimit.Domain, err)
	}

	// Derive created at time from ULID.
	createdAt, err := id.TimeFromULID(domainLimit.ID)
	if err != nil {
		err := gtserror.Newf("error converting time from id: %w", err)
		return nil, err
	}

	// Derive media policy.
	var mediaPolicy apimodel.MediaPolicy
	switch p := domainLimit.MediaPolicy; p {
	case gtsmodel.MediaPolicyNoAction:
		mediaPolicy = apimodel.MediaPolicyNoAction
	case gtsmodel.MediaPolicyMarkSensitive:
		mediaPolicy = apimodel.MediaPolicyMarkSensitive
	case gtsmodel.MediaPolicyReject:
		mediaPolicy = apimodel.MediaPolicyReject
	default:
		err := gtserror.Newf("unknown media policy %d", p)
		return nil, err
	}

	// Derive follows policy.
	var followsPolicy apimodel.FollowsPolicy
	switch p := domainLimit.FollowsPolicy; p {
	case gtsmodel.FollowsPolicyNoAction:
		followsPolicy = apimodel.FollowsPolicyNoAction
	case gtsmodel.FollowsPolicyManualApproval:
		followsPolicy = apimodel.FollowsPolicyManualApproval
	case gtsmodel.FollowsPolicyRejectNonMutual:
		followsPolicy = apimodel.FollowsPolicyRejectNonMutual
	case gtsmodel.FollowsPolicyRejectAll:
		followsPolicy = apimodel.FollowsPolicyRejectAll
	default:
		err := gtserror.Newf("unknown follows policy %d", p)
		return nil, err
	}

	// Derive statuses policy.
	var statusesPolicy apimodel.StatusesPolicy
	switch p := domainLimit.StatusesPolicy; p {
	case gtsmodel.StatusesPolicyNoAction:
		statusesPolicy = apimodel.StatusesPolicyNoAction
	case gtsmodel.StatusesPolicyFilterWarn:
		statusesPolicy = apimodel.StatusesPolicyFilterWarn
	case gtsmodel.StatusesPolicyFilterHide:
		statusesPolicy = apimodel.StatusesPolicyFilterHide
	default:
		err := gtserror.Newf("unknown accounts policy %d", p)
		return nil, err
	}

	// Derive accounts policy.
	var accountsPolicy apimodel.AccountsPolicy
	switch p := domainLimit.AccountsPolicy; p {
	case gtsmodel.AccountsPolicyNoAction:
		accountsPolicy = apimodel.AccountsPolicyNoAction
	case gtsmodel.AccountsPolicyMute:
		accountsPolicy = apimodel.AccountsPolicyMute
	default:
		err := gtserror.Newf("unknown accounts policy %d", p)
		return nil, err
	}

	return &apimodel.DomainLimit{
		ID:             domainLimit.ID,
		Domain:         domain,
		MediaPolicy:    mediaPolicy,
		FollowsPolicy:  followsPolicy,
		StatusesPolicy: statusesPolicy,
		AccountsPolicy: accountsPolicy,
		ContentWarning: domainLimit.ContentWarning,
		PublicComment:  util.PtrIf(domainLimit.PublicComment),
		PrivateComment: util.PtrIf(domainLimit.PrivateComment),
		CreatedBy:      domainLimit.CreatedByAccountID,
		CreatedAt:      util.FormatISO8601(createdAt),
	}, nil
}

func DomainLimitToAPIFilterV2(limit *gtsmodel.DomainLimit) *apimodel.FilterV2 {
	return FilterToAPIFilterV2(domainLimitToFilter(limit))
}

func domainLimitToFilter(limit *gtsmodel.DomainLimit) *gtsmodel.Filter {
	return &gtsmodel.Filter{
		ID:       limit.ID,
		Title:    "domain limit: " + limit.Domain,
		Action:   domainLimitStatusesPolicyToFilterAction(limit.StatusesPolicy),
		Contexts: gtsmodel.FilterContexts(gtsmodel.FilterContextHome | gtsmodel.FilterContextPublic | gtsmodel.FilterContextThread),
		Keywords: []*gtsmodel.FilterKeyword{{ID: limit.ID, FilterID: limit.ID, Keyword: limit.Domain, WholeWord: util.Ptr(false)}},
	}
}

func domainLimitStatusesPolicyToFilterAction(p gtsmodel.StatusesPolicy) gtsmodel.FilterAction {
	switch p {
	case gtsmodel.StatusesPolicyFilterWarn:
		return gtsmodel.FilterActionWarn
	case gtsmodel.StatusesPolicyFilterHide:
		return gtsmodel.FilterActionHide
	default:
		return gtsmodel.FilterActionNone
	}
}

// attachmentsToAPI converts database model media attachments (fetching
// using IDs if necessary) to frontend API attachment models. all errors
// are caught and logged, with the calling function name as a prefix.
func (c *Converter) attachmentsToAPI(
	ctx context.Context,
	attachments []*gtsmodel.MediaAttachment,
	attachmentIDs []string,
) []*apimodel.Attachment {

	// Check if media attachments are populated.
	if len(attachments) != len(attachmentIDs) {
		var err error

		// Media attachments are not populated, fetch from the database.
		attachments, err = c.state.DB.GetAttachmentsByIDs(ctx, attachmentIDs)
		if err != nil {

			log.Error(ctx, gtserror.NewfAt(3, "error getting media: %w", err))
			return []*apimodel.Attachment{}
		}
	}

	// Convert all db media attachments to slice of API models.
	apiModels := make([]*apimodel.Attachment, len(attachments))
	if len(apiModels) != len(attachments) {
		panic(gtserror.New("bound check elimination"))
	}
	for i, media := range attachments {
		apiModel := AttachmentToAPIAttachment(media)
		apiModels[i] = &apiModel
	}

	return apiModels
}

// emojisToAPI converts database model emojis (fetching using IDs if
// necessary) to frontend API emoji models. all errors are caught and
// logged, with the calling function name as a prefix.
func (c *Converter) emojisToAPI(
	ctx context.Context,
	emojis []*gtsmodel.Emoji,
	emojiIDs []string,
) []apimodel.Emoji {

	// Check if emojis are populated.
	if len(emojis) != len(emojiIDs) {
		var err error

		// Emojis are not populated, fetch from the database.
		emojis, err = c.state.DB.GetEmojisByIDs(ctx, emojiIDs)
		if err != nil {

			log.Error(ctx, gtserror.NewfAt(3, "error getting emojis: %w", err))
			return []apimodel.Emoji{}
		}
	}

	// Preallocate a biggest-case slice of frontend emojis.
	apiModels := make([]apimodel.Emoji, 0, len(emojis))
	for _, emoji := range emojis {

		// Convert each database emoji to API model.
		apiModel, err := c.EmojiToAPIEmoji(ctx, emoji)
		if err != nil {
			log.Error(ctx, gtserror.NewfAt(3, "error converting emoji %s: %w", emoji.ShortcodeDomain(), err))
			continue
		}

		// Append API model to the return slice.
		apiModels = append(apiModels, apiModel)
	}

	return apiModels
}

// mentionsToAPI converts database model mentions (fetching using IDs if
// necessary) to frontend API mention models. all errors are caught and
// logged, with the calling function name as a prefix.
func (c *Converter) mentionsToAPI(
	ctx context.Context,
	mentions []*gtsmodel.Mention,
	mentionIDs []string,
) []apimodel.Mention {

	// Check if mentions are populated.
	if len(mentions) != len(mentionIDs) {
		var err error

		// Mentions are not populated, fetch from the database.
		mentions, err = c.state.DB.GetMentions(ctx, mentionIDs)
		if err != nil {

			log.Error(ctx, gtserror.NewfAt(3, "error getting mentions: %w", err))
			return []apimodel.Mention{}
		}
	}

	// Preallocate a biggest-case slice of frontend mentions.
	apiModels := make([]apimodel.Mention, 0, len(mentions))
	for _, mention := range mentions {

		// Convert each database mention to frontend API model.
		apiModel, err := c.MentionToAPIMention(ctx, mention)
		if err != nil {
			log.Error(ctx, gtserror.NewfAt(3, "error converting mention %s: %w", mention.ID, err))
			continue
		}

		// Append API model to the return slice.
		apiModels = append(apiModels, apiModel)
	}

	return apiModels
}

// tagsToAPI converts database model tags (fetching using IDs if
// necessary) to frontend API tag models. all errors are caught
// and logged, with the calling function name as a prefix.
func (c *Converter) tagsToAPI(
	ctx context.Context,
	tags []*gtsmodel.Tag,
	tagIDs []string,
) []apimodel.Tag {

	// Check if mentions are populated.
	if len(tags) != len(tagIDs) {
		var err error

		// Tags not populated, fetch from database.
		tags, err = c.state.DB.GetTags(ctx, tagIDs)
		if err != nil {

			log.Error(ctx, gtserror.NewfAt(3, "error getting tags: %w", err))
			return []apimodel.Tag{}
		}
	}

	// Convert all db tags to slice of API models.
	apiModels := make([]apimodel.Tag, len(tags))
	if len(apiModels) != len(tags) {
		panic(gtserror.New("bound check elimination"))
	}
	for i, tag := range tags {
		apiModels[i] = TagToAPITag(tag, false, nil)
	}

	return apiModels
}

func (c *Converter) InstanceToAdminAPIInstance(ctx context.Context, i *gtsmodel.Instance) (*apimodel.AdminInstance, error) {
	firstSeen, err := id.TimeFromULID(i.ID)
	if err != nil {
		return nil, gtserror.Newf("error converting id to time: %w", err)
	}

	domain, err := util.DePunify(i.Domain)
	if err != nil {
		return nil, gtserror.Newf("error depunifying domain %s: %w", i.Domain, err)
	}

	var latestSuccessfulDelivery string
	if !i.LatestSuccessfulDelivery.IsZero() {
		latestSuccessfulDelivery = util.FormatISO8601(i.LatestSuccessfulDelivery)
	}

	deliveryErrors := make([]apimodel.AdminInstanceDeliveryError, 0)
	for _, dErr := range i.DeliveryErrors {
		errTime, err := id.TimeFromULID(dErr.ID)
		if err != nil {
			return nil, gtserror.Newf("error converting id to time: %w", err)
		}
		deliveryErrors = append(
			deliveryErrors,
			apimodel.AdminInstanceDeliveryError{
				Time:  util.FormatISO8601(errTime),
				Error: dErr.Error,
			},
		)
	}

	// TODO: add accounts count, statuses
	// count, following relationships, etc.

	return &apimodel.AdminInstance{
		ID:                       i.ID,
		Domain:                   domain,
		Software:                 i.Software,
		FirstSeen:                util.FormatISO8601(firstSeen),
		LatestSuccessfulDelivery: latestSuccessfulDelivery,
		DeliveryErrors:           deliveryErrors,
	}, nil
}

// toDeletedStatusPlaceholder returns a placeholder for a deleted status model.
func toDeletedStatusPlaceholder(status *gtsmodel.Status) *apimodel.Status {
	apiStatus := &apimodel.Status{
		ID:                 status.ID,
		URI:                status.URI,
		URL:                status.URL,
		CreatedAt:          util.FormatISO8601(status.CreatedAt),
		InReplyToID:        util.PtrIf(status.InReplyToID),
		InReplyToAccountID: util.PtrIf(status.InReplyToAccountID),
		Visibility:         VisToAPIVis(status.Visibility),
		LocalOnly:          status.LocalOnly(),
		Content:            "<p><em>ℹ️ deleted status ℹ️</em></p>",
		Mentions:           []apimodel.Mention{},
		Tags:               []apimodel.Tag{},
		MediaAttachments:   []*apimodel.Attachment{},
		Emojis:             []apimodel.Emoji{},
	}
	apiStatus.InReplyToID = util.PtrIf(status.InReplyToID)
	apiStatus.InReplyToAccountID = util.PtrIf(status.InReplyToAccountID)
	if apiStatus.URL == "" {
		apiStatus.URL = apiStatus.URI
	}
	return apiStatus
}

func (c *Converter) RelayConnectionToAPIRelayConnection(
	ctx context.Context,
	rc gtsmodel.RelayConnection,
) (*apimodel.RelayConnection, error) {

	// Populate matchers on the connection,
	// and get account ID (if necessary).
	var accountID string
	switch t := rc.(type) {

	case *gtsmodel.RelaySubscription:
		if err := c.state.DB.PopulateRelaySubscription(ctx, t); err != nil {
			return nil, gtserror.Newf("error populating relay subscription: %w", err)
		}
		// Set account ID only if this is a relay
		// subscription, otherwise it doesn't matter.
		accountID = t.AccountID

	case *gtsmodel.RelayPush:
		if err := c.state.DB.PopulateRelayPush(ctx, t); err != nil {
			return nil, gtserror.Newf("error populating relay push: %w", err)
		}
	}

	// Parse created at time from id.
	rcID := rc.GetID()
	createdAt, err := id.TimeFromULID(rcID)
	if err != nil {
		return nil, gtserror.Newf("error converting id to time: %w", err)
	}

	// Convert matchers (if set).
	matchers := rc.GetMatchers()
	apiMatchers := make([]apimodel.RelayMatcher, 0, len(matchers))
	for _, matcher := range matchers {
		apiMatchers = append(apiMatchers, apimodel.RelayMatcher{
			ID:        matcher.ID,
			Keyword:   matcher.Keyword,
			WholeWord: matcher.Flags.WholeWord(),
			Exclude:   matcher.Flags.Exclude(),
		})
	}

	// Get our own instance account.
	iAcct, err := c.state.DB.GetInstanceAccount(ctx, "")
	if err != nil {
		return nil, gtserror.Newf("db error getting instance account: %w", err)
	}

	// Get the relay actor account.
	relayActorURI := rc.GetRelayActorURI()
	relayActorAcct, err := c.state.DB.GetAccountByURI(ctx, relayActorURI)
	if err != nil {
		return nil, gtserror.Newf("db error getting relay actor account: %w", err)
	}

	// Relay connection is approved if our account follows their account.
	approved, err := c.state.DB.IsFollowing(ctx, iAcct.ID, relayActorAcct.ID)
	if err != nil {
		return nil, gtserror.Newf("db error checking follow: %w", err)
	}

	flags := rc.GetFlags()
	return &apimodel.RelayConnection{
		ID:              rcID,
		CreatedAt:       util.FormatISO8601(createdAt),
		AccountID:       accountID,
		RelayActorURI:   relayActorURI,
		Matchers:        apiMatchers,
		Approved:        approved,
		Public:          flags.Public(),
		Unlisted:        flags.Unlisted(),
		MatchByDefault:  flags.MatchByDefault(),
		IgnoreSensitive: flags.IgnoreSensitive(),
		IgnoreMedia:     flags.IgnoreMedia(),
		IgnoreReplies:   flags.IgnoreReplies(),
	}, nil
}
