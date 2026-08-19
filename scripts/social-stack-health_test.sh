#!/usr/bin/env bash
set -Eeuo pipefail

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
test_dir="$(mktemp -d)"
trap 'rm -rf -- "${test_dir}"' EXIT
readonly database="${test_dir}/sqlite.db"
readonly config="${test_dir}/empty.conf"
readonly error_state="${test_dir}/error-since"
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
  BLUESKY_ERROR_STATE_FILE="${error_state}" \
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
expect_failure "incoming sync scheduler is stale"

sqlite3 "${database}" "UPDATE bluesky_connections SET last_sync_at=datetime('now'); INSERT INTO bluesky_notifications VALUES ('notification', 1);"
expect_failure "1 failed incoming item(s)"

sqlite3 "${database}" "DELETE FROM bluesky_notifications; UPDATE bluesky_connections SET last_sync_error='TOKEN REFRESH FAILED: INVALID_GRANT';"
expect_failure "authorization expired"

sqlite3 "${database}" "UPDATE bluesky_connections SET last_sync_error=NULL; INSERT INTO bluesky_deliveries VALUES ('delivery', datetime('now','-16 minutes'), 0);"
expect_failure "waiting longer than 15 minutes"

sqlite3 "${database}" "DELETE FROM bluesky_deliveries; UPDATE bluesky_connections SET last_sync_error='temporary upstream failure';"
run_check >/dev/null
IFS='|' read -r error_since error_fingerprint <"${error_state}"
[[ "${error_since}" =~ ^[0-9]+$ ]]
[[ -n "${error_fingerprint}" ]]

# A different error identity starts a new grace period instead of inheriting
# the age of the previous transient failure.
printf '%s|%s\n' "$(( $(date +%s) - 601 ))" "${error_fingerprint}" >"${error_state}"
sqlite3 "${database}" "UPDATE bluesky_connections SET last_sync_error='different temporary failure';"
run_check >/dev/null
IFS='|' read -r reset_since reset_fingerprint <"${error_state}"
(( reset_since > $(date +%s) - 60 ))
[[ "${reset_fingerprint}" != "${error_fingerprint}" ]]

# Multiple accounts are fingerprinted deterministically and alert only after
# that exact combined failure state remains unchanged for the grace period.
sqlite3 "${database}" "INSERT INTO bluesky_connections (id, updated_at, oauth_session_id, oauth_data, last_sync_at, last_sync_error, crosspost_public) VALUES ('account-b', datetime('now'), 'session-b', X'02', datetime('now'), 'second account failure', 1);"
run_check >/dev/null
IFS='|' read -r combined_since combined_fingerprint <"${error_state}"
[[ "${combined_fingerprint}" != "${reset_fingerprint}" ]]
printf '%s|%s\n' "$(( $(date +%s) - 601 ))" "${combined_fingerprint}" >"${error_state}"
expect_failure "failed continuously"

sqlite3 "${database}" "UPDATE bluesky_connections SET last_sync_error=NULL;"
run_check >/dev/null
[[ ! -e "${error_state}" ]]

printf 'social-stack-health tests passed\n'
