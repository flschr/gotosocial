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

package dereferencing

import (
	"context"
	"errors"
	"net/url"

	"code.superseriousbusiness.org/gopkg/log"
	"code.superseriousbusiness.org/gotosocial/internal/ap"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtscontext"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/transport"
	"code.superseriousbusiness.org/gotosocial/internal/util"
)

// getStatusDBOnly checks in the database for
// status with the given URI (or URL), without
// doing any external dereferencing.
func (d *Dereferencer) getStatusFromDB(
	ctx context.Context,
	uriStr string,
) (*gtsmodel.Status, error) {

	// For both queries request a barebones
	// object, as it will be later populated
	// in the enrichAndStoreSafely() function.
	ctx = gtscontext.SetBarebones(ctx)

	// Search the database for existing status by URI.
	status, err := d.state.DB.GetStatusByURI(ctx, uriStr)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		return nil, gtserror.Newf("error checking database for status %s by uri: %w", uriStr, err)
	}

	if status != nil {
		// Found it,
		// stop early.
		return status, nil
	}

	// Else, search database for existing status by URL.
	status, err = d.state.DB.GetStatusByURL(ctx, uriStr)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		return nil, gtserror.Newf("error checking database for status %s by url: %w", uriStr, err)
	}

	// Return maybe status.
	return status, nil
}

// retrieveStatusable dereferences the given URI and
// processes the response into an ap.Statusable model.
//
// In case of HTTP redirects to a final URI that differs
// from the input URI, the input URI pointer will be changed
// to the final URI, and the database will be checked once
// more to see if the status was stored locally at that URI.
// If so, the stored status will be returned as alreadyStatus.
//
// Will return malformed if the final redirected URI is not
// either the AP ID/URI or the URL of the dereffed statusable.
//
// The final returned URI is the dereferenced
// ActivityPub status object's JSON-LD ID.
func (d *Dereferencer) retrieveStatusable(
	ctx context.Context,
	tsport transport.Transport,
	uri *url.URL,
) (
	statusable ap.Statusable,
	existing *gtsmodel.Status,
	statusURI *url.URL,
	err error,
) {
	// Save for later comparison.
	initialURIStr := uri.String()

	// Dereference latest version of status.
	rsp, err := tsport.Dereference(ctx, uri)
	if err != nil {
		err := gtserror.Newf("error dereferencing %s: %w", initialURIStr, err)
		return nil, nil, nil, gtserror.SetUnretrievable(err)
	}

	// Attempt to resolve ActivityPub status from response.
	statusable, err = ap.ResolveStatusable(ctx, rsp.Body)

	// Tidy up now done.
	_ = rsp.Body.Close()

	if err != nil {
		// ResolveStatusable will set gtserror.WrongType
		// on the returned error, so we don't need to do it here.
		err := gtserror.Newf("error resolving statusable %s: %w",
			initialURIStr, err)
		return nil, nil, nil, err
	}

	// Check whether input URI and final returned URI
	// have changed (i.e. we followed some redirects).
	//
	// NOTE: this URI check + database call is performed
	// AFTER reading and closing body, for performance.
	finalURI := rsp.Request.URL
	finalURIStr := finalURI.String()
	redirected := finalURIStr != initialURIStr

	if redirected {
		var err error

		// Check whether we have this status stored under
		// *final* determined URI, preferring this for return.
		existing, err = d.getStatusFromDB(ctx, finalURIStr)
		if err != nil && !errors.Is(err, db.ErrNoEntries) {
			err := gtserror.Newf("db error getting status after redirects: %w", err)
			return nil, nil, nil, err
		}
	}

	// Extract the json-ld ID, i.e. the
	// actual ActivityPub URI ID of status.
	jsonldID := ap.GetJSONLDId(statusable)

	// Ensure the final URI we fetched the status
	// from matches either (one of) the URL(s) or
	// the ID/URI of the dereferenced statusable.
	uris := append(ap.GetURL(statusable), jsonldID)
	matches, err := util.URIMatches(finalURI, uris...)
	if err != nil {
		err := gtserror.Newf("error checking uri matches %s: %w", finalURIStr, err)
		return nil, nil, nil, gtserror.SetMalformed(err)
	}

	if !matches {
		// For error, include
		// redirect for context.
		uristr := finalURIStr
		if redirected {
			uristr += " (redirected from " + initialURIStr + ")"
		}

		// No URI match, remote is doing something weird. Return malformed error type.
		err := gtserror.Newf("fetch uri %s does not match known status uri(s): %v",
			uristr, log.Formatted(uris))
		return nil, nil, nil, gtserror.SetMalformed(err)
	}

	// For the final returned URI we set
	// status' canonical JSON-LD URI, as
	// checks against its URI won't work
	// if we return an alternative URL.
	statusURI = jsonldID
	return
}

// convertStatusable converts the given statusable to its gts model equivalent.
// The requestUser param is used to ensure the status author is dereferenced.
// The URI is used to check that the status's AP ID/URI matches expectations.
func (d *Dereferencer) convertStatusable(
	ctx context.Context,
	requestUser string,
	statusURI *url.URL,
	statusable ap.Statusable,
) (*gtsmodel.Status, error) {

	// Get attributedTo URI from statusable to fetch account.
	attributedTo, err := ap.GetOneAttributedTo(statusable)
	if err != nil {
		return nil, gtserror.SetMalformed(err)
	}

	// The status author, and the status
	// JSON-LD ID must have the same host.
	if attributedTo.Host != statusURI.Host {
		err := gtserror.Newf("id and attributedTo hostnames differ: id=%s attributedTo=%s", attributedTo.Host, statusURI.Host)
		return nil, gtserror.SetMalformed(err)
	}

	// Ensure we have the author account of the status dereferenced
	// (and up-to-date); this is needed to convert to our GTS model.
	if _, _, err := d.getAccountByURI(ctx, requestUser, attributedTo, false); err != nil {

		// Note that we specifically DO NOT wrap the error, instead collapsing it as string.
		// Errors fetching an account do not necessarily relate to dereferencing the status.
		return nil, gtserror.Newf("failed to dereference status author %s: %v", statusURI, err)
	}

	// Convert ActivityPub model to our internal GTS model.
	status, err := d.converter.ASStatusToStatus(ctx, statusable)
	if err != nil {
		return nil, gtserror.Newf("error converting statusable to gts model for status %s: %w", statusURI, err)
	}

	// Micro.blog omits ActivityPub inReplyTo on hosted replies, but exposes
	// the relationship as u-in-reply-to on the public status page.
	if status.InReplyToURI == "" {
		if err := d.enrichMicroblogInReplyTo(ctx, requestUser, status); err != nil {
			log.Warnf(ctx, "error enriching Micro.blog reply %s: %v", status.URI, err)
		}
	}

	// Ensure final status isn't attempting
	// to claim being authored by local user.
	if status.Account.IsLocal() {
		return nil, gtserror.Newf(
			"dereferenced status %s claiming to be local",
			status.URI,
		)
	}

	return status, nil
}
