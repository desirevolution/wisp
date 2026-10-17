#!/bin/sh
# Run from any working directory; arguments are passed to the screenshot runner.
set -eu
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_dir"
exec npm run screenshots -- "$@"
