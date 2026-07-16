# René's GoToSocial build

This repository contains a small compatibility layer on top of upstream
[GoToSocial](https://codeberg.org/superseriousbusiness/gotosocial).

## Branches

- `upstream-v0.22.0` is an unmodified source snapshot of upstream tag `v0.22.0`
  (upstream commit `60b693f9c76e5d1a6f2a4bc80a3d5ff02db5f5c5`).
- `rene/v0.22.0` adds the patches used by `social.fischr.org`.

The snapshot branch deliberately contains no upstream Git history. The official
Codeberg repository remains configured as the local `upstream` remote and is the
source of truth for releases and history.

## Patches

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

## Updating upstream

1. Read the upstream release and migration notes.
2. Fetch the new release tag from the Codeberg `upstream` remote.
3. Create a new unchanged `upstream-vX.Y.Z` snapshot branch from that tag.
4. Create `rene/vX.Y.Z` from the snapshot.
5. Cherry-pick each still-required patch commit separately and resolve changes
   against the new upstream implementation; never apply the old combined patch
   file blindly.
6. Let GitHub Actions test and package the Linux AMD64 build.
7. Back up the database, storage, configuration, current binary, and matching
   web assets before deploying the new archive.

The binary and the `web/assets` plus `web/template` directories must always be
deployed from the same build.

## License

The upstream project and these modifications are distributed under the GNU
Affero General Public License, version 3 or later. See `LICENSE`.
