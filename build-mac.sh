#!/usr/bin/env bash
# Build the React UI and the web server for darwin/arm64.
# Output: ./bin/gonetlogo-server
# macOS ships bash 3.2 and BSD tools. No associative arrays, mapfile, or ${var,,}.
set -euo pipefail

root=$(cd "$(dirname "$0")" && pwd)
cd "$root"

for arg in "$@"; do
  case "$arg" in
    -h|--help)
      echo "usage: ./build-mac.sh"
      echo "  builds ./bin/gonetlogo-server (darwin/arm64, cgo off)"
      exit 0
      ;;
    *)
      echo "build-mac.sh: unknown argument: $arg" >&2
      echo "usage: ./build-mac.sh" >&2
      exit 1
      ;;
  esac
done

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "build-mac.sh: this script supports macOS (found $(uname -s)). On Linux, use ./build.sh." >&2
  exit 1
fi
if [[ "$(uname -m)" != "arm64" ]]; then
  echo "build-mac.sh: this script targets Apple Silicon (arm64); this Mac is $(uname -m)." >&2
  exit 1
fi

if ! command -v go >/dev/null 2>&1; then
  echo "build-mac.sh: go is not on PATH. Run ./setup-mac.sh first." >&2
  exit 1
fi
if ! command -v npm >/dev/null 2>&1; then
  echo "build-mac.sh: npm is not on PATH. Run ./setup-mac.sh first." >&2
  exit 1
fi

webdist=
cleanup() {
  if [[ -n "$webdist" ]]; then
    rm -rf "$webdist"
  fi
}
trap cleanup EXIT

echo "building frontend"
(
  cd "$root/frontend" || exit 1
  npm ci
  npm run build
)
if [[ ! -f "$root/frontend/dist/index.html" ]]; then
  echo "build-mac.sh: frontend/dist/index.html is missing after npm run build" >&2
  exit 1
fi

# -tags web embeds this directory. It is gitignored and removed after compile.
webdist="$root/backend/cmd/server/webdist"
rm -rf "$webdist"
mkdir -p "$webdist"
cp -R "$root/frontend/dist/." "$webdist/"

mkdir -p "$root/bin"
echo "building bin/gonetlogo-server"
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -tags web -o "$root/bin/gonetlogo-server" ./backend/cmd/server

echo
echo "Built $root/bin/gonetlogo-server"
echo "Start it with ./startup-mac.sh"
