#!/usr/bin/env bash
# Builds the bot image from the current code and (re)starts the container in the background.
# Run this after every `git pull` to deploy the new version.
set -euo pipefail

cd "$(dirname "$0")"

container_name="price_tracker_bot"

# One-time migration: a container with the same name created by the old `docker run` scripts
# (i.e. not managed by compose) would block compose from creating its own.
if docker ps -a --format '{{.Names}}' | grep -qx "$container_name" \
    && [ -z "$(docker inspect -f '{{ index .Config.Labels "com.docker.compose.project" }}' "$container_name")" ]; then
    echo "Removing old container '$container_name' created outside of docker compose"
    docker rm -f "$container_name"
fi

docker compose up -d --build

echo
docker compose ps
echo
echo "Follow the logs with: docker logs -f $container_name"
