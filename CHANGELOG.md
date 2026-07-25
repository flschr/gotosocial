# GoToSocial Plus changelog

This changelog summarizes user-visible product features. Individual fixes,
visual refinements, and internal implementation changes remain available in
the Git history without becoming separate changelog entries.

## Unreleased

### Reliable Mastodon quote cleanup

- Remove Mastodon's redundant leading `RE:` quote fallback whenever the
  structured `quote-inline` marker is present, including quotes without a
  matching classic preview card.
- Preserve ordinary links and user-authored `RE:` paragraphs.

## v0.22.1-plus.4 — 2026-07-25

### Reliable Bluesky author avatars

- Keep locally cached avatars for private Bluesky interaction authors reachable
  during regular media cleanup.
- Detect and replace stale local avatar references during reconciliation.
- Preserve the native-looking Bluesky reply presentation introduced in Plus 3
  without leaving broken images after cleanup.

### Better Micro.blog reply threading

- Recover missing reply relationships for Micro.blog-hosted replies from the
  public `u-in-reply-to` microformat when ActivityPub omits `inReplyTo`.
- Reuse an existing local parent status immediately so Mastodon-compatible
  clients receive normal parent, account, and thread identifiers.
- Keep the compatibility fallback restricted to `micro.blog`; retrieval or
  parsing failures remain non-fatal.

### Clear development version labels

- Identify development builds as `plus.4-dev` so public version information
  continues to show the Plus release line they are testing.
- Extend the release build checks to cover the Bluesky, media-cleaner,
  database, and federation packages touched by these fixes.

### Upgrade notes

- This release remains based on GoToSocial 0.22.1.
- No database migration or new configuration is required.
- Deploy the binary, web assets, and templates from the same release archive.

## v0.22.1-plus.3 — 2026-07-24

### More complete remote conversations

- Refresh the available ActivityPub context when opening a thread so missing
  ancestors and remote replies are recovered more reliably.
- Preserve normal federation, visibility, and authorization rules while
  loading additional context.

### Native-looking Bluesky replies

- Present imported Bluesky replies with the author's display name, handle,
  locally cached avatar, and a compact Bluesky origin marker.
- Use private virtual API identities that are not federated, discoverable, or
  indexable.
- Remove the repeated author byline and large Bluesky preview card while
  retaining the normal **View reply on Bluesky** link.
- Refresh connected clients when an imported Bluesky author's handle, name, or
  avatar changes.

### Cleaner identity and link presentation

- Add an optional instance setting that removes Unicode and custom emojis from
  account display names returned through the Mastodon API.
- Keep handles, posts, bios, profile fields, emoji metadata, and stored or
  federated profile data unchanged.
- Preserve the system-owned Bluesky marker when display-name emojis are
  hidden.
- Suppress preview cards for profile links that exist only because of a
  structured mention, including affected micro.blog replies.

### Cleaner Mastodon quote posts

- Add an optional setting that removes Mastodon's redundant leading `RE:` link
  when it exactly matches the visible quote preview card.
- Preserve ordinary links, user-authored `RE:` text, unmatched targets, and
  quote fallbacks without a card.
- Persist the setting across restarts and expose it consistently through the
  settings application and instance API.

### Upgrade notes

- This release remains based on GoToSocial 0.22.1.
- Database migrations run automatically on startup.
- The new display-name and quote-fallback options default to disabled.
- Deploy the binary, web assets, and templates from the same release archive.
- No breaking configuration or federation changes are expected.

## v0.22.1-plus.2 — 2026-07-20

### GoToSocial 0.22.1 bugfixes

- Restore outgoing HTTP proxy configuration that was accidentally removed.
- Fix voting availability for expired remote polls and the closed-poll
  detection heuristic.
- Prevent old statuses from producing new-status notifications when they are
  added to timelines.
- Fix media cleanup for attachments detached from deleted local statuses.
- Improve undeliverable-instance handling for PostgreSQL installations.
- Hide the public directory link when the directory is disabled.
- Update bundled dependencies, including SQLite.

### Public profile follow actions

- Always open the remote-follow dialog from the Mastodon follow button so a
  previously saved server remains visible and can be changed.
- Keep Mastodon and Bluesky follow actions visually consistent on desktop and
  mobile.

## v0.22.0-plus.1 — 2026-07-19

### Native Bluesky integration

- Connect an existing Bluesky account per GoToSocial user.
- Crosspost eligible public posts with text, links, images, and alt text.
- Receive Bluesky replies and mentions in Mastodon-compatible clients and
  reply from those clients as the connected Bluesky account.
- Keep mapped Bluesky posts synchronized through their deletion lifecycle;
  edits are sent to Bluesky but may not appear in Bluesky clients.
- Optionally show a Bluesky follow action on the public profile.

### Improved public profiles and media

- Choose whether the profile bio or profile links appear first.
- Optionally load older posts automatically while preserving the normal
  **Show older** fallback link.
- Optionally send profile visitors back to their own Mastodon-compatible
  Fediverse server to confirm a follow.
- Present single images proportionally without forced cropping.
- Use sharper thumbnails for newly processed media by default.
- Open media in a centered, accessible lightbox with the image description
  directly below the image.
- Keep public follow actions clear and consistent across desktop and mobile.

### GoToSocial Plus settings center

- Collect Plus features in a dedicated settings section with separate pages
  for the feature overview, Bluesky, personal profile options, and
  administrator-only instance controls.
- Show the running Plus version and a concise catalog of included features.

### Reliable publishing

- Support Mastodon-compatible idempotency keys when creating posts so clients
  and publishers can safely retry without creating duplicate posts or
  crossposts.

### Compatibility, privacy, and feeds

- Improve split-domain handle display in Mastodon clients.
- Keep local administrator and moderator roles private on public profiles.
- Add Mastodon-compatible link previews and supported privacy-friendly video
  embeds.
- Restore reliable public RSS feeds.
- Optionally show the running Plus version and source on public profiles.

Detailed release artifacts and checksums remain available on the
[GitHub Releases page](https://github.com/flschr/gotosocial-plus/releases).
