#!/usr/bin/env bash
# Install or update ErrorLens on the shared server.
#
#   ./deploy/server/deploy.sh <ssh-host>            # pull the published image
#   ./deploy/server/deploy.sh <ssh-host> --local    # build here, copy over SSH
#
# --local needs no registry access on the server: the image is built for
# linux/amd64 on this machine and streamed with `docker save | docker load`.
#
# Creates /srv/errorlens, generates secrets once (they never leave the server),
# starts the stack, installs the Caddy route and restarts the edge Caddy only if
# the route changed.
set -euo pipefail
HOST=${1:?usage: deploy.sh <ssh-host> [--local]}
MODE=${2:-}
EDGE_CADDY=${EDGE_CADDY:?set EDGE_CADDY (the edge Caddy container) or put it in deploy/server/.deploy.env}
EDGE_CONFIG=${EDGE_CONFIG:?set EDGE_CONFIG (host dir the edge Caddy imports *.caddy from)}
DIR=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$DIR/../.." && pwd)

IMAGE=${ERRORLENS_IMAGE:-errorlens/errorlens:latest}
PULL=always
if [ "$MODE" = "--local" ]; then
  REV=$(git -C "$ROOT" rev-parse --short HEAD)
  IMAGE="errorlens-local:$REV"
  PULL=never
  docker buildx build --platform linux/amd64 -f "$ROOT/deploy/Dockerfile" \
    --build-arg VERSION="$REV" --build-arg COMMIT="$REV" -t "$IMAGE" --load "$ROOT"
  docker save "$IMAGE" | gzip -1 | ssh "$HOST" 'gunzip | docker load'
fi

ssh "$HOST" 'mkdir -p /srv/errorlens && chmod 700 /srv/errorlens'
scp -q "$DIR/docker-compose.yml" "$HOST:/srv/errorlens/docker-compose.yml"
scp -q "$DIR/errorlens.caddy" "$HOST:/srv/errorlens/errorlens.caddy"
ssh "$HOST" IMAGE="$IMAGE" PULL="$PULL" EDGE_CADDY="$EDGE_CADDY" EDGE_CONFIG="$EDGE_CONFIG" 'bash -s' <<'REMOTE'
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
# Record which image runs, so a plain `docker compose up -d` keeps it.
grep -v '^ERRORLENS_IMAGE=' .env > .env.tmp || true
echo "ERRORLENS_IMAGE=$IMAGE" >> .env.tmp
mv .env.tmp .env && chmod 600 .env
docker compose up -d --pull "$PULL"
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
