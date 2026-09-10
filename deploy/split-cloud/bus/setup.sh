#!/bin/sh
# Stand up the bus box: NATS JetStream and Redis, both over TLS, reachable by a
# control plane on one host and a worker fleet on others.
#
#   ./setup.sh --domain bus.example.com
#
# Idempotent. A second run adopts what is already there: it never regenerates a
# secret, never reissues a certificate that is still valid, and never
# overwrites the .env. Re-running to pick up a config change is the intended
# way to use it.
#
# POSIX sh: this runs under whatever /bin/sh the host has.
set -eu

DOMAIN=""
EMAIL=""
CERT_DIR="/opt/warmbly/certs"
CERT_GID="2000"
CERT_GROUP="warmbly-certs"
INSTALL_DIR="/opt/warmbly/bus"
SKIP_DOCKER="false"
VERIFY_ONLY="false"
PRINT_ENV_ONLY="false"
DRY_RUN="false"

log()  { printf '%s\n' "$*"; }
step() { printf '\n==> %s\n' "$*"; }
warn() { printf '%s\n' "$*" >&2; }
die()  { printf 'error: %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'USAGE'
Stand up the Warmbly bus box: NATS JetStream and Redis, both over TLS.

  --domain <d>      Hostname for this machine, e.g. bus.example.com.       (required)
                    Its A record must already point here: the certificate
                    is proved over port 80 at this name.
  --email <e>       Address to register with Let's Encrypt for expiry
                    warnings. Omitted registers without one.
  --cert-dir <d>    Where the published certificate lives. Default /opt/warmbly/certs.
  --gid <n>         Group that may read the private key. Default 2000.
  --install-dir <d> Where the compose file lives. Default /opt/warmbly/bus.
  --skip-docker     Do not install Docker, even if it is missing.
  --verify          Run the end-to-end checks against a running box and exit.
  --print-env       Print the control plane's NATS_URL and REDIS and exit.
  --dry-run         Say what would happen, change nothing.
  -h, --help        This text.

Needs root, and a machine that already resolves at --domain.
USAGE
}

parse_args() {
  while [ $# -gt 0 ]; do
    case "$1" in
      --domain)      DOMAIN="${2:-}"; shift 2 ;;
      --email)       EMAIL="${2:-}"; shift 2 ;;
      --cert-dir)    CERT_DIR="${2:-}"; shift 2 ;;
      --gid)         CERT_GID="${2:-}"; shift 2 ;;
      --install-dir) INSTALL_DIR="${2:-}"; shift 2 ;;
      --skip-docker) SKIP_DOCKER="true"; shift ;;
      --verify)      VERIFY_ONLY="true"; shift ;;
      --print-env)   PRINT_ENV_ONLY="true"; shift ;;
      --dry-run)     DRY_RUN="true"; shift ;;
      -h|--help)     usage; exit 0 ;;
      *)             die "unknown option: $1 (try --help)" ;;
    esac
  done
}

require_args() {
  [ -n "$DOMAIN" ] || die "--domain is required (try --help)"
  if [ "$DRY_RUN" = "false" ] && [ "$PRINT_ENV_ONLY" = "false" ]; then
    [ "$(id -u)" = "0" ] || die "run as root: this installs packages and writes to $INSTALL_DIR"
  fi
}

run() {
  if [ "$DRY_RUN" = "true" ]; then
    log "  would run: $*"
    return 0
  fi
  "$@"
}

# preflight fails on the two things that waste a Let's Encrypt attempt: a name
# that does not point here, and a port 80 something else is already using.
preflight() {
  step "Checking this machine can be certified for $DOMAIN"

  resolved=$(getent hosts "$DOMAIN" 2>/dev/null | awk '{print $1}' | head -n 1)
  if [ -z "$resolved" ]; then
    die "$DOMAIN does not resolve. Add an A record pointing at this machine first.
       On Cloudflare it must be DNS only (grey cloud): the proxy does not pass
       the ports this box serves, and it would answer the challenge itself."
  fi
  log "  $DOMAIN resolves to $resolved"

  if command -v ss >/dev/null 2>&1 && ss -tln 2>/dev/null | awk '{print $4}' | grep -qE '[:.]80$'; then
    die "something is already listening on port 80. Certbot needs it to prove
       control of $DOMAIN. Stop that service and re-run."
  fi
  log "  port 80 is free for the challenge"
  return 0
}

install_docker() {
  if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
    log "  docker $(docker --version | awk '{print $3}' | tr -d ,) with compose, already installed"
    return 0
  fi
  if [ "$SKIP_DOCKER" = "true" ]; then
    die "docker with the compose plugin is required and --skip-docker was given"
  fi
  step "Installing Docker"
  run sh -c 'curl -fsSL https://get.docker.com | sh'
  run systemctl enable --now docker
  return 0
}

install_certbot() {
  if command -v certbot >/dev/null 2>&1; then
    log "  certbot already installed"
    return 0
  fi
  step "Installing certbot"
  if command -v apt-get >/dev/null 2>&1; then
    run env DEBIAN_FRONTEND=noninteractive apt-get update -qq
    run env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq certbot
  elif command -v dnf >/dev/null 2>&1; then
    run dnf install -y certbot
  else
    die "no apt-get or dnf; install certbot yourself and re-run with it on PATH"
  fi
  return 0
}

# ensure_group creates the group that may read the private key. The key is
# never world-readable: this box is also where the first worker usually runs,
# and that container should not be able to read it.
ensure_group() {
  if getent group "$CERT_GID" >/dev/null 2>&1; then
    log "  gid $CERT_GID exists ($(getent group "$CERT_GID" | cut -d: -f1))"
    return 0
  fi
  step "Creating group $CERT_GROUP (gid $CERT_GID)"
  run groupadd -g "$CERT_GID" "$CERT_GROUP"
  return 0
}

# ensure_secrets writes the two credentials once and never again. Regenerating
# them on a second run would lock out a control plane that is already using
# them, with no error anyone would connect to the cause.
ensure_secrets() {
  if [ -f "$INSTALL_DIR/.env" ]; then
    log "  .env exists, left alone (a re-run never rotates a secret)"
    return 0
  fi
  step "Generating the bus credentials"
  if [ "$DRY_RUN" = "true" ]; then
    log "  would write $INSTALL_DIR/.env with a fresh NATS_TOKEN and REDIS_PASSWORD"
    return 0
  fi
  ( umask 077
    printf 'NATS_TOKEN=%s\nREDIS_PASSWORD=%s\n' \
      "$(openssl rand -hex 32)" "$(openssl rand -hex 32)" > "$INSTALL_DIR/.env" )
  chmod 600 "$INSTALL_DIR/.env"
  log "  wrote $INSTALL_DIR/.env"
  return 0
}

# install_hook is what makes the certificate survive its first renewal. Neither
# service re-reads the file on its own, so without this everything works for 90
# days and then stops on a morning nobody expects.
install_hook() {
  step "Installing the renewal hook"
  run mkdir -p /etc/letsencrypt/renewal-hooks/deploy
  if [ "$DRY_RUN" = "true" ]; then
    log "  would install warmbly-bus.sh pinned to $DOMAIN"
    return 0
  fi
  install -m 0755 "$INSTALL_DIR/certbot-deploy-hook.sh" \
    /etc/letsencrypt/renewal-hooks/deploy/warmbly-bus.sh
  # The hook reads these, so a renewal lands in the same place as the first
  # issue rather than at whatever the defaults were.
  sed -i "s|^DOMAIN=.*|DOMAIN=\"\${WARMBLY_BUS_DOMAIN:-$DOMAIN}\"|" \
    /etc/letsencrypt/renewal-hooks/deploy/warmbly-bus.sh
  sed -i "s|^DEST=.*|DEST=\"$CERT_DIR\"|" \
    /etc/letsencrypt/renewal-hooks/deploy/warmbly-bus.sh
  sed -i "s|^CERT_GID=.*|CERT_GID=\"\${WARMBLY_CERT_GID:-$CERT_GID}\"|" \
    /etc/letsencrypt/renewal-hooks/deploy/warmbly-bus.sh
  sed -i "s|^INSTALL_DIR=.*|INSTALL_DIR=\"$INSTALL_DIR\"|" \
    /etc/letsencrypt/renewal-hooks/deploy/warmbly-bus.sh
  log "  installed /etc/letsencrypt/renewal-hooks/deploy/warmbly-bus.sh"
  return 0
}

obtain_cert() {
  if [ -d "/etc/letsencrypt/live/$DOMAIN" ]; then
    log "  certificate for $DOMAIN already issued, reusing it"
  else
    step "Obtaining a certificate for $DOMAIN"
    if [ -n "$EMAIL" ]; then
      run certbot certonly --standalone -d "$DOMAIN" \
        --non-interactive --agree-tos --email "$EMAIL" --key-type ecdsa
    else
      run certbot certonly --standalone -d "$DOMAIN" \
        --non-interactive --agree-tos --register-unsafely-without-email --key-type ecdsa
    fi
  fi
  step "Publishing the certificate to $CERT_DIR"
  run env WARMBLY_BUS_DOMAIN="$DOMAIN" WARMBLY_CERT_GID="$CERT_GID" \
    /etc/letsencrypt/renewal-hooks/deploy/warmbly-bus.sh
  return 0
}

start_stack() {
  step "Starting NATS and Redis"
  if [ "$DRY_RUN" = "true" ]; then
    log "  would run docker compose up -d in $INSTALL_DIR"
    return 0
  fi
  cd "$INSTALL_DIR"
  WARMBLY_CERT_DIR="$CERT_DIR" WARMBLY_CERT_GID="$CERT_GID" docker compose up -d
  return 0
}

# wait_healthy is the difference between "the containers were created" and "the
# services work". Every bug this bundle has had looked like a clean start and
# then sat in a restart loop.
wait_healthy() {
  step "Waiting for both services to report healthy"
  cd "$INSTALL_DIR"
  i=0
  while [ "$i" -lt 30 ]; do
    nats_state=$(docker inspect --format '{{.State.Health.Status}}' bus-nats-1 2>/dev/null || echo starting)
    redis_state=$(docker inspect --format '{{.State.Health.Status}}' bus-redis-1 2>/dev/null || echo starting)
    if [ "$nats_state" = "healthy" ] && [ "$redis_state" = "healthy" ]; then
      log "  nats healthy, redis healthy"
      return 0
    fi
    i=$((i + 1))
    sleep 2
  done
  warn ""
  warn "nats=$nats_state redis=$redis_state after 60s. Recent logs:"
  docker compose logs --tail 20 >&2
  die "the stack did not come up healthy"
}

# verify proves what the operator actually cares about: that a real client can
# connect over TLS with the credential, and that a wrong credential is refused.
# A listening port proves neither.
verify() {
  step "Verifying from a real client"
  cd "$INSTALL_DIR"
  # shellcheck disable=SC1091  # written by ensure_secrets, not in the repo
  . ./.env

  if docker run --rm natsio/nats-box:latest \
      nats --server="tls://${NATS_TOKEN}@${DOMAIN}:4222" server check connection >/dev/null 2>&1; then
    log "  nats:  TLS + token accepted"
  else
    die "nats refused a connection with its own token. Is 4222 open in the network firewall?"
  fi

  if docker run --rm natsio/nats-box:latest \
      nats --server="tls://wrong-token@${DOMAIN}:4222" server check connection >/dev/null 2>&1; then
    die "nats accepted a WRONG token; it is not enforcing authorization"
  fi
  log "  nats:  wrong token refused"

  if docker run --rm redis:7-alpine redis-cli --tls \
      -h "$DOMAIN" -p 6380 -a "$REDIS_PASSWORD" PING 2>/dev/null | grep -q PONG; then
    log "  redis: TLS + password accepted"
  else
    die "redis refused a connection with its own password. Is 6380 open in the network firewall?"
  fi

  if docker run --rm redis:7-alpine redis-cli --tls \
      -h "$DOMAIN" -p 6380 -a wrong-password PING 2>/dev/null | grep -q PONG; then
    die "redis accepted a WRONG password; it is not enforcing auth"
  fi
  log "  redis: wrong password refused"
  return 0
}

print_env() {
  # shellcheck disable=SC1091  # written by ensure_secrets, not in the repo
  . "$INSTALL_DIR/.env"
  cat <<ENVOUT

Put these on the control plane. Nodes inherit them from it, so they have to be
addresses every machine in the fleet can reach:

  NATS_URL=tls://${NATS_TOKEN}@${DOMAIN}:4222
  REDIS=rediss://:${REDIS_PASSWORD}@${DOMAIN}:6380

ENVOUT
}

finish() {
  cat <<DONE
Done. This machine is a Warmbly bus.

  NATS      $DOMAIN:4222  (TLS, token)
  Redis     $DOMAIN:6380  (TLS, password)
  Renewal   certbot timer, hook restarts both services
  Config    $INSTALL_DIR
  Logs      cd $INSTALL_DIR && docker compose logs -f

Open 4222 and 6380 inbound in your provider's firewall if you have not: both
are TLS with a 32-byte credential, and nothing else on this box is exposed.
Leave 6379 closed. That is the plaintext Redis port, and it stays on the
container network for a worker running on this same machine.

Re-run this script any time. It adopts what is here and rotates nothing.
DONE
}

main() {
  parse_args "$@"
  require_args

  if [ "$PRINT_ENV_ONLY" = "true" ]; then
    print_env
    return 0
  fi
  if [ "$VERIFY_ONLY" = "true" ]; then
    verify
    print_env
    return 0
  fi

  preflight
  install_docker
  install_certbot
  ensure_group
  run mkdir -p "$INSTALL_DIR" "$CERT_DIR"
  ensure_secrets
  install_hook
  obtain_cert
  start_stack
  if [ "$DRY_RUN" = "true" ]; then
    log ""
    log "--dry-run: nothing was changed."
    return 0
  fi
  wait_healthy
  verify
  print_env
  finish
  return 0
}

main "$@"
