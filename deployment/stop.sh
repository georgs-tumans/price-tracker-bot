#!/usr/bin/env bash
# Stops and removes the bot container. The image is kept, so start.sh can bring it back quickly.
set -euo pipefail

cd "$(dirname "$0")"

docker compose down
