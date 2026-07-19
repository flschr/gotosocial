# GoToSocial Plus changelog

This changelog summarizes user-visible product features. Individual fixes,
visual refinements, and internal implementation changes remain available in
the Git history without becoming separate changelog entries.

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
[GitHub Releases page](https://github.com/flschr/gotosocial/releases).
