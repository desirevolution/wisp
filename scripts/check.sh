#!/bin/sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"
node scripts/check-version.mjs
npm test
npm run test:dom
npm run build

# Test the same embedded frontend that the production Docker build uses.
backup=$(mktemp -d "${TMPDIR:-/tmp}/wisp-check.XXXXXX")
cp -a internal/webui/public "$backup/public"
cleanup() {
  rm -rf internal/webui/public
  cp -a "$backup/public" internal/webui/public
  rm -rf "$backup"
}
trap cleanup 0
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
rm -rf internal/webui/public
cp -a public internal/webui/public

go mod verify
go test ./...
go vet ./...
CGO_ENABLED=1 go test -race ./internal/events ./internal/store ./internal/whatsapp ./internal/httpapi ./internal/pushsvc
CGO_ENABLED=0 go build -buildvcs=false -trimpath -o "$backup/wisp" ./cmd/wisp
"$backup/wisp" --version
npm run test:browser
printf '%s\n' 'All automated checks passed. Live WhatsApp and Docker architecture smoke tests remain manual.'
