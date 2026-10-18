#!/bin/sh
set -eu
version="${VERSION:-}"
if [ -z "$version" ]; then
  version=$(git describe --tags --exact-match 2>/dev/null || printf dev)
fi
# Keep values safe for linker flags, Docker build arguments and output files.
case "$version" in
  ''|*[!A-Za-z0-9._+-]*) echo 'Invalid VERSION: use a release tag without whitespace or shell characters' >&2; exit 1 ;;
esac
if [ "${#version}" -gt 128 ]; then echo 'VERSION is too long' >&2; exit 1; fi
printf '%s\n' "$version"
