#!/bin/sh
set -eu
npm ci
npm run build
rm -rf internal/webui/public
mkdir -p internal/webui/public
cp -a public/. internal/webui/public/
VERSION=$(sh scripts/build-version.sh)
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X wisp/internal/appconfig.Version=$VERSION" -o wisp ./cmd/wisp
file wisp || true
