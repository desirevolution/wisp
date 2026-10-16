#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
OUT="${OUT:-$ROOT/dist}"
PROGRESS="${PROGRESS:-plain}"

if [ "$#" -eq 0 ]; then
  set -- linux/amd64 linux/arm64 linux/arm/v7 windows/amd64
fi
# Validate before building or using target names as paths.
for platform do
  case "$platform" in
    linux/amd64|linux/arm64|linux/arm/v6|linux/arm/v7|windows/amd64|windows/arm64) ;;
    *) echo "error: unsupported platform: $platform" >&2; exit 1 ;;
  esac
done

if ! docker buildx version >/dev/null 2>&1; then
  echo "error: Docker Buildx is required" >&2
  exit 1
fi
mkdir -p "$OUT"
temp=$(mktemp -d "${TMPDIR:-/tmp}/wisp-binaries.XXXXXX")
trap 'rm -rf "$temp"' 0
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

count=0
for platform do
  count=$((count + 1))
  os=${platform%%/*}
  rest=${platform#*/}
  arch=${rest%%/*}
  variant=""
  if [ "$rest" != "$arch" ]; then variant=${rest#*/}; fi
  suffix="${os}-${arch}${variant:+-${variant}}"
  exe=""
  if [ "$os" = windows ]; then exe=".exe"; fi
  target="$OUT/wisp-$suffix$exe"
  export_dir="$temp/$suffix"

  echo "==> [$count/$#] $platform -> $target"
  # Build stages run on BUILDPLATFORM; Go cross-compiles via these arguments.
  # The binaries target contains only /wisp, including for Windows builds.
  docker buildx build \
    --progress "$PROGRESS" \
    --build-arg "TARGETOS=$os" \
    --build-arg "TARGETARCH=$arch" \
    --build-arg "TARGETVARIANT=$variant" \
    --target binaries \
    --output "type=local,dest=$export_dir" \
    "$ROOT"

  if [ ! -f "$export_dir/wisp" ]; then
    echo "error: built binary not found for $platform" >&2
    exit 1
  fi
  cp "$export_dir/wisp" "$target"
  chmod +x "$target"
  rm -rf "$export_dir"
  echo "==> Finished $target"
done

echo "==> All $count binaries built successfully."
