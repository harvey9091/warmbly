# systemd units for a Docker-free install

One unit per Warmbly process, for a host where the binaries were built from
source and installed under `/opt/warmbly`. The step-by-step guide that uses
them is [Deploying without Docker](https://docs.warmbly.com/development/bare-metal/).

| Unit | Runs | Reads |
|------|------|-------|
| `warmbly-backend.service` | `/opt/warmbly/bin/backend` | `/etc/warmbly/warmbly.env` |
| `warmbly-forms.service` | `/opt/warmbly/bin/forms` | `/etc/warmbly/warmbly.env` |
| `warmbly-consumer.service` | `/opt/warmbly/bin/consumer` | `/etc/warmbly/warmbly.env` |
| `warmbly-tracking.service` | `/opt/warmbly/bin/tracking` | `/etc/warmbly/warmbly.env` |
| `warmbly-realtime.service` | `/opt/warmbly/realtime/bin/realtime start` | `/etc/warmbly/warmbly.env` |
| `warmbly-worker.service` | `/opt/warmbly/bin/worker` | `/etc/warmbly/worker.env` |
| `warmbly-updater.service` | `/opt/warmbly/bin/updater` | `/etc/warmbly/updater.env` |

Every unit runs as the unprivileged `warmbly` user, may write only under
`/var/lib/warmbly` (blob storage), and restarts on failure. Copy them to
`/etc/systemd/system/`, then `systemctl daemon-reload` and
`systemctl enable --now` the ones this host runs. Paths are plain strings, so
`sed` them if you install somewhere other than `/opt/warmbly`.

`deploy/config/env.example` is the template for `warmbly.env`; the worker env
file is what `POST /api/v1/workers/enroll` returns for an enrollment token.

`warmbly-updater.service` is the exception to the unprivileged rule: it runs as
the user who owns the checkout (`deploy` in the unit; change it) and drives
`scripts/upgrade-bare-metal.sh`, which builds unprivileged and then runs
`warmbly-install-release.sh` (installed root-owned at
`/usr/local/sbin/warmbly-install-release`) through sudo. That installer takes no
arguments, touches only fixed paths and refuses symlinks, and is the single
command to allow in sudoers. `updater.env` holds only `UPDATER_TOKEN` (the
backend's `INTERNAL_API_TOKEN`). See
[Updates](https://docs.warmbly.com/development/updates/).
