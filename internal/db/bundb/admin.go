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

package bundb

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"code.superseriousbusiness.org/gopkg/log"
	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtscontext"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"code.superseriousbusiness.org/gotosocial/internal/uris"
	"code.superseriousbusiness.org/gotosocial/internal/util"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"
)

// generate RSA keys of this length
const rsaKeyBits = 2048

type adminDB struct {
	db    *bun.DB
	state *state.State
}

func (a *adminDB) IsUsernameAvailable(ctx context.Context, username string) (bool, error) {
	q := a.db.
		NewSelect().
		TableExpr("? AS ?", bun.Ident("accounts"), bun.Ident("account")).
		Column("account.id").
		Where("? = ?", bun.Ident("account.username"), username).
		Where("? IS NULL", bun.Ident("account.domain"))
	return notExists(ctx, q)
}

func (a *adminDB) IsEmailAvailable(ctx context.Context, email string) (bool, error) {
	// parse the domain from the email
	m, err := mail.ParseAddress(email)
	if err != nil {
		return false, fmt.Errorf("error parsing email address %s: %s", email, err)
	}
	domain := strings.Split(m.Address, "@")[1] // domain will always be the second part after @

	// check if the email domain is blocked
	emailDomainBlockedQ := a.db.
		NewSelect().
		TableExpr("? AS ?", bun.Ident("email_domain_blocks"), bun.Ident("email_domain_block")).
		Column("email_domain_block.id").
		Where("? = ?", bun.Ident("email_domain_block.domain"), domain)
	emailDomainBlocked, err := exists(ctx, emailDomainBlockedQ)
	if err != nil {
		return false, err
	}
	if emailDomainBlocked {
		return false, fmt.Errorf("email domain %s is blocked", domain)
	}

	// check if this email is associated with a user already
	q := a.db.
		NewSelect().
		TableExpr("? AS ?", bun.Ident("users"), bun.Ident("user")).
		Column("user.id").
		Where("? = ?", bun.Ident("user.email"), email).
		WhereOr("? = ?", bun.Ident("user.unconfirmed_email"), email)
	return notExists(ctx, q)
}

func (a *adminDB) NewSignup(ctx context.Context, newSignup gtsmodel.NewSignup) (*gtsmodel.User, error) {
	// If something went wrong previously while doing a new
	// sign up with this username, we might already have an
	// account, so check first.
	account, err := a.state.DB.GetAccountByUsernameDomain(ctx, newSignup.Username, "")
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		// Real error occurred.
		err := gtserror.Newf("error checking for existing account: %w", err)
		return nil, err
	}

	// If we didn't yet have an account
	// with this username, create one now.
	if account == nil {
		uris := uris.GenerateURIsForAccount(newSignup.Username)
		accountID := id.NewRandomULID()

		privKey, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
		if err != nil {
			err := gtserror.Newf("error creating new rsa private key: %w", err)
			return nil, err
		}

		account = &gtsmodel.Account{
			ID:                           accountID,
			Username:                     newSignup.Username,
			DisplayName:                  newSignup.Username,
			URI:                          uris.UserURI,
			URL:                          uris.UserURL,
			InboxURI:                     uris.InboxURI,
			OutboxURI:                    uris.OutboxURI,
			FollowingURI:                 uris.FollowingURI,
			FollowersURI:                 uris.FollowersURI,
			FeaturedCollectionURI:        uris.FeaturedCollectionURI,
			ActorType:                    gtsmodel.AccountActorTypePerson,
			PrivateKey:                   privKey,
			PublicKey:                    &privKey.PublicKey,
			PublicKeyURI:                 uris.PublicKeyURI,
			HidesCcPublicFromUnauthedWeb: util.Ptr(true), // GtS default to hide unlisted.
		}

		// Insert the new account!
		if err := a.state.DB.PutAccount(ctx, account); err != nil {
			return nil, err
		}

		// Take user locale from form if set.
		// If not, fall back to first instance
		// language if that's set, and if it's
		// not, fall back to db default "en".
		locale := newSignup.Locale
		if locale == "" {
			instanceLanguages := config.GetInstanceLanguages()
			if len(instanceLanguages) != 0 {
				// Use first instance lang.
				locale = instanceLanguages[0].TagStr
			} else {
				// Use db default "en".
				locale = "en"
			}
		}

		// Insert basic settings for new account.
		account.Settings = &gtsmodel.AccountSettings{
			AccountID: accountID,
			Privacy:   gtsmodel.VisibilityDefault,
			Language:  locale,
		}
		if err := a.state.DB.PutAccountSettings(ctx, account.Settings); err != nil {
			return nil, err
		}

		// Stub empty stats for new account.
		if err := a.state.DB.StubAccountStats(ctx, account); err != nil {
			return nil, err
		}
	}

	// Created or already had an account.
	// Ensure user not already created.
	user, err := a.state.DB.GetUserByAccountID(ctx, account.ID)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		// Real error occurred.
		err := gtserror.Newf("error checking for existing user: %w", err)
		return nil, err
	}

	// If there's already a
	// user for this account,
	// just pin acct + return.
	if user != nil {
		user.Account = account
		return user, nil
	}

	// Had no user for this
	// account, time to create one!
	newUserID := id.NewRandomULID()

	encryptedPassword, err := bcrypt.GenerateFromPassword(
		[]byte(newSignup.Password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		err := gtserror.Newf("error hashing password: %w", err)
		return nil, err
	}

	// If no app ID was set,
	// get the instance app
	// and use its ID.
	if newSignup.AppID == "" {
		instanceApp, err := a.state.DB.GetInstanceApplication(ctx)
		if err != nil && !errors.Is(err, db.ErrNoEntries) {
			err := gtserror.Newf("db error getting instance app: %w", err)
			return nil, err
		}

		if instanceApp == nil {
			const errText = "instance application not yet created, run the server at least once *before* creating users"
			err := gtserror.New(errText)
			return nil, err
		}

		newSignup.AppID = instanceApp.ID
	}

	user = &gtsmodel.User{
		ID:                     newUserID,
		AccountID:              account.ID,
		Account:                account,
		EncryptedPassword:      string(encryptedPassword),
		SignUpIP:               newSignup.SignUpIP.To4(),
		Reason:                 newSignup.Reason,
		Locale:                 account.Settings.Language,
		UnconfirmedEmail:       newSignup.Email,
		CreatedByApplicationID: newSignup.AppID,
		ExternalID:             newSignup.ExternalID,
	}

	if newSignup.EmailVerified {
		// Mark given email as confirmed.
		user.ConfirmedAt = time.Now()
		user.Email = newSignup.Email
	}

	if newSignup.Admin {
		// Make new user mod + admin.
		user.Moderator = util.Ptr(true)
		user.Admin = util.Ptr(true)
	}

	if newSignup.PreApproved {
		// Mark new user as approved.
		user.Approved = util.Ptr(true)
	}

	// Insert the user!
	if err := a.state.DB.PutUser(ctx, user); err != nil {
		err := gtserror.Newf("db error inserting user: %w", err)
		return nil, err
	}

	return user, nil
}

func (a *adminDB) CreateInstanceAccount(ctx context.Context) error {
	username := config.GetHost()

	q := a.db.
		NewSelect().
		TableExpr("? AS ?", bun.Ident("accounts"), bun.Ident("account")).
		Column("account.id").
		Where("? = ?", bun.Ident("account.username"), username).
		Where("? IS NULL", bun.Ident("account.domain"))

	exists, err := exists(ctx, q)
	if err != nil {
		return err
	}
	if exists {
		log.Infof(ctx, "instance account %s already exists", username)
		return nil
	}

	key, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		log.Errorf(ctx, "error creating new rsa key: %s", err)
		return err
	}

	newAccountURIs := uris.GenerateURIsForAccount(username)
	acct := &gtsmodel.Account{
		ID:                    id.NewRandomULID(),
		Username:              username,
		DisplayName:           username,
		URL:                   newAccountURIs.UserURL,
		PrivateKey:            key,
		PublicKey:             &key.PublicKey,
		PublicKeyURI:          newAccountURIs.PublicKeyURI,
		ActorType:             gtsmodel.AccountActorTypeApplication,
		URI:                   newAccountURIs.UserURI,
		InboxURI:              newAccountURIs.InboxURI,
		OutboxURI:             newAccountURIs.OutboxURI,
		FollowersURI:          newAccountURIs.FollowersURI,
		FollowingURI:          newAccountURIs.FollowingURI,
		FeaturedCollectionURI: newAccountURIs.FeaturedCollectionURI,
	}

	// insert the new account!
	if err := a.state.DB.PutAccount(ctx, acct); err != nil {
		return err
	}

	log.Infof(ctx, "instance account %s CREATED with id %s", username, acct.ID)
	return nil
}

func (a *adminDB) CreateInstanceApplication(ctx context.Context) error {
	// Check if instance application already exists.
	// Instance application client_id always = the
	// instance account's ID so this is an easy check.
	instanceAcct, err := a.state.DB.GetInstanceAccount(ctx, "")
	if err != nil {
		err := gtserror.Newf("db error getting instance account: %w", err)
		return err
	}

	exists, err := exists(
		ctx,
		a.db.
			NewSelect().
			Column("application.id").
			TableExpr("? AS ?", bun.Ident("applications"), bun.Ident("application")).
			Where("? = ?", bun.Ident("application.client_id"), instanceAcct.ID),
	)
	if err != nil {
		return err
	}

	if exists {
		log.Infof(ctx, "instance application already exists")
		return nil
	}

	// Generate new IDs for this
	// application and its client.
	protocol := config.GetProtocol()
	host := config.GetHost()
	url := protocol + "://" + host

	clientID := instanceAcct.ID
	clientSecret := uuid.NewString()

	// Generate the application
	// to put in the database.
	app := &gtsmodel.Application{
		ID:           id.NewULID(),
		Name:         host + " instance application",
		Website:      url,
		RedirectURIs: []string{url},
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       "write:accounts",
	}

	// Store it.
	if err := a.state.DB.PutApplication(ctx, app); err != nil {
		err := gtserror.Newf("db error storing instance application: %w", err)
		return err
	}

	return nil
}

func (a *adminDB) GetInstanceApplication(ctx context.Context) (*gtsmodel.Application, error) {
	// Instance app clientID == instanceAcct.ID,
	// so get the instance account first.
	instanceAcct, err := a.state.DB.GetInstanceAccount(ctx, "")
	if err != nil {
		return nil, err
	}

	app := new(gtsmodel.Application)
	if err := a.db.
		NewSelect().
		Model(app).
		Where("? = ?", bun.Ident("application.client_id"), instanceAcct.ID).
		Scan(ctx); err != nil {
		return nil, err
	}

	return app, nil
}

func (a *adminDB) CountApprovedSignupsSince(ctx context.Context, since time.Time) (int, error) {
	return a.db.
		NewSelect().
		TableExpr("? AS ?", bun.Ident("users"), bun.Ident("user")).
		Where("? > ?", bun.Ident("user.created_at"), since).
		Where("? = ?", bun.Ident("user.approved"), true).
		Count(ctx)
}

func (a *adminDB) CountUnhandledSignups(ctx context.Context) (int, error) {
	return a.db.
		NewSelect().
		TableExpr("? AS ?", bun.Ident("users"), bun.Ident("user")).
		// Approved is false by default.
		// Explicitly rejected sign-ups end up elsewhere.
		Where("? = ?", bun.Ident("user.approved"), false).
		Count(ctx)
}

/*
	ACTION FUNCS
*/

func (a *adminDB) GetAdminAction(ctx context.Context, id string) (*gtsmodel.AdminAction, error) {
	action := new(gtsmodel.AdminAction)

	if err := a.db.
		NewSelect().
		Model(action).
		Scan(ctx); err != nil {
		return nil, err
	}

	return action, nil
}

func (a *adminDB) GetAdminActions(ctx context.Context) ([]*gtsmodel.AdminAction, error) {
	actions := make([]*gtsmodel.AdminAction, 0)

	if err := a.db.
		NewSelect().
		Model(&actions).
		Scan(ctx); err != nil {
		return nil, err
	}

	return actions, nil
}

func (a *adminDB) PutAdminAction(ctx context.Context, action *gtsmodel.AdminAction) error {
	_, err := a.db.
		NewInsert().
		Model(action).
		Exec(ctx)

	return err
}

func (a *adminDB) UpdateAdminAction(ctx context.Context, action *gtsmodel.AdminAction, columns ...string) error {
	// Update the action's last-updated
	action.UpdatedAt = time.Now()
	if len(columns) != 0 {
		columns = append(columns, "updated_at")
	}

	_, err := a.db.
		NewUpdate().
		Model(action).
		Where("? = ?", bun.Ident("admin_action.id"), action.ID).
		Column(columns...).
		Exec(ctx)

	return err
}

func (a *adminDB) DeleteAdminAction(ctx context.Context, id string) error {
	_, err := a.db.
		NewDelete().
		TableExpr("? AS ?", bun.Ident("admin_actions"), bun.Ident("admin_action")).
		Where("? = ?", bun.Ident("admin_action"), id).
		Exec(ctx)

	return err
}

func (a *adminDB) GetInstanceSettings(ctx context.Context) (*gtsmodel.InstanceSettings, error) {
	// Check if settings stored in the cache. Load it if not.
	s := a.state.Caches.DB.LocalInstance.Settings.Load()
	if s == nil {
		// Not hydrated.
		//
		// Load from db and store in cache.
		s = new(gtsmodel.InstanceSettings)
		if err := a.db.
			NewSelect().
			Table("instance_settings").
			Limit(1).
			Scan(ctx, s); err != nil {
			return nil, err
		}
		a.state.Caches.DB.LocalInstance.Settings.Store(s)
	}

	// Ensure no errant
	// pointer fields set.
	s.Rules = nil
	s.ContactAccount = nil

	// Copy cached settings.
	settings := new(gtsmodel.InstanceSettings)
	*settings = *s

	// If barebones, just return.
	if gtscontext.Barebones(ctx) {
		return settings, nil
	}

	// Populate settings before returning.
	if err := a.populateSettings(ctx, settings); err != nil {
		return nil, gtserror.Newf("db error populating settings: %w", err)
	}

	return settings, nil
}

func (a *adminDB) CreateInstanceSettings(ctx context.Context) error {
	// Check if settings already in the DB.
	settings, err := a.GetInstanceSettings(gtscontext.SetBarebones(ctx))
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		return err
	}

	if settings != nil {
		// Settings already exist,
		// no need to do anything.
		return nil
	}

	// Create minimum settings with just title.
	settings = &gtsmodel.InstanceSettings{
		ID:                             id.NewULID(),
		Title:                          config.GetHost(),
		AccountsUseAccountDomainInAcct: config.GetAccountsUseAccountDomainInAcct(),
		AccountsHideLocalRoles:         config.GetAccountsHideLocalRoles(),
		StatusesPreviewCards:           config.GetStatusesPreviewCards(),
		ProfilesAutoLoadOlderPosts:     false,
		ProfilesShowPlusInfo:           true,
	}

	// Put settings in the db.
	if _, err := a.db.
		NewInsert().
		Model(settings).
		Exec(ctx, settings); err != nil {
		return err
	}

	// Store in cache before returning.
	a.state.Caches.DB.LocalInstance.Settings.Store(settings)
	return nil
}

func (a *adminDB) UpdateInstanceSettings(ctx context.Context, settings *gtsmodel.InstanceSettings, columns ...string) error {
	// Update settings in the db.
	if _, err := a.db.
		NewUpdate().
		Model(settings).
		Column(columns...).
		WherePK().
		Exec(ctx); err != nil {
		return err
	}

	// Copy + depopulate settings
	// before storing in cache.
	s := new(gtsmodel.InstanceSettings)
	*s = *settings
	s.Rules = nil
	s.ContactAccount = nil
	a.state.Caches.DB.LocalInstance.Settings.Store(s)
	return nil
}

func (a *adminDB) populateSettings(ctx context.Context, settings *gtsmodel.InstanceSettings) error {
	// Populate rules if necessary.
	if settings.Rules == nil {
		var err error
		settings.Rules, err = a.state.DB.GetActiveRules(ctx)
		if err != nil {
			return err
		}
	}

	// Populate contact account if necessary.
	if settings.ContactAccountID != "" && settings.ContactAccount == nil {
		var err error
		settings.ContactAccount, err = a.state.DB.GetAccountByID(
			gtscontext.SetBarebones(ctx),
			settings.ContactAccountID,
		)
		if err != nil {
			return err
		}
	}

	return nil
}
