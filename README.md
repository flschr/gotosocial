# GoToSocial Plus

GoToSocial Plus is a small, independently maintained fork of
[GoToSocial](https://codeberg.org/superseriousbusiness/gotosocial). It stays
close to upstream and adds targeted, optional improvements for split-domain
account display, local role privacy, link preview cards, and remote follows
from public profiles. It also includes a fix for empty RSS feeds.

GoToSocial remains the project behind this software. Read the
[official documentation](https://docs.gotosocial.org/) for installation,
operation, federation, client use, and general configuration.

> [!IMPORTANT]
> GoToSocial Plus is not an official GoToSocial release. Report general
> GoToSocial problems to the upstream project and Plus-specific problems to
> this repository.

## Why this fork exists

The fork collects a small number of changes needed by
[social.fischr.org](https://social.fischr.org/) in a public, reproducible source
tree. Optional behavior can be managed individually under **Instance Settings
→ GoToSocial Plus**.

### Split-domain client compatibility

GoToSocial supports serving its API from a host such as `social.example.org`
while publishing accounts as `@user@example.org`. Follow the official
[split-domain deployment guide](https://docs.gotosocial.org/en/latest/advanced/host-account-domain/)
to configure the host, account domain, WebFinger, and redirects correctly.

That setup is necessary for federation, but it does not control how every
Mastodon client displays a local account. Some clients receive the local value
`user` from the Mastodon API and append the API host themselves, resulting in
`@user@social.example.org`.

When enabled, GoToSocial Plus returns `user@example.org` for local user accounts
and mentions in Mastodon API `acct` fields. It does not change WebFinger,
ActivityPub actor IDs, cryptographic keys, existing accounts, or federation
data.

This intentionally differs from the common Mastodon API convention that local
`acct` values contain only the username. It improves compatibility with the
observed clients, but cannot guarantee correct display in every client.

### Local role privacy

When enabled, public and blocked-account Mastodon API responses omit roles for
local users. An authenticated user continues to receive their own role and
permission information where GoToSocial needs it.

This reduces unnecessary public metadata without changing moderation powers or
stored account data.

### Link preview cards

When enabled, GoToSocial Plus creates preview cards for the first ordinary
HTTP(S) link in a non-sensitive post and exposes them through the Mastodon API
and GoToSocial's public web pages.

The implementation:

- skips mentions and hashtags;
- uses GoToSocial's SSRF-protected HTTP client;
- limits requests to four seconds and HTML responses to 1 MiB;
- caches successful results for 24 hours and unsuccessful results for one hour;
- avoids preview requests for sensitive posts;
- builds previews for older and federated posts in the background; and
- uses YouTube's small oEmbed response and `youtube-nocookie.com` for validated
  YouTube links.

External preview images are referenced rather than copied into local media
storage. A remote site can therefore still observe a visitor requesting its
image.

### Reliable RSS feeds

GoToSocial Plus includes a compatibility fix for the upstream bug where an
enabled account RSS feed can remain empty even though the account has public
posts. See
[GoToSocial issue #4664](https://codeberg.org/superseriousbusiness/gotosocial/issues/4664)
for the original report.

Unlike the optional Plus behavior, the RSS correction restores expected feed
behavior and is not an instance setting.

### Public profile experience

Three further settings control public profile pages:

- **Automatically load older posts** progressively loads the next page as the
  visitor approaches the end of the current one. It is disabled by default.
  The regular **Show older** link remains in the page as a fallback when
  JavaScript is unavailable or a request fails.
- **Show GoToSocial Plus version and source information** displays a small
  footnote after the profile content. It is enabled by default and can be hidden
  without removing the instance-wide source links required by the AGPL.
- **Show a remote Follow button** adds a prominent action to public profiles.
  On first use, visitors enter their Fediverse server and continue to its
  Mastodon-compatible `/authorize_interaction` flow. The server is remembered
  only in that browser so later follows need just the profile action and the
  confirmation on the visitor's server. Copying the account address remains a
  fallback for other Fediverse software.

Automatic loading does not rewrite browser history. Newly loaded posts are
inserted before the version footnote, so the footnote remains at the actual end
of the profile.

## Configuration

The optional features can be managed under **Instance Settings → GoToSocial
Plus**:

- **Use account domain in local API handles**
- **Hide local account roles**
- **Generate link preview cards**
- **Automatically load older posts on public profiles** (off by default)
- **Show GoToSocial Plus information on public profiles** (on by default)
- **Show a remote Follow button on public profiles** (off by default)

Existing installations can initially seed these settings with the following
configuration values:

```yaml
accounts-use-account-domain-in-acct: false
accounts-hide-local-roles: false
statuses-preview-cards: false
```

After the Plus settings migration has run, the values stored in
`instance_settings` are the source of truth and changes made in the settings UI
take effect immediately.

## Releases and installation

Ready-to-run builds are published on the
[GitHub Releases page](https://github.com/flschr/gotosocial/releases). A release
archive contains the GoToSocial binary, web assets, web templates, license,
README, and example configuration.

Always deploy the binary and web files from the same archive. Mixing a binary
with templates or compiled frontend assets from another release can leave
public pages unstyled or break the settings interface.

Before upgrading, follow the upstream release and migration notes and back up
the database, configuration, binary, web files, and media storage. Plus changes
are maintained as focused commits on top of an unmodified upstream release
branch so that every upgrade can be reviewed feature by feature.

GoToSocial Plus defaults new media thumbnails to a maximum dimension of 1024
pixels for sharper public-web presentation. An existing explicit
`media-thumb-max-pixels` configuration value continues to take precedence after
an upgrade, and existing thumbnails are not regenerated automatically.

## Source, upstream, and license

- GoToSocial Plus source: <https://github.com/flschr/gotosocial>
- Plus releases: <https://github.com/flschr/gotosocial/releases>
- Official GoToSocial source:
  <https://codeberg.org/superseriousbusiness/gotosocial>
- Official documentation: <https://docs.gotosocial.org/>

GoToSocial Plus is free software licensed under the
[GNU Affero General Public License v3 or later](LICENSE), unchanged from
upstream. Copyright for the original project remains with the GoToSocial
authors; Plus modifications are available in this repository's Git history.
