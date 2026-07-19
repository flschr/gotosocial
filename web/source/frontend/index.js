/*
	GoToSocial
	Copyright (C) GoToSocial Authors admin@gotosocial.org
	SPDX-License-Identifier: AGPL-3.0-or-later

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU Affero General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU Affero General Public License for more details.

	You should have received a copy of the GNU Affero General Public License
	along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

/*
	WHAT SHOULD GO IN THIS FILE?

	This script is loaded in the document head, and deferred + async,
	so it's *usually* run after the user is already looking at the page.
	Put stuff in here that doesn't shift the layout, and it doesn't really
	matter whether it loads immediately. So, progressive enhancement stuff.
*/

const Photoswipe = require("photoswipe/dist/umd/photoswipe.umd.min.js");
const PhotoswipeLightbox = require("photoswipe/dist/umd/photoswipe-lightbox.umd.min.js");
const PhotoswipeCaptionPlugin = require("photoswipe-dynamic-caption-plugin").default;
const ObjectPosition = require("./photoswipe-object-position.js").default;
const Plyr = require("plyr");
const Prism = require("./prism.js");

Prism.manual = true;
Prism.highlightAll();

const remoteFollow = document.querySelector("[data-remote-follow]");
if (remoteFollow) {
	const storageKey = "gotosocial.remoteFollowServer";
	const openButton = remoteFollow.querySelector("[data-remote-follow-open]");
	const changeButton = remoteFollow.querySelector("[data-remote-follow-change]");
	const dialog = remoteFollow.querySelector("dialog");
	const form = remoteFollow.querySelector("[data-remote-follow-form]");
	const serverInput = remoteFollow.querySelector("[data-remote-follow-server]");
	const error = remoteFollow.querySelector("[data-remote-follow-error]");
	const copyButton = remoteFollow.querySelector("[data-remote-follow-copy]");
	const closeButton = remoteFollow.querySelector("[data-remote-follow-close]");
	let savedServer;

	const clearError = () => {
		error.hidden = true;
		serverInput.removeAttribute("aria-invalid");
	};

	const closeDialog = () => {
		clearError();
		dialog.close();
	};

	try {
		savedServer = window.localStorage.getItem(storageKey);
	} catch {
		// Storage can be unavailable in privacy-focused browser modes.
	}

	const normalizeServer = (input) => {
		let candidate = input.trim();
		if (candidate.startsWith("@") && candidate.lastIndexOf("@") > 0) {
			candidate = candidate.slice(candidate.lastIndexOf("@") + 1);
		}
		candidate = candidate.replace(/^https?:\/\//i, "").split("/")[0];
		if (!candidate || candidate.includes("@")) {
			return;
		}

		try {
			const url = new URL(`https://${candidate}`);
			if (!url.hostname || url.username || url.password) {
				return;
			}
			return url.host;
		} catch {
			return;
		}
	};

	const followOn = (server) => {
		const target = new URL(`https://${server}/authorize_interaction`);
		target.searchParams.set("uri", remoteFollow.dataset.profileUri);
		window.location.assign(target.href);
	};

	if (savedServer) {
		const normalized = normalizeServer(savedServer);
		if (normalized) {
			savedServer = normalized;
			changeButton.hidden = false;
		} else {
			savedServer = undefined;
		}
	}

	openButton.addEventListener("click", () => {
		if (savedServer) {
			followOn(savedServer);
			return;
		}
		dialog.showModal();
		serverInput.focus();
	});

	changeButton.addEventListener("click", () => {
		serverInput.value = savedServer || "";
		dialog.showModal();
		serverInput.select();
	});

	closeButton.addEventListener("click", closeDialog);
	dialog.addEventListener("close", clearError);
	dialog.addEventListener("click", (event) => {
		if (event.target === dialog) {
			closeDialog();
		}
	});

	form.addEventListener("submit", (event) => {
		event.preventDefault();
		const server = normalizeServer(serverInput.value);
		if (!server) {
			error.hidden = false;
			serverInput.setAttribute("aria-invalid", "true");
			serverInput.focus();
			return;
		}

		try {
			window.localStorage.setItem(storageKey, server);
		} catch {
			// Following still works when storage is unavailable.
		}
		followOn(server);
	});

	serverInput.addEventListener("input", () => {
		error.hidden = true;
		serverInput.removeAttribute("aria-invalid");
	});

	copyButton.addEventListener("click", async () => {
		try {
			await navigator.clipboard.writeText(remoteFollow.dataset.profileHandle);
			copyButton.textContent = "Copied!";
		} catch {
			copyButton.textContent = remoteFollow.dataset.profileHandle;
			copyButton.title = "Select and copy this address";
		}
	});
}

const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)');

let [_, _user, type, id] = window.location.pathname.split("/");
if (type == "statuses") {
	let firstStatus = document.getElementsByClassName("thread")[0].children[0];
	if (firstStatus.id != id) {
		document.getElementById(id).scrollIntoView();
	}
}

const lightbox = new PhotoswipeLightbox({
	gallery: '.photoswipe-gallery',
	children: '.photoswipe-slide',
	pswpModule: Photoswipe,
	// Bit darker than default 0.8.
	bgOpacity: 0.9,
	loop: false,
	paddingFn(viewportSize) {
		if (viewportSize.x < 600) {
			return { top: 64, right: 16, bottom: 80, left: 16 };
		}
		return { top: 72, right: 72, bottom: 96, left: 72 };
	},
});

new PhotoswipeCaptionPlugin(lightbox, {
	type: 'below',
	verticallyCenterImage: true,
	captionContent(slide) {
		return slide.data.alt;
	}
});

// Enable object-position plugin for lightbox so that css
// object-position property can be used on preview images.
new ObjectPosition(lightbox);

lightbox.addFilter('itemData', (item) => {
	const el = item.element;
	if (
		el &&
		el.classList.contains("plyr-video") &&
		el._plyrContainer !== undefined
	) {
		const parentNode = el._plyrContainer.parentNode;
		const loopingAuto = el.classList.contains("gifv");
		return {
			alt: el.getAttribute("alt"),
			_video: {
				open(c) {
					c.appendChild(el._plyrContainer);
					if (loopingAuto) {
						// Start playing
						// when opened.
						el._player.play();
					}
				},
				close() {
					parentNode.appendChild(el._plyrContainer);
				},
				pause() {
					el._player.pause();
				},
				play() {
					el._player.play();
				}
			},
			width: parseInt(el.dataset.pswpWidth),
			height: parseInt(el.dataset.pswpHeight),
			parentStatus: el.dataset.pswpParentStatus,
			attachmentId: el.dataset.pswpAttachmentId,
			loopingAuto: loopingAuto,
		};
	}
	return item;
});

// Open video when user moves to its slide.
lightbox.on("contentActivate", (e) => {
	const { content } = e;
	if (content.data._video != undefined) {
		content.data._video.open(content.element);
	}
});

// Pause + close video when user
// moves away from its slide.
lightbox.on("contentDeactivate", (e) => {
	const { content } = e;
	if (content.data._video != undefined) {
		content.data._video.pause();
		content.data._video.close();
	}
});

// Pause video when lightbox is closed.
lightbox.on("closingAnimationStart", function () {
	if (lightbox.pswp.currSlide.data._video != undefined) {
		lightbox.pswp.currSlide.data._video.close();
	}
});
lightbox.on("close", function () {
	if (lightbox.pswp.currSlide.data._video != undefined &&
		!lightbox.pswp.currSlide.data.loopingAuto) {
		lightbox.pswp.currSlide.data._video.pause();
	}
});

// Open video when lightbox is opened.
lightbox.on("openingAnimationEnd", function () {
	if (lightbox.pswp.currSlide.data._video != undefined) {
		lightbox.pswp.currSlide.data._video.play();
	}
});

// Add "open this post" link to lightbox UI.
lightbox.on('uiRegister', function() {
	lightbox.pswp.ui.registerElement({
		name: 'open-post-link',
		ariaLabel: 'Open post',
		order: 8,
		isButton: true,
		tagName: "a",
		html: '<span title="Open post"><span class="sr-only">Open post</span><i class="fa fa-lg fa-external-link-square" aria-hidden="true"></i></span>',
		onInit: (el, pswp) => {
			el.setAttribute('target', '_blank');
			el.setAttribute('rel', 'noopener');
			pswp.on('change', () => {
				switch (true) {
					case pswp.currSlide.data.parentStatus !== undefined:
						// Link to parent status.
						el.href = pswp.currSlide.data.parentStatus;
						break;
					case pswp.currSlide.data.element !== undefined &&
						pswp.currSlide.data.element.dataset.pswpParentStatus !== undefined:
						// Link to parent status.
						el.href = pswp.currSlide.data.element.dataset.pswpParentStatus;
						break;
					default:
						// Link to profile.
						const location = window.location; 	
						el.href = "//" + location.host + location.pathname;
				}
			});
		}
	});
});

lightbox.init();

const autoLoadSection = document.querySelector('[data-auto-load-older-posts="true"]');
if (autoLoadSection && "IntersectionObserver" in window) {
	let nextLink = autoLoadSection.querySelector(".backnextlinks .next");
	let loading = false;

	const observer = new IntersectionObserver(async (entries) => {
		if (loading || !nextLink || !entries.some((entry) => entry.isIntersecting)) {
			return;
		}

		loading = true;
		observer.unobserve(nextLink);
		autoLoadSection.setAttribute("aria-busy", "true");

		const originalText = nextLink.textContent;
		nextLink.textContent = "Loading older posts…";

		try {
			const response = await fetch(nextLink.href, {
				headers: { "Accept": "text/html" },
				credentials: "same-origin",
			});
			if (!response.ok) {
				throw new Error(`unexpected response ${response.status}`);
			}

			const page = new DOMParser().parseFromString(await response.text(), "text/html");
			const loadedSection = page.querySelector(".recent.statuses");
			const loadedThread = loadedSection?.querySelector(".thread");
			const loadedNextLink = loadedSection?.querySelector(".backnextlinks .next");
			const currentThread = autoLoadSection.querySelector(".thread");
			if (!loadedSection || !loadedThread || !currentThread) {
				throw new Error("older posts were missing from the response");
			}

			currentThread.append(...loadedThread.children);
			if (loadedNextLink) {
				nextLink.href = new URL(loadedNextLink.getAttribute("href"), response.url).href;
				nextLink.textContent = originalText;
				observer.observe(nextLink);
			} else {
				nextLink.remove();
				nextLink = null;
				observer.disconnect();
			}
		} catch {
			// Keep the regular link usable when automatic loading fails.
			nextLink.textContent = originalText;
			observer.disconnect();
		} finally {
			loading = false;
			autoLoadSection.removeAttribute("aria-busy");
		}
	}, { rootMargin: "0px 0px 320px 0px" });

	if (nextLink) {
		observer.observe(nextLink);
	}
}

Array.from(document.getElementsByClassName("plyr-video")).forEach((video) => {
	const loopingAuto = !reduceMotion.matches && video.classList.contains("gifv");
	let player = new Plyr(video, {
		title: video.title,
		settings: [],
		// Only show controls for video and audio,
		// not looping soundless gifv. Don't show
		// volume slider as it's unusable anyway
		// when the video is inside a lightbox,
		// mute toggle will have to be enough.
		controls: loopingAuto
			? []
			: [
				'play-large',   // The large play button in the center
				'restart',      // Restart playback
				'rewind',       // Rewind by the seek time (default 10 seconds)
				'play',         // Play/pause playback
				'fast-forward', // Fast forward by the seek time (default 10 seconds)
				'current-time', // The current time of playback
				'duration',     // The full duration of the media
				'mute',         // Toggle mute
				'fullscreen',   // Toggle fullscreen
			],
		tooltips: { controls: true, seek: true },
		iconUrl: "/assets/plyr.svg",
		invertTime: false,
		hideControls: false,
		listeners: {
			play: (_) => {
				if (!inLightbox(video)) {
					// If the video isn't open in the lightbox
					// as the current photoswipe slide, clicking
					// on it to play it opens it in the lightbox.
					lightbox.loadAndOpen(parseInt(video.dataset.pswpIndex), {
						gallery: video.closest(".photoswipe-gallery")
					});
				} else if (!loopingAuto) {
					// If the video *is* open in the lightbox,
					// and it's not a looping gifv, clicking
					// play just plays or pauses the video.
					player.togglePlay();
				}
				return false;
			},
		}
	});

	player.elements.container.title = video.title;
	video._player = player;
	video._plyrContainer = player.elements.container;
});

// Return true if the photoswipe lightbox is
// open with this element as the current slide.
function inLightbox(element) {
	if (lightbox.pswp === undefined) {
		return false;
	}

	if (lightbox.pswp.currSlide === undefined) {
		return false;
	}

	return element.dataset.pswpAttachmentId ===
		lightbox.pswp.currSlide.data.attachmentId;
}

// When clicking anywhere that's not an open
// stats-info-more-content details dropdown,
// close that open dropdown.
document.body.addEventListener("click", (e) => {
	const openStats = document.querySelector("details.stats-more-info[open]");
	if (!openStats) {
		// No open stats
		// details element.
		return;
	}

	if (openStats.contains(e.target)) {
		// Click is within stats
		// element, leave it alone.
		return;
	}

	// Click was outside of
	// stats elements, close it. 
	openStats.removeAttribute("open");
});

// Scan for the first ListenBrainz profile field and replace
// its value with currently listening track if available.
//
// ListenBrainz allows a lot of leeway in usernames so be gentle here:
//
// See:
//
// - https://github.com/metabrainz/musicbrainz-server/blob/master/lib/MusicBrainz/Server/Form/Utils.pm#L264-L288
// - https://regex101.com/r/k5ij9F/1
const listenbrainzRe = new RegExp(/^https:\/\/listenbrainz\.org\/user\/([^/]+)\/$/, "u");
let calledListenBrainz = false;
document.querySelectorAll("div#profile-fields dl div.field").forEach((field) => {
	// If we called ListenBrainz once
	// already this page load, bail.
	if (calledListenBrainz) {
		return;
	}

	const k = field.querySelector("dt");
	if (!k) {
		// No <dt> inside this
		// field? Weird but OK.
		return;
	}

	const kText = k.textContent;
	if (kText === null) {
		// Also strange but
		// let's just bail.
		return;
	}
	
	// Check if key == "ListenBrainz" (case insensitive).
	if (kText.localeCompare("ListenBrainz", undefined, { sensitivity: "base" }) !== 0) {
		// Not interested.
		return;
	}

	// Get the value.
	const v = field.querySelector("dd");
	if (!v) {
		// No <dd> inside this
		// field? Weird but OK.
		return;
	}

	// Look for an <a> tag inside the <dd>.
	const oldAs = v.getElementsByTagName("a");
	if (oldAs.length !== 1) {
		// Nothing
		// in here.
		return;
	}

	const oldA = oldAs[0];
	const profileURL = oldA.textContent;
	if (!profileURL) {
		// Also strange but
		// let's just bail.
		return;
	}

	// We're looking for a listenbrainz URL.
	const match = profileURL.match(listenbrainzRe);
	if (match.length !== 2) {
		// Not a match.
		return;
	}
	const lbUsername = match[1];
	
	try {
		// MusicBrainz/ListenBrainz is very permissive
		// re: usernames so make sure to encode the URI
		// when doing the fetch, to avoid any shenanigans.
		const apiURL = encodeURI(`https://api.listenbrainz.org/1/user/${lbUsername}/playing-now`);
		fetch(apiURL).then(res => {
			// Mark that we
			// called LB already.
			calledListenBrainz = true;
			
			// Check result...
			if (!res.ok) {
				throw new Error(`Response status: ${res.status}`);
			}
			
			return res.json();
		}).then(json => {
			// Parse out the object.
			const payload = json.payload;
			if (!payload) {
				// Can't do anything
				// with no payload.
				return;
			}

			const listens = payload.listens;
			if (!listens || !Array.isArray(listens) || listens.length !== 1) {
				// Can't do anything
				// with no listens.
				return;
			}

			const listen = listens[0];
			const trackMetadata = listen.track_metadata;
			if (!trackMetadata) {
				// Can't do anything
				// with no track metadata.
				return;
			}

			const artistName = trackMetadata.artist_name;
			const trackName = trackMetadata.track_name;
			if (artistName === undefined || trackName === undefined) {
				// Can't display
				// this track.
				return;
			}

			// We can work with this.
			//
			// Rewrite the existing <dd> with the
			// current listening song, and keep the
			// link to the user's ListenBrainz profile.
			const vNew = document.createElement("dd");
			
			// Lil music note icon.
			const i = document.createElement("i");
			i.ariaHidden = "true";
			i.className = "fa fa-fw fa-music";
			vNew.appendChild(i);
			
			vNew.appendChild(document.createTextNode(" Now listening to: "));
			vNew.appendChild(document.createElement("br"));

			// Build the new link, taking
			// the href from the old link.
			const a = document.createElement("a");
			a.href = oldA.href;
			a.rel = "nofollow noreferrer noopener";
			a.target = "_blank";

			// Add track name in bold.
			const trackNameE = document.createElement("b");
			trackNameE.textContent = trackName;
			a.appendChild(trackNameE);

			// Add joiner in normal font.
			a.appendChild(document.createTextNode(" by "));
			
			// Add artist name in bold.
			const artistNameE = document.createElement("b");
			artistNameE.textContent = artistName;
			a.appendChild(artistNameE);

			// Put the link
			// in the definish.
			vNew.appendChild(a);

			// Do the replacement.
			field.replaceChild(vNew, v);
		});
	} catch (error) {
		// eslint-disable-next-line no-console
		console.error(error.message);
	}
});

// Scan for the first Träwelling profile field and replace
// its value with the currently boarded service, if available.
//
// Träwelling usernames seem to follow a similar structure to
// how Twitter did it back in the day (considering that Träwelling
// started out as a platform linked to Twitter and building on top
// of it), so parsing usernames this way should be fine…?
// (feel free to correct me on any of this, by the way!!)
//
// See:
//
// - https://traewelling.de/api/documentation
const traewellingRe = new RegExp(/^https:\/\/traewelling\.de\/@([^/]+)$/, "u");
let calledTraewelling = false;
document.querySelectorAll("div#profile-fields dl div.field").forEach((field) => {
	// If we called Träwelling once
	// already this page load, bail.
	if (calledTraewelling) {
		return;
	}

	const k = field.querySelector("dt");
	if (!k) {
		// No <dt> inside this
		// field? Weird but OK.
		return;
	}

	const kText = k.textContent;
	if (kText === null) {
		// Also strange but
		// let's just bail.
		return;
	}
	
	// Check if key == "Träwelling" or == "Traewelling" (case insensitive).
	// TODO: add handling of "Traewelling"
	if (kText.localeCompare("Träwelling", undefined, { sensitivity: "base" }) !== 0) {
		// Not interested.
		return;
	}

	// Get the value.
	const v = field.querySelector("dd");
	if (!v) {
		// No <dd> inside this
		// field? Weird but OK.
		return;
	}

	// Look for an <a> tag inside the <dd>.
	const oldAs = v.getElementsByTagName("a");
	if (oldAs.length !== 1) {
		// Nothing
		// in here.
		return;
	}

	const oldA = oldAs[0];
	const profileURL = oldA.textContent;
	if (!profileURL) {
		// Also strange but
		// let's just bail.
		return;
	}

	// We're looking for a Träwelling URL.
	const match = profileURL.match(traewellingRe);
	if (match.length !== 2) {
		// Not a match.
		return;
	}
	const trUsername = match[1];
	
	try {
		const apiURL = encodeURI(`https://traewelling.de/api/v1/user/${trUsername}/statuses`);
		fetch(apiURL).then(res => {
			// Mark that we
			// called Träwelling already.
			calledTraewelling = true;
			
			// Check result...
			if (!res.ok) {
				throw new Error(`Response status: ${res.status}`);
			}
			return res.json();
		}).then(json => {
			let foundCurrentlyValidJourney = false;
			let currentlyProcessedJourney = 0;
			while(currentlyProcessedJourney < 15) {
				// If we already found a valid journey at some point down the code,
				// break out of the while loop here.
				if(foundCurrentlyValidJourney === true) {break;}
				// Parse out the object.
				const payload = json.data[currentlyProcessedJourney];
				currentlyProcessedJourney++;
				if (!payload) {
					// Can't do anything
					// with no payload.
					break;
				}

				const currentJourney = payload.train;
				if (!currentJourney) {
					// Can't do anything
					// with no journeys.
					break;
				}

				const lineName = currentJourney.lineName;
				if (!lineName) {
					// Can't do anything
					// with no line name, really.
					continue;
				}

				// this will be useful at some later point :3
				const trainType = currentJourney.category;

				const origin = currentJourney.origin.name;
				const destination = currentJourney.destination.name;
				if (origin === undefined || destination === undefined) {
					// Can't display this train.
					continue;
				}

				// TODO: We're pulling in the origin's departure time and station name,
				//       but we still don't do anything with it (yet).
				const realDeparture = currentJourney.origin.departureReal;
				const scheduledDeparture = currentJourney.origin.departurePlanned;
				const manualDeparture = currentJourney.manualDeparture;
				const realArrival = currentJourney.destination.arrivalReal;
				const scheduledArrival = currentJourney.destination.arrivalPlanned;
				const manualArrival = currentJourney.manualArrival;
				if(payload.event) { var eventHashtag = payload.event.hashtag; }
				let departure, arrival;
				let /*departureType,*/ arrivalType;
				
				// Träwelling implements the priority of time info this way:
				// manual > real-time > scheduled.

				// Check if there *isn't* a manual departure time…
				if (manualDeparture === null) {
					// …and if there isn't even a real-time departure time…
					if(realDeparture === null) {
						// …then we'll take the scheduled departure.
						departure = scheduledDeparture;
						// departureType = "scheduled";
					} else {
						// else we'll use the real-time departure.
						departure = realDeparture;
						// departureType = "realtime";
					}
				} else {
					// though ideally we'd like to use the manually entered departure time.
					departure = manualDeparture;
					// departureType = "manual";
				}
				

				// Same logic as above, but find-and-replace departure with arrival.
				if (manualArrival === null) {
					if(realArrival === null) {
						arrival = scheduledArrival;
						arrivalType = "scheduled";
					} else {
						arrival = realArrival;
						arrivalType = "realtime";
					}
				} else {
					arrival = manualArrival;
					arrivalType = "manual";
				}

				// Check if the browser's local time is within
				// the specified (most recent) check-in.
				const currentTime = new Date();

				if(currentTime < new Date(departure)) {
					// Can't work with check-ins before the departure time.
					continue;
				}

				if(currentTime > new Date(arrival)) {
					// Can't work with check-ins past the arrival time.
					continue;
				}

				// Time to do some date() shenanigans. :3
				// (yes, the pun was intended.)
				//
				// TODO: clean up this mess lmfao
				const formattedArrival = new Intl.DateTimeFormat(undefined, {hour: '2-digit', minute: '2-digit', hour12: false}).format(new Date(arrival));
				const formattedScheduledArrival = new Intl.DateTimeFormat(undefined, {hour: '2-digit', minute: '2-digit', hour12: false}).format(new Date(currentJourney.destination.arrivalPlanned));
				//const formattedDeparture = new Intl.DateTimeFormat(undefined, {hour: '2-digit', minute: '2-digit', hour12: false}).format(new Date(departure));
				//const formattedScheduledDeparture = new Intl.DateTimeFormat(undefined, {hour: '2-digit', minute: '2-digit', hour12: false}).format(new Date(currentJourney.destination.departurePlanned));

				// We can actually work with this now.
				//
				// Rewrite the existing <dd> with the
				// currently boarded train, and keep the
				// link to the user's Träwelling profile.
				const vNew = document.createElement("dd");
				
				// cute little public transit icons!
				const i = document.createElement("i");
				i.ariaHidden = "true";
				// see, I said it's going to be useful later!
				switch (trainType) {
					case "regional":         // RE, RB
					case "regionalExp":      // IC
					case "tram":             // trams (hopefully self-explanatory)
					case "nationalExpress":  // FlixTrain (so far)
					case "suburban":         // S-Bahn
						i.className = "fa fa-fw fa-train";
						break;
					case "subway":           // U-Bahn
						i.className = "fa fa-fw fa-subway";
						break;
					case "ferry":            // self-explanatory
						i.className = "fa fa-fw fa-ship";
						break;
					case "bus":              // self-explanatory
						i.className = "fa fa-fw fa-bus";
						break;
					default:                 // for when there's no good match!
						i.className = "fa fa-fw fa-train";
				}
				vNew.appendChild(i);

				// Add some whitespace between the icon and the following text.
				vNew.appendChild(document.createTextNode(" "));

				// Build a new link, taking the href from the old link.
				const a = document.createElement("a");
				a.href = oldA.href;
				a.rel = "nofollow noreferrer noopener";
				a.target = "_blank";

				// Add user's display name to the original link in bold.
				const displayNameE = document.createElement("b");
				displayNameE.textContent = `${payload.userDetails.displayName}`;
				a.appendChild(displayNameE);
				vNew.appendChild(a);

				// probably added the träwelling user's display name in the jankiest way possible
				vNew.appendChild(document.createTextNode(" is currently on the "));

				// Build a brand-new link, taking
				// the href from the check-in ID.
				const aStatus = document.createElement("a");
				aStatus.href = `https://traewelling.de/status/${payload.id}`;
				aStatus.rel = "nofollow noreferrer noopener";
				aStatus.target = "_blank";

				// Add train name in bold.
				const trainNameE = document.createElement("b");
				trainNameE.textContent = lineName;
				aStatus.appendChild(trainNameE);

				// Add joiner in normal font.
				aStatus.appendChild(document.createTextNode(" service to "));
				
				// Add destination name in bold.
				const destinationNameE = document.createElement("b");
				destinationNameE.textContent = destination;
				aStatus.appendChild(destinationNameE);

				// Add event-related hashtag (if available), in normal font.
				if(eventHashtag !== null && eventHashtag !== undefined) {
					aStatus.appendChild(document.createTextNode(` for \#${eventHashtag}`));
				}

				// Add yet another joiner in normal font.
				// Here, we're checking if it's a manual/scheduled/real-time arrival.
				switch(arrivalType) {
					case "manual":   // This is for when it's an arrival time has been manually entered by the user.
						aStatus.appendChild(document.createTextNode(", manually told to arrive at "));
						break;
					case "realtime": // This is for when it's an arrival time with real-time info.
						aStatus.appendChild(document.createTextNode(", arriving at "));
						break;
					case "scheduled": // And this is for when it's an arrival time only with scheduled info.
						aStatus.appendChild(document.createTextNode(", scheduled to arrive at "));
						break;
				}
				
				// Add arrival time in bold.
				// TODO: consider adding the difference between scheduled and real times?
				//       (that is, if there's real-time data available.)
				const arrivalTimeE = document.createElement("b");
				arrivalTimeE.textContent = formattedArrival;
				aStatus.appendChild(arrivalTimeE);
				// If there's anything other than scheduled info available, *and*
				// if the scheduled info isn't the same as the other info, display
				// the scheduled arrival time as well, but in a separate element.
				if(arrivalType !== "scheduled" && formattedArrival !== formattedScheduledArrival) {
					const scheduledArrivalTimeE = document.createElement("i");
					scheduledArrivalTimeE.textContent = ` (scheduled arrival at ${formattedScheduledArrival})`;
					aStatus.appendChild(scheduledArrivalTimeE);
				}

				// Put the link
				// in the definish.
				vNew.appendChild(aStatus);

				// Do the replacement.
				field.replaceChild(vNew, v);

				// Tell the variable that we found a valid journey.
				foundCurrentlyValidJourney = true;
			}
		});
	} catch (error) {
		// eslint-disable-next-line no-console
		console.error(error.message);
	}
});
