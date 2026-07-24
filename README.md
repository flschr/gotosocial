# GoToSocial Plus

GoToSocial Plus is a small, independently maintained fork of
[GoToSocial](https://codeberg.org/superseriousbusiness/gotosocial). It stays
close to upstream while adding a focused set of compatibility, privacy, and
publishing improvements used by
[social.fischr.org](https://social.fischr.org/).

> [!IMPORTANT]
> GoToSocial Plus is not an official GoToSocial release. Use the
> [official documentation](https://docs.gotosocial.org/) for general
> installation, operation, federation, and client guidance.

## Included features

- **Native Bluesky integration:** connect an existing Bluesky account,
  crosspost eligible public posts, receive replies and mentions, and answer
  them from Mastodon-compatible clients.
- **Complete remote conversations:** refresh available ActivityPub context
  when opening a thread so older and remote replies appear more reliably.
- **Cleaner timelines:** optional link previews, redundant quote-fallback
  removal, compact Bluesky replies, and suppression of profile cards created
  only by mentions.
- **Identity and privacy controls:** improve split-domain handles, hide local
  roles, and optionally remove profile-supplied emojis from display names.
- **Improved public profiles:** remote follow actions, configurable profile
  presentation, and automatic loading of older posts.
- **Better media presentation:** proportional profile images, sharper
  thumbnails, and an accessible media viewer with descriptions.
- **Reliable publishing and feeds:** duplicate-safe post creation and
  dependable public RSS output.

See the [Plus feature guide](GOTOSOCIAL_PLUS.md) for behavior, configuration,
privacy notes, and limitations. See the [changelog](CHANGELOG.md) for changes
in each release.

## Releases and installation

Ready-to-run Linux AMD64 archives are published on the
[GitHub Releases page](https://github.com/flschr/gotosocial-plus/releases).
Release names follow `v<GoToSocial version>-plus.<Plus release>`.

GoToSocial Plus currently tracks GoToSocial 0.22.1. Always read the upstream
release and migration notes before upgrading.

A release archive contains the binary, compiled web assets, templates,
license, documentation, and example configuration. Deploy the binary,
`web/assets`, and `web/template` from the same archive; mixing files from
different builds can break public pages or the settings interface.

Before upgrading, back up:

- the database and media storage;
- `config.yaml`;
- the current binary; and
- the matching web assets and templates.

Database migrations run automatically when the new binary starts. Release
notes document the defaults and activation requirements of newly added
settings.

## Support and source

- Plus source and issues:
  <https://github.com/flschr/gotosocial-plus>
- Plus releases:
  <https://github.com/flschr/gotosocial-plus/releases>
- Official GoToSocial source:
  <https://codeberg.org/superseriousbusiness/gotosocial>
- Official GoToSocial documentation:
  <https://docs.gotosocial.org/>

Report general GoToSocial problems to the upstream project and Plus-specific
problems to this repository.

## License

GoToSocial Plus is free software licensed under the
[GNU Affero General Public License v3 or later](LICENSE), unchanged from
upstream. Copyright for the original project remains with the GoToSocial
authors; Plus modifications are available in this repository's Git history.
