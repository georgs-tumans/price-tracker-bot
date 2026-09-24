#!/usr/bin/env bash
# Stops and removes the bot container. The tracker state volume and the image are kept.
# shellcheck source=deployment/common.sh
source "$(dirname "$0")/common.sh"

compose down
