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

package federation

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"

	"code.superseriousbusiness.org/activity/pub"
	"code.superseriousbusiness.org/activity/streams"
	"code.superseriousbusiness.org/activity/streams/vocab"
	"code.superseriousbusiness.org/gopkg/log"
	"code.superseriousbusiness.org/gopkg/xslices"
	"code.superseriousbusiness.org/gotosocial/internal/ap"
	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtscontext"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/uris"
	"codeberg.org/gruf/go-kv/v2"
)

/*
	GO FED FEDERATING PROTOCOL INTERFACE
	FederatingProtocol contains behaviors an application needs to satisfy for the
	full ActivityPub S2S implementation to be supported by this library.
	It is only required if the client application wants to support the server-to-
	server, or federating, protocol.
	It is passed to the library as a dependency injection from the client
	application.
*/

// PostInboxRequestBodyHook callback after parsing the request body for a
// federated request to the Actor's inbox.
//
// Can be used to set contextual information based on the Activity received.
//
// Warning: Neither authentication nor authorization has taken place at
// this time. Doing anything beyond setting contextual information is
// strongly discouraged.
//
// If an error is returned, it is passed back to the caller of PostInbox.
// In this case, the DelegateActor implementation must not write a response
// to the ResponseWriter as is expected that the caller to PostInbox will
// do so when handling the error.
func (f *Federator) PostInboxRequestBodyHook(ctx context.Context, r *http.Request, activity pub.Activity) (context.Context, error) {

	// Extract any other IRIs
	// involved in this activity.
	var otherIRIs []*url.URL

	// Get the ID of the Activity itslf.
	activityID, _ := pub.GetId(activity)
	if activityID != nil {
		otherIRIs = append(otherIRIs, activityID)
		ctx = gtscontext.SetActivityID(ctx, activityID)
	}

	// Check if the Activity has an 'inReplyTo'.
	if replyToable, ok := activity.(ap.ReplyToable); ok {
		otherIRIs = append(otherIRIs, ap.GetInReplyTo(replyToable)...)
	}

	// Check for TO and CC URIs on the Activity.
	if addressable, ok := activity.(ap.Addressable); ok {
		otherIRIs = append(otherIRIs, ap.GetTo(addressable)...)
		otherIRIs = append(otherIRIs, ap.GetCc(addressable)...)
	}

	// Now perform the same checks, but
	// for any Object(s) of the Activity.
	objectProp := activity.GetActivityStreamsObject()
	if objectProp != nil {
		for iter := objectProp.Begin(); iter != objectProp.End(); iter = iter.Next() {
			if iter.IsIRI() {
				otherIRIs = append(otherIRIs, iter.GetIRI())
				continue
			}

			t := iter.GetType()
			if t == nil {
				continue
			}

			objectID, _ := pub.GetId(t)
			if objectID != nil {
				otherIRIs = append(otherIRIs, objectID)
			}

			if replyToable, ok := t.(ap.ReplyToable); ok {
				otherIRIs = append(otherIRIs, ap.GetInReplyTo(replyToable)...)
			}

			if addressable, ok := t.(ap.Addressable); ok {
				otherIRIs = append(otherIRIs, ap.GetTo(addressable)...)
				otherIRIs = append(otherIRIs, ap.GetCc(addressable)...)
			}
		}
	}

	// Clean any instances of the public URI, since
	// we don't care about that in this context.
	otherIRIs = func(iris []*url.URL) []*url.URL {
		np := make([]*url.URL, 0, len(iris))

		for _, i := range iris {
			if !pub.IsPublic(i.String()) {
				np = append(np, i)
			}
		}

		return np
	}(otherIRIs)

	// OtherIRIs will likely contain some
	// duplicate entries now, so remove them.
	otherIRIs = xslices.DeduplicateFunc(otherIRIs,
		(*url.URL).String, // serialized URL is 'key()'
	)

	// Finished, set other IRIs on the context
	// so they can be checked for blocks later.
	ctx = gtscontext.SetOtherIRIs(ctx, otherIRIs)
	return ctx, nil
}

// AuthenticatePostInbox delegates the authentication of a POST to an
// inbox.
//
// If an error is returned, it is passed back to the caller of
// PostInbox. In this case, the implementation must not write a
// response to the ResponseWriter as is expected that the client will
// do so when handling the error. The 'authenticated' is ignored.
//
// If no error is returned, but authentication or authorization fails,
// then authenticated must be false and error nil. It is expected that
// the implementation handles writing to the ResponseWriter in this
// case.
//
// Finally, if the authentication and authorization succeeds, then
// authenticated must be true and error nil. The request will continue
// to be processed.
func (f *Federator) AuthenticatePostInbox(ctx context.Context, w http.ResponseWriter, r *http.Request) (context.Context, bool, error) {
	log.Tracef(ctx, "received request to authenticate inbox %s", r.URL.String())

	// Ensure this is an inbox path, and fetch the inbox owner
	// account by parsing username from `/users/{username}/inbox`.
	username, err := uris.ParseInboxPath(r.URL)
	if err != nil {
		err := gtserror.Newf("could not parse %s as inbox path: %w", r.URL.String(), err)
		return nil, false, err
	}

	if username == "" {
		err := gtserror.New("inbox username was empty")
		return nil, false, err
	}

	// Get the receiving local account inbox
	// owner with given username from database.
	receiver, err := f.db.GetAccountByUsernameDomain(ctx, username, "")
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		err := gtserror.Newf("db error getting receiving account %s: %w", username, err)
		return nil, false, err
	}

	if receiver == nil {
		// Maybe we had this account at some point and someone
		// manually deleted it from the DB. Just return not found.
		err := gtserror.Newf("receiving account %s not found in the db", username)
		errWithCode := gtserror.NewErrorNotFound(err)
		return ctx, false, errWithCode
	}

	// Check who's trying to deliver to us by inspecting the http signature.
	pubKeyAuth, errWithCode := f.AuthenticateFederatedRequest(ctx, receiver.Username)
	if errWithCode != nil {

		// Check if we got code 410 Gone from a remote
		// instance while trying to dereference the pub
		// key owner who's trying to post to this inbox,
		// or if we already had a tombstone stored for them.
		if gtserror.StatusCode(errWithCode) == http.StatusGone {
			// If the pub key owner's key/account has
			// gone, then inbox post was likely a Delete.
			//
			// If so, we can just write 202 and leave, as
			// either we'll have already processed any Deletes
			// sent by this account, or we never met the account
			// in the first place so we don't have any of their
			// stuff stored to actually delete.
			w.WriteHeader(http.StatusAccepted)
			return ctx, false, nil
		}

		// In all other cases, obey the go-fed
		// interface by writing the status
		// code from the returned ErrWithCode.
		w.WriteHeader(errWithCode.Code())

		// We still return the error
		// for later request logging.
		return ctx, false, errWithCode
	}

	if pubKeyAuth.Handshaking {
		// There is a mutal handshake occurring between us and
		// the owner URI. Return 202 and leave as we can't do
		// much else until the handshake procedure has finished.
		w.WriteHeader(http.StatusAccepted)
		return ctx, false, nil
	}

	// We have everything we need now, set the requesting
	// and receiving accounts on the context for later use.
	ctx = gtscontext.SetRequestingAccount(ctx, pubKeyAuth.Owner)
	ctx = gtscontext.SetReceivingAccount(ctx, receiver)

	// Note: we do not check here yet whether requesting
	// account has been suspended or self-deleted, as that
	// is handled in *federatingActor.PostInboxScheme
	return ctx, true, nil
}

// Blocked should determine whether to permit a set of actors given by
// their ids are able to interact with this particular end user due to
// being blocked or other application-specific logic.
func (f *Federator) Blocked(ctx context.Context, actorIRIs []*url.URL) (bool, error) {
	// Fetch relevant items from request context.
	// These should have been set further up the flow.
	receivingAccount := gtscontext.ReceivingAccount(ctx)
	if receivingAccount == nil {
		err := gtserror.New("couldn't determine blocks (receiving account not set on request context)")
		return false, err
	}

	requestingAccount := gtscontext.RequestingAccount(ctx)
	if requestingAccount == nil {
		err := gtserror.New("couldn't determine blocks (requesting account not set on request context)")
		return false, err
	}

	// This is a forwarded message if the actor
	// IRIs don't include the requesting account
	// (ie., the account doing the delivery).
	forwarded := !slices.ContainsFunc(
		actorIRIs,
		func(actorIRI *url.URL) bool {
			return actorIRI.String() == requestingAccount.URI
		},
	)

	l := log.
		WithContext(ctx).
		WithFields(kv.Fields{
			{"actorIRIs", actorIRIs},
			{"receivingAccount", receivingAccount.URI},
			{"requestingAccount", requestingAccount.URI},
			{"forwarded", forwarded},
		}...)
	l.Trace("checking blocks")

	// First ensure receiver does
	// not user-level block requester.
	if blocked, err := f.db.IsBlocked(ctx,
		receivingAccount.ID,
		requestingAccount.ID,
	); err != nil {
		err := gtserror.Newf("db error checking block between receiver and requester: %w", err)
		return false, err
	} else if blocked {
		l.Debug("receiving account blocks requesting account")
		return blocked, nil
	}

	// Check domain-level blocks for given actor IRIs;
	// if any of them are domain blocked then return.
	switch blocked, err := f.db.AreURIsBlocked(ctx, actorIRIs); {
	case err != nil:
		err := gtserror.Newf("db error checking domain blocks of actorIRIs: %w", err)
		return false, err

	case blocked && forwarded:
		// If this is a forwarded message then it's likely
		// an account from a Mastodon instance sending a
		// thread reply along to us. Eg., user@instance1
		// is replied to by user@instance2, forwards the
		// reply to us because we follow user@instance1.
		//
		// We don't want to return 403 to user@instance1
		// just because we block instance2, so just send
		// back a NotRelevant error the caller can check.
		const text = "forwarded activity actor domain blocked"
		l.Debug(text)
		err := gtserror.SetNotRelevant(errors.New(text))
		return false, err

	case blocked && !forwarded:
		// If one or more actors in the activity are domain
		// blocked and this isn't a forwarded message, 403.
		//
		// Actually we shouldn't reach this point because we
		// know the requesting account is one of the actors,
		// and domain blocks of requesting accounts should be
		// caught by signature check, but check to be safe.
		l.Debug("one or more actorIRIs are domain blocked")
		return blocked, nil
	}

	// We've established that no blocks exist between directly
	// involved actors, but what about IRIs of other actors and
	// objects which are tangentially involved in the activity
	// (ie., replied to, boosted)?
	//
	// If one or more of these other IRIs is domain blocked, or
	// blocked by the receiving account, this shouldn't return
	// blocked=true to send a 403, since that would be rather
	// silly behavior. Instead, we return a NotRelevant error
	// that the caller can check to just return 202 Accepted.
	otherIRIs := gtscontext.OtherIRIs(ctx)
	if otherIRIs == nil {
		err := gtserror.New("couldn't determine blocks (otherIRIs not set on request context)")
		return false, err
	}
	l = l.WithField("otherIRIs", otherIRIs)

	// Check domain blocks of each otherIRI entry first.
	if blocked, err := f.db.AreURIsBlocked(ctx, otherIRIs); err != nil {
		err := gtserror.Newf("error checking domain block of otherIRIs: %w", err)
		return false, err
	} else if blocked {
		const text = "one or more otherIRIs are domain blocked"
		l.Debug(text)
		err := gtserror.SetNotRelevant(errors.New(text))
		return false, err
	}

	// For each otherIRI entry, check whether the IRI
	// points to an account or a status, and try to get
	// (an) accountID(s) from it to do further checks on.
	//
	// We use a map for this instead of a slice in order to
	// deduplicate entries and avoid doing the same check twice.
	// The map value is the host of the otherIRI.
	accountIDs := make(map[string]string, len(otherIRIs))
	for _, iri := range otherIRIs {
		// Assemble iri string just once.
		iriStr := iri.String()

		account, err := f.db.GetAccountByURI(
			// We're on a hot path, fetch bare minimum.
			gtscontext.SetBarebones(ctx),
			iriStr,
		)
		if err != nil && !errors.Is(err, db.ErrNoEntries) {
			err := gtserror.Newf("db error getting account %s: %w", iriStr, err)
			return false, err
		} else if err == nil {
			// IRI is for an account.
			accountIDs[account.ID] = iri.Host
			continue
		}

		status, err := f.db.GetStatusByURI(
			// We're on a hot path, fetch bare minimum.
			gtscontext.SetBarebones(ctx),
			iriStr,
		)
		if err != nil && !errors.Is(err, db.ErrNoEntries) {
			err := gtserror.Newf("db error getting status %s: %w", iriStr, err)
			return false, err
		} else if err == nil {
			// IRI is for a status.
			accountIDs[status.AccountID] = iri.Host
			continue
		}
	}

	// Get our own host value
	// once outside the loop.
	ourHost := config.GetHost()

	for accountID, iriHost := range accountIDs {
		// Receiver shouldn't block other IRI owner.
		//
		// This check protects against cases where someone on our
		// instance is receiving a boost from someone they don't
		// block, but the boost target is the status of an account
		// they DO have blocked, or the boosted status mentions an
		// account they have blocked. In this case, it's v. unlikely
		// they care to see the boost in their timeline, so there's
		// no point in us processing it.
		if blocked, err := f.db.IsBlocked(ctx, receivingAccount.ID, accountID); err != nil {
			err := gtserror.Newf("db error checking block: %w", err)
			return false, err
		} else if blocked {
			const text = "receiving account blocks one or more otherIRIs"
			l.Debug(text)
			err := gtserror.SetNotRelevant(errors.New(text))
			return false, err
		}

		// If other account is from our instance (indicated by the
		// host of the URI stored in the map), ensure they don't block
		// the requester.
		//
		// This check protects against cases where one of our users
		// might be mentioned by the requesting account, and therefore
		// appear in otherIRIs, but the activity itself has been sent
		// to a different account on our instance. In other words, two
		// accounts are gossiping about + trying to tag a third account
		// who has one or the other of them blocked.
		if iriHost == ourHost {
			if blocked, err := f.db.IsBlocked(ctx, accountID, requestingAccount.ID); err != nil {
				err := gtserror.Newf("db error checking block: %w", err)
				return false, err
			} else if blocked {
				const text = "one or more otherIRIs belonging to us blocks requesting account"
				l.Debug(text)
				err := gtserror.SetNotRelevant(errors.New(text))
				return false, err
			}
		}
	}

	return false, nil
}

// FederatingCallbacks returns the application logic that handles
// ActivityStreams received from federating peers.
//
// Note that certain types of callbacks will be 'wrapped' with default
// behaviors supported natively by the library. Other callbacks
// compatible with streams.TypeResolver can be specified by 'other'.
//
// For example, setting the 'Create' field in the
// FederatingWrappedCallbacks lets an application dependency inject
// additional behaviors they want to take place, including the default
// behavior supplied by this library. This is guaranteed to be compliant
// with the ActivityPub Social protocol.
//
// To override the default behavior, instead supply the function in
// 'other', which does not guarantee the application will be compliant
// with the ActivityPub Social Protocol.
//
// Applications are not expected to handle every single ActivityStreams
// type and extension. The unhandled ones are passed to DefaultCallback.
func (f *Federator) FederatingCallbacks(ctx context.Context) (
	wrapped pub.FederatingWrappedCallbacks,
	other []any,
	err error,
) {
	wrapped = f.wrapped
	other = f.callback
	return
}

// DefaultCallback is called for types that go-fed can deserialize but
// are not handled by the application's callbacks returned in the
// Callbacks method.
//
// Applications are not expected to handle every single ActivityStreams
// type and extension, so the unhandled ones are passed to
// DefaultCallback.
func (f *Federator) DefaultCallback(ctx context.Context, activity pub.Activity) error {
	log.Debugf(ctx, "received unhandle-able activity type (%s) so ignoring it", activity.GetTypeName())
	return nil
}

// MaxInboxForwardingRecursionDepth determines how deep to search within
// an activity to determine if inbox forwarding needs to occur.
//
// Zero or negative numbers indicate infinite recursion.
func (f *Federator) MaxInboxForwardingRecursionDepth(ctx context.Context) int {
	// TODO
	return 4
}

// MaxDeliveryRecursionDepth determines how deep to search within
// collections owned by peers when they are targeted to receive a
// delivery.
//
// Zero or negative numbers indicate infinite recursion.
func (f *Federator) MaxDeliveryRecursionDepth(ctx context.Context) int {
	// TODO
	return 4
}

// FilterForwarding allows the implementation to apply business logic
// such as blocks, spam filtering, and so on to a list of potential
// Collections and OrderedCollections of recipients when inbox
// forwarding has been triggered.
//
// The activity is provided as a reference for more intelligent
// logic to be used, but the implementation must not modify it.
func (f *Federator) FilterForwarding(ctx context.Context, potentialRecipients []*url.URL, a pub.Activity) ([]*url.URL, error) {
	// TODO
	return []*url.URL{}, nil
}

// GetInbox returns the OrderedCollection inbox of the actor for this
// context. It is up to the implementation to provide the correct
// collection for the kind of authorization given in the request.
//
// AuthenticateGetInbox will be called prior to this.
//
// Always called, regardless whether the Federated Protocol or Social
// API is enabled.
func (f *Federator) GetInbox(ctx context.Context, r *http.Request) (vocab.ActivityStreamsOrderedCollectionPage, error) {
	// IMPLEMENTATION NOTE: For GoToSocial, we serve GETS to outboxes and inboxes through
	// the CLIENT API, not through the federation API, so we just do nothing here.
	return streams.NewActivityStreamsOrderedCollectionPage(), nil
}
