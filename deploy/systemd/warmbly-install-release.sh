#!/usr/bin/env bash
#
# The privileged half of a bare-metal upgrade: install the artifacts that
# scripts/upgrade-bare-metal.sh built under /opt/warmbly/src/out and friends,
# then restart the units, backend first. It takes no arguments and touches only
# fixed paths, so it is the one command the checkout's owner may run through
# sudo without a password. Install it root-owned and not writable by that user:
#
#   sudo install -o root -g root -m 0755 deploy/systemd/warmbly-install-release.sh /usr/local/sbin/warmbly-install-release
#   deploy ALL=(root) NOPASSWD: /usr/local/sbin/warmbly-install-release      # visudo
#
# A symlink under the build output is refused: a caller who owns the checkout
# must not be able to make root read or install a file from somewhere else.
set -euo pipefail

SRC=/opt/warmbly/src
PREFIX=/opt/warmbly
HEALTH=http://127.0.0.1:8080/health

[[ "$(id -u)" -eq 0 ]] || { echo "run through sudo" >&2; exit 1; }
[[ $# -eq 0 ]] || { echo "takes no arguments" >&2; exit 2; }

log() { printf '==> %s\n' "$*"; }
regular() { [[ -f "$1" && ! -L "$1" ]]; }
# A directory qualifies only when nothing inside it is a symlink either: the
# checkout's owner must not be able to point root at a file elsewhere.
plain_dir() {
  [[ -d "$1" && ! -L "$1" ]] || return 1
  if find "$1" -type l -print -quit | grep -q .; then
    log "refusing $1: it contains a symlink"
    return 1
  fi
}
has_unit() { systemctl list-unit-files "warmbly-$1.service" --no-legend 2>/dev/null | grep -q .; }

log "installing binaries"
for bin in backend forms consumer worker migrate warmblyctl updater; do
  f="$SRC/out/$bin"
  if regular "$f"; then
    install -o root -g root -m 0755 "$f" "$PREFIX/bin/$bin"
  fi
done
ln -sf "$PREFIX/bin/warmblyctl" /usr/local/bin/warmblyctl

if has_unit tracking && regular "$SRC/tracking/target/release/tracking"; then
  install -o root -g root -m 0755 "$SRC/tracking/target/release/tracking" "$PREFIX/bin/tracking"
fi

if has_unit realtime && plain_dir "$SRC/realtime/_build/prod/rel/realtime"; then
  log "installing realtime"
  rm -rf "$PREFIX/realtime"
  cp -r --no-dereference "$SRC/realtime/_build/prod/rel/realtime" "$PREFIX/realtime"
  chown -R warmbly:warmbly "$PREFIX/realtime"
fi

# The runtime config.js is written by hand on a bare-metal install and a
# rebuilt dist/ would drop it, so it is kept across the copy.
for app in web admin; do
  if plain_dir "$SRC/$app/dist" && plain_dir "$PREFIX/$app"; then
    log "installing $app"
    cfg="$(mktemp)"
    [[ -f "$PREFIX/$app/config.js" ]] && cp "$PREFIX/$app/config.js" "$cfg"
    rm -rf "$PREFIX/$app"
    cp -r --no-dereference "$SRC/$app/dist" "$PREFIX/$app"
    [[ -s "$cfg" ]] && cp --remove-destination "$cfg" "$PREFIX/$app/config.js"
    rm -f "$cfg"
    chown -R root:root "$PREFIX/$app"
    chmod -R a+rX "$PREFIX/$app"
  fi
done
if plain_dir "$SRC/forms/dist" && plain_dir "$PREFIX/forms"; then
  log "installing forms"
  rm -rf "$PREFIX/forms/dist"
  cp -r --no-dereference "$SRC/forms/dist" "$PREFIX/forms/dist"
  chown -R root:root "$PREFIX/forms/dist"
  chmod -R a+rX "$PREFIX/forms/dist"
fi

log "restarting backend"
systemctl restart warmbly-backend
healthy=0
deadline=$((SECONDS + 120))
while [[ $SECONDS -lt $deadline ]]; do
  if curl -fsS --connect-timeout 2 --max-time 5 "$HEALTH" >/dev/null 2>&1; then healthy=1; break; fi
  sleep 2
done
if [[ "$healthy" -ne 1 ]]; then
  log "the backend did not answer at $HEALTH within two minutes; the other services were not restarted"
  exit 1
fi

rest=()
for svc in forms consumer tracking realtime worker; do
  has_unit "$svc" && rest+=("warmbly-$svc")
done
if [[ ${#rest[@]} -gt 0 ]]; then
  log "restarting ${rest[*]}"
  systemctl restart "${rest[@]}"
fi
log "installed"
