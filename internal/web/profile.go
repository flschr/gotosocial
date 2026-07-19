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

package web

import (
	"context"
	"maps"
	"net/http"
	"net/url"

	"code.superseriousbusiness.org/gopkg/log"
	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	apiutil "code.superseriousbusiness.org/gotosocial/internal/api/util"
	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/paging"
	"code.superseriousbusiness.org/gotosocial/internal/processing/account"
	"github.com/gin-gonic/gin"
)

type profile struct {
	instance          *apimodel.InstanceV1
	account           *apimodel.WebAccount
	rssFeed           string
	robotsMeta        string
	pinnedStatuses    []*apimodel.WebStatus
	statusResp        *apimodel.PageableResponse
	paging            bool
	includeBoostsLink string
	excludeBoostsLink string
}

// prepareProfile does content type checks, fetches the
// targeted account from the db, and converts it to its
// web representation, along with other data needed to
// render the web view of the account.
func (m *Module) prepareProfile(c *gin.Context) *profile {
	ctx := c.Request.Context()

	// We'll need the instance later, and we can also use it
	// before then to make it easier to return a web error.
	instance, errWithCode := m.processor.InstanceGetV1(ctx)
	if errWithCode != nil {
		apiutil.WebErrorHandler(c, errWithCode, m.processor.InstanceGetV1)
		return nil
	}

	// Return instance we already got from the db,
	// don't try to fetch it again when erroring.
	instanceGet := func(ctx context.Context) (*apimodel.InstanceV1, gtserror.WithCode) {
		return instance, nil
	}

	// Parse + normalize account username from the URL.
	requestedUser, errWithCode := apiutil.ParseUsername(c.Param(apiutil.UsernameKey))
	if errWithCode != nil {
		apiutil.WebErrorHandler(c, errWithCode, instanceGet)
		return nil
	}

	// Check what type of content is being requested.
	// If we're getting an AP request on this endpoint
	// we should render the AP representation instead.
	accept, err := apiutil.NegotiateAccept(c, apiutil.HTMLOrActivityPubHeaders...)
	if err != nil {
		apiutil.WebErrorHandler(c, gtserror.NewErrorNotAcceptable(err, err.Error()), instanceGet)
		return nil
	}

	if apiutil.ASContentType(accept) {
		// AP account representation has
		// been requested, return that.
		user, errWithCode := m.processor.Fedi().UserGet(c.Request.Context(), requestedUser)
		if errWithCode != nil {
			apiutil.WebErrorHandler(c, errWithCode, m.processor.InstanceGetV1)
			return nil
		}

		apiutil.JSONType(c, http.StatusOK, accept, user)
		return nil
	}

	// text/html has been requested.
	//
	// Proceed with getting the web
	// representation of the account.
	account, errWithCode := m.processor.Account().GetWeb(ctx, requestedUser)
	if errWithCode != nil {
		apiutil.WebErrorHandler(c, errWithCode, instanceGet)
		return nil
	}

	if host := config.GetHost(); account.Username == host {
		// If this is the instance account, we can
		// return early without trying to fetch statuses
		// or do further checks for pinned, rss, etc.
		//
		// Rewrite a few bits and bobs on the account so
		// visitors to the page aren't completely baffled.
		account.Note = "This is the instance service/application actor account for " +
			host + "; it is used only for relaying and admin tasks."
		account.HideCollections = true
		return &profile{
			instance:   instance,
			account:    account,
			statusResp: paging.EmptyResponse(),
		}
	}

	// If target account is suspended,
	// this page should not be visible.
	//
	// TODO: change this to 410?
	if account.Suspended {
		err := gtserror.Newf("target account %s is suspended", requestedUser)
		apiutil.WebErrorHandler(c, gtserror.NewErrorNotFound(err), instanceGet)
		return nil
	}

	// Only generate RSS link if
	// account has RSS enabled.
	var rssFeed string
	if account.EnableRSS {
		rssFeed = "/@" + account.Username + "/feed.rss"
	}

	// Since we serve the profile and posts together,
	// only allow search robots if account is discoverable
	// *and* indexable.
	var robotsMeta string
	if account.Discoverable && account.Indexable {
		robotsMeta = apiutil.RobotsDirectivesAllowSome
	}

	// Check if paging.
	maxStatusID := apiutil.ParseMaxID(c.Query(apiutil.MaxIDKey), "")
	doPaging := (maxStatusID != "")

	var (
		mediaOnly      = account.WebLayout == "gallery"
		pinnedStatuses []*apimodel.WebStatus
	)

	if !doPaging {
		// If not paging, load pinned statuses.
		var errWithCode gtserror.WithCode
		pinnedStatuses, errWithCode = m.processor.Account().WebStatusesGetPinned(
			ctx,
			account.ID,
			mediaOnly,
		)
		if errWithCode != nil {
			apiutil.WebErrorHandler(c, errWithCode, instanceGet)
			return nil
		}
	}

	// Limit varies depending on whether this is a gallery view or not.
	// If gallery view, we want a nice full screen of media, else we
	// don't want to overwhelm the viewer with a shitload of posts.
	var limit int
	if account.WebLayout == "gallery" {
		limit = 40
	} else {
		limit = 20
	}

	// Parse the "include_boosts" query parameter, if provided.
	// This might not actually result in boosts being included
	// in the response, depending on what target account allows.
	preferIncludeBoosts, errWithCode := apiutil.ParseWebIncludeBoosts(c.Query(apiutil.WebIncludeBoostsKey), nil)
	if errWithCode != nil {
		apiutil.WebErrorHandler(c, errWithCode, instanceGet)
		return nil
	}

	// Get statuses from maxStatusID onwards (or from top if empty string).
	// The return boolean will indicate whether boosts were actually included.
	statusResp, errWithCode := m.processor.Account().WebStatusesGet(
		ctx,
		account.ID,
		&paging.Page{Max: paging.MaxID(maxStatusID), Limit: limit},
		mediaOnly,
		preferIncludeBoosts,
	)
	if errWithCode != nil {
		apiutil.WebErrorHandler(c, errWithCode, instanceGet)
		return nil
	}

	// Link to this page but with boosts explicitly excluded or with
	// the include_boosts param removed so default (true) is used.
	includeBoostsLink, excludeBoostsLink := includeExcludeBoostsLinks(c, statusResp)

	return &profile{
		instance:          instance,
		account:           account,
		rssFeed:           rssFeed,
		robotsMeta:        robotsMeta,
		pinnedStatuses:    pinnedStatuses,
		statusResp:        statusResp.PageableResponse,
		paging:            doPaging,
		includeBoostsLink: includeBoostsLink,
		excludeBoostsLink: excludeBoostsLink,
	}
}

func includeExcludeBoostsLinks(
	c *gin.Context,
	statusResp *account.WebStatusesGetResp,
) (
	includeBoostsLink string,
	excludeBoostsLink string,
) {
	// Only populate either of these links
	// if the account actually offers a
	// choice between boosts / no boosts.
	if !statusResp.AllowsIncludingBoosts {
		return
	}

	// Copy request URL
	// as basis of link.
	reqURL := c.Request.URL
	uri := &url.URL{
		Scheme: config.GetProtocol(),
		Host:   config.GetHost(),
		Path:   reqURL.Path,
	}

	if statusResp.IncludedBoosts {
		// Boosts were included, provide
		// a link that excludes them.
		const excludeBoosts = apiutil.WebIncludeBoostsKey + "=false"
		uri.RawQuery = reqURL.RawQuery + "&" + excludeBoosts
		excludeBoostsLink = uri.String()
	} else {
		// Boosts were not included, but they can be,
		// so provide a link to include them by just
		// removing include_boosts from the query.
		newQ := maps.Clone(reqURL.Query())
		newQ.Del(apiutil.WebIncludeBoostsKey)
		uri.RawQuery = newQ.Encode()
		includeBoostsLink = uri.String()
	}

	return
}

// profileGETHandler selects the appropriate rendering
// mode for the target account profile, and serves that.
func (m *Module) profileGETHandler(c *gin.Context) {
	p := m.prepareProfile(c)
	if p == nil {
		// Something went wrong,
		// error already written.
		return
	}

	// Choose desired web renderer for this acct.
	switch wrm := p.account.WebLayout; wrm {

	// El classico.
	case "", "microblog":
		m.profileMicroblog(c, p)

	// 'gram style media gallery.
	case "gallery":
		m.profileGallery(c, p)

	default:
		log.Panicf(
			c.Request.Context(),
			"unknown webrenderingmode %s", wrm,
		)
	}
}

// profileMicroblog serves the profile
// in classic GtS "microblog" view.
func (m *Module) profileMicroblog(c *gin.Context, p *profile) {
	// Prepare stylesheets for profile.
	stylesheets := make([]string, 0, 7)

	// Basic profile stylesheets.
	stylesheets = append(
		stylesheets,
		[]string{
			cssFA,
			cssStatus,
			cssThread,
			cssProfile,
		}...,
	)

	// If no posts are shown on the web,
	// show stats wide in single-column view.
	if p.account.WebVisibility == apimodel.VisibilityNone {
		stylesheets = append(stylesheets, cssProfileWideStats)
	}

	// User-selected theme if set.
	if theme := p.account.Theme; theme != "" {
		stylesheets = append(
			stylesheets,
			themesPathPrefix+"/"+theme,
		)
	}

	// Custom CSS for this user last in cascade.
	stylesheets = append(
		stylesheets,
		"/@"+p.account.Username+"/custom.css",
	)

	page := apiutil.WebPage{
		Template:    "profile.tmpl",
		Instance:    p.instance,
		OGMeta:      apiutil.OGAccount(p.instance, p.account),
		Stylesheets: stylesheets,
		Javascript: []apiutil.JavascriptEntry{
			frontendJavascript(),
			{
				Bottom: true,
				Src:    jsFrontendPrerender,
			},
		},
		Extra: map[string]any{
			"account":           p.account,
			"rssFeed":           p.rssFeed,
			"robotsMeta":        p.robotsMeta,
			"statuses":          p.statusResp.Items,
			"statuses_next":     p.statusResp.NextLink,
			"pinned_statuses":   p.pinnedStatuses,
			"show_back_to_top":  p.paging,
			"includeBoostsLink": p.includeBoostsLink,
			"excludeBoostsLink": p.excludeBoostsLink,
		},
	}

	apiutil.TemplateWebPage(c, page)
}

// profileMicroblog serves the profile
// in media-only 'gram-style gallery view.
func (m *Module) profileGallery(c *gin.Context, p *profile) {
	// Get just attachments from pinned,
	// making a rough guess for slice size.
	pinnedGalleryItems := make([]*apimodel.WebAttachment, 0, len(p.pinnedStatuses)*4)
	for _, status := range p.pinnedStatuses {
		pinnedGalleryItems = append(pinnedGalleryItems, status.MediaAttachments...)
	}

	// Get just attachments from statuses,
	// making a rough guess for slice size.
	galleryItems := make([]*apimodel.WebAttachment, 0, len(p.statusResp.Items)*4)
	for _, statusI := range p.statusResp.Items {
		status := statusI.(*apimodel.WebStatus)
		if status.Reblog != nil {
			// Take gallery items from reblogged status.
			galleryItems = append(galleryItems, status.Reblog.MediaAttachments...)
		} else {
			// Take gallery items from status itself.
			galleryItems = append(galleryItems, status.MediaAttachments...)
		}
	}

	// Prepare stylesheets for profile.
	stylesheets := make([]string, 0, 4)

	// Profile gallery stylesheets.
	stylesheets = append(
		stylesheets,
		[]string{
			cssFA,
			cssProfileGallery,
			// Show stats wide
			// in single-column.
			cssProfileWideStats,
		}...)

	// User-selected theme if set.
	if theme := p.account.Theme; theme != "" {
		stylesheets = append(
			stylesheets,
			themesPathPrefix+"/"+theme,
		)
	}

	// Custom CSS for this
	// user last in cascade.
	stylesheets = append(
		stylesheets,
		"/@"+p.account.Username+"/custom.css",
	)

	page := apiutil.WebPage{
		Template:    "profile-gallery.tmpl",
		Instance:    p.instance,
		OGMeta:      apiutil.OGAccount(p.instance, p.account),
		Stylesheets: stylesheets,
		Javascript: []apiutil.JavascriptEntry{
			frontendJavascript(),
			{
				Bottom: true,
				Src:    jsFrontendPrerender,
			},
		},
		Extra: map[string]any{
			"account":            p.account,
			"rssFeed":            p.rssFeed,
			"robotsMeta":         p.robotsMeta,
			"pinnedGalleryItems": pinnedGalleryItems,
			"galleryItems":       galleryItems,
			"statuses":           p.statusResp.Items,
			"statuses_next":      p.statusResp.NextLink,
			"pinned_statuses":    p.pinnedStatuses,
			"show_back_to_top":   p.paging,
			"includeBoostsLink":  p.includeBoostsLink,
			"excludeBoostsLink":  p.excludeBoostsLink,
		},
	}

	apiutil.TemplateWebPage(c, page)
}
