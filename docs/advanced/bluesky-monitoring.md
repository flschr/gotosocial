# Bluesky sync monitoring

GoToSocial Plus includes a production monitor for failures that ordinary HTTP
health checks cannot see: expired Bluesky OAuth sessions, stuck outgoing posts,
dead-lettered jobs, stale incoming synchronization, and sustained remote
errors.

The versioned deployment contract consists of:

- `scripts/social-stack-health`
- `example/social-stack-health.service`
- `example/social-stack-health.timer`
- `example/social-stack-monitoring.conf.example`

## Installation

Back up the existing script and units before replacing them. Then install the
versioned files with root ownership:

```sh
sudo install -o root -g root -m 0755 scripts/social-stack-health /usr/local/sbin/social-stack-health
sudo install -o root -g root -m 0644 example/social-stack-health.service /etc/systemd/system/social-stack-health.service
sudo install -o root -g root -m 0644 example/social-stack-health.timer /etc/systemd/system/social-stack-health.timer
sudo install -o root -g root -m 0600 example/social-stack-monitoring.conf.example /etc/social-stack-monitoring.conf.example
sudo test -e /etc/social-stack-monitoring.conf || sudo install -o root -g root -m 0600 example/social-stack-monitoring.conf.example /etc/social-stack-monitoring.conf
```

On first installation, replace the commented placeholder in
`/etc/social-stack-monitoring.conf` with the real `HC_STACK_URL`. On upgrades,
preserve the existing active configuration and compare it with the newly
installed `.example` file. Keep the active file mode at `0600`; the URL is a
credential.

Reload systemd, verify the unit definitions, run one foreground check, and then
enable the timer:

```sh
sudo systemctl daemon-reload
sudo systemd-analyze verify /etc/systemd/system/social-stack-health.service /etc/systemd/system/social-stack-health.timer
sudo systemctl start social-stack-health.service
sudo systemctl enable --now social-stack-health.timer
```

## Verification

```sh
systemctl show social-stack-health.service -p Result -p ExecMainStatus
systemctl show social-stack-health.timer -p ActiveState -p SubState -p LastTriggerUSec
scripts/social-stack-health_test.sh
scripts/social-stack-health_stack_test.sh
```

A healthy service reports `Result=success` and `ExecMainStatus=0`; the timer is
`active` and `waiting`. Fixture tests inject fake logger and network binaries,
so expected failure cases never write to the host journal or contact an
external service.

The error-duration state under `/run` stores only hex-encoded account IDs and
timestamps. It is deleted automatically after complete recovery.

## End-to-end notification test

Automated checks prove delivery only as far as Healthchecks.io. After initial
installation and after changing the notification integration, send a clearly
labelled controlled failure and recovery through the real stack check:

```sh
sudo bash -c '
  set -euo pipefail
  set -a
  source /etc/social-stack-monitoring.conf
  set +a
  test -n "${HC_STACK_URL:-}"
  /usr/bin/curl --fail --silent --show-error --max-time 10 \
    --data-binary "TEST ONLY: controlled Bluesky monitor notification test" \
    "${HC_STACK_URL}/fail" >/dev/null
  /usr/bin/curl --fail --silent --show-error --max-time 10 \
    --data-binary "TEST ONLY: controlled Bluesky monitor recovery" \
    "${HC_STACK_URL}" >/dev/null
'
```

The recipient must confirm that the failure notification reached the expected
inbox and was not classified as spam. Record the date and receiving channel in
the operational issue or runbook. Provider-side status alone is not proof of
last-mile receipt.

For higher assurance, configure a second independent Healthchecks.io
integration such as ntfy, Signal, Telegram, or a webhook. Assign it to the
Social Stack check and repeat the controlled test for both channels.
