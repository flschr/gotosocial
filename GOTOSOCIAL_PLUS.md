# GoToSocial Plus

This repository contains a small compatibility layer on top of upstream
[GoToSocial](https://codeberg.org/superseriousbusiness/gotosocial).

## Branches

- `upstream-v0.22.0` is an unmodified source snapshot of upstream tag `v0.22.0`
  (upstream commit `60b693f9c76e5d1a6f2a4bc80a3d5ff02db5f5c5`).
- `plus/v0.22.0` adds the optional compatibility features used by `social.fischr.org`.

The snapshot branch deliberately contains no upstream Git history. The official
Codeberg repository remains configured as the local `upstream` remote and is the
source of truth for releases and history.

## Included features

GoToSocial Plus currently provides these product-level features:

- **Native Bluesky integration:** connect a Bluesky account, crosspost eligible
  public posts, and handle Bluesky replies from Mastodon clients.
- **Improved public profiles:** optional remote-follow actions, configurable
  profile ordering, automatic loading of older posts, and clearer Plus
  information.
- **Better media presentation:** uncropped profile images and a centered,
  accessible media viewer with image descriptions.
- **Rich link previews:** Mastodon-compatible cards for links, images, and
  supported privacy-friendly video embeds.
- **Split-domain compatibility:** consistent local handles in Mastodon clients
  when the public account domain differs from the server domain.
- **Privacy controls:** local administrator and moderator labels can remain
  private on public profiles.
- **Reliable publishing and feeds:** duplicate-safe publishing for compatible
  clients and dependable public RSS output.

The settings application shows this same concise feature catalog together with
the running Plus version. Bug fixes and internal refinements remain traceable in
Git history and release notes, but are not listed as separate product features.

## Patches

Instance-wide optional behavior is controlled by these configuration keys:

- `accounts-use-account-domain-in-acct`
- `accounts-hide-local-roles`
- `statuses-preview-cards`
- `profiles-auto-load-older-posts`
- `profiles-show-plus-info`
- `profiles-show-remote-follow`

### Split-domain handles in Mastodon API responses

For local, non-instance accounts, API `acct` values and mentions include the
configured `account-domain`. This lets Mastodon clients display
`@rene@fischr.org` instead of reconstructing `@rene@social.fischr.org` from the
API host. ActivityPub actor IDs, WebFinger, database records, and federation are
unchanged.

This intentionally differs from Mastodon's convention of returning only the
username for local accounts. The change is isolated in commit history so it can
be removed when clients or upstream handle split-domain accounts as desired.

### Mastodon-compatible link preview cards

For non-sensitive statuses, the API converter finds the first ordinary HTTP(S)
link, excluding mentions and hashtags, and fills the existing Mastodon `card`
field from Open Graph, Twitter Card, or standard HTML metadata.

Preview requests use GoToSocial's shared outgoing HTTP client, retaining its
SSRF protection and blocked-address rules. Fetches are fast-fail, limited to
four seconds and 1 MiB of HTML, and cached for 24 hours (failed previews for one
hour). Newly created local posts fetch synchronously so their create response
contains the card. Older and remote statuses warm the cache in the background,
preventing a timeline with several uncached links from blocking serially.
Sensitive statuses never trigger a preview request. Preview images are
referenced by their public URL and are not copied into instance storage.

The public GoToSocial web views render preview cards with lazily loaded images
and no referrer header. Enabling preview cards therefore allows visitors'
browsers to request HTTPS preview images from the linked sites. YouTube
watch, short, embed, and `youtu.be` links additionally receive a video card
with a privacy-enhanced `youtube-nocookie.com` player. The player URL is built
only from a validated YouTube video ID; arbitrary third-party embed HTML is
never rendered.

### Keep local account roles private

Public and blocked account representations omit local role information. The
authenticated account's sensitive representation still receives its role and
permission bitmap directly from its user record. This preserves the privacy
behavior of the previously deployed `account-domain-private-role` build while
keeping it isolated from the split-domain patch.

## Updating upstream

1. Read the upstream release and migration notes.
2. Fetch the new release tag from the Codeberg `upstream` remote.
3. Create a new unchanged `upstream-vX.Y.Z` snapshot branch from that tag.
4. Create `plus/vX.Y.Z` from the snapshot.
5. Cherry-pick each still-required patch commit separately and resolve changes
   against the new upstream implementation; never apply the old combined patch
   file blindly.
6. Let GitHub Actions test and package the Linux AMD64 build.
7. Back up the database, storage, configuration, current binary, and matching
   web assets before deploying the new archive.

The binary and the `web/assets` plus `web/template` directories must always be
deployed from the same build.

The frontend build must run `yarn ts-patch install` after installing packages
and before `yarn build`. The settings application uses Typia's TypeScript
transform and otherwise builds successfully but fails at runtime with an empty
settings page.

## License

The upstream project and these modifications are distributed under the GNU
Affero General Public License, version 3 or later. See `LICENSE`.
