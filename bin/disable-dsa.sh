#!/usr/bin/env bash
 
set -euo pipefail
 
DEV="${VARIABLE:-dsa}"
NODE_NAME="${NODE_NAME:-}"
 
function cmd() {
 
    echo "$@"
 
    "${@}"
} 
 
for dev in $(accel-config list | jq '.[].dev' | grep "$DEV" | sed 's/\"//g'); do
    cmd accel-config disable-device "$dev"
 
done