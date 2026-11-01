#!/usr/bin/env bash
# Build the React UI, the web server, and the Fyne desktop app for darwin/arm64.
# Output: ./bin/gonetlogo-server and ./bin/gonetlogo
# Optional: ./build-mac.sh --app writes ./bin/GoNetLogo.app when the fyne CLI is on PATH.
# macOS ships bash 3.2 and BSD tools. No associative arrays, mapfile, or ${var,,}.
set -euo pipefail

root=$(cd "$(dirname "$0")" && pwd)
cd "$root"

app=0
for arg in "$@"; do
  case "$arg" in
    --app|--package) app=1 ;;
    --desktop) ;;
    -h|--help)
      echo "usage: ./build-mac.sh [--app]"
      echo "  builds ./bin/gonetlogo-server and ./bin/gonetlogo (darwin/arm64, cgo)"
      echo "  --desktop is accepted; the desktop binary is always built"
      echo "  --app also writes ./bin/GoNetLogo.app when the fyne CLI is on PATH"
      exit 0
      ;;
    *)
      echo "build-mac.sh: unknown argument: $arg" >&2
      echo "usage: ./build-mac.sh [--app]" >&2
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
if ! command -v clang >/dev/null 2>&1; then
  echo "build-mac.sh: clang is not on PATH. Run ./setup-mac.sh first." >&2
  exit 1
fi

webdist=
work=
app_staged=
cleanup() {
  if [[ -n "$webdist" ]]; then
    rm -rf "$webdist"
  fi
  if [[ -n "$work" ]]; then
    rm -rf "$work"
  fi
  if [[ -n "$app_staged" && -d "$root/GoNetLogo.app" ]]; then
    rm -rf "$root/GoNetLogo.app"
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
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -tags web -o "$root/bin/gonetlogo-server" ./backend/cmd/server

echo "building bin/gonetlogo (desktop)"
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -o "$root/bin/gonetlogo" ./cmd/gonetlogo

# fyne package requires a PNG. This is a plain placeholder so --app can run
# without an Icon.png in the repo. A real Icon.png next to cmd/gonetlogo wins.
write_icon() {
  local dest="$1"
  local b64="iVBORw0KGgoAAAANSUhEUgAAAIAAAACACAIAAABMXPacAAAAyUlEQVR42u3RMQ0AAAgEsffBgjL8y0AGDE1OwTU1rcNiAQAAAgBAAAAIAAABACAAAAQAgAAAEAAAAgBAAAAIAAABACAAAAQAgAAAEAAAAgBAAAAIAAABACAAAAQAgAAAEAAAAgBAAAAIAAABACAAAAQAgAAAAOACAAACAEAAAAgAAAEAIAAABACAAAAQAAACAEAAAAgAAAEAIAAABACAAAAQAAACAEAAAAgAAAEAIAAABACAAAAQAAACAEAAAAgAAAEAIAAABOBDC0SJQjti8mrhAAAAAElFTkSuQmCC"
  rm -f "$dest"
  if printf '%s' "$b64" | openssl base64 -d -A >"$dest" 2>/dev/null && [[ -s "$dest" ]]; then
    return 0
  fi
  rm -f "$dest"
  if printf '%s' "$b64" | base64 -D >"$dest" 2>/dev/null && [[ -s "$dest" ]]; then
    return 0
  fi
  rm -f "$dest"
  if printf '%s' "$b64" | base64 -d >"$dest" 2>/dev/null && [[ -s "$dest" ]]; then
    return 0
  fi
  echo "build-mac.sh: could not decode a PNG icon for fyne package" >&2
  return 1
}

package_app() {
  local fyne icon
  if ! command -v fyne >/dev/null 2>&1; then
    echo "build-mac.sh: warning: fyne is not on PATH, so bin/GoNetLogo.app was not created." >&2
    echo "The binaries are in ${root}/bin. The fyne CLI is optional. Install it with:" >&2
    echo "  go install fyne.io/fyne/v2/cmd/fyne@v2.5.4" >&2
    echo "Then re-run ./build-mac.sh --app" >&2
    return 0
  fi
  fyne=$(command -v fyne)
  work=$(mktemp -d)
  if [[ -f "$root/cmd/gonetlogo/Icon.png" ]]; then
    icon="$root/cmd/gonetlogo/Icon.png"
  elif [[ -f "$root/Icon.png" ]]; then
    icon="$root/Icon.png"
  else
    icon="$work/Icon.png"
    write_icon "$icon"
  fi
  echo "packaging bin/GoNetLogo.app"
  app_staged=1
  (
    cd "$root" || exit 1
    "$fyne" package -os darwin \
      -executable "$root/bin/gonetlogo" \
      -name GoNetLogo \
      -src "$root/cmd/gonetlogo" \
      -icon "$icon" \
      -appID com.gonetlogo.desktop
  )
  if [[ ! -d "$root/GoNetLogo.app" ]]; then
    echo "build-mac.sh: fyne package did not write GoNetLogo.app" >&2
    exit 1
  fi
  rm -rf "$root/bin/GoNetLogo.app"
  mv "$root/GoNetLogo.app" "$root/bin/GoNetLogo.app"
  app_staged=
  echo "Built $root/bin/GoNetLogo.app"
}

echo
echo "Built $root/bin/gonetlogo-server"
echo "Built $root/bin/gonetlogo"
if [[ "$app" -eq 1 ]]; then
  package_app
fi
if [[ "$app" -eq 1 && -d "$root/bin/GoNetLogo.app" ]]; then
  echo "Built $root/bin/GoNetLogo.app"
fi
echo "Start it with ./startup-mac.sh"
