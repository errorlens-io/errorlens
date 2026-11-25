#!/usr/bin/env bash
# Install or update ErrorLens on the shared server.
#   ./deploy/server/deploy.sh <ssh-host>          # first install or update
# Creates /srv/errorlens, generates secrets once, pulls the image, starts it,
# installs the Caddy route and restarts the edge Caddy only if the route changed.
set -euo pipefail
HOST=${1:?usage: deploy.sh <ssh-host>}
EDGE_CADDY=${EDGE_CADDY:?set EDGE_CADDY (the edge Caddy container) or put it in deploy/server/.deploy.env}
EDGE_CONFIG=${EDGE_CONFIG:?set EDGE_CONFIG (host dir the edge Caddy imports *.caddy from)}
DIR=$(cd "$(dirname "$0")" && pwd)

ssh "$HOST" 'mkdir -p /srv/errorlens && chmod 700 /srv/errorlens'
scp -q "$DIR/docker-compose.yml" "$HOST:/srv/errorlens/docker-compose.yml"
scp -q "$DIR/errorlens.caddy" "$HOST:/srv/errorlens/errorlens.caddy"
ssh "$HOST" EDGE_CADDY="$EDGE_CADDY" EDGE_CONFIG="$EDGE_CONFIG" 'bash -s' <<'REMOTE'
set -euo pipefail
cd /srv/errorlens
if [ ! -f .env ]; then
  umask 077
  {
    echo "POSTGRES_PASSWORD=$(openssl rand -hex 24)"
    echo "ERRORLENS_ENCRYPTION_KEY=$(openssl rand -base64 32)"
    echo "ERRORLENS_PUBLIC_URL=https://errorlens.io"
  } > .env
  echo "created /srv/errorlens/.env (back up ERRORLENS_ENCRYPTION_KEY)"
fi
docker compose pull -q
docker compose up -d
for i in $(seq 1 30); do
  docker exec errorlens /usr/local/bin/errorlens version >/dev/null 2>&1 && break; sleep 2
done
if ! cmp -s errorlens.caddy "$EDGE_CONFIG/errorlens.caddy"; then
  cp errorlens.caddy "$EDGE_CONFIG/errorlens.caddy"
  docker restart "$EDGE_CADDY" >/dev/null
  echo "route installed; restarted $EDGE_CADDY"
fi
docker compose ps
REMOTE
