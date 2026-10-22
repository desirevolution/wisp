#!/bin/sh
# Exercise the Docker backend's module-preload/build boundary with a cold cache.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
stage=$(mktemp -d "${TMPDIR:-/tmp}/wisp-go-targets.XXXXXX")
cleanup() {
  chmod -R u+w "$stage" 2>/dev/null || true
  rm -rf "$stage"
}
trap cleanup 0
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
cp "$ROOT/go.mod" "$ROOT/go.sum" "$stage/"
mkdir "$stage/modules"
export GOMODCACHE="$stage/modules" GOWORK=off
cd "$stage"
# Only the manifests are present during preloading, exactly as in Docker.
go mod download all
go mod verify
cp -R "$ROOT/cmd" "$ROOT/internal" .
for arch in amd64 arm64; do
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" GOPROXY=off \
    go build -mod=readonly -buildvcs=false -trimpath -o "$stage/wisp-$arch" ./cmd/wisp
  printf 'Cold-cache offline build passed: linux/%s\n' "$arch"
done
