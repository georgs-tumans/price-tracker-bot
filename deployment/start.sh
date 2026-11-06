#!/usr/bin/env bash
# Starts the bot in the background. Pulls the configured image first if it isn't on this machine yet.
# To get a newer release, use update.sh instead.
# shellcheck source=deployment/common.sh
source "$(dirname "$0")/common.sh"

remove_legacy_container
compose up -d
show_status
