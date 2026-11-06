#!/usr/bin/env bash
# Shared setup for start.sh, stop.sh and update.sh; not meant to be run on its own.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

container_name="price_tracker_bot"

if [[ ! -f ../.env ]]; then
    echo "Missing .env in the project root; create it from .env.example first" >&2
    exit 1
fi

# The root .env configures the container and also provides IMAGE_TAG for the compose file
compose() {
    docker compose --env-file ../.env "$@"
}

# One-time migration: a container with the same name created by the old `docker run` scripts
# (i.e. not managed by compose) would block compose from creating its own.
remove_legacy_container() {
    if docker ps -a --format '{{.Names}}' | grep -qx "$container_name" \
        && [[ -z "$(docker inspect -f '{{ index .Config.Labels "com.docker.compose.project" }}' "$container_name")" ]]; then
        echo "Removing old container '$container_name' created outside of docker compose"
        docker rm -f "$container_name"
    fi
}

show_status() {
    echo
    compose ps
    echo
    echo "Follow the logs with: docker logs -f $container_name"
}
