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
	"errors"
	"time"

	"code.superseriousbusiness.org/gopkg/log"
	"code.superseriousbusiness.org/gopkg/xslices"
	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtscontext"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/paging"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"code.superseriousbusiness.org/gotosocial/internal/util"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect"
)

type instanceDB struct {
	db    *bun.DB
	state *state.State
}

func (i *instanceDB) CountInstanceAccounts(ctx context.Context) (int, error) {
	// Check for a cached instance accounts count. If present return this.
	if n := i.state.Caches.DB.LocalInstance.Accounts.Load(); n != nil {
		return *n, nil
	}

	count, err := i.db.
		NewSelect().
		TableExpr("? AS ?", bun.Ident("accounts"), bun.Ident("account")).
		// Just select IDs.
		Column("account.id").
		// Local accounts only.
		Where("? IS NULL", bun.Ident("account.domain")).
		// Ignore instance account.
		Where("? != ?", bun.Ident("account.username"), config.GetHost()).
		// Exclude suspended accounts.
		Where("? IS NULL", bun.Ident("account.suspended_at")).
		Count(ctx)
	if err != nil {
		return 0, err
	}

	// Update cached instance accounts count value.
	i.state.Caches.DB.LocalInstance.Accounts.Store(&count)
	return count, nil
}

func (i *instanceDB) CountInstanceStatuses(ctx context.Context) (int, error) {
	// Check for a cached instance statuses count. If present return this.
	if n := i.state.Caches.DB.LocalInstance.Statuses.Load(); n != nil {
		return *n, nil
	}

	// Select from
	// local count view.
	var count int
	if err := i.db.
		NewSelect().
		Table("statuses_local_count_view").
		Scan(ctx, &count); err != nil {
		return 0, err
	}

	// Update cached instance statuses count value.
	i.state.Caches.DB.LocalInstance.Statuses.Store(&count)
	return count, nil
}

func (i *instanceDB) CountInstancePeers(ctx context.Context) (int, error) {
	// Check for a cached instance peers count. If present return this.
	if n := i.state.Caches.DB.LocalInstance.Peers.Load(); n != nil {
		return *n, nil
	}

	// Select just the domain
	// part of all known instances.
	domains := []string{}
	if err := i.db.
		NewSelect().
		Table("instances").
		Column("domain").
		Scan(ctx, &domains); err != nil {
		return 0, err
	}

	var count int
	for _, domain := range domains {
		// For each domain, check if
		// we're federating with it.
		blocked, err := i.state.DB.IsDomainBlocked(ctx, domain)
		if err != nil {
			return 0, gtserror.Newf("db error checking block: %w", err)
		}

		if blocked {
			// Doesn't count as a
			// peer if it's blocked.
			continue
		}

		// Count as a peer.
		count++
	}

	// Update cached instance peers count value.
	i.state.Caches.DB.LocalInstance.Peers.Store(&count)
	return count, nil
}

func (i *instanceDB) GetInstancePeers(ctx context.Context, includeSuspended bool) ([]*gtsmodel.Instance, error) {
	// Select just the domain
	// part of all known instances.
	domains := []string{}
	if err := i.db.
		NewSelect().
		Table("instances").
		Column("domain").
		Scan(ctx, &domains); err != nil {
		return nil, err
	}

	if len(domains) == 0 {
		// Empty response.
		return make([]*gtsmodel.Instance, 0), nil
	}

	instances := make([]*gtsmodel.Instance, 0, len(domains))
	for _, domain := range domains {
		if !includeSuspended {
			// Ensure peer not blocked.
			blocked, err := i.state.DB.IsDomainBlocked(ctx, domain)
			if err != nil {
				return nil, gtserror.Newf("db error checking block: %w", err)
			}

			if blocked {
				// Skip this one.
				continue
			}
		}

		// Select instance.
		instance, err := i.GetInstance(ctx, domain)
		if err != nil {
			log.Errorf(ctx, "db error getting instance %q: %v", domain, err)
			continue
		}

		// Append to return slice.
		instances = append(instances, instance)
	}

	return instances, nil
}

func (i *instanceDB) GetInstance(ctx context.Context, domain string) (*gtsmodel.Instance, error) {
	// Normalize the domain as punycode.
	var err error
	domain, err = util.Punify(domain)
	if err != nil {
		return nil, gtserror.Newf("error punifying domain %s: %w", domain, err)
	}

	return i.getInstance(ctx,
		"Domain",
		func(instance *gtsmodel.Instance) error {
			return i.db.NewSelect().
				Model(instance).
				Where("? = ?", bun.Ident("instance.domain"), domain).
				Scan(ctx)
		},
		domain,
	)
}

func (i *instanceDB) GetInstanceByID(ctx context.Context, id string) (*gtsmodel.Instance, error) {
	return i.getInstance(ctx,
		"ID",
		func(instance *gtsmodel.Instance) error {
			return i.db.NewSelect().
				Model(instance).
				Where("? = ?", bun.Ident("instance.id"), id).
				Scan(ctx)
		},
		id,
	)
}

func (i *instanceDB) GetInstancesPage(
	ctx context.Context,
	page *paging.Page,
	domain string,
	orderBy gtsmodel.InstanceOrderBy,
	undeliverableOnly bool,
) ([]*gtsmodel.Instance, error) {
	var (
		// Extract page params.
		minID = page.GetMin()
		maxID = page.GetMax()
		limit = page.GetLimit()
		order = page.GetOrder()

		// Pre-allocate slice of instances
		// to select column values into.
		selectInto = make([]gtsmodel.Instance, 0, limit)

		// We know orderBy is either Latest or Alphabetical.
		orderByAlphabetical = orderBy == gtsmodel.InstanceOrderByAlphabetical

		// Query differs if we're running on Postgres.
		postgres = i.db.Dialect().Name() == dialect.PG
	)

	q := i.db.
		NewSelect().
		// Select just the ID
		// of each instance.
		Column("instance.id").
		TableExpr("? AS ?", bun.Ident("instances"), bun.Ident("instance"))

	if undeliverableOnly {
		// Select only instances with matching
		// federation errors, and use DISTINCT
		// to avoid duplicate rows in results.
		q = q.Join(
			"RIGHT JOIN ? AS ? ON ? = ?",
			bun.Ident("federation_errors"),
			bun.Ident("federation_error"),
			bun.Ident("instance.id"),
			bun.Ident("federation_error.instance_id"),
		).Distinct()
	}

	if domain != "" {
		// Normalize the
		// domain as punycode.
		var err error
		domain, err = util.Punify(domain)
		if err != nil {
			return nil, gtserror.Newf("error punifying domain %s: %w", domain, err)
		}

		// Get any instances *starting with* the given domain.
		q = q.Where("? LIKE ?", bun.Ident("instance.domain"), domain+"%")
	}

	// Paging parameters.

	if maxID != "" {
		if orderByAlphabetical {
			// Get instance for max ID.
			maxIDInstance, err := i.GetInstanceByID(
				gtscontext.SetBarebones(ctx),
				maxID,
			)
			if err != nil {
				err := gtserror.Newf("db error getting maxID instance %s: %w", maxID, err)
				return nil, err
			}
			// Order alpahetically (a-z) by domain.
			q = q.Where("? > ?", bun.Ident("instance.domain"), maxIDInstance.Domain)
		} else {
			// Order by ID, which indicates creation time.
			q = q.Where("? < ?", bun.Ident("instance.id"), maxID)
		}
	}

	if minID != "" {
		if orderByAlphabetical {
			// Get instance for min ID.
			minIDInstance, err := i.GetInstanceByID(
				gtscontext.SetBarebones(ctx),
				minID,
			)
			if err != nil {
				err := gtserror.Newf("db error getting minID instance %s: %w", minID, err)
				return nil, err
			}
			// Order alpahetically (a-z) by domain.
			q = q.Where("? < ?", bun.Ident("instance.domain"), minIDInstance.Domain)
		} else {
			// Order by ID, which indicates creation time.
			q = q.Where("? > ?", bun.Ident("instance.id"), minID)
		}
	}

	switch {
	case !orderByAlphabetical && order == paging.OrderDescending:
		// Order by ID (paging down).
		q = q.Order("instance.id DESC")

	case !orderByAlphabetical && order == paging.OrderAscending:
		// Order by ID (paging up).
		q = q.Order("instance.id ASC")

	case orderByAlphabetical && order == paging.OrderDescending:
		// Order alphabetically (paging down).
		// Z > A in ASCII so use ASC.
		if postgres {
			// When doing DISTINCT Postgres wants
			// anything used in ORDER BY to also
			// be included in the SELECT columns.
			q = q.Column("instance.domain")
		}
		q = q.Order("instance.domain ASC")

	case orderByAlphabetical && order == paging.OrderAscending:
		// Order alphabetically (paging up).
		// A < Z in ASCII so use DESC.
		if postgres {
			// When doing DISTINCT Postgres wants
			// anything used in ORDER BY to also
			// be included in the SELECT columns.
			q = q.Column("instance.domain")
		}
		q = q.Order("instance.domain DESC")
	}

	// Limit amount of
	// instances returned.
	q = q.Limit(limit)

	// Run the query.
	if err := q.Scan(ctx, &selectInto); err != nil {
		return nil, err
	}

	count := len(selectInto)
	if count == 0 {
		// Nothing for
		// this query.
		return nil, nil
	}

	// Preallocate slice of instances,
	// and fetch each instance by ID.
	instances := make([]*gtsmodel.Instance, 0, count)
	for _, v := range selectInto {
		instance, err := i.GetInstanceByID(ctx, v.ID)
		if err != nil {
			err := gtserror.Newf("db error getting instance: %w", err)
			return nil, err
		}
		instances = append(instances, instance)
	}
	return instances, nil
}

func (i *instanceDB) getInstance(
	ctx context.Context,
	lookup string,
	dbQuery func(*gtsmodel.Instance) error,
	keyParts ...any,
) (*gtsmodel.Instance, error) {
	// Fetch instance from db cache with loader callback.
	instance, err := i.state.Caches.DB.Instance.LoadOne(
		lookup,
		func() (*gtsmodel.Instance, error) {
			// Not cached! Perform database query.
			var instance gtsmodel.Instance
			if err := dbQuery(&instance); err != nil {
				return nil, err
			}

			return &instance, nil
		},
		keyParts...,
	)
	if err != nil {
		return nil, gtserror.Newf("db error getting instance: %w", err)
	}

	if gtscontext.Barebones(ctx) {
		// No need to populate.
		return instance, nil
	}

	// Set delivery errors on instance model.
	dErrs, err := i.getFederationErrors(ctx,
		instance.ID,
		gtsmodel.FederationErrorTypeDelivery,
	)
	if err != nil {
		return nil, gtserror.Newf("db error getting delivery errors: %w", err)
	}
	instance.DeliveryErrors = dErrs

	// Return populated instance.
	return instance, nil
}

func (i *instanceDB) PutInstance(ctx context.Context, instance *gtsmodel.Instance) error {
	// Normalize the domain as punycode, note the extra
	// validation step for domain name write operations.
	var err error
	instance.Domain, err = util.PunifySafely(instance.Domain)
	if err != nil {
		return gtserror.Newf("error punifying domain %s: %w", instance.Domain, err)
	}

	// Store the new instance model in database, invalidating cache.
	return i.state.Caches.DB.Instance.Store(instance, func() error {
		_, err := i.db.NewInsert().Model(instance).Exec(ctx)
		return err
	})
}

func (i *instanceDB) GetInstanceAccounts(ctx context.Context, domain string, maxID string, limit int) ([]*gtsmodel.Account, error) {
	// Ensure reasonable
	if limit < 0 {
		limit = 0
	}

	var err error

	// Normalize the domain as punycode
	domain, err = util.Punify(domain)
	if err != nil {
		return nil, gtserror.Newf("error punifying domain %s: %w", domain, err)
	}

	// Make educated guess for slice size
	accountIDs := make([]string, 0, limit)

	q := i.db.
		NewSelect().
		TableExpr("? AS ?", bun.Ident("accounts"), bun.Ident("account")).
		// Select just the account ID.
		Column("account.id").
		// Select accounts belonging to given domain.
		Where("? = ?", bun.Ident("account.domain"), domain).
		Order("account.id DESC")

	if maxID == "" {
		maxID = id.Highest
	}
	q = q.Where("? < ?", bun.Ident("account.id"), maxID)

	if limit > 0 {
		q = q.Limit(limit)
	}

	if err := q.Scan(ctx, &accountIDs); err != nil {
		return nil, err
	}

	// Catch case of no accounts early.
	count := len(accountIDs)
	if count == 0 {
		return nil, db.ErrNoEntries
	}

	// Select each account by its ID.
	accounts := make([]*gtsmodel.Account, 0, count)
	for _, id := range accountIDs {
		account, err := i.state.DB.GetAccountByID(ctx, id)
		if err != nil {
			log.Errorf(ctx, "error getting account %q: %v", id, err)
			continue
		}

		// Append to return slice.
		accounts = append(accounts, account)
	}

	return accounts, nil
}

func (i *instanceDB) GetInstanceModeratorAddresses(ctx context.Context) ([]string, error) {
	addresses := []string{}

	// Select email addresses of approved, confirmed,
	// and enabled moderators or admins.

	q := i.db.
		NewSelect().
		TableExpr("? AS ?", bun.Ident("users"), bun.Ident("user")).
		Column("user.email").
		Where("? = ?", bun.Ident("user.approved"), true).
		Where("? IS NOT NULL", bun.Ident("user.confirmed_at")).
		Where("? = ?", bun.Ident("user.disabled"), false).
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.
				Where("? = ?", bun.Ident("user.moderator"), true).
				WhereOr("? = ?", bun.Ident("user.admin"), true)
		}).
		OrderExpr("? ASC", bun.Ident("user.email"))

	if err := q.Scan(ctx, &addresses); err != nil {
		return nil, err
	}

	if len(addresses) == 0 {
		return nil, db.ErrNoEntries
	}

	return addresses, nil
}

func (i *instanceDB) GetInstanceModerators(ctx context.Context) ([]*gtsmodel.Account, error) {
	accountIDs := []string{}

	// Select account IDs of approved, confirmed,
	// and enabled moderators or admins.

	q := i.db.
		NewSelect().
		TableExpr("? AS ?", bun.Ident("users"), bun.Ident("user")).
		Column("user.account_id").
		Where("? = ?", bun.Ident("user.approved"), true).
		Where("? IS NOT NULL", bun.Ident("user.confirmed_at")).
		Where("? = ?", bun.Ident("user.disabled"), false).
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.
				Where("? = ?", bun.Ident("user.moderator"), true).
				WhereOr("? = ?", bun.Ident("user.admin"), true)
		})

	if err := q.Scan(ctx, &accountIDs); err != nil {
		return nil, err
	}

	if len(accountIDs) == 0 {
		return nil, db.ErrNoEntries
	}

	return i.state.DB.GetAccountsByIDs(ctx, accountIDs)
}

func (i *instanceDB) AddInstanceDeliveryError(
	ctx context.Context,
	domain string,
	errMsg string,
) error {
	// Fetch instance with the given domain.
	instance, err := i.GetInstance(
		gtscontext.SetBarebones(ctx),
		domain,
	)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		return gtserror.Newf("db error getting instance: %w", err)
	}

	if instance == nil {
		// No entry so nothing to do. Weird though.
		log.Warnf(ctx, "no instance entry found for domain %s", domain)
		return nil
	}

	// Prepare delivery error.
	dErr := &gtsmodel.FederationError{
		ID:         id.NewULID(),
		InstanceID: instance.ID,
		Type:       gtsmodel.FederationErrorTypeDelivery,
		Error:      errMsg,
	}

	// Insert delivery error.
	if err := i.state.Caches.DB.FederationError.Store(dErr, func() error {
		_, err := i.db.
			NewInsert().
			Model(dErr).
			Exec(ctx)
		return err
	}); err != nil {
		return gtserror.Newf("db error putting delivery error: %w", err)
	}

	// Get ids of delivery errors for this instance.
	dErrIDs, err := i.getFederationErrorIDs(ctx,
		instance.ID,
		gtsmodel.FederationErrorTypeDelivery,
	)
	if err != nil {
		return gtserror.Newf("db error getting existing delivery error IDs: %w", err)
	}

	// If we don't have more than
	// maxDeliveryErrors stored,
	// don't bother tidying up.
	const maxDeliveryErrors = 20
	if len(dErrIDs) <= maxDeliveryErrors {
		return nil
	}

	// Remove any surplus instance delivery errors.
	surplusErrIDs := dErrIDs[maxDeliveryErrors:]
	if _, err := i.db.
		NewDelete().
		Table("federation_errors").
		Where("? IN (?)", bun.Ident("id"), bun.List(surplusErrIDs)).
		Exec(ctx); err != nil {
		return gtserror.Newf("db error deleting surplus delivery errors: %w", err)
	}

	// Invalidate surplus errors from cache.
	i.state.Caches.DB.FederationError.InvalidateIDs("ID", surplusErrIDs)
	return nil
}

func (i *instanceDB) SetInstanceSuccessfulDelivery(
	ctx context.Context,
	domain string,
) error {
	// Fetch instance with
	// the given domain.
	instance, err := i.GetInstance(
		gtscontext.SetBarebones(ctx),
		domain,
	)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		return gtserror.Newf("db error getting instance: %w", err)
	}

	if instance == nil {
		// No entry so nothing to do. Weird though.
		log.Warnf(ctx, "no instance entry found for domain %s", domain)
		return nil
	}

	// Set latest successful delivery to now.
	instance.LatestSuccessfulDelivery = time.Now()

	// Update the instance entry.
	if err := i.state.Caches.DB.Instance.Store(instance, func() error {
		_, err := i.db.
			NewUpdate().
			Model(instance).
			Column("latest_successful_delivery").
			Where("? = ?", bun.Ident("instance.id"), instance.ID).
			Exec(ctx)
		return err
	}); err != nil {
		return gtserror.Newf("db error updating instance: %w", err)
	}

	// Clear delivery errors for this instance (if any).
	if err := i.ClearFederationErrors(ctx,
		instance.ID,
		gtsmodel.FederationErrorTypeDelivery,
	); err != nil {
		return gtserror.Newf("db error clearing delivery errors: %w", err)
	}

	return nil
}

func (i *instanceDB) getFederationErrorIDs(
	ctx context.Context,
	instanceID string,
	errType gtsmodel.FederationErrorType,
) ([]string, error) {
	// Get IDs of federation
	// errors for this instance.
	ids := []string{}
	if err := i.db.
		NewSelect().
		Column("id").
		Table("federation_errors").
		Where("? = ?", bun.Ident("instance_id"), instanceID).
		Where("? = ?", bun.Ident("type"), errType).
		OrderExpr("? DESC", bun.Ident("id")).
		Scan(ctx, &ids); err != nil && !errors.Is(err, db.ErrNoEntries) {
		return nil, err
	}

	return ids, nil
}

func (i *instanceDB) getFederationErrors(
	ctx context.Context,
	instanceID string,
	errType gtsmodel.FederationErrorType,
) ([]*gtsmodel.FederationError, error) {
	// Get IDs of federation
	// errors for this instance.
	ids, err := i.getFederationErrorIDs(ctx, instanceID, errType)
	if err != nil {
		return nil, err
	}

	// Check for 0 entries.
	if len(ids) == 0 {
		return nil, nil
	}

	// Load federation errors.
	return i.getFederationErrorsByIDs(ctx, ids)
}

func (i *instanceDB) ClearFederationErrors(
	ctx context.Context,
	instanceID string,
	errType gtsmodel.FederationErrorType,
) error {
	ids := []string{}
	if _, err := i.db.
		NewDelete().
		Table("federation_errors").
		Where("? = ?", bun.Ident("instance_id"), instanceID).
		Where("? = ?", bun.Ident("type"), errType).
		Returning("id").
		Exec(ctx, &ids); err != nil {
		return err
	}

	i.state.Caches.DB.FederationError.InvalidateIDs("ID", ids)
	return nil
}

func (i *instanceDB) getFederationErrorsByIDs(ctx context.Context, ids []string) ([]*gtsmodel.FederationError, error) {
	fErrs, err := i.state.Caches.DB.FederationError.LoadIDs("ID",
		ids,
		func(uncached []string) ([]*gtsmodel.FederationError, error) {
			fErrs := make([]*gtsmodel.FederationError, 0, len(uncached))
			// Perform database query scanning
			// the remaining (uncached) err IDs.
			if err := i.db.
				NewSelect().
				Model(&fErrs).
				Where("? IN (?)", bun.Ident("id"), bun.List(uncached)).
				Scan(ctx); err != nil {
				return nil, err
			}

			return fErrs, nil
		},
	)
	if err != nil {
		return nil, err
	}

	// Reorder the errors by their
	// IDs to ensure in correct order.
	getID := func(t *gtsmodel.FederationError) string { return t.ID }
	xslices.OrderBy(fErrs, ids, getID)

	return fErrs, nil
}
