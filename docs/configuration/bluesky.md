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

The settings page reports whether the connector is healthy, syncing, retrying,
or needs to be reconnected. OAuth revocation, an unusable refresh token, and a
missing encryption key are shown as actionable messages; protocol details stay
in the server log.

Disconnecting removes the local OAuth session and attempts to revoke it at the
provider. Existing post mappings remain available so historical relationships
are not guessed from post text or URLs. After reconnecting the same Bluesky
account, edits and deletions can therefore continue to target their exact
remote records.

A disconnected account remains bound to the same Bluesky DID. Reconnecting
with a different Bluesky identity is rejected to prevent retained mappings
from editing the wrong account. Users who intentionally want to switch can
choose **Forget saved Bluesky account** after disconnecting. Forgetting removes
the binding and all retained mappings; already published Bluesky posts remain
online and can no longer be managed by GoToSocial.
