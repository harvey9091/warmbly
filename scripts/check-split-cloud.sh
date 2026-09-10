#!/bin/sh
# Everything CI should know about the split-deployment bus bundle.
#
# It exists because three defects shipped in that bundle at once and every one
# of them passed review: NATS given a store_dir twice and refusing to start,
# Redis unable to read its own private key because the image's entrypoint drops
# privileges with gosu and discards the added group, and a healthcheck pointed
# at `localhost` when the monitor binds IPv4 loopback, so the service sat
# unhealthy while serving traffic perfectly well.
#
# None of them is visible in the file. All three are obvious the moment the
# stack is actually started, which is what this does: it brings the real
# compose file up against a self-signed certificate and asserts both services
# reach `healthy`.
#
# POSIX sh, no Docker required to fail cleanly: without a daemon it skips,
# because a shell parse and a lint are still worth running everywhere.
set -eu

BUNDLE="deploy/split-cloud/bus"
fail() { printf 'check-split-cloud: %s\n' "$*" >&2; exit 1; }
ok()   { printf '  ok  %s\n' "$*"; }
skip() { printf '  --  %s\n' "$*"; }

[ -d "$BUNDLE" ] || fail "$BUNDLE not found (run from the repository root)"

# ---- static checks, everywhere ---------------------------------------------

parse_check() {
  if command -v dash >/dev/null 2>&1; then
    dash -n "$1" || fail "dash -n failed on $1"
  else
    sh -n "$1" || fail "sh -n failed on $1"
  fi
}

for f in "$BUNDLE/setup.sh" "$BUNDLE/certbot-deploy-hook.sh"; do
  parse_check "$f"
done
parse_check "$0"
ok "POSIX parse (setup, hook, checker)"

if command -v shellcheck >/dev/null 2>&1; then
  shellcheck -s sh "$BUNDLE/setup.sh" "$BUNDLE/certbot-deploy-hook.sh" "$0" \
    || fail "shellcheck failed"
  ok "shellcheck -s sh"
else
  skip "shellcheck not installed; skipped"
fi

# The exec bit has to be what git RECORDS, not what the working tree happens to
# have: a checkout on a filesystem that does not preserve it silently drops the
# mode, and the Makefile then fails with "Permission denied" in CI only. This
# already shipped once.
for f in "$BUNDLE/setup.sh" "$BUNDLE/certbot-deploy-hook.sh" "$0"; do
  mode=$(git ls-files -s "$f" 2>/dev/null | awk '{print $1}')
  [ -n "$mode" ] || continue
  case "$mode" in
    100755) ;;
    *) fail "$f is recorded as $mode; it is executed directly, so it needs 100755 (git update-index --chmod=+x $f)" ;;
  esac
done
ok "scripts are recorded executable"

out=$(sh "$BUNDLE/setup.sh" --help) || fail "--help exited non-zero"
printf '%s' "$out" | grep -q -- "--domain" || fail "--help does not document --domain"
ok "--help"

# --domain is what every other step depends on; without it the script must
# refuse rather than proceed against an empty hostname.
if sh "$BUNDLE/setup.sh" --dry-run >/dev/null 2>&1; then
  fail "setup.sh ran without --domain"
fi
ok "a missing --domain is refused"

# ---- the part that needs Docker --------------------------------------------

if ! docker info >/dev/null 2>&1; then
  skip "no Docker daemon; skipped the part that starts the stack"
  printf 'check-split-cloud: static checks passed\n'
  exit 0
fi

WORK=$(mktemp -d)
CERTS="$WORK/certs"

# An explicit, unique project name. Compose otherwise derives one from the
# directory, which would be "bus" — the same name a real deployment uses, and
# the teardown below would then delete a running production stack along with
# its JetStream volume.
COMPOSE_PROJECT_NAME="splitcloudcheck$$"
export COMPOSE_PROJECT_NAME

cleanup() {
  if [ -n "${WORK:-}" ] && [ -d "$WORK" ]; then
    ( cd "$WORK/bus" 2>/dev/null && \
      WARMBLY_CERT_DIR="$CERTS" docker compose down -v --remove-orphans >/dev/null 2>&1 ) || true
    rm -rf "$WORK"
  fi
}
trap cleanup EXIT INT TERM

mkdir -p "$CERTS" "$WORK/bus"
cp "$BUNDLE/docker-compose.yml" "$BUNDLE/nats.conf" "$WORK/bus/"

# A self-signed certificate is enough: this asserts the services start and can
# READ the key, which is where the bundle broke. Whether a public CA signed it
# is Let's Encrypt's problem, not the compose file's.
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$CERTS/privkey.pem" \
  -out "$CERTS/fullchain.pem" -days 2 -subj "/CN=bus.test" >/dev/null 2>&1 \
  || fail "could not generate a test certificate"
cp "$CERTS/fullchain.pem" "$CERTS/chain.pem"

# The permissions the real deploy hook sets: group-readable, never world. Get
# this wrong and Redis fails exactly as it did in production.
CERT_GID=$(id -g)
chmod 0644 "$CERTS/fullchain.pem" "$CERTS/chain.pem"
chgrp "$CERT_GID" "$CERTS/privkey.pem" 2>/dev/null || true
chmod 0640 "$CERTS/privkey.pem"

cat > "$WORK/bus/.env" <<ENVEOF
NATS_TOKEN=checktoken
REDIS_PASSWORD=checkpassword
ENVEOF

cd "$WORK/bus"
export WARMBLY_CERT_DIR="$CERTS" WARMBLY_CERT_GID="$CERT_GID"

# Ports are remapped: CI hosts and developer machines often have something on
# 4222 or 6380 already, and a port clash reads as a bundle defect otherwise.
cat > docker-compose.override.yml <<'OVERRIDE'
services:
  nats:
    ports: !override ["14222:4222"]
  redis:
    ports: !override ["16380:6380"]
OVERRIDE

if ! docker compose up -d >/dev/null 2>&1; then
  docker compose logs --tail 30 >&2
  fail "docker compose up failed"
fi

# Health, not "created". Every defect this bundle has had produced containers
# that were created and then never worked.
i=0
nats_state=starting
redis_state=starting
while [ "$i" -lt 45 ]; do
  nats_state=$(docker inspect --format '{{.State.Health.Status}}' "$(docker compose ps -q nats 2>/dev/null)" 2>/dev/null || echo missing)
  redis_state=$(docker inspect --format '{{.State.Health.Status}}' "$(docker compose ps -q redis 2>/dev/null)" 2>/dev/null || echo missing)
  if [ "$nats_state" = "healthy" ] && [ "$redis_state" = "healthy" ]; then
    break
  fi
  i=$((i + 1))
  sleep 2
done

if [ "$nats_state" != "healthy" ] || [ "$redis_state" != "healthy" ]; then
  printf 'nats=%s redis=%s\n' "$nats_state" "$redis_state" >&2
  docker compose logs --tail 40 >&2
  fail "the bundle did not reach healthy (nats=$nats_state redis=$redis_state)"
fi
ok "both services reach healthy against the real compose file"

# Redis serving TLS at all proves it opened the private key, which is the
# specific thing the gosu privilege drop broke.
if ! docker run --rm --network host redis:7-alpine redis-cli --tls --insecure \
    -h 127.0.0.1 -p 16380 -a checkpassword PING 2>/dev/null | grep -q PONG; then
  fail "redis did not answer an authenticated TLS PING; it cannot read the key"
fi
ok "redis answers over TLS with its password"

if docker run --rm --network host redis:7-alpine redis-cli --tls --insecure \
    -h 127.0.0.1 -p 16380 -a wrong PING 2>/dev/null | grep -q PONG; then
  fail "redis accepted a wrong password"
fi
ok "redis refuses a wrong password"

# JetStream has to be on, or every durable stream the platform creates fails
# at runtime rather than here.
if ! docker compose exec -T nats wget -q -O - http://127.0.0.1:8222/varz 2>/dev/null \
    | grep -q '"jetstream"'; then
  fail "nats is not reporting JetStream; durable streams would fail at runtime"
fi
ok "nats has JetStream enabled"

printf 'check-split-cloud: all checks passed\n'
