#!/usr/bin/env bash
# Build the React UI and a single Go server that embeds it.
# Output: ./bin/gonetlogo-server
# Optional: ./build.sh --desktop also writes ./bin/gonetlogo (Fyne, needs cgo).
set -euo pipefail

root=$(cd "$(dirname "$0")" && pwd)
cd "$root"

desktop=0
for arg in "$@"; do
  case "$arg" in
    --desktop) desktop=1 ;;
    -h|--help)
      echo "usage: ./build.sh [--desktop]"
      echo "  builds ./bin/gonetlogo-server (web UI + GraphQL on one port)"
      echo "  --desktop also builds ./bin/gonetlogo, the Fyne window"
      exit 0
      ;;
    *)
      echo "build.sh: unknown argument: $arg" >&2
      echo "usage: ./build.sh [--desktop]" >&2
      exit 1
      ;;
  esac
done

if ! command -v go >/dev/null 2>&1; then
  echo "build.sh: go is not on PATH. Run ./setup.sh first." >&2
  exit 1
fi
if ! command -v npm >/dev/null 2>&1; then
  echo "build.sh: npm is not on PATH. Run ./setup.sh first." >&2
  exit 1
fi

echo "building frontend"
(
  cd "$root/frontend"
  npm ci
  npm run build
)
if [[ ! -f "$root/frontend/dist/index.html" ]]; then
  echo "build.sh: frontend/dist/index.html is missing after npm run build" >&2
  exit 1
fi

# -tags web embeds this directory. It is gitignored and removed after compile.
webdist="$root/backend/cmd/server/webdist"
rm -rf "$webdist"
mkdir -p "$webdist"
cp -a "$root/frontend/dist/." "$webdist/"
cleanup() { rm -rf "$webdist"; }
trap cleanup EXIT

mkdir -p "$root/bin"
echo "building bin/gonetlogo-server"
CGO_ENABLED=0 go build -tags web -o "$root/bin/gonetlogo-server" ./backend/cmd/server

if [[ "$desktop" -eq 1 ]]; then
  echo "building bin/gonetlogo (desktop)"
  CGO_ENABLED=1 go build -o "$root/bin/gonetlogo" ./cmd/gonetlogo
fi

echo
echo "Built $root/bin/gonetlogo-server"
if [[ "$desktop" -eq 1 ]]; then
  echo "Built $root/bin/gonetlogo"
fi
echo "Start it with ./startup.sh"
