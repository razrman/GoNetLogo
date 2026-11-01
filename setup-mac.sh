#!/usr/bin/env bash
# Install GoNetLogo's native build dependencies on macOS (Apple Silicon).
# Xcode Command Line Tools supply clang for cgo. Homebrew supplies Go, Node.js,
# and pkg-config. Fyne on macOS does not need X11 or Mesa headers.
# Safe to run more than once: anything already new enough is left alone.
# macOS ships bash 3.2 and BSD tools. No associative arrays, mapfile, or ${var,,}.
set -euo pipefail

root=$(cd "$(dirname "$0")" && pwd)
cd "$root"

if [[ $# -gt 0 ]]; then
  case "$1" in
    -h|--help)
      echo "usage: ./setup-mac.sh"
      echo "  installs Xcode Command Line Tools (if missing), checks Homebrew,"
      echo "  and installs Go, Node.js, pkg-config, and the frontend dependencies"
      exit 0
      ;;
    *)
      echo "setup-mac.sh: unknown argument: $1" >&2
      echo "usage: ./setup-mac.sh" >&2
      exit 1
      ;;
  esac
fi

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "setup-mac.sh: this script supports macOS (found $(uname -s)). On Linux, use ./setup.sh." >&2
  exit 1
fi
if [[ "$(uname -m)" != "arm64" ]]; then
  echo "setup-mac.sh: this script targets Apple Silicon (arm64); this Mac is $(uname -m)." >&2
  exit 1
fi
if [[ "$(id -u)" -eq 0 ]]; then
  echo "setup-mac.sh: run this as the user that owns Homebrew, not as root." >&2
  exit 1
fi

if command -v sw_vers >/dev/null 2>&1; then
  echo "macOS: $(sw_vers -productVersion) ($(uname -m))"
fi

# True when $1 is greater than or equal to $2. A leading "v" or "go" is ignored.
# Compared numerically per dot component so BSD sort (no -V) is enough.
version_ge() {
  local a="${1#v}" b="${2#v}"
  a="${a#go}"
  b="${b#go}"
  a="${a%%rc*}"
  b="${b%%rc*}"
  a="${a%%beta*}"
  b="${b%%beta*}"
  local IFS=.
  local a1 a2 a3 b1 b2 b3
  # shellcheck disable=SC2086
  set -- $a
  a1=${1:-0}
  a2=${2:-0}
  a3=${3:-0}
  # shellcheck disable=SC2086
  set -- $b
  b1=${1:-0}
  b2=${2:-0}
  b3=${3:-0}
  a1=${a1%%[^0-9]*}
  a2=${a2%%[^0-9]*}
  a3=${a3%%[^0-9]*}
  b1=${b1%%[^0-9]*}
  b2=${b2%%[^0-9]*}
  b3=${b3%%[^0-9]*}
  a1=${a1:-0}
  a2=${a2:-0}
  a3=${a3:-0}
  b1=${b1:-0}
  b2=${b2:-0}
  b3=${b3:-0}
  if ((10#$a1 > 10#$b1)); then return 0; fi
  if ((10#$a1 < 10#$b1)); then return 1; fi
  if ((10#$a2 > 10#$b2)); then return 0; fi
  if ((10#$a2 < 10#$b2)); then return 1; fi
  if ((10#$a3 >= 10#$b3)); then return 0; fi
  return 1
}

go_required() {
  awk '/^go [0-9]/ { print $2; exit }' "$root/go.mod"
}

go_installed() {
  if ! command -v go >/dev/null 2>&1; then
    return 1
  fi
  local line
  line=$(go version 2>/dev/null || true)
  # "go version go1.22.2 darwin/arm64"
  local ver="${line#go version go}"
  ver="${ver%% *}"
  [[ -n "$ver" ]] || return 1
  printf '%s\n' "$ver"
}

go_native() {
  local line
  line=$(go version 2>/dev/null || true)
  [[ "$line" == *" darwin/arm64" ]]
}

# Vite 8 (frontend/package.json) needs Node.js 20.19+ or 22.12+.
node_ok() {
  command -v node >/dev/null 2>&1 || return 1
  command -v npm >/dev/null 2>&1 || return 1
  local ver
  ver=$(node -v 2>/dev/null || true)
  ver="${ver#v}"
  [[ -n "$ver" ]] || return 1
  local major=${ver%%.*}
  local rest=${ver#*.}
  local minor=${rest%%.*}
  if [[ "$major" -gt 22 ]]; then
    return 0
  fi
  if [[ "$major" -eq 22 && "$minor" -ge 12 ]]; then
    return 0
  fi
  if [[ "$major" -eq 20 && "$minor" -ge 19 ]]; then
    return 0
  fi
  return 1
}

node_native() {
  local arch
  arch=$(node -p process.arch 2>/dev/null || true)
  [[ "$arch" == "arm64" ]]
}

require_clt() {
  if xcode-select -p >/dev/null 2>&1; then
    echo "Xcode Command Line Tools: $(xcode-select -p)"
    return 0
  fi
  echo "setup-mac.sh: Xcode Command Line Tools are not installed." >&2
  echo "A dialog will open. When it finishes, run ./setup-mac.sh again." >&2
  xcode-select --install || true
  exit 1
}

require_clang() {
  if ! command -v clang >/dev/null 2>&1; then
    echo "setup-mac.sh: clang is not on PATH. Re-run after the Command Line Tools finish installing." >&2
    exit 1
  fi
  echo "clang: $(clang --version | head -n 1)"
  echo "Fyne: cgo uses this clang. X11 and Mesa packages are not installed on macOS."
}

find_brew() {
  if command -v brew >/dev/null 2>&1; then
    command -v brew
    return 0
  fi
  if [[ -x /opt/homebrew/bin/brew ]]; then
    printf '%s\n' /opt/homebrew/bin/brew
    return 0
  fi
  if [[ -x /usr/local/bin/brew ]]; then
    printf '%s\n' /usr/local/bin/brew
    return 0
  fi
  return 1
}

brew_hint() {
  cat >&2 <<'EOF'
setup-mac.sh: Homebrew is not installed.
Install it, then run ./setup-mac.sh again:
  /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
On Apple Silicon, Homebrew lives in /opt/homebrew. Add it to your shell:
  echo 'eval "$(/opt/homebrew/bin/brew shellenv)"' >> ~/.zprofile
  eval "$(/opt/homebrew/bin/brew shellenv)"
EOF
  exit 1
}

install_shellenv() {
  local prefix="$1"
  local profile="${HOME:-}/.zprofile"
  local marker="${prefix}/bin/brew shellenv"
  if [[ -z "${HOME:-}" ]]; then
    echo "Homebrew: HOME is unset; shellenv was loaded for this run only."
    echo "  eval \"\$(${prefix}/bin/brew shellenv)\""
    return 0
  fi
  if [[ -f "$profile" ]] && grep -F -q "$marker" "$profile"; then
    return 0
  fi
  if [[ ! -w "$HOME" ]]; then
    echo "Homebrew: add this to your shell profile so new terminals find brew:"
    echo "  eval \"\$(${prefix}/bin/brew shellenv)\""
    return 0
  fi
  echo "Homebrew: adding shellenv to ${profile}"
  printf '\n# Added by GoNetLogo setup-mac.sh. Homebrew is not on the default PATH.\n%s\n' \
    "eval \"\$(${prefix}/bin/brew shellenv)\"" >>"$profile"
}

brew_formula_installed() {
  brew list --formula "$1" >/dev/null 2>&1
}

brew_install() {
  local formula="$1"
  if brew_formula_installed "$formula"; then
    echo "brew: ${formula} already installed"
    return 0
  fi
  echo "brew: installing ${formula}"
  brew install "$formula"
}

use_brew_path() {
  export PATH="${BREW_PREFIX}/bin:${PATH}"
  hash -r || true
}

install_go() {
  local need have
  need=$(go_required)
  if [[ -z "$need" ]]; then
    echo "setup-mac.sh: go.mod has no go version" >&2
    exit 1
  fi
  if have=$(go_installed) && go_native && version_ge "$have" "$need"; then
    echo "Go: $(go version) already satisfies go.mod ($need)"
    return 0
  fi
  echo "Go: installing a darwin/arm64 release (>= $need) with Homebrew"
  if brew_formula_installed go; then
    brew upgrade go
  else
    brew install go
  fi
  use_brew_path
  if ! have=$(go_installed) || ! go_native || ! version_ge "$have" "$need"; then
    echo "setup-mac.sh: Go is still $(go version 2>/dev/null || echo missing). Need >= ${need} for darwin/arm64." >&2
    echo "setup-mac.sh: open a new shell, or prepend ${BREW_PREFIX}/bin." >&2
    exit 1
  fi
  echo "Go: $(go version) installed"
}

install_node() {
  if node_ok && node_native; then
    echo "Node.js: $(node -v) and npm $(npm -v) already satisfy Vite"
    return 0
  fi
  echo "Node.js: installing from Homebrew (Vite needs 20.19+, or 22.12+)"
  if brew_formula_installed node; then
    brew upgrade node
  else
    brew install node
  fi
  use_brew_path
  if ! node_ok || ! node_native; then
    echo "setup-mac.sh: node on PATH is still $(command -v node 2>/dev/null || echo missing) ($(node -v 2>/dev/null || echo missing))." >&2
    echo "setup-mac.sh: open a new shell, or prepend ${BREW_PREFIX}/bin." >&2
    exit 1
  fi
  echo "Node.js: $(node -v) and npm $(npm -v) installed"
}

install_frontend() {
  local lock="$root/frontend/package-lock.json"
  local stamp="$root/frontend/node_modules/.package-lock.json"
  if [[ ! -f "$lock" ]]; then
    echo "setup-mac.sh: frontend/package-lock.json is missing" >&2
    exit 1
  fi
  if [[ -d "$root/frontend/node_modules" && -f "$stamp" ]] && cmp -s "$lock" "$stamp"; then
    echo "frontend: dependencies already match package-lock.json"
    return 0
  fi
  echo "frontend: npm ci"
  (
    cd "$root/frontend" || exit 1
    npm ci
  )
}

require_clt
require_clang

brew_bin=
if ! brew_bin=$(find_brew); then
  brew_hint
fi
# shellcheck disable=SC1090
eval "$("$brew_bin" shellenv)"
BREW_PREFIX=$(brew --prefix)
use_brew_path
echo "Homebrew: ${BREW_PREFIX}"
install_shellenv "$BREW_PREFIX"
export NONINTERACTIVE=1
export HOMEBREW_NO_ENV_HINTS=1

if ! command -v curl >/dev/null 2>&1; then
  brew_install curl
  use_brew_path
fi
if ! command -v tar >/dev/null 2>&1; then
  echo "setup-mac.sh: tar is not on PATH" >&2
  exit 1
fi
if command -v pkg-config >/dev/null 2>&1; then
  echo "pkg-config: $(pkg-config --version)"
else
  brew_install pkg-config
  use_brew_path
fi

install_go
install_node
install_frontend

echo
echo "Ready to build. From the repo root:"
echo "  ./build-mac.sh"
echo "  ./startup-mac.sh"
echo "Desktop window: ./build-mac.sh writes ./bin/gonetlogo"
echo "Optional app bundle: ./build-mac.sh --app"
