#!/usr/bin/env bash
set -Eeuo pipefail

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
test_dir="$(mktemp -d)"
trap 'rm -rf -- "${test_dir}"' EXIT
readonly database="${test_dir}/sqlite.db"
readonly config="${test_dir}/empty.conf"
readonly error_state="${test_dir}/error-since"
readonly fake_curl="${test_dir}/curl"
readonly fake_logger="${test_dir}/logger"
readonly logger_output="${test_dir}/logger-output"
touch "${config}"
printf '%s\n' '#!/usr/bin/env bash' 'exit 1' >"${fake_curl}"
printf '%s\n' '#!/usr/bin/env bash' 'printf '\''%s\n'\'' "$*" >>"${TEST_LOGGER_OUTPUT}"' >"${fake_logger}"
chmod +x "${fake_curl}" "${fake_logger}"

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

run_missing_database_check() {
  SOCIAL_MONITORING_CONFIG="${config}" \
  GTS_DATABASE="${test_dir}/missing/sqlite.db" \
  SOCIAL_STACK_HEALTH_BLUESKY_ONLY=true \
  BLUESKY_ERROR_STATE_FILE="${error_state}" \
  HC_STACK_URL="https://example.invalid/SECRET-PING-KEY" \
  CURL_BIN="${fake_curl}" \
  LOGGER_BIN="${fake_logger}" \
  TEST_LOGGER_OUTPUT="${logger_output}" \
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

set +e
missing_output="$(run_missing_database_check)"
missing_status=$?
set -e
[[ "${missing_status}" -ne 0 ]]
[[ "$(grep -Fc 'Could not inspect the Bluesky sync database.' <<<"${missing_output}")" -eq 1 ]]
[[ "$(grep -Fc 'Could not deliver Healthchecks.io stack/fail ping' "${logger_output}")" -eq 1 ]]
! grep -F 'SECRET-PING-KEY' <<<"${missing_output}" >/dev/null
! grep -F 'SECRET-PING-KEY' "${logger_output}" >/dev/null

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
IFS='|' read -r error_account error_since <"${error_state}"
[[ -n "${error_account}" ]]
[[ "${error_since}" =~ ^[0-9]+$ ]]

# A changing message on the same account must not reset its failure duration.
printf '%s|%s\n' "${error_account}" "$(( $(date +%s) - 601 ))" >"${error_state}"
sqlite3 "${database}" "UPDATE bluesky_connections SET last_sync_error='different temporary failure';"
expect_failure "1 of 1 erroring account(s)"

# A newly failing second account must not reset the first account's timer.
sqlite3 "${database}" "INSERT INTO bluesky_connections (id, updated_at, oauth_session_id, oauth_data, last_sync_at, last_sync_error, crosspost_public) VALUES ('account-b', datetime('now'), 'session-b', X'02', datetime('now'), 'second account failure', 1);"
expect_failure "1 of 2 erroring account(s)"
[[ "$(wc -l <"${error_state}" | tr -d ' ')" -eq 2 ]]

# Recovery removes only the recovered account and preserves the other timer.
sqlite3 "${database}" "UPDATE bluesky_connections SET last_sync_error=NULL WHERE id='account';"
run_check >/dev/null
[[ "$(wc -l <"${error_state}" | tr -d ' ')" -eq 1 ]]
grep -F "$(sqlite3 "${database}" "SELECT hex('account-b');")|" "${error_state}" >/dev/null

sqlite3 "${database}" "UPDATE bluesky_connections SET last_sync_error=NULL;"
run_check >/dev/null
[[ ! -e "${error_state}" ]]

printf 'social-stack-health tests passed\n'
