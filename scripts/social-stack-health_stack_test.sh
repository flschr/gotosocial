#!/usr/bin/env bash
set -Eeuo pipefail

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
test_dir="$(mktemp -d)"
trap 'rm -rf -- "${test_dir}"' EXIT
readonly database="${test_dir}/sqlite.db"
readonly config="${test_dir}/empty.conf"
readonly fake_systemctl="${test_dir}/systemctl"
readonly fake_docker="${test_dir}/docker"
readonly fake_curl="${test_dir}/curl"
readonly fake_logger="${test_dir}/logger"
readonly fake_sleep="${test_dir}/sleep"
readonly curl_output="${test_dir}/curl-output"
readonly logger_output="${test_dir}/logger-output"
readonly sleep_output="${test_dir}/sleep-output"
touch "${config}" "${curl_output}" "${logger_output}" "${sleep_output}"

printf '%s\n' \
  '#!/usr/bin/env bash' \
  '[[ "${FAIL_SYSTEMCTL:-}" != true ]]' >"${fake_systemctl}"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'container="${!#}"' \
  '[[ "${FAIL_CONTAINER:-}" != "${container}" ]] || exit 1' \
  'if [[ "$*" == *State.Health* ]]; then' \
  '  if [[ "${UNHEALTHY_CONTAINER:-}" == "${container}" ]]; then printf '\''unhealthy\n'\''; else printf '\''healthy\n'\''; fi' \
  'else' \
  '  printf '\''running\n'\''' \
  'fi' >"${fake_docker}"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'printf '\''%s\n'\'' "$*" >>"${TEST_CURL_OUTPUT}"' \
  '[[ -z "${FAIL_URL:-}" || "$*" != *"${FAIL_URL}"* ]]' >"${fake_curl}"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'printf '\''%s\n'\'' "$*" >>"${TEST_LOGGER_OUTPUT}"' \
  '[[ "${FAIL_LOGGER:-}" != true ]]' >"${fake_logger}"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'printf '\''%s\n'\'' "$*" >>"${TEST_SLEEP_OUTPUT}"' >"${fake_sleep}"
chmod +x "${fake_systemctl}" "${fake_docker}" "${fake_curl}" "${fake_logger}" "${fake_sleep}"

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
SQL

run_check() {
  SOCIAL_MONITORING_CONFIG="${config}" \
  GTS_DATABASE="${database}" \
  HC_STACK_URL="${TEST_HC_URL:-https://example.invalid/stack-check}" \
  CURL_BIN="${fake_curl}" \
  LOGGER_BIN="${fake_logger}" \
  SYSTEMCTL_BIN="${fake_systemctl}" \
  DOCKER_BIN="${fake_docker}" \
  SLEEP_BIN="${fake_sleep}" \
  TEST_CURL_OUTPUT="${curl_output}" \
  TEST_LOGGER_OUTPUT="${logger_output}" \
  TEST_SLEEP_OUTPUT="${sleep_output}" \
  FAIL_SYSTEMCTL="${FAIL_SYSTEMCTL:-}" \
  FAIL_CONTAINER="${FAIL_CONTAINER:-}" \
  UNHEALTHY_CONTAINER="${UNHEALTHY_CONTAINER:-}" \
  FAIL_URL="${FAIL_URL:-}" \
  FAIL_LOGGER="${FAIL_LOGGER:-}" \
  "${SCRIPT_DIR}/social-stack-health" 2>&1
}

expect_failure() {
  local expected="$1" output status
  : >"${sleep_output}"
  set +e
  output="$(run_check)"
  status=$?
  set -e
  [[ "${status}" -ne 0 ]]
  grep -F "${expected}" <<<"${output}" >/dev/null
  [[ "$(wc -l <"${sleep_output}" | tr -d ' ')" -eq 2 ]]
}

expect_direct_failure() {
  local expected="$1" output status
  : >"${sleep_output}"
  set +e
  output="$(run_check)"
  status=$?
  set -e
  [[ "${status}" -ne 0 ]]
  grep -F "${expected}" <<<"${output}" >/dev/null
  [[ ! -s "${sleep_output}" ]]
}

run_check >/dev/null
grep -F 'https://social.fischr.org/api/v1/instance' "${curl_output}" >/dev/null
grep -F 'https://pds.fischr.org/xrpc/_health' "${curl_output}" >/dev/null
grep -F 'https://example.invalid/stack-check' "${curl_output}" >/dev/null

set +e
missing_config_output="$(env -u HC_STACK_URL \
  SOCIAL_MONITORING_CONFIG="${config}" \
  SOCIAL_STACK_HEALTH_BLUESKY_ONLY=true \
  LOGGER_BIN="${fake_logger}" \
  TEST_LOGGER_OUTPUT="${logger_output}" \
  "${SCRIPT_DIR}/social-stack-health" 2>&1)"
missing_config_status=$?
set -e
[[ "${missing_config_status}" -ne 0 ]]
grep -F 'Healthchecks.io stack ping URL must be a valid HTTPS URL.' <<<"${missing_config_output}" >/dev/null

TEST_HC_URL=not-a-url expect_direct_failure "Healthchecks.io stack ping URL must be a valid HTTPS URL."
FAIL_URL=example.invalid/stack-check expect_direct_failure "Could not deliver Healthchecks.io healthy stack ping."

FAIL_SYSTEMCTL=true expect_failure "GoToSocial service is not active."
FAIL_CONTAINER=caddy expect_failure "Could not inspect the caddy container."
UNHEALTHY_CONTAINER=caddy expect_failure "caddy is not healthy"
FAIL_CONTAINER=pds expect_failure "Could not inspect the pds container."
UNHEALTHY_CONTAINER=pds expect_failure "pds is not healthy"
FAIL_URL=social.fischr.org expect_failure "GoToSocial API is not reachable."
FAIL_URL=pds.fischr.org expect_failure "Bluesky PDS API is not reachable."

: >"${curl_output}"
FAIL_SYSTEMCTL=true FAIL_LOGGER=true expect_failure "GoToSocial service is not active."
[[ "$(grep -Fc 'https://example.invalid/stack-check/fail' "${curl_output}")" -eq 1 ]]

printf 'social-stack-health stack tests passed\n'
