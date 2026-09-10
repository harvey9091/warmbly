#!/bin/sh
# Certbot deploy hook: publish a renewed certificate where the containers can
# read it, then restart the two services that hold it open.
#
# Install as /etc/letsencrypt/renewal-hooks/deploy/warmbly-bus.sh, mode 0755.
#
# Certbot's own live directory is root-owned and 0700 on the private key, which
# neither container can read: NATS runs as uid 1000 and Redis as uid 999. The
# copy exists to widen that deliberately and in one place, rather than by
# loosening /etc/letsencrypt.
set -eu

DOMAIN="${WARMBLY_BUS_DOMAIN:-bus.example.com}"
SRC="/etc/letsencrypt/live/$DOMAIN"
DEST="/opt/warmbly/certs"

[ -d "$SRC" ] || { echo "no certificate at $SRC" >&2; exit 1; }

mkdir -p "$DEST"
cp "$SRC/fullchain.pem" "$DEST/fullchain.pem"
cp "$SRC/chain.pem"     "$DEST/chain.pem"
cp "$SRC/privkey.pem"   "$DEST/privkey.pem"

# World-readable on a box whose only job is this. Narrow it to a shared group
# if anything else ever runs here.
chmod 0644 "$DEST/fullchain.pem" "$DEST/chain.pem"
chmod 0644 "$DEST/privkey.pem"

# Both hold the certificate open and neither re-reads it on its own.
cd /opt/warmbly/bus && docker compose restart nats redis
