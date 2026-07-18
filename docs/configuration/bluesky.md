# Bluesky integration

GoToSocial Plus can connect an existing Bluesky account using OAuth. The
connection is optional and configured separately by each user under
**Settings → Bluesky**.

## Instance configuration

Generate a stable 32-byte encryption key:

```sh
openssl rand -base64 32
```

Set the result in `config.yaml`:

```yaml
bluesky-oauth-encryption-key: "YOUR_GENERATED_KEY"
```

Alternatively, set `GTS_BLUESKY_OAUTH_ENCRYPTION_KEY`. Keep the key in your
secret store and include it in your backup process. Changing or losing it
invalidates existing Bluesky connections because their OAuth tokens and DPoP
keys can no longer be decrypted.

## User behavior

After connecting an account, a user can independently enable:

- automatic publishing of public, original posts to Bluesky; and
- a **Follow on Bluesky** button on their public profile.

Automatic publishing excludes replies, mentions, boosts, polls, imported
posts, and every non-public visibility. Images retain their alt text. A post
uses either up to four images or its external link preview, matching Bluesky's
embed model.

Replies and mentions received from Bluesky appear as direct, local-only
statuses in Mastodon-compatible clients. Replies written from those clients
are sent to Bluesky under the connected Bluesky identity. The proxy statuses
and local replies are never federated over ActivityPub.

Disconnecting removes the local OAuth session and attempts to revoke it at the
provider. Existing post mappings remain available so historical relationships
are not guessed from post text or URLs.
