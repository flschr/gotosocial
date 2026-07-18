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

	"code.superseriousbusiness.org/gopkg/log"
	"code.superseriousbusiness.org/gotosocial/internal/ap"
	"code.superseriousbusiness.org/gotosocial/internal/bluesky"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtscontext"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/messages"
	"code.superseriousbusiness.org/gotosocial/internal/processing/account"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"code.superseriousbusiness.org/gotosocial/internal/surfacing"
	"code.superseriousbusiness.org/gotosocial/internal/typeutils"
	"code.superseriousbusiness.org/gotosocial/internal/uris"
	"code.superseriousbusiness.org/gotosocial/internal/util"
	"codeberg.org/gruf/go-kv/v2"
)

// clientAPI wraps processing functions
// specifically for messages originating
// from the client/REST API.
type clientAPI struct {
	state    *state.State
	surfacer *surfacing.Surfacer
	federate *federate
	account  *account.Processor
	utils    *utils
}

func (p *Processor) ProcessFromClientAPI(ctx context.Context, cMsg *messages.FromClientAPI) error {
	// Allocate new log fields slice
	fields := make([]kv.Field, 3, 4)
	fields[0] = kv.Field{"activityType", cMsg.APActivityType}
	fields[1] = kv.Field{"objectType", cMsg.APObjectType}
	fields[2] = kv.Field{"fromAccount", cMsg.Origin.Username}

	// Include GTSModel in logs if appropriate.
	if cMsg.GTSModel != nil &&
		log.Level() <= log.DEBUG {
		fields = append(fields, kv.Field{
			"model", cMsg.GTSModel,
		})
	}

	l := log.WithContext(ctx).WithFields(fields...)
	l.Info("processing from client API")

	switch cMsg.APActivityType {

	// CREATE SOMETHING
	case ap.ActivityCreate:
		switch cMsg.APObjectType {

		// CREATE USER (ie., new user+account sign-up)
		case ap.ObjectProfile:
			return p.clientAPI.CreateUser(ctx, cMsg)

		// CREATE NOTE/STATUS
		case ap.ObjectNote:
			return p.clientAPI.CreateStatus(ctx, cMsg)

		// CREATE QUESTION
		// (note we don't handle poll *votes* as AS
		// question type when federating (just notes),
		// but it makes for a nicer type switch here.
		case ap.ActivityQuestion:
			return p.clientAPI.CreatePollVote(ctx, cMsg)

		// CREATE REPLY REQUEST
		case ap.ActivityReplyRequest:
			return p.clientAPI.CreateReplyRequest(ctx, cMsg)

		// CREATE FOLLOW (request)
		case ap.ActivityFollow:
			return p.clientAPI.CreateFollowReq(ctx, cMsg)

		// CREATE LIKE/FAVE
		case ap.ActivityLike:
			return p.clientAPI.CreateLike(ctx, cMsg)

		// CREATE LIKE REQUEST
		case ap.ActivityLikeRequest:
			return p.clientAPI.CreateLikeRequest(ctx, cMsg)

		// CREATE ANNOUNCE/BOOST
		case ap.ActivityAnnounce:
			return p.clientAPI.CreateAnnounce(ctx, cMsg)

		// CREATE ANNOUNCE REQUEST
		case ap.ActivityAnnounceRequest:
			return p.clientAPI.CreateAnnounceRequest(ctx, cMsg)

		// CREATE BLOCK
		case ap.ActivityBlock:
			return p.clientAPI.CreateBlock(ctx, cMsg)
		}

	// UPDATE SOMETHING
	case ap.ActivityUpdate:
		switch cMsg.APObjectType {

		// UPDATE NOTE/STATUS
		case ap.ObjectNote:
			return p.clientAPI.UpdateStatus(ctx, cMsg)

		// UPDATE ACCOUNT (ie., bio, settings, etc)
		case ap.ActorPerson:
			return p.clientAPI.UpdateAccount(ctx, cMsg)

		// UPDATE A FLAG/REPORT (mark as resolved/closed)
		case ap.ActivityFlag:
			return p.clientAPI.UpdateReport(ctx, cMsg)

		// UPDATE USER (ie., email address)
		case ap.ObjectProfile:
			return p.clientAPI.UpdateUser(ctx, cMsg)
		}

	// ACCEPT SOMETHING
	case ap.ActivityAccept:
		switch cMsg.APObjectType { //nolint:gocritic

		// ACCEPT FOLLOW (request)
		case ap.ActivityFollow:
			return p.clientAPI.AcceptFollow(ctx, cMsg)

		// ACCEPT USER (ie., new user+account sign-up)
		case ap.ObjectProfile:
			return p.clientAPI.AcceptUser(ctx, cMsg)

		// ACCEPT NOTE/STATUS (ie., accept a reply)
		case ap.ObjectNote:
			return p.clientAPI.AcceptReply(ctx, cMsg)

		// ACCEPT LIKE
		case ap.ActivityLike:
			return p.clientAPI.AcceptLike(ctx, cMsg)

		// ACCEPT BOOST
		case ap.ActivityAnnounce:
			return p.clientAPI.AcceptAnnounce(ctx, cMsg)
		}

	// REJECT SOMETHING
	case ap.ActivityReject:
		switch cMsg.APObjectType { //nolint:gocritic

		// REJECT FOLLOW (request)
		case ap.ActivityFollow:
			return p.clientAPI.RejectFollowRequest(ctx, cMsg)

		// REJECT USER (ie., new user+account sign-up)
		case ap.ObjectProfile:
			return p.clientAPI.RejectUser(ctx, cMsg)

		// REJECT NOTE/STATUS (ie., reject a reply)
		case ap.ObjectNote:
			return p.clientAPI.RejectReply(ctx, cMsg)

		// REJECT LIKE
		case ap.ActivityLike:
			return p.clientAPI.RejectLike(ctx, cMsg)

		// REJECT BOOST
		case ap.ActivityAnnounce:
			return p.clientAPI.RejectAnnounce(ctx, cMsg)
		}

	// UNDO SOMETHING
	case ap.ActivityUndo:
		switch cMsg.APObjectType {

		// UNDO FOLLOW (request)
		case ap.ActivityFollow:
			return p.clientAPI.UndoFollow(ctx, cMsg)

		// UNDO BLOCK
		case ap.ActivityBlock:
			return p.clientAPI.UndoBlock(ctx, cMsg)

		// UNDO LIKE/FAVE
		case ap.ActivityLike:
			return p.clientAPI.UndoFave(ctx, cMsg)

		// UNDO ANNOUNCE/BOOST
		case ap.ActivityAnnounce:
			return p.clientAPI.UndoAnnounce(ctx, cMsg)
		}

	// DELETE SOMETHING
	case ap.ActivityDelete:
		switch cMsg.APObjectType {

		// DELETE NOTE/STATUS
		case ap.ObjectNote:
			return p.clientAPI.DeleteStatus(ctx, cMsg)

		// DELETE REMOTE ACCOUNT or LOCAL USER+ACCOUNT
		case ap.ActorPerson, ap.ObjectProfile:
			return p.clientAPI.DeleteAccountOrUser(ctx, cMsg)
		}

	// FLAG/REPORT SOMETHING
	case ap.ActivityFlag:
		switch cMsg.APObjectType { //nolint:gocritic

		// FLAG/REPORT ACCOUNT
		case ap.ActorPerson:
			return p.clientAPI.ReportAccount(ctx, cMsg)
		}

	// MOVE SOMETHING
	case ap.ActivityMove:
		switch cMsg.APObjectType { //nolint:gocritic

		// MOVE ACCOUNT
		case ap.ActorPerson:
			return p.clientAPI.MoveAccount(ctx, cMsg)
		}
	}

	return gtserror.Newf("unhandled: %s %s", cMsg.APActivityType, cMsg.APObjectType)
}

func (p *clientAPI) CreateUser(ctx context.Context, cMsg *messages.FromClientAPI) error {
	newUser, ok := cMsg.GTSModel.(*gtsmodel.User)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.User", cMsg.GTSModel)
	}

	// Notify mods of the new signup.
	if err := p.surfacer.NotifySignup(ctx, newUser); err != nil {
		log.Errorf(ctx, "error notifying mods of new sign-up: %v", err)
	}

	// Send "new sign up" email to mods.
	if err := p.surfacer.EmailAdminNewSignup(ctx, newUser); err != nil {
		log.Errorf(ctx, "error emailing new signup: %v", err)
	}

	// Send "please confirm your address" email to the new user.
	if err := p.surfacer.EmailUserPleaseConfirm(ctx, newUser, true); err != nil {
		log.Errorf(ctx, "error emailing confirm: %v", err)
	}

	return nil
}

func (p *clientAPI) CreateStatus(ctx context.Context, cMsg *messages.FromClientAPI) error {
	status, ok := cMsg.GTSModel.(*gtsmodel.Status)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Status", cMsg.GTSModel)
	}

	if err := p.surfacer.TimelineAndNotifyStatus(ctx, status); err != nil {
		log.Errorf(ctx, "error timelining and notifying status: %v", err)
	}

	// Send the status out to followers / mentioned accounts / relays.
	if err := p.federate.CreateStatus(ctx, status); err != nil {
		log.Errorf(ctx, "error federating status: %v", err)
	}

	connection, err := p.state.DB.GetBlueskyConnectionByAccountID(ctx, status.AccountID)
	_, interactionErr := p.state.DB.GetBlueskyInteractionByStatusID(ctx, status.InReplyToID)
	if err == nil && (interactionErr == nil || bluesky.EligibleForCrosspost(status, connection)) {
		delivery, queueErr := bluesky.QueueStatus(ctx, p.state, status)
		if queueErr != nil {
			log.Errorf(ctx, "error queueing Bluesky crosspost: %v", queueErr)
		} else if publishErr := bluesky.ProcessDelivery(ctx, p.state, p.utils.converter, delivery); publishErr != nil {
			log.Errorf(ctx, "error publishing status to Bluesky; queued for retry: %v", publishErr)
		}
	} else if err != nil && !errors.Is(err, db.ErrNoEntries) {
		log.Errorf(ctx, "error checking Bluesky crosspost settings: %v", err)
	} else if interactionErr != nil && !errors.Is(interactionErr, db.ErrNoEntries) {
		log.Errorf(ctx, "error checking Bluesky reply mapping: %v", interactionErr)
	}

	return nil
}

func (p *clientAPI) CreateReplyRequest(ctx context.Context, cMsg *messages.FromClientAPI) error {
	reply, ok := cMsg.GTSModel.(*gtsmodel.Status)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Status", cMsg.GTSModel)
	}

	// Create a polite reply request.
	intReqID := id.NewULIDFromTime(reply.CreatedAt)
	intReq := &gtsmodel.InteractionRequest{
		ID:                    intReqID,
		TargetStatusID:        reply.InReplyToID,
		TargetStatus:          reply.InReplyTo,
		TargetAccountID:       reply.InReplyToAccountID,
		TargetAccount:         reply.InReplyToAccount,
		InteractingAccountID:  reply.AccountID,
		InteractingAccount:    reply.Account,
		InteractionRequestURI: uris.GenerateURIForReplyRequest(reply.Account.Username, intReqID),
		InteractionURI:        reply.URI,
		InteractionType:       gtsmodel.InteractionReply,
		Polite:                util.Ptr(true),
		Reply:                 reply,
	}

	if !reply.PreApproved {
		// If the reply is not pre-approved, just
		// store the interaction request, notify
		// (local) target or federate request.
		if err := p.utils.storeInteractionRequest(
			ctx, intReq,
		); err != nil {
			return gtserror.Newf("error storing interaction request: %w", err)
		}

		// Notify target account (if local) of pending reply.
		if err := p.surfacer.NotifyPendingReply(ctx, intReq.Reply); err != nil {
			return gtserror.Newf("error notifying pending reply: %w", err)
		}

		// Send interaction request to target account (if remote).
		if err := p.federate.InteractionRequest(ctx, intReq); err != nil {
			return gtserror.Newf("error federating interaction request: %w", err)
		}

		return nil
	}

	// If the reply is pre-approved, then it must
	// target a status on our instance, and the
	// replier gets automatic approval due to being
	// in the author's followers/following collection.
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
	reply.Flags.SetPendingApproval(false)
	reply.PreApproved = false
	reply.ApprovedByURI = intReq.AuthorizationURI
	if err := p.state.DB.UpdateStatus(ctx,
		reply,
		"flags",
		"approved_by_uri",
	); err != nil {
		return gtserror.Newf("db error updating status: %w", err)
	}

	// Send out the approval as Accept.
	if err := p.federate.AcceptInteraction(ctx, intReq); err != nil {
		return gtserror.Newf("error federating pre-approval of reply: %w", err)
	}

	// Timeline + notify the reply.
	if err := p.surfacer.TimelineAndNotifyStatus(ctx, reply); err != nil {
		log.Errorf(ctx, "error timelining and notifying status: %v", err)
	}

	// Send out the approved reply.
	if err := p.federate.CreateStatus(ctx, reply); err != nil {
		log.Errorf(ctx, "error federating status: %v", err)
	}

	return nil
}

func (p *clientAPI) CreatePollVote(ctx context.Context, cMsg *messages.FromClientAPI) error {
	// Cast the create poll vote attached to message.
	vote, ok := cMsg.GTSModel.(*gtsmodel.PollVote)
	if !ok {
		return gtserror.Newf("cannot cast %T -> *gtsmodel.Pollvote", cMsg.GTSModel)
	}

	// Ensure the vote is fully populated in order to get original poll.
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
		// These are poll votes in a local status, we only need to
		// federate the updated status model with latest vote counts.
		if err := p.federate.UpdateStatus(ctx, status); err != nil {
			log.Errorf(ctx, "error federating status update: %v", err)
		}
	} else {
		// These are votes in a remote poll, federate to origin the new poll vote(s).
		if err := p.federate.CreatePollVote(ctx, vote.Poll, vote); err != nil {
			log.Errorf(ctx, "error federating poll vote: %v", err)
		}
	}

	return nil
}

func (p *clientAPI) CreateFollowReq(ctx context.Context, cMsg *messages.FromClientAPI) error {
	followRequest, ok := cMsg.GTSModel.(*gtsmodel.FollowRequest)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.FollowRequest", cMsg.GTSModel)
	}

	// If target is a local, unlocked account,
	// we can skip side effects for the follow
	// request and accept the follow immediately.
	if cMsg.Target.IsLocal() && !*cMsg.Target.Locked {
		// Accept the FR first to get the Follow.
		follow, err := p.state.DB.AcceptFollowRequest(
			ctx,
			cMsg.Origin.ID,
			cMsg.Target.ID,
		)
		if err != nil {
			return gtserror.Newf("db error accepting follow req: %w", err)
		}

		// Use AcceptFollow to do side effects.
		return p.AcceptFollow(ctx, &messages.FromClientAPI{
			APObjectType:   ap.ActivityFollow,
			APActivityType: ap.ActivityAccept,
			GTSModel:       follow,
			Origin:         cMsg.Origin,
			Target:         cMsg.Target,
		})
	}

	if err := p.surfacer.NotifyFollowRequest(ctx, followRequest); err != nil {
		log.Errorf(ctx, "error notifying follow request: %v", err)
	}

	// Convert the follow request to follow model (requests are sent as follows).
	follow := typeutils.FollowRequestToFollow(followRequest)

	if err := p.federate.Follow(
		ctx,
		follow,
	); err != nil {
		log.Errorf(ctx, "error federating follow request: %v", err)
	}

	return nil
}

func (p *clientAPI) CreateLike(ctx context.Context, cMsg *messages.FromClientAPI) error {
	fave, ok := cMsg.GTSModel.(*gtsmodel.StatusFave)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.StatusFave", cMsg.GTSModel)
	}

	// Ensure fave populated.
	if err := p.state.DB.PopulateStatusFave(ctx, fave); err != nil {
		return gtserror.Newf("error populating status fave: %w", err)
	}

	if err := p.surfacer.NotifyFave(ctx, fave); err != nil {
		log.Errorf(ctx, "error notifying fave: %v", err)
	}

	if err := p.federate.Like(ctx, fave); err != nil {
		log.Errorf(ctx, "error federating like: %v", err)
	}

	return nil
}

func (p *clientAPI) CreateLikeRequest(ctx context.Context, cMsg *messages.FromClientAPI) error {
	fave, ok := cMsg.GTSModel.(*gtsmodel.StatusFave)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.StatusFave", cMsg.GTSModel)
	}

	// Create a polite like request.
	intReqID := id.NewULIDFromTime(fave.CreatedAt)
	intReq := &gtsmodel.InteractionRequest{
		ID:                    intReqID,
		TargetStatusID:        fave.StatusID,
		TargetStatus:          fave.Status,
		TargetAccountID:       fave.TargetAccountID,
		TargetAccount:         fave.TargetAccount,
		InteractingAccountID:  fave.AccountID,
		InteractingAccount:    fave.Account,
		InteractionRequestURI: uris.GenerateURIForLikeRequest(fave.Account.Username, intReqID),
		InteractionURI:        fave.URI,
		InteractionType:       gtsmodel.InteractionLike,
		Polite:                util.Ptr(true),
		Like:                  fave,
	}

	if !fave.PreApproved {
		// If the fave is not pre-approved, just
		// store the interaction request, notify
		// (local) target or federate request.
		if err := p.utils.storeInteractionRequest(
			ctx, intReq,
		); err != nil {
			return gtserror.Newf("error storing interaction request: %w", err)
		}

		// Notify target account (if local) of pending fave.
		if err := p.surfacer.NotifyPendingFave(ctx, intReq.Like); err != nil {
			return gtserror.Newf("error notifying pending fave: %w", err)
		}

		// Send interaction request to target account (if remote).
		if err := p.federate.InteractionRequest(ctx, intReq); err != nil {
			return gtserror.Newf("error federating interaction request: %w", err)
		}

		return nil
	}

	// If the fave is pre-approved, then it must
	// target a status on our instance, and the
	// faver gets automatic approval due to being
	// in the author's followers/following collection.
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

	// Send out the approval as an Accept.
	if err := p.federate.AcceptInteraction(ctx, intReq); err != nil {
		return gtserror.Newf("error federating pre-approval of fave: %w", err)
	}

	// Notify the status author about the fave.
	if err := p.surfacer.NotifyFave(ctx, fave); err != nil {
		log.Errorf(ctx, "error notifying fave: %v", err)
	}

	// We don't (yet) federate Likes out
	// to anyone but the target of the like,
	// so there's no need to send it anywhere.
	// Just return.
	return nil
}

func (p *clientAPI) CreateAnnounce(ctx context.Context, cMsg *messages.FromClientAPI) error {
	boost, ok := cMsg.GTSModel.(*gtsmodel.Status)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Status", cMsg.GTSModel)
	}

	// Timeline and notify the boost wrapper status.
	if err := p.surfacer.TimelineAndNotifyStatus(ctx, boost); err != nil {
		log.Errorf(ctx, "error timelining and notifying status: %v", err)
	}

	// Notify the boost target account (if local).
	if err := p.surfacer.NotifyAnnounce(ctx, boost); err != nil {
		log.Errorf(ctx, "error notifying boost: %v", err)
	}

	// Send out the Announce.
	if err := p.federate.Announce(ctx, boost); err != nil {
		log.Errorf(ctx, "error federating announce: %v", err)
	}

	return nil
}

func (p *clientAPI) CreateAnnounceRequest(ctx context.Context, cMsg *messages.FromClientAPI) error {
	boost, ok := cMsg.GTSModel.(*gtsmodel.Status)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Status", cMsg.GTSModel)
	}

	// Create a polite reply request.
	intReqID := id.NewULIDFromTime(boost.CreatedAt)
	intReq := &gtsmodel.InteractionRequest{
		ID:                    intReqID,
		TargetStatusID:        boost.BoostOfID,
		TargetStatus:          boost.BoostOf,
		TargetAccountID:       boost.BoostOfAccountID,
		TargetAccount:         boost.BoostOfAccount,
		InteractingAccountID:  boost.AccountID,
		InteractingAccount:    boost.Account,
		InteractionRequestURI: uris.GenerateURIForAnnounceRequest(boost.Account.Username, intReqID),
		InteractionURI:        boost.URI,
		InteractionType:       gtsmodel.InteractionAnnounce,
		Polite:                util.Ptr(true),
		Announce:              boost,
	}

	if !boost.PreApproved {
		// If the boost is not pre-approved, just
		// store the interaction request, notify
		// (local) target or federate request.
		if err := p.utils.storeInteractionRequest(
			ctx, intReq,
		); err != nil {
			return gtserror.Newf("error storing interaction request: %w", err)
		}

		// Notify target account (if local) of pending announce.
		if err := p.surfacer.NotifyPendingAnnounce(ctx, intReq.Announce); err != nil {
			return gtserror.Newf("error notifying pending announce: %w", err)
		}

		// Send interaction request to target account (if remote).
		if err := p.federate.InteractionRequest(ctx, intReq); err != nil {
			return gtserror.Newf("error federating interaction request: %w", err)
		}

		return nil
	}

	// If the boost is pre-approved, then it must
	// target a status on our instance, and the
	// booster gets automatic approval due to being
	// in the author's followers/following collection.
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

	// Timeline and notify the boost wrapper status.
	if err := p.surfacer.TimelineAndNotifyStatus(ctx, boost); err != nil {
		log.Errorf(ctx, "error timelining and notifying status: %v", err)
	}

	// Notify the boost target account.
	if err := p.surfacer.NotifyAnnounce(ctx, boost); err != nil {
		log.Errorf(ctx, "error notifying boost: %v", err)
	}

	// Send out the approval as an Accept.
	if err := p.federate.AcceptInteraction(ctx, intReq); err != nil {
		return gtserror.Newf("error federating pre-approval of boost: %w", err)
	}

	// Send out the announce itself.
	if err := p.federate.Announce(ctx, boost); err != nil {
		log.Errorf(ctx, "error federating announce: %v", err)
	}

	return nil
}

func (p *clientAPI) CreateBlock(ctx context.Context, cMsg *messages.FromClientAPI) error {
	block, ok := cMsg.GTSModel.(*gtsmodel.Block)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Block", cMsg.GTSModel)
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

	// TODO: same with notifications?
	// TODO: same with bookmarks?

	if err := p.federate.Block(ctx, block); err != nil {
		log.Errorf(ctx, "error federating block: %v", err)
	}

	return nil
}

func (p *clientAPI) UpdateStatus(ctx context.Context, cMsg *messages.FromClientAPI) error {
	// Cast the updated Status model attached to msg.
	status, ok := cMsg.GTSModel.(*gtsmodel.Status)
	if !ok {
		return gtserror.Newf("cannot cast %T -> *gtsmodel.Status", cMsg.GTSModel)
	}

	// Federate the updated status changes out remotely.
	if err := p.federate.UpdateStatus(ctx, status); err != nil {
		log.Errorf(ctx, "error federating status update: %v", err)
	}

	// Stream and notify relevant local users that the status has been edited.
	if err := p.surfacer.TimelineAndNotifyStatusUpdate(ctx, status); err != nil {
		log.Errorf(ctx, "error streaming status edit: %v", err)
	}

	return nil
}

func (p *clientAPI) UpdateAccount(ctx context.Context, cMsg *messages.FromClientAPI) error {
	account, ok := cMsg.GTSModel.(*gtsmodel.Account)
	if !ok {
		return gtserror.Newf("cannot cast %T -> *gtsmodel.Account", cMsg.GTSModel)
	}

	if err := p.federate.UpdateAccount(ctx, account); err != nil {
		log.Errorf(ctx, "error federating account update: %v", err)
	}

	return nil
}

func (p *clientAPI) UpdateReport(ctx context.Context, cMsg *messages.FromClientAPI) error {
	report, ok := cMsg.GTSModel.(*gtsmodel.Report)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Report", cMsg.GTSModel)
	}

	if report.Account.IsRemote() {
		// Report creator is a remote account,
		// we shouldn't try to email them!
		return nil
	}

	if err := p.surfacer.EmailUserReportClosed(ctx, report); err != nil {
		log.Errorf(ctx, "error emailing report closed: %v", err)
	}

	return nil
}

func (p *clientAPI) UpdateUser(ctx context.Context, cMsg *messages.FromClientAPI) error {
	user, ok := cMsg.GTSModel.(*gtsmodel.User)
	if !ok {
		return gtserror.Newf("cannot cast %T -> *gtsmodel.User", cMsg.GTSModel)
	}

	// The only possible "UpdateUser" action is to update the
	// user's email address, so we can safely assume by this
	// point that a new unconfirmed email address has been set.
	if err := p.surfacer.EmailUserPleaseConfirm(ctx, user, false); err != nil {
		log.Errorf(ctx, "error emailing report closed: %v", err)
	}

	return nil
}

func (p *clientAPI) AcceptFollow(ctx context.Context, cMsg *messages.FromClientAPI) error {
	follow, ok := cMsg.GTSModel.(*gtsmodel.Follow)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Follow", cMsg.GTSModel)
	}

	if err := p.surfacer.NotifyFollow(ctx, follow); err != nil {
		log.Errorf(ctx, "error notifying follow: %v", err)
	}

	if err := p.federate.AcceptFollow(ctx, follow); err != nil {
		log.Errorf(ctx, "error federating follow accept: %v", err)
	}

	return nil
}

func (p *clientAPI) RejectFollowRequest(ctx context.Context, cMsg *messages.FromClientAPI) error {
	followReq, ok := cMsg.GTSModel.(*gtsmodel.FollowRequest)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.FollowRequest", cMsg.GTSModel)
	}

	if err := p.federate.RejectFollow(ctx,
		typeutils.FollowRequestToFollow(followReq),
	); err != nil {
		log.Errorf(ctx, "error federating follow reject: %v", err)
	}

	return nil
}

func (p *clientAPI) UndoFollow(ctx context.Context, cMsg *messages.FromClientAPI) error {
	follow, ok := cMsg.GTSModel.(*gtsmodel.Follow)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Follow", cMsg.GTSModel)
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
	}

	if err := p.federate.UndoFollow(ctx, follow); err != nil {
		log.Errorf(ctx, "error federating follow undo: %v", err)
	}

	return nil
}

func (p *clientAPI) UndoBlock(ctx context.Context, cMsg *messages.FromClientAPI) error {
	block, ok := cMsg.GTSModel.(*gtsmodel.Block)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Block", cMsg.GTSModel)
	}

	if err := p.federate.UndoBlock(ctx, block); err != nil {
		log.Errorf(ctx, "error federating block undo: %v", err)
	}

	return nil
}

func (p *clientAPI) UndoFave(ctx context.Context, cMsg *messages.FromClientAPI) error {
	statusFave, ok := cMsg.GTSModel.(*gtsmodel.StatusFave)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.StatusFave", cMsg.GTSModel)
	}

	if err := p.federate.UndoLike(ctx, statusFave); err != nil {
		log.Errorf(ctx, "error federating like undo: %v", err)
	}

	return nil
}

func (p *clientAPI) UndoAnnounce(ctx context.Context, cMsg *messages.FromClientAPI) error {
	status, ok := cMsg.GTSModel.(*gtsmodel.Status)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Status", cMsg.GTSModel)
	}

	// Delete the boost wrapper status from timelines.
	p.surfacer.DeleteStatusFromTimelines(ctx, status.ID)

	if err := p.federate.UndoAnnounce(ctx, status); err != nil {
		log.Errorf(ctx, "error federating announce undo: %v", err)
	}

	return nil
}

func (p *clientAPI) DeleteStatus(ctx context.Context, cMsg *messages.FromClientAPI) error {
	status, ok := cMsg.GTSModel.(*gtsmodel.Status)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Status", cMsg.GTSModel)
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

	// Federate delete activity targeting status to remote servers.
	if err := p.federate.DeleteStatus(ctx, status); err != nil {
		log.Errorf(ctx, "error federating status %s delete: %v", status.URI, err)
	}

	// Don't delete attachments, just unattach them:
	// this request comes from the client API and the
	// poster may want to use attachments again later.
	const attachments = false

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

func (p *clientAPI) DeleteAccountOrUser(ctx context.Context, cMsg *messages.FromClientAPI) error {
	// The originID of the delete, one of:
	//   - ID of a domain block, for which
	//     this account delete is a side effect.
	//   - ID of the deleted account itself (self delete).
	//   - ID of an admin account (account suspension).
	var originID string

	if domainBlock, ok := cMsg.GTSModel.(*gtsmodel.DomainBlock); ok {
		// Origin is a domain block.
		originID = domainBlock.ID
	} else {
		// Origin is whichever account
		// originated this message.
		originID = cMsg.Origin.ID
	}

	// Extract target account.
	account := cMsg.Target

	// Drop any outgoing queued AP requests to / from / targeting
	// this account, (stops queued likes, boosts, creates etc).
	p.state.Workers.Delivery.Queue.Delete("ActorID", account.URI)
	p.state.Workers.Delivery.Queue.Delete("ObjectID", account.URI)
	p.state.Workers.Delivery.Queue.Delete("TargetID", account.URI)

	// Drop any incoming queued client messages to / from this
	// account, (stops processing of local origin data for acccount).
	p.state.Workers.Client.Queue.Delete("Origin.ID", account.ID)
	p.state.Workers.Client.Queue.Delete("Target.ID", account.ID)
	p.state.Workers.Client.Queue.Delete("TargetURI", account.URI)

	// Drop any incoming queued federator messages to this account,
	// (stops processing of remote origin data targeting this account).
	p.state.Workers.Federator.Queue.Delete("Receiving.ID", account.ID)
	p.state.Workers.Federator.Queue.Delete("TargetURI", account.URI)

	// Remove any entries authored by account from timelines.
	p.surfacer.RemoveTimelineEntriesByAccount(account.ID)

	// Remove any of their cached timelines.
	p.state.Caches.Timelines.Home.Delete(account.ID)

	// Get the IDs of all the lists owned by the given account ID.
	listIDs, err := p.state.DB.GetListIDsByAccountID(ctx, account.ID)
	if err != nil {
		log.Errorf(ctx, "error getting lists for account %s: %v", account.ID, err)
	}

	// Remove account's list timelines.
	for _, listID := range listIDs {
		p.state.Caches.Timelines.List.Delete(listID)
	}

	// Federate delete activity targeting account to remote servers.
	if err := p.federate.DeleteAccount(ctx, cMsg.Target); err != nil {
		log.Errorf(ctx, "error federating account %s delete: %v", account.URI, err)
	}

	// And finally, perform the actual account deletion synchronously.
	if err := p.account.Delete(ctx, account, originID); err != nil {
		log.Errorf(ctx, "error deleting account %s: %v", account.URI, err)
	}

	return nil
}

func (p *clientAPI) ReportAccount(ctx context.Context, cMsg *messages.FromClientAPI) error {
	report, ok := cMsg.GTSModel.(*gtsmodel.Report)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.Report", cMsg.GTSModel)
	}

	// Federate this report to the
	// remote instance if desired.
	if *report.Forwarded {
		if err := p.federate.Flag(ctx, report); err != nil {
			log.Errorf(ctx, "error federating flag: %v", err)
		}
	}

	if err := p.surfacer.EmailAdminReportOpened(ctx, report); err != nil {
		log.Errorf(ctx, "error emailing report opened: %v", err)
	}

	return nil
}

func (p *clientAPI) MoveAccount(ctx context.Context, cMsg *messages.FromClientAPI) error {
	// Redirect each local follower of
	// OriginAccount to follow move target.
	p.utils.redirectFollowers(ctx, cMsg.Origin, cMsg.Target)

	// At this point, we know OriginAccount has the
	// Move set on it. Just make sure it's populated.
	if err := p.state.DB.PopulateMove(ctx, cMsg.Origin.Move); err != nil {
		return gtserror.Newf("error populating Move: %w", err)
	}

	// Now send the Move message out to
	// OriginAccount's (remote) followers.
	if err := p.federate.MoveAccount(ctx, cMsg.Origin); err != nil {
		return gtserror.Newf("error federating account move: %w", err)
	}

	// Mark the move attempt as successful.
	cMsg.Origin.Move.SucceededAt = cMsg.Origin.Move.AttemptedAt
	if err := p.state.DB.UpdateMove(
		ctx,
		cMsg.Origin.Move,
		"succeeded_at",
	); err != nil {
		return gtserror.Newf("error marking move as successful: %w", err)
	}

	return nil
}

func (p *clientAPI) AcceptUser(ctx context.Context, cMsg *messages.FromClientAPI) error {
	newUser, ok := cMsg.GTSModel.(*gtsmodel.User)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.User", cMsg.GTSModel)
	}

	// Mark user as approved + clear sign-up IP.
	newUser.Approved = util.Ptr(true)
	newUser.SignUpIP = nil
	if err := p.state.DB.UpdateUser(ctx, newUser, "approved", "sign_up_ip"); err != nil {
		// Error now means we should return without
		// sending email + let admin try to approve again.
		return gtserror.Newf("db error updating user %s: %w", newUser.ID, err)
	}

	// Send "your sign-up has been approved" email to the new user.
	if err := p.surfacer.EmailUserSignupApproved(ctx, newUser); err != nil {
		log.Errorf(ctx, "error emailing: %v", err)
	}

	return nil
}

func (p *clientAPI) RejectUser(ctx context.Context, cMsg *messages.FromClientAPI) error {
	deniedUser, ok := cMsg.GTSModel.(*gtsmodel.DeniedUser)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.DeniedUser", cMsg.GTSModel)
	}

	// Remove the account.
	if err := p.state.DB.DeleteAccount(ctx, cMsg.Target.ID); err != nil {
		log.Errorf(ctx,
			"db error deleting account %s: %v",
			cMsg.Target.ID, err,
		)
	}

	// Remove the user.
	if err := p.state.DB.DeleteUserByID(ctx, deniedUser.ID); err != nil {
		log.Errorf(ctx,
			"db error deleting user %s: %v",
			deniedUser.ID, err,
		)
	}

	// Store the deniedUser entry.
	if err := p.state.DB.PutDeniedUser(ctx, deniedUser); err != nil {
		log.Errorf(ctx,
			"db error putting denied user %s: %v",
			deniedUser.ID, err,
		)
	}

	if *deniedUser.SendEmail {
		// Send "your sign-up has been rejected" email to the denied user.
		if err := p.surfacer.EmailUserSignupRejected(ctx, deniedUser); err != nil {
			log.Errorf(ctx, "error emailing: %v", err)
		}
	}

	return nil
}

func (p *clientAPI) AcceptLike(ctx context.Context, cMsg *messages.FromClientAPI) error {
	req, ok := cMsg.GTSModel.(*gtsmodel.InteractionRequest)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.InteractionRequest", cMsg.GTSModel)
	}

	// Notify the fave (distinct from the notif for the pending fave).
	if err := p.surfacer.NotifyFave(ctx, req.Like); err != nil {
		log.Errorf(ctx, "error notifying fave: %v", err)
	}

	// Send out the Accept.
	if err := p.federate.AcceptInteraction(ctx, req); err != nil {
		log.Errorf(ctx, "error federating approval of like: %v", err)
	}

	return nil
}

func (p *clientAPI) AcceptReply(ctx context.Context, cMsg *messages.FromClientAPI) error {
	req, ok := cMsg.GTSModel.(*gtsmodel.InteractionRequest)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.InteractionRequest", cMsg.GTSModel)
	}

	reply := req.Reply

	// Timeline the reply + notify relevant accounts.
	if err := p.surfacer.TimelineAndNotifyStatus(ctx, reply); err != nil {
		log.Errorf(ctx, "error timelining and notifying status reply: %v", err)
	}

	// Send out the Accept.
	if err := p.federate.AcceptInteraction(ctx, req); err != nil {
		log.Errorf(ctx, "error federating approval of reply: %v", err)
	}

	return nil
}

func (p *clientAPI) AcceptAnnounce(ctx context.Context, cMsg *messages.FromClientAPI) error {
	req, ok := cMsg.GTSModel.(*gtsmodel.InteractionRequest)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.InteractionRequest", cMsg.GTSModel)
	}

	boost := req.Announce

	// Timeline and notify the announce.
	if err := p.surfacer.TimelineAndNotifyStatus(ctx, boost); err != nil {
		log.Errorf(ctx, "error timelining and notifying status: %v", err)
	}

	// Notify the announce (distinct from the notif for the pending announce).
	if err := p.surfacer.NotifyAnnounce(ctx, boost); err != nil {
		log.Errorf(ctx, "error notifying announce: %v", err)
	}

	// Send out the Accept.
	if err := p.federate.AcceptInteraction(ctx, req); err != nil {
		log.Errorf(ctx, "error federating approval of announce: %v", err)
	}

	return nil
}

func (p *clientAPI) RejectLike(ctx context.Context, cMsg *messages.FromClientAPI) error {
	req, ok := cMsg.GTSModel.(*gtsmodel.InteractionRequest)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.InteractionRequest", cMsg.GTSModel)
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

	// Delete the status fave.
	if err := p.state.DB.DeleteStatusFaveByID(ctx, fave.ID); err != nil {
		return gtserror.Newf("db error deleting status fave: %w", err)
	}

	return nil
}

func (p *clientAPI) RejectReply(ctx context.Context, cMsg *messages.FromClientAPI) error {
	req, ok := cMsg.GTSModel.(*gtsmodel.InteractionRequest)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.InteractionRequest", cMsg.GTSModel)
	}

	// At this point the InteractionRequest should already
	// be in the database, we just need to do side effects.

	// Federate our the Reject to those remote parties involved.
	if err := p.federate.RejectInteraction(ctx, req); err != nil {
		log.Errorf(ctx, "error federating rejection of reply: %v", err)
	}

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

func (p *clientAPI) RejectAnnounce(ctx context.Context, cMsg *messages.FromClientAPI) error {
	req, ok := cMsg.GTSModel.(*gtsmodel.InteractionRequest)
	if !ok {
		return gtserror.Newf("%T not parseable as *gtsmodel.InteractionRequest", cMsg.GTSModel)
	}

	// At this point the InteractionRequest should already
	// be in the database, we just need to do side effects.

	// Send out the Reject.
	if err := p.federate.RejectInteraction(ctx, req); err != nil {
		log.Errorf(ctx, "error federating rejection of announce: %v", err)
	}

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
