# GoToSocial Plus feature guide

GoToSocial Plus is a focused compatibility layer maintained on top of
upstream [GoToSocial](https://codeberg.org/superseriousbusiness/gotosocial).
This guide describes behavior added by the fork. The
[official documentation](https://docs.gotosocial.org/) remains the source of
truth for general GoToSocial installation and operation.

## Settings

Plus features are collected in the **GoToSocial Plus** section of the settings
application. Bluesky settings belong to individual accounts; instance
features require an administrator.

These configuration keys can seed optional behavior when instance settings
are created:

- `accounts-use-account-domain-in-acct`
- `accounts-hide-local-roles`
- `accounts-hide-name-emojis`
- `statuses-preview-cards`
- `statuses-hide-quote-fallback`

After the Plus settings migration has run, values stored in
`instance_settings` are authoritative. Changes made in the settings
application take effect immediately and persist across restarts. Public-profile
controls are managed directly in the settings application.

## Native Bluesky integration

Each user can optionally connect an existing Bluesky account. GoToSocial Plus
can then publish eligible original public posts to Bluesky with text, links,
up to four images, and available alt text.

Replies, mentions, boosts, polls, and non-public posts are not crossposted
automatically. Existing mapped posts keep their lifecycle when automatic
crossposting is later disabled. Deleting a mapped GoToSocial post also deletes
its Bluesky crosspost. Edits are sent to Bluesky, although Bluesky clients may
continue showing an earlier version.

Bluesky replies and mentions are imported into GoToSocial. They can be read
and answered from Mastodon-compatible clients while retaining the correct
Bluesky conversation context. Supported images, animated GIFs, and videos are
stored as local media attachments with available descriptions.

Imported replies use a private virtual API identity with:

- the Bluesky display name and handle;
- a locally cached avatar;
- a small Bluesky origin marker; and
- no large Bluesky link-preview card.

These identities are presentation objects for the owner of the private
interaction. They are not ActivityPub actors, are not federated, cannot be
discovered, and are marked as non-indexable.

The public GoToSocial profile can optionally show a separate Bluesky follow
action. Connection, automatic crossposting, and the public follow action are
managed under **GoToSocial Plus → Bluesky**.

## Complete remote conversations

When a user opens a status thread, GoToSocial Plus refreshes the available
ActivityPub conversation context. This can recover older ancestors and remote
replies that were not present in the local database when the original status
arrived.

Micro.blog-hosted replies may omit the ActivityPub `inReplyTo` property while
still publishing the parent relationship as `u-in-reply-to` on the public
status page. For status URLs on `micro.blog`, Plus uses that public
microformat as a compatibility fallback so available replies retain their
normal parent and thread relationship.

The refresh uses normal federation and visibility rules. It does not provide
access to private or otherwise unavailable posts, and a remote server can
still return an incomplete conversation.

## Link preview cards

When enabled, GoToSocial Plus creates a Mastodon-compatible preview card for
the first ordinary HTTP(S) link in a non-sensitive post.

Preview handling:

- skips hashtags and structured mentions;
- also skips profile URLs that correspond to a status mention, even when the
  remote HTML omitted the usual `mention` class;
- uses GoToSocial's SSRF-protected outgoing HTTP client;
- limits a request to four seconds and HTML to 1 MiB;
- caches successful results for 24 hours and misses for one hour;
- warms previews for older and remote posts in the background; and
- uses validated YouTube metadata with a `youtube-nocookie.com` player.

Sensitive posts never trigger preview requests. External preview images are
referenced rather than copied into local storage, so the image host can still
observe a visitor requesting the image.

## Cleaner Mastodon quote posts

Mastodon adds a leading `RE:` link to native quote posts for clients that do
not support quotes. When link previews are enabled, clients can otherwise show
this fallback directly above a card for the same target.

The optional quote-fallback setting removes only a leading Mastodon
`quote-inline` paragraph whose URL exactly matches the visible preview card.

Quote permissions for public and unlisted posts are controlled per account
under **User Settings → Posts → Default Interaction Policies → Quote**.
Followers-only posts remain quoteable only by their author. These are standard
interaction-policy settings rather than a separate Plus configuration flag.
The pending interaction requests page can also be filtered to include or
exclude quotes.
Normal links, user-authored `RE:` text, unmatched targets, and fallbacks
without a card remain unchanged.

## Display-name emoji control

The optional display-name setting removes Unicode emoji sequences and known
custom-emoji shortcodes from account display names returned through the
Mastodon API.

It is a reversible presentation setting:

- account handles and usernames remain unchanged;
- posts, profile bios, profile fields, and ordinary text remain unchanged;
- stored and federated profile data remains unchanged; and
- custom-emoji metadata remains available for content outside the display
  name.

The system-owned Bluesky marker is preserved because it identifies the origin
of an imported reply rather than decorating a federated profile.

## Split-domain compatibility

GoToSocial supports serving its API from a host such as
`social.example.org` while publishing accounts as `@user@example.org`.
Administrators must still follow the official
[split-domain guide](https://docs.gotosocial.org/en/latest/advanced/host-account-domain/)
for WebFinger, redirects, and federation.

Some Mastodon clients reconstruct local handles from the API host. When the
Plus compatibility option is enabled, local account and mention `acct` values
include the configured account domain. ActivityPub actor IDs, WebFinger,
cryptographic keys, database identities, and federation data remain
unchanged.

This intentionally differs from the common Mastodon API convention that a
local `acct` contains only the username. It improves compatibility with
observed clients but cannot control every client's presentation.

## Local role privacy

When enabled, public and blocked-account API responses omit administrator and
moderator roles for local users. Authenticated users still receive their own
role and permission information where GoToSocial requires it. Moderation
powers and stored user data are unchanged.

## Public profiles and media

Instance and account settings provide:

- optional remote-follow actions for Mastodon-compatible visitors;
- configurable ordering of profile information;
- optional automatic loading of older posts with the normal **Show older**
  link retained as a fallback;
- optional Plus version and source information on profiles;
- proportional single-image presentation;
- sharper thumbnails for newly processed media; and
- a centered, accessible media viewer with image descriptions.

An existing explicit `media-thumb-max-pixels` value continues to take
precedence. Existing thumbnails are not regenerated automatically.

## Reliable publishing and feeds

GoToSocial Plus supports Mastodon-compatible idempotency keys for status
creation, allowing clients and publishers to retry without creating duplicate
posts or Bluesky crossposts.

It also includes a compatibility correction for public RSS feeds that could
remain empty despite containing public posts. This restores expected feed
behavior and is not an optional setting.

## Updating and building

Plus changes are maintained as focused commits on top of an unmodified
upstream release line. When updating upstream:

1. read the upstream release and migration notes;
2. create or update the unmodified upstream snapshot branch;
3. apply each still-required Plus change separately;
4. run backend, migration, and frontend tests;
5. build a complete release archive; and
6. back up the database, storage, configuration, binary, assets, and templates
   before deployment.

The frontend build must run `yarn ts-patch install` after installing packages
and before `yarn build`. Always deploy the binary, `web/assets`, and
`web/template` from the same archive.
