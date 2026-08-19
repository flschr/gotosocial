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
```

A healthy service reports `Result=success` and `ExecMainStatus=0`; the timer is
`active` and `waiting`. Fixture tests inject fake logger and network binaries,
so expected failure cases never write to the host journal or contact an
external service.

The error-duration state under `/run` stores only hex-encoded account IDs and
timestamps. It is deleted automatically after complete recovery.
