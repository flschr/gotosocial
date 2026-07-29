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

package workers

import (
	"context"
	"net/url"

	"code.superseriousbusiness.org/activity/streams"
	"code.superseriousbusiness.org/gopkg/log"
	"code.superseriousbusiness.org/gopkg/xslices"
	"code.superseriousbusiness.org/gotosocial/internal/ap"
	"code.superseriousbusiness.org/gotosocial/internal/federation"
	"code.superseriousbusiness.org/gotosocial/internal/filter/relay"
	"code.superseriousbusiness.org/gotosocial/internal/gtscontext"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"code.superseriousbusiness.org/gotosocial/internal/typeutils"
)

// federate wraps functions for federating
// something out via ActivityPub in response
// to message processing.
type federate struct {
	// Embed federator to give access
	// to send and retrieve functions.
	*federation.Federator
	state       *state.State
	relayFilter *relay.Filter
	converter   *typeutils.Converter
}

// parseURI is a cheeky little
// shortcut to wrap parsing errors.
//
// The returned err will be prepended
// with the name of the function that
// called this function, so it can be
// returned without further wrapping.
func parseURI(s string) (*url.URL, error) {
	const (
		// Provides enough calldepth to
		// prepend the name of whatever
		// function called *this* one,
		// so that they don't have to
		// wrap the error themselves.
		calldepth = 3
		errFmt    = "error parsing uri %s: %w"
	)

	uri, err := url.Parse(s)
	if err != nil {
		return nil, gtserror.NewfAt(calldepth, errFmt, s, err)
	}

	return uri, err
}

// RelayForwarding checks the given status to see if the author
// has configured any relay push connections that the status
// should be pushed to. If so, it will use the instance account
// to forward the activity to relevant relay actors' inboxes.
func (f *federate) RelayForwarding(
	ctx context.Context,
	status *gtsmodel.Status,
	activity ap.Activityable,
) error {
	// Check if we need to forward the status to any relay pushes.
	pushActorURIs, err := f.relayFilter.FilteredPushActorURIs(ctx, status)
	if err != nil {
		return gtserror.Newf("error filtering relay push actor URIs: %w", err)
	}

	// If there's no push actor URIs we
	// don't need to forward anywhere.
	l := len(pushActorURIs)
	if l == 0 {
		return nil
	}

	// Prepare list of (shared) inboxes
	// to forward this status to using
	// our instance service account.
	inboxes := make([]*url.URL, 0, l)
	for _, pushActorURI := range pushActorURIs {
		// Use the federatingDB to derive the
		// inbox URI for this push actor ID/URI.
		fdb := f.FederatingDB()
		inbox, err := fdb.InboxesForIRI(ctx, pushActorURI)
		if err != nil {
			log.Errorf(ctx, "error getting inbox for %s: %v", pushActorURI, err)
			continue
		}
		inboxes = append(inboxes, inbox...)
	}

	// Deduplicate inboxes to account for
	// any shared inboxes (multiple actors
	// on the same instance for example).
	inboxes = xslices.Deduplicate(inboxes)

	// Forwarding to relays is done using the instance account,
	// which is also the account we use to subscribe to relays.
	instanceAcct, err := f.state.DB.GetInstanceAccount(
		gtscontext.SetBarebones(ctx), "",
	)
	if err != nil {
		return gtserror.Newf("db error getting instance account: %w", err)
	}

	// Fetch transport to do deliveries.
	tsport, err := f.TransportController().NewTransport(
		instanceAcct.PublicKeyURI,
		instanceAcct.PrivateKey,
	)
	if err != nil {
		return gtserror.Newf("couldn't create transport: %w", err)
	}

	// Serialize the activity once.
	m, err := ap.Serialize(activity)
	if err != nil {
		return gtserror.Newf("error serializing %T: %w", activity, err)
	}

	// Prepare requests and add them to delivery queues.
	if err := tsport.BatchDeliver(ctx, m, inboxes); err != nil {
		return gtserror.Newf("error preparing delivery: %w", err)
	}

	return nil
}

func (f *federate) DeleteAccount(ctx context.Context, account *gtsmodel.Account) error {
	// Do nothing if it's not our
	// account that's been deleted.
	if !account.IsLocal() {
		return nil
	}

	// Parse relevant URI(s).
	outboxIRI, err := parseURI(account.OutboxURI)
	if err != nil {
		return err
	}

	actorIRI, err := parseURI(account.URI)
	if err != nil {
		return err
	}

	followersIRI, err := parseURI(account.FollowersURI)
	if err != nil {
		return err
	}

	// Create a new delete.
	// todo: tc.AccountToASDelete
	delete := streams.NewActivityStreamsDelete()

	// Set the Actor for the delete; no matter
	// who actually did the delete, we should
	// use the account owner for this.
	deleteActor := streams.NewActivityStreamsActorProperty()
	deleteActor.AppendIRI(actorIRI)
	delete.SetActivityStreamsActor(deleteActor)

	// Set the account's IRI as the 'object' property.
	deleteObject := streams.NewActivityStreamsObjectProperty()
	deleteObject.AppendIRI(actorIRI)
	delete.SetActivityStreamsObject(deleteObject)

	// Address the delete To followers.
	deleteTo := streams.NewActivityStreamsToProperty()
	deleteTo.AppendIRI(followersIRI)
	delete.SetActivityStreamsTo(deleteTo)

	// Address the delete CC public.
	deleteCC := streams.NewActivityStreamsCcProperty()
	deleteCC.AppendIRI(ap.PublicIRI())
	delete.SetActivityStreamsCc(deleteCC)

	// Send Delete via the Actor's outbox.
	if _, err := f.FederatingActor().Send(
		ctx, outboxIRI, delete,
	); err != nil {
		return gtserror.Newf(
			"error sending activity %T via outbox %s: %w",
			delete, outboxIRI, err,
		)
	}

	return nil
}

// CreateStatus sends the given status out to relevant
// recipients with the Outbox of the status creator.
func (f *federate) CreateStatus(ctx context.Context, status *gtsmodel.Status) error {
	// Do nothing if the status
	// shouldn't be federated.
	if status.LocalOnly() {
		return nil
	}

	// Do nothing if this
	// isn't our status.
	if !status.Flags.Local() {
		return nil
	}

	// Ensure the status model is fully populated.
	if err := f.state.DB.PopulateStatus(ctx, status); err != nil {
		return gtserror.Newf("error populating status: %w", err)
	}

	// Convert status to AS Statusable implementing type.
	statusable, err := f.converter.StatusToAS(ctx, status)
	if err != nil {
		return gtserror.Newf("error converting status to Statusable: %w", err)
	}

	// Parse the outbox URI of the status author.
	outboxIRI, err := parseURI(status.Account.OutboxURI)
	if err != nil {
		return err
	}

	// Wrap status in Create activity.
	create := typeutils.WrapStatusableInCreate(statusable)

	// Send the Create via the Actor's outbox.
	if _, err := f.FederatingActor().Send(
		ctx, outboxIRI, create,
	); err != nil {
		return gtserror.Newf(
			"error sending Create activity via outbox %s: %w",
			outboxIRI, err,
		)
	}

	// If this is a status of relayable visibility
	// (ie., public or unlisted), forward Create to
	// push relays configured by the status author.
	if status.ToOrCcPublic() {
		if err := f.RelayForwarding(
			ctx, status, create,
		); err != nil {
			return gtserror.Newf(
				"error relaying activity %T: %w",
				create, err,
			)
		}
	}

	return nil
}

func (f *federate) CreatePollVote(ctx context.Context, poll *gtsmodel.Poll, vote *gtsmodel.PollVote) error {
	// Extract status from poll.
	status := poll.Status

	// Do nothing if the status
	// shouldn't be federated.
	if status.LocalOnly() {
		return nil
	}

	// Do nothing if this is
	// a vote in our status.
	if status.Flags.Local() {
		return nil
	}

	// Parse the outbox URI of the poll vote author.
	outboxIRI, err := parseURI(vote.Account.OutboxURI)
	if err != nil {
		return err
	}

	// Convert vote to AS Creates with vote choices as Objects.
	creates, err := f.converter.PollVoteToASCreates(ctx, vote)
	if err != nil {
		return gtserror.Newf("error converting to notes: %w", err)
	}

	var errs gtserror.MultiError

	// Send each create activity.
	actor := f.FederatingActor()
	for _, create := range creates {
		if _, err := actor.Send(ctx, outboxIRI, create); err != nil {
			errs.Appendf("error sending Create activity via outbox %s: %w", outboxIRI, err)
		}
	}

	return errs.Combine()
}

func (f *federate) DeleteStatus(ctx context.Context, status *gtsmodel.Status) error {
	// Do nothing if the status
	// shouldn't be federated.
	if status.LocalOnly() {
		return nil
	}

	// Do nothing if this
	// isn't our status.
	if !status.Flags.Local() {
		return nil
	}

	// Ensure the status model is fully populated.
	if err := f.state.DB.PopulateStatus(ctx, status); err != nil {
		return gtserror.Newf("error populating status: %w", err)
	}

	// Convert status to ActivityStreams Statusable implementing type.
	statusable, err := f.converter.StatusToAS(ctx, status)
	if err != nil {
		return gtserror.Newf("error converting status to Statusable: %w", err)
	}

	// Parse the outbox URI of the status author.
	outboxIRI, err := parseURI(status.Account.OutboxURI)
	if err != nil {
		return err
	}

	// Wrap status in Delete activity.
	delete := typeutils.WrapStatusableInDelete(statusable)

	// Send the Delete via the Actor's outbox.
	if _, err := f.FederatingActor().Send(
		ctx, outboxIRI, delete,
	); err != nil {
		return gtserror.Newf(
			"error sending activity %T via outbox %s: %w",
			delete, outboxIRI, err,
		)
	}

	// If this is a status of relayable visibility
	// (ie., public or unlisted), forward Delete to
	// push relays configured by the status author.
	if status.ToOrCcPublic() {
		if err := f.RelayForwarding(
			ctx, status, delete,
		); err != nil {
			return gtserror.Newf(
				"error relaying activity %T: %w",
				delete, err,
			)
		}
	}

	return nil
}

// ForwardQuoteAuthorizationDelete forwards a remote authorization revocation
// to the audience of the locally-owned quote, as required by FEP-044f.
func (f *federate) ForwardQuoteAuthorizationDelete(
	ctx context.Context,
	req *gtsmodel.InteractionRequest,
	deleteID string,
	authorizationIRI *url.URL,
) error {
	if err := f.state.DB.PopulateInteractionRequest(ctx, req); err != nil {
		return gtserror.Newf("error populating quote interaction request: %w", err)
	}
	if req.InteractionType != gtsmodel.InteractionQuote ||
		req.Quote == nil ||
		!req.Quote.Flags.Local() {
		return nil
	}
	if deleteID == "" {
		return gtserror.New("incoming authorization Delete had no activity ID")
	}

	statusable, err := f.converter.StatusToAS(ctx, req.Quote)
	if err != nil {
		return gtserror.Newf("error converting quote status: %w", err)
	}

	delete := streams.NewActivityStreamsDelete()
	if err := ap.SetJSONLDIdStr(delete, deleteID); err != nil {
		return gtserror.Newf("error setting Delete ID: %w", err)
	}
	actorIRI, err := parseURI(req.TargetAccount.URI)
	if err != nil {
		return err
	}
	ap.AppendActorIRIs(delete, actorIRI)
	ap.AppendObjectIRIs(delete, authorizationIRI)

	recipients := make([]*url.URL, 0)
	publicURI := ap.PublicIRI().String()
	for _, iri := range ap.GetTo(statusable) {
		ap.AppendTo(delete, iri)
		if iri.String() != publicURI {
			recipients = append(recipients, iri)
		}
	}
	for _, iri := range ap.GetCc(statusable) {
		ap.AppendCc(delete, iri)
		if iri.String() != publicURI {
			recipients = append(recipients, iri)
		}
	}
	recipients = xslices.Deduplicate(recipients)

	inboxes := make([]*url.URL, 0, len(recipients))
	for _, recipient := range recipients {
		resolved, err := f.FederatingDB().InboxesForIRI(ctx, recipient)
		if err != nil {
			log.Errorf(ctx, "error resolving revocation recipient %s: %v", recipient, err)
			continue
		}
		inboxes = append(inboxes, resolved...)
	}
	inboxes = xslices.Deduplicate(inboxes)
	if len(inboxes) == 0 {
		return nil
	}

	instanceAcct, err := f.state.DB.GetInstanceAccount(
		gtscontext.SetBarebones(ctx), "",
	)
	if err != nil {
		return gtserror.Newf("db error getting instance account: %w", err)
	}
	tsport, err := f.TransportController().NewTransport(
		instanceAcct.PublicKeyURI,
		instanceAcct.PrivateKey,
	)
	if err != nil {
		return gtserror.Newf("couldn't create transport: %w", err)
	}
	serialized, err := ap.Serialize(delete)
	if err != nil {
		return gtserror.Newf("error serializing authorization Delete: %w", err)
	}
	if err := tsport.BatchDeliver(ctx, serialized, inboxes); err != nil {
		return gtserror.Newf("error preparing revocation deliveries: %w", err)
	}

	return nil
}

func (f *federate) UpdateStatus(ctx context.Context, status *gtsmodel.Status) error {
	// Do nothing if the status
	// shouldn't be federated.
	if status.LocalOnly() {
		return nil
	}

	// Do nothing if this
	// isn't our status.
	if !status.Flags.Local() {
		return nil
	}

	// Ensure the status model is fully populated.
	if err := f.state.DB.PopulateStatus(ctx, status); err != nil {
		return gtserror.Newf("error populating status: %w", err)
	}

	// Convert status to ActivityStreams Statusable implementing type.
	statusable, err := f.converter.StatusToAS(ctx, status)
	if err != nil {
		return gtserror.Newf("error converting status to Statusable: %w", err)
	}

	// Parse the outbox URI of the status author.
	outboxIRI, err := parseURI(status.Account.OutboxURI)
	if err != nil {
		return err
	}

	// Wrap status in Update activity.
	update := typeutils.WrapStatusableInUpdate(statusable)

	// Send the Update via the Actor's outbox.
	if _, err := f.FederatingActor().Send(
		ctx, outboxIRI, update,
	); err != nil {
		return gtserror.Newf(
			"error sending Update activity via outbox %s: %w",
			outboxIRI, err,
		)
	}

	// If this is a status of relayable visibility
	// (ie., public or unlisted), forward Update to
	// push relays configured by the status author.
	if status.ToOrCcPublic() {
		if err := f.RelayForwarding(
			ctx, status, update,
		); err != nil {
			return gtserror.Newf(
				"error relaying activity %T: %w",
				update, err,
			)
		}
	}

	return nil
}

func (f *federate) Follow(ctx context.Context, follow *gtsmodel.Follow) error {
	// Populate model.
	if err := f.state.DB.PopulateFollow(ctx, follow); err != nil {
		return gtserror.Newf("error populating follow: %w", err)
	}

	// Do nothing if both accounts are local.
	if follow.Account.IsLocal() &&
		follow.TargetAccount.IsLocal() {
		return nil
	}

	// Parse relevant URI(s).
	outboxIRI, err := parseURI(follow.Account.OutboxURI)
	if err != nil {
		return err
	}

	// Convert follow to ActivityStreams Follow.
	asFollow, err := f.converter.FollowToAS(ctx, follow)
	if err != nil {
		return gtserror.Newf("error converting follow to AS: %s", err)
	}

	// Send the Follow via the Actor's outbox.
	if _, err := f.FederatingActor().Send(
		ctx, outboxIRI, asFollow,
	); err != nil {
		return gtserror.Newf(
			"error sending activity %T via outbox %s: %w",
			asFollow, outboxIRI, err,
		)
	}

	return nil
}

func (f *federate) UndoFollow(ctx context.Context, follow *gtsmodel.Follow) error {
	// Populate model.
	if err := f.state.DB.PopulateFollow(ctx, follow); err != nil {
		return gtserror.Newf("error populating follow: %w", err)
	}

	// Do nothing if both accounts are local.
	if follow.Account.IsLocal() &&
		follow.TargetAccount.IsLocal() {
		return nil
	}

	// Parse relevant URI(s).
	outboxIRI, err := parseURI(follow.Account.OutboxURI)
	if err != nil {
		return err
	}

	targetAccountIRI, err := parseURI(follow.TargetAccount.URI)
	if err != nil {
		return err
	}

	// Recreate the ActivityStreams Follow.
	asFollow, err := f.converter.FollowToAS(ctx, follow)
	if err != nil {
		return gtserror.Newf("error converting follow to AS: %w", err)
	}

	// Create a new Undo.
	// todo: tc.FollowToASUndo
	undo := streams.NewActivityStreamsUndo()

	// Set the Actor for the Undo:
	// same as the actor for the Follow.
	undo.SetActivityStreamsActor(asFollow.GetActivityStreamsActor())

	// Set recreated Follow as the 'object' property.
	//
	// For most AP implementations, it's not enough
	// to just send the URI of the original Follow,
	// we have to send the whole object again.
	undoObject := streams.NewActivityStreamsObjectProperty()
	undoObject.AppendActivityStreamsFollow(asFollow)
	undo.SetActivityStreamsObject(undoObject)

	// Address the Undo To the target account.
	undoTo := streams.NewActivityStreamsToProperty()
	undoTo.AppendIRI(targetAccountIRI)
	undo.SetActivityStreamsTo(undoTo)

	// Send the Undo via the Actor's outbox.
	if _, err := f.FederatingActor().Send(
		ctx, outboxIRI, undo,
	); err != nil {
		return gtserror.Newf(
			"error sending activity %T via outbox %s: %w",
			undo, outboxIRI, err,
		)
	}

	return nil
}

func (f *federate) UndoLike(ctx context.Context, fave *gtsmodel.StatusFave) error {
	// Populate model.
	if err := f.state.DB.PopulateStatusFave(ctx, fave); err != nil {
		return gtserror.Newf("error populating fave: %w", err)
	}

	// Do nothing if both accounts are local.
	if fave.Account.IsLocal() &&
		fave.TargetAccount.IsLocal() {
		return nil
	}

	// Parse relevant URI(s).
	outboxIRI, err := parseURI(fave.Account.OutboxURI)
	if err != nil {
		return err
	}

	targetAccountIRI, err := parseURI(fave.TargetAccount.URI)
	if err != nil {
		return err
	}

	// Recreate the ActivityStreams Like.
	like, err := f.converter.FaveToAS(ctx, fave)
	if err != nil {
		return gtserror.Newf("error converting fave to AS: %w", err)
	}

	// Create a new Undo.
	// todo: tc.FaveToASUndo
	undo := streams.NewActivityStreamsUndo()

	// Set the Actor for the Undo:
	// same as the actor for the Like.
	undo.SetActivityStreamsActor(like.GetActivityStreamsActor())

	// Set recreated Like as the 'object' property.
	//
	// For most AP implementations, it's not enough
	// to just send the URI of the original Like,
	// we have to send the whole object again.
	undoObject := streams.NewActivityStreamsObjectProperty()
	undoObject.AppendActivityStreamsLike(like)
	undo.SetActivityStreamsObject(undoObject)

	// Address the Undo To the target account.
	undoTo := streams.NewActivityStreamsToProperty()
	undoTo.AppendIRI(targetAccountIRI)
	undo.SetActivityStreamsTo(undoTo)

	// Send the Undo via the Actor's outbox.
	if _, err := f.FederatingActor().Send(
		ctx, outboxIRI, undo,
	); err != nil {
		return gtserror.Newf(
			"error sending activity %T via outbox %s: %w",
			undo, outboxIRI, err,
		)
	}

	return nil
}

func (f *federate) UndoAnnounce(ctx context.Context, boost *gtsmodel.Status) error {
	// Populate model.
	if err := f.state.DB.PopulateStatus(ctx, boost); err != nil {
		return gtserror.Newf("error populating status: %w", err)
	}

	// Do nothing if boosting
	// account isn't ours.
	if !boost.Account.IsLocal() {
		return nil
	}

	// Parse relevant URI(s).
	outboxIRI, err := parseURI(boost.Account.OutboxURI)
	if err != nil {
		return err
	}

	// Recreate the ActivityStreams Announce.
	asAnnounce, err := f.converter.BoostToAS(ctx, boost)
	if err != nil {
		return gtserror.Newf("error converting boost to AS: %w", err)
	}

	// Create a new Undo.
	// todo: tc.AnnounceToASUndo
	undo := streams.NewActivityStreamsUndo()

	// Set the Actor for the Undo:
	// same as the actor for the Announce.
	undo.SetActivityStreamsActor(asAnnounce.GetActivityStreamsActor())

	// Set recreated Announce as the 'object' property.
	//
	// For most AP implementations, it's not enough
	// to just send the URI of the original Announce,
	// we have to send the whole object again.
	undoObject := streams.NewActivityStreamsObjectProperty()
	undoObject.AppendActivityStreamsAnnounce(asAnnounce)
	undo.SetActivityStreamsObject(undoObject)

	// Address the Undo To the Announce To.
	undo.SetActivityStreamsTo(asAnnounce.GetActivityStreamsTo())

	// Address the Undo CC the Announce CC.
	undo.SetActivityStreamsCc(asAnnounce.GetActivityStreamsCc())

	// Send the Undo via the Actor's outbox.
	if _, err := f.FederatingActor().Send(
		ctx, outboxIRI, undo,
	); err != nil {
		return gtserror.Newf(
			"error sending activity %T via outbox %s: %w",
			undo, outboxIRI, err,
		)
	}

	return nil
}

func (f *federate) AcceptFollow(ctx context.Context, follow *gtsmodel.Follow) error {
	// Populate model.
	if err := f.state.DB.PopulateFollow(ctx, follow); err != nil {
		return gtserror.Newf("error populating follow: %w", err)
	}

	// Bail if requesting account is ours:
	// we've already accepted internally and
	// shouldn't send an Accept to ourselves.
	if follow.Account.IsLocal() {
		return nil
	}

	// Bail if target account isn't ours:
	// we can't Accept a follow on
	// another instance's behalf.
	if follow.TargetAccount.IsRemote() {
		return nil
	}

	// Parse relevant URI(s).
	outboxIRI, err := parseURI(follow.TargetAccount.OutboxURI)
	if err != nil {
		return err
	}

	acceptingAccountIRI, err := parseURI(follow.TargetAccount.URI)
	if err != nil {
		return err
	}

	requestingAccountIRI, err := parseURI(follow.Account.URI)
	if err != nil {
		return err
	}

	// Recreate the ActivityStreams Follow.
	asFollow, err := f.converter.FollowToAS(ctx, follow)
	if err != nil {
		return gtserror.Newf("error converting follow to AS: %w", err)
	}

	// Create a new Accept.
	// todo: tc.FollowToASAccept
	accept := streams.NewActivityStreamsAccept()

	// Set the requestee as Actor of the Accept.
	acceptActorProp := streams.NewActivityStreamsActorProperty()
	acceptActorProp.AppendIRI(acceptingAccountIRI)
	accept.SetActivityStreamsActor(acceptActorProp)

	// Set recreated Follow as the 'object' property.
	//
	// For most AP implementations, it's not enough
	// to just send the URI of the original Follow,
	// we have to send the whole object again.
	acceptObject := streams.NewActivityStreamsObjectProperty()
	acceptObject.AppendActivityStreamsFollow(asFollow)
	accept.SetActivityStreamsObject(acceptObject)

	// Address the Accept To the Follow requester.
	acceptTo := streams.NewActivityStreamsToProperty()
	acceptTo.AppendIRI(requestingAccountIRI)
	accept.SetActivityStreamsTo(acceptTo)

	// Send the Accept via the Actor's outbox.
	if _, err := f.FederatingActor().Send(
		ctx, outboxIRI, accept,
	); err != nil {
		return gtserror.Newf(
			"error sending activity %T via outbox %s: %w",
			accept, outboxIRI, err,
		)
	}

	return nil
}

func (f *federate) RejectFollow(ctx context.Context, follow *gtsmodel.Follow) error {
	// Ensure follow populated before proceeding.
	if err := f.state.DB.PopulateFollow(ctx, follow); err != nil {
		return gtserror.Newf("error populating follow: %w", err)
	}

	// Bail if requesting account is ours:
	// we've already rejected internally and
	// shouldn't send an Reject to ourselves.
	if follow.Account.IsLocal() {
		return nil
	}

	// Bail if target account isn't ours:
	// we can't Reject a follow on
	// another instance's behalf.
	if follow.TargetAccount.IsRemote() {
		return nil
	}

	// Parse relevant URI(s).
	outboxIRI, err := parseURI(follow.TargetAccount.OutboxURI)
	if err != nil {
		return err
	}

	rejectingAccountIRI, err := parseURI(follow.TargetAccount.URI)
	if err != nil {
		return err
	}

	requestingAccountIRI, err := parseURI(follow.Account.URI)
	if err != nil {
		return err
	}

	// Recreate the ActivityStreams Follow.
	asFollow, err := f.converter.FollowToAS(ctx, follow)
	if err != nil {
		return gtserror.Newf("error converting follow to AS: %w", err)
	}

	// Create a new Reject.
	// todo: tc.FollowRequestToASReject
	reject := streams.NewActivityStreamsReject()

	// Set the requestee as Actor of the Reject.
	rejectActorProp := streams.NewActivityStreamsActorProperty()
	rejectActorProp.AppendIRI(rejectingAccountIRI)
	reject.SetActivityStreamsActor(rejectActorProp)

	// Set recreated Follow as the 'object' property.
	//
	// For most AP implementations, it's not enough
	// to just send the URI of the original Follow,
	// we have to send the whole object again.
	rejectObject := streams.NewActivityStreamsObjectProperty()
	rejectObject.AppendActivityStreamsFollow(asFollow)
	reject.SetActivityStreamsObject(rejectObject)

	// Address the Reject To the Follow requester.
	rejectTo := streams.NewActivityStreamsToProperty()
	rejectTo.AppendIRI(requestingAccountIRI)
	reject.SetActivityStreamsTo(rejectTo)

	// Send the Reject via the Actor's outbox.
	if _, err := f.FederatingActor().Send(
		ctx, outboxIRI, reject,
	); err != nil {
		return gtserror.Newf(
			"error sending activity %T via outbox %s: %w",
			reject, outboxIRI, err,
		)
	}

	return nil
}

// Like sends the given fave out to relevant
// recipients with the Outbox of the status creator.
func (f *federate) Like(ctx context.Context, fave *gtsmodel.StatusFave) error {
	// Populate model.
	if err := f.state.DB.PopulateStatusFave(ctx, fave); err != nil {
		return gtserror.Newf("error populating fave: %w", err)
	}

	// Do nothing if both accounts are local.
	if fave.Account.IsLocal() &&
		fave.TargetAccount.IsLocal() {
		return nil
	}

	// Create the ActivityStreams Like.
	like, err := f.converter.FaveToAS(ctx, fave)
	if err != nil {
		return gtserror.Newf("error converting fave to AS Like: %w", err)
	}

	// Parse relevant URI(s).
	outboxIRI, err := parseURI(fave.Account.OutboxURI)
	if err != nil {
		return err
	}

	// Send the Like via the Actor's outbox.
	if _, err := f.FederatingActor().Send(
		ctx, outboxIRI, like,
	); err != nil {
		return gtserror.Newf(
			"error sending activity %T via outbox %s: %w",
			like, outboxIRI, err,
		)
	}

	return nil
}

// Announce sends the given boost out to relevant
// recipients with the Outbox of the status creator.
func (f *federate) Announce(ctx context.Context, boost *gtsmodel.Status) error {
	// Populate model.
	if err := f.state.DB.PopulateStatus(ctx, boost); err != nil {
		return gtserror.Newf("error populating status: %w", err)
	}

	// Do nothing if boosting
	// account isn't ours.
	if !boost.Account.IsLocal() {
		return nil
	}

	// Create the ActivityStreams Announce.
	announce, err := f.converter.BoostToAS(ctx, boost)
	if err != nil {
		return gtserror.Newf("error converting boost to AS: %w", err)
	}

	// Parse relevant URI(s).
	outboxIRI, err := parseURI(boost.Account.OutboxURI)
	if err != nil {
		return err
	}

	// Send the Announce via the Actor's outbox.
	if _, err := f.FederatingActor().Send(
		ctx, outboxIRI, announce,
	); err != nil {
		return gtserror.Newf(
			"error sending activity %T via outbox %s: %w",
			announce, outboxIRI, err,
		)
	}

	return nil
}

func (f *federate) UpdateAccount(ctx context.Context, account *gtsmodel.Account) error {
	// Populate model.
	if err := f.state.DB.PopulateAccount(ctx, account); err != nil {
		return gtserror.Newf("error populating account: %w", err)
	}

	// Parse relevant URI(s).
	outboxIRI, err := parseURI(account.OutboxURI)
	if err != nil {
		return err
	}

	// Convert account to Accountable.
	accountable, err := f.converter.AccountToAS(ctx, account)
	if err != nil {
		return gtserror.Newf("error converting account to Person: %w", err)
	}

	// Use Accountable as Object of Update.
	update, err := f.converter.WrapAccountableInUpdate(accountable)
	if err != nil {
		return gtserror.Newf("error wrapping Person in Update: %w", err)
	}

	// Send the Update via the Actor's outbox.
	if _, err := f.FederatingActor().Send(
		ctx, outboxIRI, update,
	); err != nil {
		return gtserror.Newf(
			"error sending activity %T via outbox %s: %w",
			update, outboxIRI, err,
		)
	}

	return nil
}

func (f *federate) Block(ctx context.Context, block *gtsmodel.Block) error {
	// Populate model.
	if err := f.state.DB.PopulateBlock(ctx, block); err != nil {
		return gtserror.Newf("error populating block: %w", err)
	}

	// Do nothing if both accounts are local.
	if block.Account.IsLocal() &&
		block.TargetAccount.IsLocal() {
		return nil
	}

	// Parse relevant URI(s).
	outboxIRI, err := parseURI(block.Account.OutboxURI)
	if err != nil {
		return err
	}

	// Convert block to ActivityStreams Block.
	asBlock, err := f.converter.BlockToAS(ctx, block)
	if err != nil {
		return gtserror.Newf("error converting block to AS: %w", err)
	}

	// Send the Block via the Actor's outbox.
	if _, err := f.FederatingActor().Send(
		ctx, outboxIRI, asBlock,
	); err != nil {
		return gtserror.Newf(
			"error sending activity %T via outbox %s: %w",
			asBlock, outboxIRI, err,
		)
	}

	return nil
}

func (f *federate) UndoBlock(ctx context.Context, block *gtsmodel.Block) error {
	// Populate model.
	if err := f.state.DB.PopulateBlock(ctx, block); err != nil {
		return gtserror.Newf("error populating block: %w", err)
	}

	// Do nothing if both accounts are local.
	if block.Account.IsLocal() &&
		block.TargetAccount.IsLocal() {
		return nil
	}

	// Parse relevant URI(s).
	outboxIRI, err := parseURI(block.Account.OutboxURI)
	if err != nil {
		return err
	}

	targetAccountIRI, err := parseURI(block.TargetAccount.URI)
	if err != nil {
		return err
	}

	// Convert block to ActivityStreams Block.
	asBlock, err := f.converter.BlockToAS(ctx, block)
	if err != nil {
		return gtserror.Newf("error converting block to AS: %w", err)
	}

	// Create a new Undo.
	// todo: tc.BlockToASUndo
	undo := streams.NewActivityStreamsUndo()

	// Set the Actor for the Undo:
	// same as the actor for the Block.
	undo.SetActivityStreamsActor(asBlock.GetActivityStreamsActor())

	// Set Block as the 'object' property.
	//
	// For most AP implementations, it's not enough
	// to just send the URI of the original Block,
	// we have to send the whole object again.
	undoObject := streams.NewActivityStreamsObjectProperty()
	undoObject.AppendActivityStreamsBlock(asBlock)
	undo.SetActivityStreamsObject(undoObject)

	// Address the Undo To the target account.
	undoTo := streams.NewActivityStreamsToProperty()
	undoTo.AppendIRI(targetAccountIRI)
	undo.SetActivityStreamsTo(undoTo)

	// Send the Undo via the Actor's outbox.
	if _, err := f.FederatingActor().Send(
		ctx, outboxIRI, undo,
	); err != nil {
		return gtserror.Newf(
			"error sending activity %T via outbox %s: %w",
			undo, outboxIRI, err,
		)
	}

	return nil
}

func (f *federate) Flag(ctx context.Context, report *gtsmodel.Report) error {
	// Populate model.
	if err := f.state.DB.PopulateReport(ctx, report); err != nil {
		return gtserror.Newf("error populating report: %w", err)
	}

	// Do nothing if report target
	// is not remote account.
	if report.TargetAccount.IsLocal() {
		return nil
	}

	// Get our instance account from the db:
	// to anonymize the report, we'll deliver
	// using the outbox of the instance account.
	instanceAcct, err := f.state.DB.GetInstanceAccount(ctx, "")
	if err != nil {
		return gtserror.Newf("error getting instance account: %w", err)
	}

	// Parse relevant URI(s).
	outboxIRI, err := parseURI(instanceAcct.OutboxURI)
	if err != nil {
		return err
	}

	targetAccountIRI, err := parseURI(report.TargetAccount.URI)
	if err != nil {
		return err
	}

	// Convert report to ActivityStreams Flag.
	flag, err := f.converter.ReportToASFlag(ctx, report)
	if err != nil {
		return gtserror.Newf("error converting report to AS: %w", err)
	}

	// To is not set explicitly on Flags. Instead,
	// address Flag BTo report target account URI.
	// This ensures that our federating actor still
	// knows where to send the report, but the BTo
	// property will be stripped before sending.
	//
	// Happily, BTo does not prevent federating
	// actor from using shared inbox to deliver.
	bTo := streams.NewActivityStreamsBtoProperty()
	bTo.AppendIRI(targetAccountIRI)
	flag.SetActivityStreamsBto(bTo)

	// Send the Flag via the Actor's outbox.
	if _, err := f.FederatingActor().Send(
		ctx, outboxIRI, flag,
	); err != nil {
		return gtserror.Newf(
			"error sending activity %T via outbox %s: %w",
			flag, outboxIRI, err,
		)
	}

	return nil
}

func (f *federate) MoveAccount(ctx context.Context, account *gtsmodel.Account) error {
	// Do nothing if it's not our
	// account that's been moved.
	if !account.IsLocal() {
		return nil
	}

	// Parse relevant URI(s).
	outboxIRI, err := parseURI(account.OutboxURI)
	if err != nil {
		return err
	}

	// Actor doing the Move.
	actorIRI := account.Move.Origin

	// Destination Actor of the Move.
	targetIRI := account.Move.Target

	followersIRI, err := parseURI(account.FollowersURI)
	if err != nil {
		return err
	}

	// Create a new move.
	move := streams.NewActivityStreamsMove()

	// Set the Move ID.
	if err := ap.SetJSONLDIdStr(move, account.Move.URI); err != nil {
		return err
	}

	// Set the Actor for the Move.
	ap.AppendActorIRIs(move, actorIRI)

	// Set the account's IRI as the 'object' property.
	ap.AppendObjectIRIs(move, actorIRI)

	// Set the target's IRI as the 'target' property.
	ap.AppendTargetIRIs(move, targetIRI)

	// Address the move To followers.
	ap.AppendTo(move, followersIRI)

	// Address the move CC public.
	ap.AppendCc(move, ap.PublicIRI())

	// Send the Move via the Actor's outbox.
	if _, err := f.FederatingActor().Send(
		ctx, outboxIRI, move,
	); err != nil {
		return gtserror.Newf(
			"error sending activity %T via outbox %s: %w",
			move, outboxIRI, err,
		)
	}

	return nil
}

func (f *federate) AcceptInteraction(
	ctx context.Context,
	req *gtsmodel.InteractionRequest,
) error {
	// Populate model.
	if err := f.state.DB.PopulateInteractionRequest(ctx, req); err != nil {
		return gtserror.Newf("error populating request: %w", err)
	}

	// Bail if interacting account is ours:
	// we've already accepted internally and
	// shouldn't send an Accept to ourselves.
	if req.InteractingAccount.IsLocal() {
		return nil
	}

	// Bail if account isn't ours:
	// we can't Accept on another
	// instance's behalf. (This
	// should never happen but...)
	if req.TargetAccount.IsRemote() {
		return nil
	}

	// Parse outbox URI.
	outboxIRI, err := parseURI(req.TargetAccount.OutboxURI)
	if err != nil {
		return err
	}

	// Convert req to Accept.
	accept, err := f.converter.InteractionReqToASAccept(ctx, req)
	if err != nil {
		return gtserror.Newf("error converting request to Accept: %w", err)
	}

	// Send the Accept via the Actor's outbox.
	if _, err := f.FederatingActor().Send(
		ctx, outboxIRI, accept,
	); err != nil {
		return gtserror.Newf(
			"error sending activity %T for %v via outbox %s: %w",
			accept, req.InteractionType, outboxIRI, err,
		)
	}

	return nil
}

func (f *federate) RejectInteraction(
	ctx context.Context,
	req *gtsmodel.InteractionRequest,
) error {
	// Populate model.
	if err := f.state.DB.PopulateInteractionRequest(ctx, req); err != nil {
		return gtserror.Newf("error populating request: %w", err)
	}

	// Bail if interacting account is ours:
	// we've already rejected internally and
	// shouldn't send an Reject to ourselves.
	if req.InteractingAccount.IsLocal() {
		return nil
	}

	// Bail if account isn't ours:
	// we can't Reject on another
	// instance's behalf. (This
	// should never happen but...)
	if req.TargetAccount.IsRemote() {
		return nil
	}

	// Parse outbox URI.
	outboxIRI, err := parseURI(req.TargetAccount.OutboxURI)
	if err != nil {
		return err
	}

	// Convert req to Reject.
	reject, err := f.converter.InteractionReqToASReject(ctx, req)
	if err != nil {
		return gtserror.Newf("error converting request to Reject: %w", err)
	}

	// Send the Reject via the Actor's outbox.
	if _, err := f.FederatingActor().Send(
		ctx, outboxIRI, reject,
	); err != nil {
		return gtserror.Newf(
			"error sending activity %T for %v via outbox %s: %w",
			reject, req.InteractionType, outboxIRI, err,
		)
	}

	return nil
}

// InteractionRequest sends out the given
// *gtsmodel.InteractionRequest as a polite
// LikeRequest, ReplyRequest, or AnnounceRequest,
// to the interaction target's inbox.
func (f *federate) InteractionRequest(
	ctx context.Context,
	req *gtsmodel.InteractionRequest,
) error {
	// Populate model.
	if err := f.state.DB.PopulateInteractionRequest(ctx, req); err != nil {
		return gtserror.Newf("error populating request: %w", err)
	}

	// Bail if the interacter is remote
	// or the target is local, we don't
	// need to do anything then.
	if req.InteractingAccount.IsRemote() ||
		req.TargetAccount.IsLocal() {
		return nil
	}

	// Bail if the request is
	// already approved or rejected.
	if !req.IsPending() {
		return nil
	}

	// Parse outbox URI.
	outboxIRI, err := parseURI(req.InteractingAccount.OutboxURI)
	if err != nil {
		return err
	}

	// Convert req to InteractionRequestable.
	intReqable, err := f.converter.InteractionReqToASInteractionRequestable(ctx, req)
	if err != nil {
		return gtserror.Newf("error converting request to InteractionRequestable: %w", err)
	}

	// Send the interaction request
	// via the Actor's outbox.
	if _, err := f.FederatingActor().Send(
		ctx, outboxIRI, intReqable,
	); err != nil {
		return gtserror.Newf(
			"error sending activity %T for %v via outbox %s: %w",
			intReqable, req.InteractionType, outboxIRI, err,
		)
	}

	return nil
}
