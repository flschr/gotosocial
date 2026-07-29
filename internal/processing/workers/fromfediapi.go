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
	"errors"
	"net/url"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/ap"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/federation/dereferencing"
	"code.superseriousbusiness.org/gotosocial/internal/gtscontext"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/surfacing"
	"code.superseriousbusiness.org/gotosocial/internal/typeutils"
	"code.superseriousbusiness.org/gotosocial/internal/uris"
	"codeberg.org/gruf/go-kv/v2"

	"code.superseriousbusiness.org/gopkg/log"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/messages"
	"code.superseriousbusiness.org/gotosocial/internal/processing/account"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"code.superseriousbusiness.org/gotosocial/internal/util"
)

// fediAPI wraps processing functions
// specifically for messages originating
// from the federation/ActivityPub API.
type fediAPI struct {
	state    *state.State
	surfacer *surfacing.Surfacer
	federate *federate
	account  *account.Processor
	utils    *utils
}

func (p *Processor) ProcessFromFediAPI(ctx context.Context, fMsg *messages.FromFediAPI) error {
	// Allocate new log fields slice
	fields := make([]kv.Field, 3, 5)
	fields[0] = kv.Field{"activityType", fMsg.APActivityType}
	fields[1] = kv.Field{"objectType", fMsg.APObjectType}
	fields[2] = kv.Field{"toAccount", fMsg.Receiving.Username}

	if fMsg.APIRI != nil {
		// An IRI was supplied, append to log
		fields = append(fields, kv.Field{
			"iri", fMsg.APIRI,
		})
	}

	// Include GTSModel in logs if appropriate.
	if fMsg.GTSModel != nil &&
		log.Level() <= log.DEBUG {
		fields = append(fields, kv.Field{
			"model", fMsg.GTSModel,
		})
	}

	l := log.WithContext(ctx).WithFields(fields...)
	l.Info("processing from fedi API")

	switch fMsg.APActivityType {

	// CREATE SOMETHING
	case ap.ActivityCreate:
		switch fMsg.APObjectType {

		// CREATE NOTE/STATUS
		case ap.ObjectNote:
			return p.fediAPI.CreateStatus(ctx, fMsg)

		// REQUEST TO REPLY TO A STATUS
		case ap.ActivityReplyRequest:
			return p.fediAPI.CreateReplyRequest(ctx, fMsg)

		// CREATE FOLLOW (request)
		case ap.ActivityFollow:
			return p.fediAPI.CreateFollowReq(ctx, fMsg)

		// CREATE LIKE/FAVE
		case ap.ActivityLike:
			return p.fediAPI.CreateLike(ctx, fMsg)

		// REQUEST TO LIKE A STATUS
		case ap.ActivityLikeRequest:
			return p.fediAPI.CreateLikeRequest(ctx, fMsg)

		// CREATE ANNOUNCE/BOOST
		case ap.ActivityAnnounce:
			return p.fediAPI.CreateAnnounce(ctx, fMsg)

		// REQUEST TO BOOST A STATUS
		case ap.ActivityAnnounceRequest:
			return p.fediAPI.CreateAnnounceRequest(ctx, fMsg)

		// REQUEST TO QUOTE A STATUS
		case ap.ActivityQuoteRequest:
			return p.fediAPI.CreateQuoteRequest(ctx, fMsg)

		// CREATE BLOCK
		case ap.ActivityBlock:
			return p.fediAPI.CreateBlock(ctx, fMsg)

		// CREATE FLAG/REPORT
		case ap.ActivityFlag:
			return p.fediAPI.CreateFlag(ctx, fMsg)

		// CREATE QUESTION
		case ap.ActivityQuestion:
			return p.fediAPI.CreatePollVote(ctx, fMsg)
		}

	// UPDATE SOMETHING
	case ap.ActivityUpdate:
		switch fMsg.APObjectType {

		// UPDATE NOTE/STATUS
		case ap.ObjectNote:
			return p.fediAPI.UpdateStatus(ctx, fMsg)

		// UPDATE ACCOUNT
		case ap.ActorPerson:
			return p.fediAPI.UpdateAccount(ctx, fMsg)

		// UPDATE QUESTION
		case ap.ActivityQuestion:
			return p.fediAPI.UpdatePollVote(ctx, fMsg)
		}

	// ACCEPT SOMETHING
	case ap.ActivityAccept:
		switch fMsg.APObjectType {

		// ACCEPT (pending) FOLLOW
		case ap.ActivityFollow:
			return p.fediAPI.AcceptFollow(ctx, fMsg)

		// ACCEPT (pending) LIKE
		case ap.ActivityLike:
			return p.fediAPI.AcceptLike(ctx, fMsg)

		// ACCEPT (pending) REPLY
		case ap.ObjectNote:
			return p.fediAPI.AcceptReply(ctx, fMsg)

		// ACCEPT (pending) POLITE REPLY REQUEST
		case ap.ActivityReplyRequest:
			return p.fediAPI.AcceptPoliteReplyRequest(ctx, fMsg)

		// ACCEPT (pending) POLITE QUOTE REQUEST
		case ap.ActivityQuoteRequest:
			return p.fediAPI.AcceptPoliteQuoteRequest(ctx, fMsg)

		// ACCEPT (pending) ANNOUNCE
		case ap.ActivityAnnounce:
			return p.fediAPI.AcceptAnnounce(ctx, fMsg)

		// ACCEPT (remote) IMPOLITE REPLY or ANNOUNCE
		case ap.ObjectUnknown:
			return p.fediAPI.AcceptRemoteStatus(ctx, fMsg)
		}

	// REJECT SOMETHING
	case ap.ActivityReject:
		switch fMsg.APObjectType {

		// REJECT LIKE
		case ap.ActivityLike:
			return p.fediAPI.RejectLike(ctx, fMsg)

		// REJECT NOTE/STATUS (ie., reject a reply)
		case ap.ObjectNote:
			return p.fediAPI.RejectReply(ctx, fMsg)

		// REJECT BOOST
		case ap.ActivityAnnounce:
			return p.fediAPI.RejectAnnounce(ctx, fMsg)
		}

	// DELETE SOMETHING
	case ap.ActivityDelete:
		switch fMsg.APObjectType {

		// DELETE / REVOKE QUOTE AUTHORIZATION
		case ap.ObjectQuoteAuthorization:
			return p.fediAPI.DeleteQuoteAuthorization(ctx, fMsg)

		// DELETE NOTE/STATUS
		case ap.ObjectNote:
			return p.fediAPI.DeleteStatus(ctx, fMsg)

		// DELETE ACCOUNT
		case ap.ActorPerson:
			return p.fediAPI.DeleteAccount(ctx, fMsg)
		}

	// MOVE SOMETHING
	case ap.ActivityMove:

		// MOVE ACCOUNT
		// fromfediapi_move.go.
		if fMsg.APObjectType == ap.ActorPerson {
			return p.fediAPI.MoveAccount(ctx, fMsg)
		}

	// UNDO SOMETHING
	case ap.ActivityUndo:

		switch fMsg.APObjectType {
		// UNDO FOLLOW
		case ap.ActivityFollow:
			return p.fediAPI.UndoFollow(ctx, fMsg)

		// UNDO BLOCK
		case ap.ActivityBlock:
			return p.fediAPI.UndoBlock(ctx, fMsg)

		// UNDO ANNOUNCE
		case ap.ActivityAnnounce:
			return p.fediAPI.UndoAnnounce(ctx, fMsg)

		// UNDO LIKE
		case ap.ActivityLike:
			return p.fediAPI.UndoFave(ctx, fMsg)
		}
	}

	return gtserror.Newf("unhandled: %s %s", fMsg.APActivityType, fMsg.APObjectType)
}

// CreateStatus handles the creation of a status/post sent as a Create message.
// It is also capable of handling impolite reply requests to local + remote statuses,
// ie., replies sent directly without doing the ReplyRequest process first.
func (p *fediAPI) CreateStatus(ctx context.Context, fMsg *messages.FromFediAPI) error {

	// If we received this status via our instance account,
	// it must originate from a relay actor we subscribe to.
	if fMsg.Receiving.IsInstance() {
		return p.createStatusFromRelay(ctx, fMsg)
	}

	// Non-relay status,
	// process normally.
	var (
		status     *gtsmodel.Status
		statusable ap.Statusable
		err        error
	)

	var ok bool

	switch {
	case fMsg.APObject != nil:
		// A model was provided, extract this from message.
		statusable, ok = fMsg.APObject.(ap.Statusable)
		if !ok {
			return gtserror.Newf("cannot cast %T -> ap.Statusable", fMsg.APObject)
		}

		// Create bare-bones model to pass
		// into RefreshStatus(), which it will
		// further populate and insert as new.
		bareStatus := new(gtsmodel.Status)
		bareStatus.Flags.SetLocal(false)
		bareStatus.URI = ap.GetJSONLDId(statusable).String()

		// Call RefreshStatus() to parse and process the provided
		// statusable model, which it will use to further flesh out
		// the bare bones model and insert it into the database.
		//
		// NOTE: dereferencer hook will handle surfacing logic.
		status, statusable, err = p.federate.RefreshStatus(ctx,
			fMsg.Receiving.Username,
			bareStatus,
			statusable,

			// Force refresh
			// within 5min window.
			&dereferencing.Fresh,
		)
		if err != nil {
			return gtserror.Newf("error processing new status %s: %w", bareStatus.URI, err)
		}

	case fMsg.APIRI != nil:
		// Model was not set, deref with IRI (this is a forward).
		// This will also cause the status to be inserted into the db.
		//
		// NOTE: dereferencer hook will handle surfacing logic.
		status, statusable, err = p.federate.GetStatusByURI(ctx,
			fMsg.Receiving.Username,
			fMsg.APIRI,
		)
		if err != nil {
			return gtserror.Newf("error dereferencing forwarded status %s: %w", fMsg.APIRI, err)
		}

	default:
		return gtserror.New("neither APObjectModel nor APIri set")
	}

	if statusable == nil {
		// Another thread beat us to
		// creating this status! Return
		// here and let the other thread
		// handle timelining + notifying.
		return nil
	}

	// If pending approval is true then
	// status must reply to a LOCAL status
	// that requires approval for the reply.
	if status.Flags.PendingApproval() {
		intReqID := id.NewULIDFromTime(status.CreatedAt)
		intReq := &gtsmodel.InteractionRequest{
			ID:                    intReqID,
			TargetStatusID:        status.InReplyToID,
			TargetStatus:          status.InReplyTo,
			TargetAccountID:       status.InReplyToAccountID,
			TargetAccount:         status.InReplyToAccount,
			InteractingAccountID:  status.AccountID,
			InteractingAccount:    status.Account,
			InteractionRequestURI: status.URI + gtsmodel.ImpoliteReplyRequestSuffix,
			InteractionURI:        status.URI,
			InteractionType:       gtsmodel.InteractionReply,
			Polite:                util.Ptr(false),
			Reply:                 status,
		}

		if !status.PreApproved {
			// If approval is required and reply isn't
			// preapproved, just store the interaction request
			// and notify the account that's being interacted
			// with, they can handle the interaction later.
			if err := p.utils.storeInteractionRequest(
				ctx, intReq,
			); err != nil {
				return gtserror.Newf("error storing interaction request: %w", err)
			}

			// Notify target account (if local) of pending reply.
			if err := p.surfacer.NotifyPendingReply(ctx, intReq.Reply); err != nil {
				return gtserror.Newf("error notifying pending reply: %w", err)
			}

			return nil
		}

		// If approval is required and status *is* preapproved,
		// that means this is a reply to one of our statuses
		// that was allowed based on replier's presence in a
		// following/followers collection.
		//
		// Mark the interaction request as accepted, store it,
		// mark the interaction as approved, and then continue
		// with side effects as normal.

		// Update intReq fields to
		// mark it as accepted.
		intReq.MarkAccepted()

		// Put it in the DB.
		if err := p.utils.storeInteractionRequest(
			ctx, intReq,
		); err != nil {
			return gtserror.Newf("error storing interaction request: %w", err)
		}

		// Mark the status as now approved, referring to
		// the accepted interaction request we just stored.
		status.PreApproved = false
		status.Flags.SetPendingApproval(false)
		status.ApprovedByURI = intReq.AuthorizationURI
		if err := p.state.DB.UpdateStatus(ctx,
			status,
			"flags",
			"approved_by_uri",
		); err != nil {
			return gtserror.Newf("db error updating status: %w", err)
		}

		// Send out the approval as Accept.
		if err := p.federate.AcceptInteraction(ctx, intReq); err != nil {
			return gtserror.Newf("error federating pre-approval of reply: %w", err)
		}

		// Timeline and notify the status, since the missing ApprovedByURI during
		// status dereferencer stage will cause it to skip typical surfacing logic.
		if err := p.surfacer.TimelineAndNotifyStatus(ctx, status); err != nil {
			log.Errorf(ctx, "error timelining and notifying status: %v", err)
		}
	}

	return nil
}

func (p *fediAPI) createStatusFromRelay(ctx context.Context, fMsg *messages.FromFediAPI) error {
	// Retrieve the relayed status, and store it
	// if it's matched by a relay subscription.
	//
	// NOTE: dereferencer hook handles surfacing logic.
	status, err := p.federate.GetRelayedStatus(ctx,
		fMsg.Receiving,  // Our instance account.
		fMsg.Requesting, // Relaying account.
		fMsg.APIRI,      // Relayed status ID/URI.
	)

	uriStr := fMsg.APIRI.String()
	l := log.
		WithContext(ctx).
		WithField("uri", uriStr)

	// Check for semi-expected errors
	// encountered during dereferencing.
	switch {
	case err == nil:
		// No problem.

	case gtserror.IsUnretrievable(err):
		// Remote domain probably blocked.
		l.Debugf("status not retrievable: %v", err)
		return nil

	case gtserror.IsMalformed(err):
		// Final URI probably borked.
		l.Debugf("status malformed: %v", err)
		return nil

	default:
		// Actual error (db error, transport error, etc).
		return gtserror.Newf("error dereferencing %s: %w", uriStr, err)
	}

	// If status is nil that means it probably
	// didn't match a relay subscription, or it
	// wasn't permitted by interaction policy checks.
	if status == nil {
		l.Debug("status dropped as not relayable")
		return nil
	}

	return nil
}

// CreateReplyRequest handles a polite ReplyRequest.
// This is distinct from CreateStatus, which is capable
// of handling both "normal" top-level status creation,
// in addition to *impolite* reply requests.
func (p *fediAPI) CreateReplyRequest(ctx context.Context, fMsg *messages.FromFediAPI) error {
	// Extract the ap model Statusable
	// set by the federating db.
	statusable, ok := fMsg.APObject.(ap.Statusable)
	if !ok {
		return gtserror.Newf("cannot cast %T -> ap.Statusable", fMsg.APObject)
	}

	// Call RefreshStatus to parse and process the
	// statusable. This will also check permissions.
	replyURI := ap.GetJSONLDId(statusable).String()
	reply, _, err := p.federate.RefreshStatus(ctx,
		fMsg.Receiving.Username,
		&gtsmodel.Status{URI: replyURI},
		statusable,

		// Force refresh
		// within 5min window.
		&dereferencing.Fresh,
	)

	switch {
	case err == nil:
		// All fine.

	case gtserror.IsNotPermitted(err):
		// Reply is straight up not permitted by
		// the interaction policy of the status
		// it's replying to. Nothing more to do.
		log.Debugf(ctx,
			"dropping unpermitted ReplyRequest with instrument %s",
			replyURI,
		)
		return nil

	default:
		// There's some real error.
		return gtserror.Newf(
			"error processing ReplyRequest with instrument %s: %w",
			replyURI, err,
		)
	}

	// The reply is permitted. Check if we
	// should send out an Accept immediately.
	manualApproval := reply.Flags.PendingApproval() && !reply.PreApproved
	if manualApproval {

		// The reply requires manual approval.
		//
		// Just notify target account about
		// the requested interaction.
		if err := p.surfacer.NotifyPendingReply(ctx, reply); err != nil {
			return gtserror.Newf("error notifying pending reply: %w", err)
		}
		return nil
	}

	// The reply is automatically approved,
	// handle side effects of this.
	req, ok := fMsg.GTSModel.(*gtsmodel.InteractionRequest)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.InteractionRequest", fMsg.GTSModel)
	}

	// Mark the request as accepted.
	req.AcceptedAt = time.Now()
	req.ResponseURI = uris.GenerateURIForAccept(
		req.TargetAccount.Username, req.ID)
	req.AuthorizationURI = uris.GenerateURIForAuthorization(
		req.TargetAccount.Username, req.ID)

	// Update in the db.
	if err := p.state.DB.UpdateInteractionRequest(
		ctx,
		req,
		"accepted_at",
		"response_uri",
		"authorization_uri",
	); err != nil {
		return gtserror.Newf("db error updating interaction request: %w", err)
	}

	// Mark the reply as now approved, referring to
	// the accepted interaction request we just stored.
	reply.PreApproved = false
	reply.Flags.SetPendingApproval(false)
	reply.ApprovedByURI = req.AuthorizationURI
	if err := p.state.DB.UpdateStatus(ctx,
		reply,
		"flags",
		"approved_by_uri",
	); err != nil {
		return gtserror.Newf("db error updating status: %w", err)
	}

	// Send out the accept.
	if err := p.federate.AcceptInteraction(ctx, req); err != nil {
		log.Errorf(ctx, "error federating accept: %v", err)
	}

	// Timeline and notify the status, since the missing ApprovedByURI during
	// status dereferencer stage will cause it to skip typical surfacing logic.
	if err := p.surfacer.TimelineAndNotifyStatus(ctx, reply); err != nil {
		log.Errorf(ctx, "error timelining and notifying status: %v", err)
	}

	return nil
}

func (p *fediAPI) CreatePollVote(ctx context.Context, fMsg *messages.FromFediAPI) error {
	// Cast poll vote type from the worker message.
	vote, ok := fMsg.GTSModel.(*gtsmodel.PollVote)
	if !ok {
		return gtserror.Newf("cannot cast %T -> *gtsmodel.PollVote", fMsg.GTSModel)
	}

	// Insert the new poll vote in the database, note this
	// will handle updating votes on the poll model itself.
	if err := p.state.DB.PutPollVote(ctx, vote); err != nil {
		return gtserror.Newf("error inserting poll vote in db: %w", err)
	}

	// Ensure the poll vote is fully populated at this point.
	if err := p.state.DB.PopulatePollVote(ctx, vote); err != nil {
		return gtserror.Newf("error populating poll vote from db: %w", err)
	}

	// Ensure the poll on the vote is fully populated to get origin status.
	if err := p.state.DB.PopulatePoll(ctx, vote.Poll); err != nil {
		return gtserror.Newf("error populating poll from db: %w", err)
	}

	// Get the origin status,
	// (also set the poll on it).
	status := vote.Poll.Status
	status.Poll = vote.Poll

	if status.Flags.Local() {
		// Before federating it, increment the poll vote
		// and voter counts, *only on our local copy*.
		status.Poll.IncrementVotes(vote.Choices, true)

		// These were poll votes in a local status, we need to
		// federate the updated status model with latest vote counts.
		if err := p.federate.UpdateStatus(ctx, status); err != nil {
			log.Errorf(ctx, "error federating status update: %v", err)
		}
	}

	return nil
}

func (p *fediAPI) UpdatePollVote(ctx context.Context, fMsg *messages.FromFediAPI) error {
	// Cast poll vote type from the worker message.
	vote, ok := fMsg.GTSModel.(*gtsmodel.PollVote)
	if !ok {
		return gtserror.Newf("cannot cast %T -> *gtsmodel.PollVote", fMsg.GTSModel)
	}

	// Update poll vote model (specifically only choices) in the database.
	if err := p.state.DB.UpdatePollVote(ctx, vote, "choices"); err != nil {
		return gtserror.Newf("error updating poll vote in db: %w", err)
	}

	// Update the vote counts on the poll model itself. These will have
	// been updated by message pusher as we can't know which were new.
	if err := p.state.DB.UpdatePoll(ctx, vote.Poll, "votes"); err != nil {
		return gtserror.Newf("error updating poll in db: %w", err)
	}

	// Get the origin status.
	reply := vote.Poll.Status

	if reply.Flags.Local() {
		// These were poll votes in a local status, we need to
		// federate the updated status model with latest vote counts.
		if err := p.federate.UpdateStatus(ctx, reply); err != nil {
			log.Errorf(ctx, "error federating status update: %v", err)
		}
	}

	return nil
}

func (p *fediAPI) CreateFollowReq(ctx context.Context, fMsg *messages.FromFediAPI) error {
	followReq, ok := fMsg.GTSModel.(*gtsmodel.FollowRequest)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.FollowRequest", fMsg.GTSModel)
	}

	if err := p.state.DB.PopulateFollowRequest(ctx, followReq); err != nil {
		return gtserror.Newf("error populating follow request: %w", err)
	}

	// Figure out what we should do with this follow request.
	//
	// If it's from a domain with a domain limit we may need
	// to handle it differently depending on the Follows policy.
	var (
		// Assume not rejecting.
		reject bool

		// Assume accepting if our account isn't locked.
		// We'll override this if necessary.
		accept bool = !*followReq.TargetAccount.Locked
	)

	// See if there's a limit on the would-be follower's domain.
	limit, err := p.state.DB.MatchDomainLimit(ctx, followReq.Account.Domain)
	if err != nil {
		return gtserror.Newf("error matching domain limit: %w", err)
	}

	if limit != nil {

		// Limit is in place so
		// check the follows policy.
		switch limit.FollowsPolicy {

		case gtsmodel.FollowsPolicyNoAction:
			// Noop policy,
			// let things proceed.

		case gtsmodel.FollowsPolicyManualApproval:
			// Don't reject but don't
			// accept automatically either.
			accept = false

		case gtsmodel.FollowsPolicyRejectNonMutual:
			// Check if our account
			// follows the remote.
			if mufo, err := p.state.DB.IsFollowing(
				ctx,
				followReq.TargetAccountID,
				followReq.AccountID,
			); err != nil {
				return gtserror.Newf("db error checking following: %w", err)
			} else if mufo {
				// We follow the remote so
				// leave reject = false.
				break
			}

			// We don't follow the remote but
			// do we follow *request* them?
			if mufo, err := p.state.DB.IsFollowRequested(
				ctx,
				followReq.TargetAccountID,
				followReq.AccountID,
			); err != nil {
				return gtserror.Newf("db error checking following: %w", err)
			} else if mufo {
				// We follow req the remote
				// so leave reject = false.
				break
			}

			// We don't follow or follow
			// request the remote so
			// reject their follow of us.
			reject = true

		case gtsmodel.FollowsPolicyRejectAll:
			// Reject out of hand.
			reject = true
		}
	}

	if reject {
		// We're rejecting this
		// follow request out of hand.
		//
		// Delete the req from the db.
		if err := p.state.DB.RejectFollowRequest(
			ctx,
			followReq.AccountID,
			followReq.TargetAccountID,
		); err != nil {
			return gtserror.Newf("db error rejecting follow request: %w", err)
		}

		// Reconstruct + reject the follow.
		follow := typeutils.FollowRequestToFollow(followReq)
		if err := p.federate.RejectFollow(ctx, follow); err != nil {
			log.Errorf(ctx, "error federating follow reject: %v", err)
		}

		// That's it, no need to
		// notify the target account.
		return nil
	}

	// If we're not accepting the follow
	// request automatically, just notify
	// about it and leave.
	if !accept {
		if err := p.surfacer.NotifyFollowRequest(ctx, followReq); err != nil {
			return gtserror.Newf("error notifying follow request: %w", err)
		}

		return nil
	}

	// Automatically accept the follow request
	// and notify about the new follower.
	follow, err := p.state.DB.AcceptFollowRequest(
		ctx,
		followReq.AccountID,
		followReq.TargetAccountID,
	)
	if err != nil {
		return gtserror.Newf("error accepting follow request: %w", err)
	}

	if err := p.federate.AcceptFollow(ctx, follow); err != nil {
		log.Errorf(ctx, "error federating follow request accept: %v", err)
	}

	if err := p.surfacer.NotifyFollow(ctx, follow); err != nil {
		log.Errorf(ctx, "error notifying follow: %v", err)
	}

	return nil
}

// CreateLike handles an impolite Like, ie., a Like sent directly.
// This is different from the CreateLikeRequest function, which handles polite LikeRequests.
func (p *fediAPI) CreateLike(ctx context.Context, fMsg *messages.FromFediAPI) error {
	fave, ok := fMsg.GTSModel.(*gtsmodel.StatusFave)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.StatusFave", fMsg.GTSModel)
	}

	// Ensure fave populated.
	if err := p.state.DB.PopulateStatusFave(ctx, fave); err != nil {
		return gtserror.Newf("error populating status fave: %w", err)
	}

	// If pending approval is true then
	// fave must target a LOCAL status
	// that requires approval for the fave.
	if util.PtrOrZero(fave.PendingApproval) {
		intReqID := id.NewULIDFromTime(fave.CreatedAt)
		intReq := &gtsmodel.InteractionRequest{
			ID:                    intReqID,
			TargetStatusID:        fave.StatusID,
			TargetStatus:          fave.Status,
			TargetAccountID:       fave.TargetAccountID,
			TargetAccount:         fave.TargetAccount,
			InteractingAccountID:  fave.AccountID,
			InteractingAccount:    fave.Account,
			InteractionRequestURI: fave.URI + gtsmodel.ImpoliteLikeRequestSuffix,
			InteractionURI:        fave.URI,
			InteractionType:       gtsmodel.InteractionLike,
			Polite:                util.Ptr(false),
			Like:                  fave,
		}

		if !fave.PreApproved {
			// If approval is required and status fave isn't
			// preapproved, just store the interaction request
			// and notify the account that's being interacted
			// with, they can handle the interaction later.
			if err := p.utils.storeInteractionRequest(
				ctx, intReq,
			); err != nil {
				return gtserror.Newf("error storing interaction request: %w", err)
			}

			// Notify target account (if local) of pending like.
			if err := p.surfacer.NotifyPendingFave(ctx, intReq.Like); err != nil {
				return gtserror.Newf("error notifying pending fave: %w", err)
			}

			return nil
		}

		// If approval is required and fave *is* preapproved,
		// that means this is a fave of one of our statuses
		// that was allowed based on faver's presence in a
		// following/followers collection.
		//
		// Mark the interaction request as accepted, store it,
		// mark the interaction as approved, and then continue
		// with side effects as normal.

		// Update intReq fields to
		// mark it as accepted.
		intReq.MarkAccepted()

		// Put it in the DB.
		if err := p.utils.storeInteractionRequest(
			ctx, intReq,
		); err != nil {
			return gtserror.Newf("error storing interaction request: %w", err)
		}

		// Mark the fave as now approved, referring to
		// the accepted interaction request we just stored.
		fave.PendingApproval = util.Ptr(false)
		fave.PreApproved = false
		fave.ApprovedByURI = intReq.AuthorizationURI
		if err := p.state.DB.UpdateStatusFave(ctx,
			fave,
			"pending_approval",
			"approved_by_uri",
		); err != nil {
			return gtserror.Newf("db error updating status fave: %w", err)
		}

		// Send out the approval as Accept.
		if err := p.federate.AcceptInteraction(ctx, intReq); err != nil {
			return gtserror.Newf("error federating pre-approval of fave: %w", err)
		}

		// Don't return, just continue
		// side effects as normal.
	}

	if err := p.surfacer.NotifyFave(ctx, fave); err != nil {
		log.Errorf(ctx, "error notifying fave: %v", err)
	}

	return nil
}

// CreateLikeRequest handles a polite LikeRequest, as
// opposed to CreateLike, which handles *impolite* like
// requests (ie., Likes sent directly).
func (p *fediAPI) CreateLikeRequest(ctx context.Context, fMsg *messages.FromFediAPI) error {
	req, ok := fMsg.GTSModel.(*gtsmodel.InteractionRequest)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.InteractionRequest", fMsg.GTSModel)
	}

	// At this point the not-yet-approved
	// interaction request, and the pending
	// fave, are both in the database.

	if !req.Like.PreApproved {
		// The fave is *not* pre-approved, and
		// therefore requires manual approval.
		//
		// Just notify target account about
		// the requested interaction.
		if err := p.surfacer.NotifyPendingFave(ctx, req.Like); err != nil {
			return gtserror.Newf("error notifying pending like: %w", err)
		}

		return nil
	}

	// If it's pre-approved on the other hand
	// we can handle everything immediately.

	// Mark the request as accepted.
	req.AcceptedAt = time.Now()
	req.ResponseURI = uris.GenerateURIForAccept(
		req.TargetAccount.Username, req.ID,
	)
	req.AuthorizationURI = uris.GenerateURIForAuthorization(
		req.TargetAccount.Username, req.ID,
	)

	// Update in the db.
	if err := p.state.DB.UpdateInteractionRequest(
		ctx,
		req,
		"accepted_at",
		"response_uri",
		"authorization_uri",
	); err != nil {
		return gtserror.Newf("db error updating interaction request: %w", err)
	}

	// Mark the status fave as now approved, referring to
	// the accepted interaction request we just stored.
	req.Like.PreApproved = false
	req.Like.PendingApproval = util.Ptr(false)
	req.Like.ApprovedByURI = req.AuthorizationURI
	if err := p.state.DB.UpdateStatusFave(ctx,
		req.Like,
		"pending_approval",
		"approved_by_uri",
	); err != nil {
		return gtserror.Newf("db error updating status fave: %w", err)
	}

	// Send out the accept.
	if err := p.federate.AcceptInteraction(ctx, req); err != nil {
		log.Errorf(ctx, "error federating accept: %v", err)
	}

	// Notify the faved account.
	if err := p.surfacer.NotifyFave(ctx, req.Like); err != nil {
		log.Errorf(ctx, "error notifying fave: %v", err)
	}

	return nil
}

func (p *fediAPI) CreateAnnounce(ctx context.Context, fMsg *messages.FromFediAPI) error {
	// If we received this announce via our instance account,
	// it must originate from a relay actor we subscribe to.
	if fMsg.Receiving.IsInstance() {
		return p.createAnnounceFromRelay(ctx, fMsg)
	}

	// Non-relay announce, process normally.
	boost, ok := fMsg.GTSModel.(*gtsmodel.Status)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Status", fMsg.GTSModel)
	}

	// Dereference into a boost wrapper status.
	//
	// Note: this will handle storing the boost in
	// the db, and dereferencing the target status
	// ancestors / descendants where appropriate.
	boost, err := p.federate.EnrichAnnounce(ctx,
		boost,
		fMsg.Receiving.Username,
	)
	if err != nil {
		if gtserror.IsUnretrievable(err) ||
			gtserror.IsNotPermitted(err) {
			// Boosted status domain blocked, or
			// otherwise not permitted, nothing to do.
			log.Debugf(ctx, "skipping announce: %v", err)
			return nil
		}

		// Actual error.
		return gtserror.Newf("error dereferencing announce: %w", err)
	}

	// If pending approval is true then
	// boost must target a LOCAL status
	// that requires approval for the boost.
	if boost.Flags.PendingApproval() {
		intReqID := id.NewULIDFromTime(boost.CreatedAt)
		intReq := &gtsmodel.InteractionRequest{
			ID:                    intReqID,
			TargetStatusID:        boost.BoostOfID,
			TargetStatus:          boost.BoostOf,
			TargetAccountID:       boost.BoostOfAccountID,
			TargetAccount:         boost.BoostOfAccount,
			InteractingAccountID:  boost.AccountID,
			InteractingAccount:    boost.Account,
			InteractionRequestURI: boost.URI + gtsmodel.ImpoliteAnnounceRequestSuffix,
			InteractionURI:        boost.URI,
			InteractionType:       gtsmodel.InteractionAnnounce,
			Polite:                util.Ptr(false),
			Announce:              boost,
		}

		if !boost.PreApproved {
			// If approval is required and announce isn't
			// preapproved, just store the interaction request
			// and notify the account that's being interacted
			// with, they can handle the interaction later.
			if err := p.utils.storeInteractionRequest(
				ctx, intReq,
			); err != nil {
				return gtserror.Newf("error storing interaction request: %w", err)
			}

			// Notify target account (if local) of pending announce.
			if err := p.surfacer.NotifyPendingAnnounce(ctx, intReq.Announce); err != nil {
				return gtserror.Newf("error notifying pending announce: %w", err)
			}

			return nil
		}

		// If approval is required and boost *is* preapproved,
		// that means this is a boost of one of our statuses
		// that was allowed based on booster's presence in a
		// following/followers collection.
		//
		// Mark the interaction request as accepted, store it,
		// mark the interaction as approved, and then continue
		// with side effects as normal.

		// Update intReq fields to
		// mark it as accepted.
		intReq.MarkAccepted()

		// Put it in the DB.
		if err := p.utils.storeInteractionRequest(
			ctx, intReq,
		); err != nil {
			return gtserror.Newf("error storing interaction request: %w", err)
		}

		// Mark the boost itself as now approved.
		boost.Flags.SetPendingApproval(false)
		boost.PreApproved = false
		boost.ApprovedByURI = intReq.AuthorizationURI
		if err := p.state.DB.UpdateStatus(ctx,
			boost,
			"flags",
			"approved_by_uri",
		); err != nil {
			return gtserror.Newf("db error updating status: %w", err)
		}

		// Send out the approval as Accept.
		if err := p.federate.AcceptInteraction(ctx, intReq); err != nil {
			return gtserror.Newf("error federating pre-approval of boost: %w", err)
		}

		// Timeline and notify the status, since the missing ApprovedByURI during
		// status dereferencer stage will cause it to skip typical surfacing logic.
		if err := p.surfacer.TimelineAndNotifyStatus(ctx, boost); err != nil {
			log.Errorf(ctx, "error timelining and notifying status: %v", err)
		}
	}

	if err := p.surfacer.NotifyAnnounce(ctx, boost); err != nil {
		log.Errorf(ctx, "error notifying announce: %v", err)
	}

	return nil
}

func (p *fediAPI) createAnnounceFromRelay(ctx context.Context, fMsg *messages.FromFediAPI) error {
	// Unpack the boost wrapper.
	boost, ok := fMsg.GTSModel.(*gtsmodel.Status)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Status", fMsg.GTSModel)
	}

	// Retrieve the relayed status wrapped in the boost,
	// and store it if it's matched by a relay subscription.
	//
	// Note that this doesn't store the boost wrapper status
	// itself as we don't really care about it that much.
	status, err := p.federate.GetRelayedAnnounce(ctx,
		fMsg.Receiving,  // Our instance account.
		fMsg.Requesting, // Relaying account.
		boost,           // Boost wrapper status.
	)

	uriStr := boost.BoostOfURIStr
	l := log.
		WithContext(ctx).
		WithField("uri", uriStr)

	// Check for semi-expected errors
	// encountered during dereferencing.
	switch {
	case err == nil:
		// No problem.

	case gtserror.IsUnretrievable(err):
		// Remote domain probably blocked.
		l.Debugf("status not retrievable: %v", err)
		return nil

	case gtserror.IsMalformed(err):
		// Final URI probably borked.
		l.Debugf("status malformed: %v", err)
		return nil

	case gtserror.IsWrongType(err):
		// Probably Announce of Announce
		// or something, we ignore these.
		l.Debugf("wrong type: %v", err)
		return nil

	default:
		// Actual error (db error, transport error, etc).
		return gtserror.Newf("error dereferencing %s: %w", uriStr, err)
	}

	// If status is nil that means it probably
	// didn't match a relay subscription, or it
	// wasn't permitted by interaction policy checks.
	if status == nil {
		l.Debug("status dropped as not relayable")
		return nil
	}

	return nil
}

func (p *fediAPI) CreateAnnounceRequest(ctx context.Context, fMsg *messages.FromFediAPI) error {
	req, ok := fMsg.GTSModel.(*gtsmodel.InteractionRequest)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.InteractionRequest", fMsg.GTSModel)
	}

	// At this point the not-yet-handled interaction req
	// is in the database, but the announce isn't yet.
	//
	// We can check permissions for the announce *and*
	// put it in the db (if acceptable) by doing Enrich.
	boost, err := p.federate.EnrichAnnounce(ctx,
		req.Announce,
		fMsg.Receiving.Username,
	)

	switch {
	case err == nil:
		// All fine.

	case gtserror.IsNotPermitted(err):
		// Announce is straight up not permitted
		// by the interaction policy of the status
		// it's targeting. Nothing more to do.
		log.Debugf(ctx,
			"dropping unpermitted AnnounceRequest with instrument %s",
			req.Announce.URI,
		)
		return nil

	default:
		// There's a real error.
		return gtserror.Newf(
			"error processing AnnounceRequest with instrument %s: %w",
			req.Announce.URI, err,
		)
	}

	// The announce is permitted. Check if we
	// should send out an Accept immediately.
	manualApproval := boost.Flags.PendingApproval() && !boost.PreApproved
	if manualApproval {

		// The announce requires manual approval.
		//
		// Just notify target account about
		// the requested interaction.
		if err := p.surfacer.NotifyPendingAnnounce(ctx, boost); err != nil {
			return gtserror.Newf("error notifying pending announce: %w", err)
		}
		return nil
	}

	// The announce is automatically approved,
	// mark the request as accepted.
	req.AcceptedAt = time.Now()
	req.ResponseURI = uris.GenerateURIForAccept(
		req.TargetAccount.Username, req.ID)
	req.AuthorizationURI = uris.GenerateURIForAuthorization(
		req.TargetAccount.Username, req.ID)

	// Update in the db.
	if err := p.state.DB.UpdateInteractionRequest(ctx,
		req,
		"accepted_at",
		"response_uri",
		"authorization_uri",
	); err != nil {
		return gtserror.Newf("db error updating interaction request: %w", err)
	}

	// Mark the boost as now approved, referring to
	// the accepted interaction request we just stored.
	boost.PreApproved = false
	boost.Flags.SetPendingApproval(false)
	boost.ApprovedByURI = req.AuthorizationURI
	if err := p.state.DB.UpdateStatus(ctx,
		boost,
		"flags",
		"approved_by_uri",
	); err != nil {
		return gtserror.Newf("db error updating status: %w", err)
	}

	// Send out the accept.
	if err := p.federate.AcceptInteraction(ctx, req); err != nil {
		log.Errorf(ctx, "error federating accept: %v", err)
	}

	// Timeline and notify the boost, since the missing ApprovedByURI during
	// status dereferencer stage will cause it to skip typical surfacing logic.
	if err := p.surfacer.TimelineAndNotifyStatus(ctx, boost); err != nil {
		log.Errorf(ctx, "error timelining and notifying status: %v", err)
	}

	return nil
}

func (p *fediAPI) CreateQuoteRequest(ctx context.Context, fMsg *messages.FromFediAPI) error {
	req, ok := fMsg.GTSModel.(*gtsmodel.InteractionRequest)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.InteractionRequest", fMsg.GTSModel)
	}

	// Persist the quoting status through the normal dereferencing path. This
	// stores pending quotes without surfacing them, and makes the instrument
	// available when the interaction request is loaded again after this
	// worker message and its in-memory Quote pointer are gone.
	statusable, ok := fMsg.APObject.(ap.Statusable)
	if !ok {
		return gtserror.Newf("cannot cast %T -> ap.Statusable", fMsg.APObject)
	}

	quoteURI := ap.GetJSONLDId(statusable).String()
	if quoteURI != req.InteractionURI {
		return gtserror.Newf(
			"quote request instrument URI %s does not match interaction URI %s",
			quoteURI,
			req.InteractionURI,
		)
	}

	// Mark the bare status pending before dereferencing so the dereferencer's
	// surfacing hook never exposes a QuoteRequest instrument. RefreshStatus
	// deliberately carries this flag across its policy checks.
	bareQuote := &gtsmodel.Status{URI: quoteURI}
	bareQuote.Flags.SetPendingApproval(true)
	quote, _, err := p.federate.RefreshStatus(ctx,
		fMsg.Receiving.Username,
		bareQuote,
		statusable,
		&dereferencing.Fresh,
	)
	if err != nil {
		return gtserror.Newf(
			"error processing QuoteRequest instrument %s: %w",
			quoteURI,
			err,
		)
	}
	automaticallyApproved := req.Quote != nil && req.Quote.PreApproved
	req.Quote = quote

	// Manual quote policies leave the request pending for the target account.
	if !automaticallyApproved {
		return nil
	}

	// The quote target was permission-checked when the QuoteRequest arrived
	// and policy grants automatic approval, so issue the authorization.
	req.AcceptedAt = time.Now()
	req.ResponseURI = uris.GenerateURIForAccept(
		req.TargetAccount.Username, req.ID)
	req.AuthorizationURI = uris.GenerateURIForAuthorization(
		req.TargetAccount.Username, req.ID)

	if err := p.state.DB.UpdateInteractionRequest(ctx,
		req,
		"accepted_at",
		"response_uri",
		"authorization_uri",
	); err != nil {
		return gtserror.Newf("db error updating interaction request: %w", err)
	}

	// Send out the Accept; its `result` points at the QuoteAuthorization,
	// which we serve at the authorization URI for the quoter to attach.
	if err := p.federate.AcceptInteraction(ctx, req); err != nil {
		log.Errorf(ctx, "error federating quote accept: %v", err)
	}

	return nil
}

func (p *fediAPI) AcceptPoliteQuoteRequest(ctx context.Context, fMsg *messages.FromFediAPI) error {
	req, ok := fMsg.GTSModel.(*gtsmodel.InteractionRequest)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.InteractionRequest", fMsg.GTSModel)
	}
	if err := p.state.DB.PopulateInteractionRequest(ctx, req); err != nil {
		return gtserror.Newf("error populating accepted quote request: %w", err)
	}
	if req.Quote == nil || req.Quote.QuoteApprovalURI == "" {
		return gtserror.New("accepted quote request has no quote authorization")
	}

	// Now that the authorization is attached, federate the quote status.
	if err := p.federate.CreateStatus(ctx, req.Quote); err != nil {
		return gtserror.Newf("error federating authorized quote status: %w", err)
	}

	return nil
}

func (p *fediAPI) DeleteQuoteAuthorization(ctx context.Context, fMsg *messages.FromFediAPI) error {
	req, ok := fMsg.GTSModel.(*gtsmodel.InteractionRequest)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.InteractionRequest", fMsg.GTSModel)
	}
	if fMsg.APIRI == nil {
		return gtserror.New("deleted quote authorization has no object URI")
	}

	if err := p.federate.ForwardQuoteAuthorizationDelete(
		ctx,
		req,
		fMsg.TargetURI,
		fMsg.APIRI,
	); err != nil {
		return gtserror.Newf("error forwarding quote authorization revocation: %w", err)
	}

	return nil
}

func (p *fediAPI) CreateBlock(ctx context.Context, fMsg *messages.FromFediAPI) error {
	block, ok := fMsg.GTSModel.(*gtsmodel.Block)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Block", fMsg.GTSModel)
	}

	if block.Account.IsLocal() {
		// Remove posts by target from origin's timelines.
		p.surfacer.RemoveRelationshipFromTimelines(ctx,
			block.AccountID,
			block.TargetAccountID,
		)
	}

	if block.TargetAccount.IsLocal() {
		// Remove posts by origin from target's timelines.
		p.surfacer.RemoveRelationshipFromTimelines(ctx,
			block.TargetAccountID,
			block.AccountID,
		)
	}

	// Remove any follows that existed between blocker + blockee.
	// (note this handles removing any necessary list entries).
	if err := p.state.DB.DeleteFollow(ctx,
		block.AccountID,
		block.TargetAccountID,
	); err != nil {
		log.Errorf(ctx, "error deleting follow from block -> target: %v", err)
	}

	if err := p.state.DB.DeleteFollow(ctx,
		block.TargetAccountID,
		block.AccountID,
	); err != nil {
		log.Errorf(ctx, "error deleting follow from target -> block: %v", err)
	}

	// Remove any follow requests that existed between blocker + blockee.
	if err := p.state.DB.DeleteFollowRequest(ctx,
		block.AccountID,
		block.TargetAccountID,
	); err != nil {
		log.Errorf(ctx, "error deleting follow request from block -> target: %v", err)
	}

	if err := p.state.DB.DeleteFollowRequest(ctx,
		block.TargetAccountID,
		block.AccountID,
	); err != nil {
		log.Errorf(ctx, "error deleting follow request from target -> block: %v", err)
	}

	return nil
}

func (p *fediAPI) CreateFlag(ctx context.Context, fMsg *messages.FromFediAPI) error {
	incomingReport, ok := fMsg.GTSModel.(*gtsmodel.Report)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Report", fMsg.GTSModel)
	}

	// TODO: handle additional side effects of flag creation:
	// - notify admins by dm / notification

	if err := p.surfacer.EmailAdminReportOpened(ctx, incomingReport); err != nil {
		log.Errorf(ctx, "error emailing report opened: %v", err)
	}

	return nil
}

func (p *fediAPI) UpdateAccount(ctx context.Context, fMsg *messages.FromFediAPI) error {
	// Parse the old/existing account model.
	account, ok := fMsg.GTSModel.(*gtsmodel.Account)
	if !ok {
		return gtserror.Newf("cannot cast %T -> *gtsmodel.Account", fMsg.GTSModel)
	}

	// Because this was an Update, the new Accountable should be set on the message.
	apubAcc, ok := fMsg.APObject.(ap.Accountable)
	if !ok {
		return gtserror.Newf("cannot cast %T -> ap.Accountable", fMsg.APObject)
	}

	// Fetch up-to-date bio, avatar, header, etc.
	_, _, err := p.federate.RefreshAccount(
		ctx,
		fMsg.Receiving.Username,
		account,
		apubAcc,

		// Force refresh.
		//
		// Missing account updates could be
		// detrimental to federation if they
		// include public key changes.
		&dereferencing.Freshest,
	)
	if err != nil {
		log.Errorf(ctx, "error refreshing account: %v", err)
	}

	return nil
}

func (p *fediAPI) AcceptFollow(ctx context.Context, fMsg *messages.FromFediAPI) error {
	return nil
}

func (p *fediAPI) AcceptLike(ctx context.Context, fMsg *messages.FromFediAPI) error {
	// TODO: Add something here if we ever implement sending out Likes to
	// followers more broadly and not just the owner of the Liked status.
	return nil
}

func (p *fediAPI) AcceptReply(ctx context.Context, fMsg *messages.FromFediAPI) error {
	reply, ok := fMsg.GTSModel.(*gtsmodel.Status)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Status", fMsg.GTSModel)
	}

	// Timeline and notify the status.
	if err := p.surfacer.TimelineAndNotifyStatus(ctx, reply); err != nil {
		log.Errorf(ctx, "error timelining and notifying status: %v", err)
	}

	// Send out the reply.
	if err := p.federate.CreateStatus(ctx, reply); err != nil {
		log.Errorf(ctx, "error federating status: %v", err)
	}

	return nil
}

func (p *fediAPI) AcceptRemoteStatus(ctx context.Context, fMsg *messages.FromFediAPI) error {
	// See if we can accept a remote
	// status we don't have stored yet.
	objectIRI, ok := fMsg.APObject.(*url.URL)
	if !ok {
		return gtserror.Newf("%T not parseable as *url.URL", fMsg.APObject)
	}

	approvedByURI := fMsg.APIRI
	if approvedByURI == nil {
		return gtserror.New("approvedByURI was nil")
	}

	// Assume we're accepting a status; create a
	// barebones status for dereferencing purposes.
	bareStatus := &gtsmodel.Status{
		URI:           objectIRI.String(),
		ApprovedByURI: approvedByURI.String(),
	}

	// Call RefreshStatus() to process the provided
	// barebones status and insert it into the database,
	// if indeed it's actually a status URI we can fetch.
	//
	// This will also check whether the given approvedByURI
	// actually grants permission for this status.
	_, _, err := p.federate.RefreshStatus(ctx,
		fMsg.Receiving.Username,
		bareStatus,
		nil,
		nil,
	)
	if err != nil {
		return gtserror.Newf("error processing accepted status %s: %w", bareStatus.URI, err)
	}

	return nil
}

func (p *fediAPI) AcceptPoliteReplyRequest(ctx context.Context, fMsg *messages.FromFediAPI) error {
	if util.IsNil(fMsg.GTSModel) {
		// If the interaction request is nil, this
		// must be an accept of a remote ReplyRequest
		// not targeting one of our statuses.
		//
		// Just pass it to the AcceptRemoteStatus
		// func to do dereferencing + side effects.
		log.Debug(ctx, "accepting remote ReplyRequest for remote reply")
		return p.AcceptRemoteStatus(ctx, fMsg)
	}

	// If the interaction request is not nil, this will
	// be an accept of one of our replies to a remote.
	//
	// Since the int req + reply have already been updated
	// in the federatingDB, we just need to do side effects.
	intReq, ok := fMsg.GTSModel.(*gtsmodel.InteractionRequest)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.InteractionRequest", fMsg.GTSModel)
	}

	// Ensure reply populated.
	reply := intReq.Reply
	if err := p.state.DB.PopulateStatus(ctx, reply); err != nil {
		return gtserror.Newf("error populating status: %w", err)
	}

	// Timeline and notify the status.
	if err := p.surfacer.TimelineAndNotifyStatus(ctx, reply); err != nil {
		log.Errorf(ctx, "error timelining and notifying status: %v", err)
	}

	// Send out the reply with approval attached.
	if err := p.federate.CreateStatus(ctx, reply); err != nil {
		log.Errorf(ctx, "error federating status: %v", err)
	}

	return nil
}

func (p *fediAPI) AcceptAnnounce(ctx context.Context, fMsg *messages.FromFediAPI) error {
	boost, ok := fMsg.GTSModel.(*gtsmodel.Status)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Status", fMsg.GTSModel)
	}

	// Timeline and notify the boost wrapper status.
	if err := p.surfacer.TimelineAndNotifyStatus(ctx, boost); err != nil {
		log.Errorf(ctx, "error timelining and notifying status: %v", err)
	}

	// Send out the boost again, fully this time.
	if err := p.federate.Announce(ctx, boost); err != nil {
		log.Errorf(ctx, "error federating announce: %v", err)
	}

	return nil
}

func (p *fediAPI) UpdateStatus(ctx context.Context, fMsg *messages.FromFediAPI) error {
	// Cast the existing Status model attached to msg.
	existing, ok := fMsg.GTSModel.(*gtsmodel.Status)
	if !ok {
		return gtserror.Newf("cannot cast %T -> *gtsmodel.Status", fMsg.GTSModel)
	}

	var freshness *dereferencing.FreshnessWindow

	// Cast the updated ActivityPub statusable object .
	apStatus, _ := fMsg.APObject.(ap.Statusable)

	if apStatus != nil {
		// If an AP object was provided, we
		// allow very fast refreshes that likely
		// indicate a status edit after post.
		freshness = &dereferencing.Freshest
	}

	// Fetch up-to-date attach status attachments.
	// NOTE: dereferencer hook handles surfacing.
	_, _, err := p.federate.RefreshStatus(ctx,
		fMsg.Receiving.Username,
		existing,
		apStatus,
		freshness,
	)
	if err != nil {
		log.Errorf(ctx, "error refreshing status: %v", err)
	}

	return nil
}

func (p *fediAPI) DeleteStatus(ctx context.Context, fMsg *messages.FromFediAPI) error {
	status, ok := fMsg.GTSModel.(*gtsmodel.Status)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Status", fMsg.GTSModel)
	}

	// Try to populate status structs if possible,
	// in order to more thoroughly remove them.
	if err := p.state.DB.PopulateStatus(
		ctx, status,
	); err != nil && !errors.Is(err, db.ErrNoEntries) {
		return gtserror.Newf("db error populating status: %w", err)
	}

	// Drop any outgoing queued AP requests about / targeting
	// this status, (stops queued likes, boosts, creates etc).
	p.state.Workers.Delivery.Queue.Delete("ObjectID", status.URI)
	p.state.Workers.Delivery.Queue.Delete("TargetID", status.URI)

	// Drop any incoming queued client messages about / targeting
	// status, (stops processing of local origin data for status).
	p.state.Workers.Client.Queue.Delete("TargetURI", status.URI)

	// Drop any incoming queued federator messages targeting status,
	// (stops processing of remote origin data targeting this status).
	p.state.Workers.Federator.Queue.Delete("TargetURI", status.URI)

	// Delete attachments from this status, since this request
	// comes from the federating API, and there's no way the
	// poster can do a delete + redraft for it on our instance.
	const attachments = true

	// This is just a deletion, not a Reject,
	// we don't need to take a copy of this status.
	const sinBin = false

	// Don't wipe the status, just stub it out,
	// in order to preserve threading (if any).
	// Will otherwise get cleaned up later.
	const wipe = false

	// Perform actual status deletion.
	if err := p.utils.deleteStatus(ctx,
		status,
		attachments,
		sinBin,
		wipe,
	); err != nil {
		log.Errorf(ctx, "error deleting status %s: %v", status.URI, err)
	}

	return nil
}

func (p *fediAPI) DeleteAccount(ctx context.Context, fMsg *messages.FromFediAPI) error {
	account, ok := fMsg.GTSModel.(*gtsmodel.Account)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Account", fMsg.GTSModel)
	}

	// Drop any outgoing queued AP requests to / from / targeting
	// this account, (stops queued likes, boosts, creates etc).
	p.state.Workers.Delivery.Queue.Delete("ObjectID", account.URI)
	p.state.Workers.Delivery.Queue.Delete("TargetID", account.URI)

	// Drop any incoming queued client messages to / from this
	// account, (stops processing of local origin data for acccount).
	p.state.Workers.Client.Queue.Delete("Target.ID", account.ID)
	p.state.Workers.Client.Queue.Delete("TargetURI", account.URI)

	// Drop any incoming queued federator messages to this account,
	// (stops processing of remote origin data targeting this account).
	p.state.Workers.Federator.Queue.Delete("Requesting.ID", account.ID)
	p.state.Workers.Federator.Queue.Delete("TargetURI", account.URI)

	// Remove any entries authored by account from timelines.
	p.surfacer.RemoveTimelineEntriesByAccount(account.ID)

	// And finally, perform the actual account deletion synchronously.
	if err := p.account.Delete(ctx, account, account.ID); err != nil {
		log.Errorf(ctx, "error deleting account: %v", err)
	}

	return nil
}

func (p *fediAPI) RejectLike(ctx context.Context, fMsg *messages.FromFediAPI) error {
	req, ok := fMsg.GTSModel.(*gtsmodel.InteractionRequest)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.InteractionRequest", fMsg.GTSModel)
	}

	// At this point the InteractionRequest should already
	// be in the database, we just need to do side effects.

	// Send out the Reject.
	if err := p.federate.RejectInteraction(ctx, req); err != nil {
		log.Errorf(ctx, "error federating rejection of like: %v", err)
	}

	// Get the rejected fave.
	fave, err := p.state.DB.GetStatusFaveByURI(
		gtscontext.SetBarebones(ctx),
		req.InteractionURI,
	)
	if err != nil {
		return gtserror.Newf("db error getting rejected fave: %w", err)
	}

	// Delete the fave.
	if err := p.state.DB.DeleteStatusFaveByID(ctx, fave.ID); err != nil {
		return gtserror.Newf("db error deleting fave: %w", err)
	}

	return nil
}

func (p *fediAPI) RejectReply(ctx context.Context, fMsg *messages.FromFediAPI) error {
	req, ok := fMsg.GTSModel.(*gtsmodel.InteractionRequest)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.InteractionRequest", fMsg.GTSModel)
	}

	// At this point the InteractionRequest should already
	// be in the database, we just need to do side effects.

	// Get the rejected status model from db.
	reply, err := p.state.DB.GetStatusByURI(
		gtscontext.SetBarebones(ctx),
		req.InteractionURI,
	)
	if err != nil {
		return gtserror.Newf("db error getting rejected reply: %w", err)
	}

	// Delete attachments from this status.
	// It's rejected so there's no possibility
	// for the poster to delete + redraft it.
	const attachments = true

	// Keep a copy of the status in
	// the sin bin for future review.
	const sinBin = true

	// Unpermitted statuses should be
	// wiped as opposed to stubbed.
	const wipe = true

	// Perform actual status deletion.
	if err := p.utils.deleteStatus(ctx,
		reply,
		attachments,
		sinBin,
		wipe,
	); err != nil {
		log.Errorf(ctx, "error deleting reply %s: %v", reply.URI, err)
	}

	return nil
}

func (p *fediAPI) RejectAnnounce(ctx context.Context, fMsg *messages.FromFediAPI) error {
	req, ok := fMsg.GTSModel.(*gtsmodel.InteractionRequest)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.InteractionRequest", fMsg.GTSModel)
	}

	// At this point the InteractionRequest should already
	// be in the database, we just need to do side effects.

	// Get the rejected boost model from db.
	boost, err := p.state.DB.GetStatusByURI(
		gtscontext.SetBarebones(ctx),
		req.InteractionURI,
	)
	if err != nil {
		return gtserror.Newf("db error getting rejected announce: %w", err)
	}

	// Perform actual boost deletion.
	if err := p.utils.deleteBoost(ctx,
		boost); err != nil {
		log.Errorf(ctx, "error deleting boost %s: %v", boost.URI, err)
	}

	return nil
}

func (p *fediAPI) UndoFollow(ctx context.Context, fMsg *messages.FromFediAPI) error {
	follow, ok := fMsg.GTSModel.(*gtsmodel.Follow)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Follow", fMsg.GTSModel)
	}

	// Clear any notifications that were
	// generated by this follow (request).
	if err := p.state.DB.DeleteNotifications(
		ctx,
		[]gtsmodel.NotificationType{
			gtsmodel.NotificationFollow,
			gtsmodel.NotificationFollowRequest,
		},
		follow.TargetAccountID,
		follow.AccountID,
	); err != nil {
		return gtserror.Newf("db error deleting notifications: %w", err)
	}

	if follow.Account.IsLocal() {
		// Remove posts by target from origin's timelines.
		p.surfacer.RemoveRelationshipFromTimelines(ctx,
			follow.AccountID,
			follow.TargetAccountID,
		)
	}

	if follow.TargetAccount.IsLocal() {
		// Remove posts by origin from target's timelines.
		p.surfacer.RemoveRelationshipFromTimelines(ctx,
			follow.TargetAccountID,
			follow.AccountID,
		)
	}

	return nil
}

func (p *fediAPI) UndoBlock(ctx context.Context, fMsg *messages.FromFediAPI) error {
	_, ok := fMsg.GTSModel.(*gtsmodel.Block)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Block", fMsg.GTSModel)
	}
	// TODO: any required changes
	return nil
}

func (p *fediAPI) UndoAnnounce(ctx context.Context, fMsg *messages.FromFediAPI) error {
	boost, ok := fMsg.GTSModel.(*gtsmodel.Status)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Status", fMsg.GTSModel)
	}

	// Perform actual boost deletion.
	if err := p.utils.deleteBoost(ctx,
		boost); err != nil {
		log.Errorf(ctx, "error deleting boost %s: %v", boost.URI, err)
	}

	return nil
}

func (p *fediAPI) UndoFave(ctx context.Context, fMsg *messages.FromFediAPI) error {
	_, ok := fMsg.GTSModel.(*gtsmodel.StatusFave)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.StatusFave", fMsg.GTSModel)
	}
	// TODO: any required changes
	return nil
}
