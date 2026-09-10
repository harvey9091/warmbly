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
#
# It is widened to a group, not to the world. The compose file puts both
# containers in gid 2000 with group_add, so each can read the key and nothing
# else on the machine can. Create it once:
#
#   groupadd -g 2000 warmbly-certs
#
# This matters more than it looks: the same guide suggests running the first
# worker on this box, and a world-readable private key is readable by that
# container too.
set -eu

CERT_GID="${WARMBLY_CERT_GID:-2000}"

DOMAIN="${WARMBLY_BUS_DOMAIN:-bus.example.com}"
SRC="/etc/letsencrypt/live/$DOMAIN"
DEST="/opt/warmbly/certs"

[ -d "$SRC" ] || { echo "no certificate at $SRC" >&2; exit 1; }

mkdir -p "$DEST"
cp "$SRC/fullchain.pem" "$DEST/fullchain.pem"
cp "$SRC/chain.pem"     "$DEST/chain.pem"
cp "$SRC/privkey.pem"   "$DEST/privkey.pem"

# The chain is public by definition. The key is not: group-readable only, and
# only for the group the two containers are added to.
chmod 0644 "$DEST/fullchain.pem" "$DEST/chain.pem"
chgrp "$CERT_GID" "$DEST/privkey.pem" || {
  echo "no group $CERT_GID; create it with: groupadd -g $CERT_GID warmbly-certs" >&2
  exit 1
}
chmod 0640 "$DEST/privkey.pem"

# Both hold the certificate open and neither re-reads it on its own.
cd /opt/warmbly/bus && docker compose restart nats redis
