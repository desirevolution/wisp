#!/bin/sh
# No local Node/browser installation needed. Docker Compose builds the runner.
set -eu
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_dir"
mkdir -p screenshots
# Write screenshots with the invoking user's ownership on Linux.
LOCAL_UID=$(id -u)
LOCAL_GID=$(id -g)
export LOCAL_UID LOCAL_GID
exec docker compose -f compose.screenshots.yml run --rm --build screenshots "$@"
