#!/usr/bin/env bash
# Install GoNetLogo's native build dependencies on Ubuntu 22.04 or 24.04.
# Apt supplies the C toolchain and Fyne headers. Go and Node.js come from
# their official tarballs when the copies already on PATH are too old.
# Safe to run more than once: anything already new enough is left alone.
set -euo pipefail

root=$(cd "$(dirname "$0")" && pwd)
cd "$root"

# Temp downloads. The EXIT trap covers a failed install; success paths delete it too.
workdir=
cleanup() {
  if [[ -n "${workdir}" ]]; then
    rm -rf "$workdir"
    workdir=
  fi
}
trap cleanup EXIT

if [[ -r /etc/os-release ]]; then
  # shellcheck disable=SC1091
  . /etc/os-release
else
  echo "setup.sh: /etc/os-release is missing; this script supports Ubuntu 22.04 and 24.04." >&2
  exit 1
fi
if [[ "${ID:-}" != "ubuntu" ]]; then
  echo "setup.sh: this script supports Ubuntu 22.04 and 24.04 (found ${ID:-unknown})." >&2
  exit 1
fi
case "${VERSION_ID:-}" in
  22.04|24.04) ;;
  *)
    echo "setup.sh: warning: tested on Ubuntu 22.04 and 24.04; this host is ${VERSION_ID:-unknown}. Continuing." >&2
    ;;
esac

as_root() {
  if [[ "$(id -u)" -eq 0 ]]; then
    "$@"
  else
    sudo "$@"
  fi
}

version_ge() {
  # True when $1 is greater than or equal to $2. A leading "v" is ignored.
  local a="${1#v}" b="${2#v}"
  [[ "$a" == "$b" ]] && return 0
  local first
  first=$(printf '%s\n%s\n' "$a" "$b" | sort -V | head -n1)
  [[ "$first" == "$b" ]]
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
  # "go version go1.22.2 linux/amd64"
  local ver="${line#go version go}"
  ver="${ver%% *}"
  [[ -n "$ver" ]] || return 1
  printf '%s\n' "$ver"
}

# Vite 8 (frontend/package.json) needs Node.js 20.19+ or 22.12+.
# Node 22 is what the rest of this repo documents.
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

install_apt() {
  local pkg missing=()
  local pkgs=(
    build-essential
    gcc
    pkg-config
    libgl1-mesa-dev
    xorg-dev
    ca-certificates
    curl
    tar
    xz-utils
  )
  for pkg in "${pkgs[@]}"; do
    if ! dpkg-query -W -f='${Status}' "$pkg" 2>/dev/null | grep -q 'install ok installed'; then
      missing+=("$pkg")
    fi
  done
  if [[ ${#missing[@]} -eq 0 ]]; then
    echo "apt: build tools, OpenGL, and X11 headers already installed"
    return 0
  fi
  echo "apt: installing ${missing[*]}"
  export DEBIAN_FRONTEND=noninteractive
  as_root apt-get update
  as_root apt-get install -y "${missing[@]}"
}

cpu_arch() {
  case "$(uname -m)" in
    x86_64) printf 'amd64\n' ;;
    aarch64|arm64) printf 'arm64\n' ;;
    *)
      echo "setup.sh: unsupported architecture $(uname -m)" >&2
      exit 1
      ;;
  esac
}

node_arch() {
  case "$(uname -m)" in
    x86_64) printf 'x64\n' ;;
    aarch64|arm64) printf 'arm64\n' ;;
    *)
      echo "setup.sh: unsupported architecture $(uname -m)" >&2
      exit 1
      ;;
  esac
}

require_python() {
  if command -v python3 >/dev/null 2>&1; then
    return 0
  fi
  echo "apt: installing python3 (used to read the Go and Node release indexes)"
  export DEBIAN_FRONTEND=noninteractive
  as_root apt-get update
  as_root apt-get install -y python3 ca-certificates
}

install_go() {
  local need have
  need=$(go_required)
  if [[ -z "$need" ]]; then
    echo "setup.sh: go.mod has no go version" >&2
    exit 1
  fi
  if have=$(go_installed) && version_ge "$have" "$need"; then
    echo "Go: $(go version) already satisfies go.mod ($need)"
    return 0
  fi
  require_python
  local arch
  arch=$(cpu_arch)
  echo "Go: installing the latest stable release (>= $need) from go.dev"
  local meta
  meta=$(python3 - "$need" "$arch" <<'PY'
import json, sys, urllib.request
need, arch = sys.argv[1], sys.argv[2]

def parse(v):
    core = v[2:] if v.startswith("go") else v
    core = core.split("rc")[0].split("beta")[0]
    return tuple(int(p) for p in core.split(".") if p.isdigit())

need_t = parse("go" + need)
req = urllib.request.Request("https://go.dev/dl/?mode=json", headers={"User-Agent": "gonetlogo-setup"})
with urllib.request.urlopen(req, timeout=60) as resp:
    releases = json.load(resp)
for rel in releases:
    if not rel.get("stable"):
        continue
    if parse(rel["version"]) < need_t:
        continue
    for f in rel["files"]:
        if f.get("os") == "linux" and f.get("arch") == arch and f.get("kind") == "archive" and f["filename"].endswith(".tar.gz"):
            print(rel["version"])
            print(f["filename"])
            print(f["sha256"])
            sys.exit(0)
sys.exit("no stable Go tarball >= %s for linux/%s" % (need, arch))
PY
)
  local version filename sha
  version=$(printf '%s\n' "$meta" | sed -n '1p')
  filename=$(printf '%s\n' "$meta" | sed -n '2p')
  sha=$(printf '%s\n' "$meta" | sed -n '3p')
  workdir=$(mktemp -d)
  echo "Go: downloading ${filename}"
  curl -fsSL "https://go.dev/dl/${filename}" -o "$workdir/$filename"
  echo "${sha}  $workdir/$filename" | sha256sum -c -
  echo "Go: installing ${version} into /usr/local/go"
  as_root rm -rf /usr/local/go
  as_root tar -C /usr/local -xzf "$workdir/$filename"
  cleanup
  install_path_profile
  export PATH="/usr/local/go/bin:${PATH}"
  hash -r || true
  echo "Go: $(/usr/local/go/bin/go version) installed"
}

install_node() {
  if node_ok; then
    echo "Node.js: $(node -v) and npm $(npm -v) already satisfy Vite"
    return 0
  fi
  require_python
  local arch
  arch=$(node_arch)
  echo "Node.js: installing the latest Node.js 22 release from nodejs.org"
  local meta
  meta=$(python3 - "$arch" <<'PY'
import json, sys, urllib.request
arch = sys.argv[1]
req = urllib.request.Request("https://nodejs.org/dist/index.json", headers={"User-Agent": "gonetlogo-setup"})
with urllib.request.urlopen(req, timeout=60) as resp:
    releases = json.load(resp)
for rel in releases:
    ver = rel["version"]
    if not ver.startswith("v22."):
        continue
    if rel.get("lts") in (False, None, ""):
        # Prefer a 22.x LTS build. Fall through if this entry is not LTS.
        continue
    name = "node-%s-linux-%s.tar.xz" % (ver, arch)
    print(ver)
    print(name)
    sys.exit(0)
# Node 22 may have left LTS. Take the newest 22.x anyway.
for rel in releases:
    ver = rel["version"]
    if ver.startswith("v22."):
        name = "node-%s-linux-%s.tar.xz" % (ver, arch)
        print(ver)
        print(name)
        sys.exit(0)
sys.exit("no Node.js 22 tarball in the nodejs.org index")
PY
)
  local version filename
  version=$(printf '%s\n' "$meta" | sed -n '1p')
  filename=$(printf '%s\n' "$meta" | sed -n '2p')
  workdir=$(mktemp -d)
  echo "Node.js: downloading ${filename}"
  curl -fsSL "https://nodejs.org/dist/${version}/${filename}" -o "$workdir/$filename"
  curl -fsSL "https://nodejs.org/dist/${version}/SHASUMS256.txt" -o "$workdir/SHASUMS256.txt"
  (
    cd "$workdir"
    grep " ${filename}$" SHASUMS256.txt | sha256sum -c -
  )
  as_root tar -C /usr/local --strip-components=1 -xJf "$workdir/$filename"
  cleanup
  install_path_profile
  export PATH="/usr/local/bin:${PATH}"
  hash -r || true
  if ! node_ok; then
    echo "setup.sh: node on PATH is still $(command -v node) ($(node -v 2>/dev/null || echo missing))." >&2
    echo "setup.sh: open a new shell, or prepend /usr/local/bin." >&2
    exit 1
  fi
  echo "Node.js: $(node -v) and npm $(npm -v) installed"
}

install_path_profile() {
  local dest=/etc/profile.d/gonetlogo.sh
  local body
  body=$(cat <<'EOF'
# Added by GoNetLogo setup.sh. Official Go and Node installs live here.
export PATH="/usr/local/go/bin:/usr/local/bin:${PATH}"
EOF
)
  if [[ -f "$dest" ]] && [[ "$(cat "$dest")" == "$body" ]]; then
    return 0
  fi
  printf '%s\n' "$body" | as_root tee "$dest" >/dev/null
  as_root chmod 644 "$dest"
}

install_apt
install_go
install_node

echo
echo "Ready to build. From the repo root:"
echo "  ./build.sh"
echo "  ./startup.sh"
echo "Optional desktop window: ./build.sh --desktop && ./bin/gonetlogo"
