#!/usr/bin/env bash
set -Eeuo pipefail

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
test_dir="$(mktemp -d)"
trap 'rm -rf -- "${test_dir}"' EXIT
readonly database="${test_dir}/sqlite.db"
readonly config="${test_dir}/empty.conf"
touch "${config}"

sqlite3 "${database}" <<'SQL'
CREATE TABLE bluesky_connections (
  id TEXT PRIMARY KEY,
  updated_at TEXT NOT NULL,
  oauth_session_id TEXT,
  oauth_data BLOB,
  last_sync_at TEXT,
  last_sync_error TEXT,
  last_sync_error_code TEXT,
  crosspost_public INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE bluesky_deliveries (
  id TEXT PRIMARY KEY,
  created_at TEXT NOT NULL,
  dead_letter INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE bluesky_notifications (
  id TEXT PRIMARY KEY,
  dead_letter INTEGER NOT NULL DEFAULT 0
);
INSERT INTO bluesky_connections
  (id, updated_at, oauth_session_id, oauth_data, last_sync_at, crosspost_public)
VALUES
  ('account', datetime('now'), 'session', X'01', datetime('now'), 1);
SQL

run_check() {
  SOCIAL_MONITORING_CONFIG="${config}" \
  GTS_DATABASE="${database}" \
  SOCIAL_STACK_HEALTH_BLUESKY_ONLY=true \
  "${SCRIPT_DIR}/social-stack-health" 2>&1
}

expect_failure() {
  local expected="$1" output
  set +e
  output="$(run_check)"
  status=$?
  set -e
  [[ "${status}" -ne 0 ]]
  grep -F "${expected}" <<<"${output}" >/dev/null
}

run_check >/dev/null

sqlite3 "${database}" "UPDATE bluesky_connections SET oauth_session_id=NULL, oauth_data=NULL;"
expect_failure "crossposting is enabled"

sqlite3 "${database}" "UPDATE bluesky_connections SET oauth_session_id='session', oauth_data=X'01', last_sync_at=datetime('now','-11 minutes');"
expect_failure "incoming sync is stale"

sqlite3 "${database}" "UPDATE bluesky_connections SET last_sync_at=datetime('now'); INSERT INTO bluesky_notifications VALUES ('notification', 1);"
expect_failure "1 failed incoming item(s)"

sqlite3 "${database}" "DELETE FROM bluesky_notifications; UPDATE bluesky_connections SET last_sync_error='TOKEN REFRESH FAILED: INVALID_GRANT';"
expect_failure "authorization expired"

sqlite3 "${database}" "UPDATE bluesky_connections SET last_sync_error=NULL; INSERT INTO bluesky_deliveries VALUES ('delivery', datetime('now','-16 minutes'), 0);"
expect_failure "waiting longer than 15 minutes"

printf 'social-stack-health tests passed\n'
