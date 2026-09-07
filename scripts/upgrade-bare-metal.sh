#!/usr/bin/env bash
#
# Rebuild a Docker-free Warmbly install after the checkout moved, then hand the
# artifacts to the privileged installer. This is what the updater runs in
# UPDATER_MODE=command (deploy/systemd/warmbly-updater.service), and what you
# run by hand after `git pull`. It is the "Upgrading" section of
# docs/content/docs/development/bare-metal.mdx as a script.
#
#   scripts/upgrade-bare-metal.sh            # build, then install + restart
#   scripts/upgrade-bare-metal.sh --pull     # git pull --ff-only first
#
# Run it as the user who owns the checkout. Everything here runs unprivileged;
# the only root step is the fixed-path installer, which is the single command
# that user may run through sudo (see deploy/systemd/warmbly-install-release.sh).
set -euo pipefail

SRC="${WARMBLY_SRC:-/opt/warmbly/src}"
PREFIX="${WARMBLY_PREFIX:-/opt/warmbly}"
INSTALLER="${WARMBLY_INSTALLER:-/usr/local/sbin/warmbly-install-release}"

log() { printf '==> %s\n' "$*"; }

if [[ "${1:-}" == "--pull" ]]; then
  log "pulling"
  git -C "$SRC" pull --ff-only
fi

[[ -x "$INSTALLER" ]] || {
  echo "$INSTALLER is missing. Install it root-owned first:" >&2
  echo "  sudo install -o root -g root -m 0755 $SRC/deploy/systemd/warmbly-install-release.sh $INSTALLER" >&2
  exit 1
}

cd "$SRC"
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
COMMIT="$(git rev-parse HEAD 2>/dev/null || true)"
BUILT_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-s -w -X github.com/warmbly/warmbly/internal/version.Version=$VERSION -X github.com/warmbly/warmbly/internal/version.Commit=$COMMIT -X github.com/warmbly/warmbly/internal/version.BuiltAt=$BUILT_AT"

# A service is built only if this host runs it: no Rust toolchain means no
# tracking rebuild, and a unit that is not installed is skipped.
has_unit() { systemctl list-unit-files "warmbly-$1.service" --no-legend 2>/dev/null | grep -q .; }

log "building Go services ($VERSION)"
export CGO_ENABLED=0
mkdir -p out
for cmd in backend forms consumer worker migrate warmblyctl updater; do
  go build -ldflags="$LDFLAGS" -o "out/$cmd" "./cmd/$cmd"
done

if has_unit tracking && command -v cargo >/dev/null 2>&1; then
  log "building tracking"
  (cd tracking && cargo build --release)
fi

if has_unit realtime && command -v mix >/dev/null 2>&1; then
  log "building realtime"
  (cd realtime && MIX_ENV=prod mix deps.get --only prod && MIX_ENV=prod mix compile && MIX_ENV=prod mix release --overwrite)
fi

if command -v pnpm >/dev/null 2>&1; then
  for app in web admin forms; do
    if [[ -d "$PREFIX/$app" ]]; then
      log "building $app"
      (cd "$app" && pnpm install --frozen-lockfile && pnpm build)
    fi
  done
fi

log "installing and restarting (sudo $INSTALLER)"
if [[ "$(id -u)" -eq 0 ]]; then
  "$INSTALLER"
else
  sudo -n "$INSTALLER"
fi

log "done: $VERSION"
