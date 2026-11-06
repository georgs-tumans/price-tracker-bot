#!/usr/bin/env bash
# Updates to the latest release (or the version set in IMAGE_TAG):
#  1. pulls the latest deployment files (compose file, scripts) from git,
#  2. pulls the image,
#  3. recreates the container if the image changed.
# There's no need to stop the bot first: compose replaces the container in one step, so the bot is only
# down for a few seconds, and nothing happens if it's already up to date.
# shellcheck source=deployment/common.sh
source "$(dirname "$0")/common.sh"

if [[ "${UPDATE_SH_REEXEC:-}" != "1" ]]; then
    echo "Pulling the latest deployment files..."
    git pull --ff-only

    # Continue in the freshly pulled version of this script (common.sh already switched to its directory)
    UPDATE_SH_REEXEC=1 exec bash ./update.sh "$@"
fi

echo "Pulling the image..."
compose pull

remove_legacy_container
compose up -d
show_status

# Give a recreated container a moment to log its startup line
sleep 3
echo "Running: $(docker logs "$container_name" 2>&1 | grep -o 'Starting bot service (version [^)]*)' | tail -1 || true)"
