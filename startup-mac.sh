#!/usr/bin/env bash
# Start the native GoNetLogo server (web UI and GraphQL on one port).
# Foreground is the default. Background mode records a pid and a log.
# --desktop launches the Fyne window. --open uses open(1) for the browser.
#
#   ./startup-mac.sh                      # foreground, http://localhost:8080
#   PORT=9090 ./startup-mac.sh            # another port
#   HOST=0.0.0.0 ./startup-mac.sh         # listen beyond localhost
#   ./startup-mac.sh --background         # detach; pid and log under ./run
#   ./startup-mac.sh --open               # also open the web UI
#   ./startup-mac.sh --desktop            # Fyne window, no server
#   ./startup-mac.sh --desktop --server   # window and server
#   ./startup-mac.sh stop
#   ./startup-mac.sh status
#
# macOS ships bash 3.2 and BSD tools. No associative arrays, mapfile, or ${var,,}.
# Log level and log file are the same as the binaries: GONETLOGO_LOG_LEVEL and
# GONETLOGO_LOG_FILE. This script does not add -log-level arguments.
set -euo pipefail

root=$(cd "$(dirname "$0")" && pwd)
cd "$root"

HOST="${HOST:-127.0.0.1}"
PORT="${PORT:-8080}"
BIN="${BIN:-$root/bin/gonetlogo-server}"
DESKTOP_BIN="${DESKTOP_BIN:-$root/bin/gonetlogo}"
APP_BUNDLE="${APP_BUNDLE:-$root/bin/GoNetLogo.app}"
RUNDIR="${RUNDIR:-$root/run}"
PIDFILE="${PIDFILE:-$RUNDIR/gonetlogo-server.pid}"
LOGFILE="${LOGFILE:-$RUNDIR/gonetlogo-server.log}"
DESKTOP_LOG="${DESKTOP_LOG:-$RUNDIR/gonetlogo-desktop.log}"

usage() {
  cat <<EOF
usage: ./startup-mac.sh [start] [--background|-d] [--open] [--desktop] [--server]
       ./startup-mac.sh stop
       ./startup-mac.sh status

  HOST         bind address (default 127.0.0.1)
  PORT         bind port (default 8080)
  BIN          server binary (default ./bin/gonetlogo-server)
  DESKTOP_BIN  desktop binary (default ./bin/gonetlogo)

Foreground is the default. Ctrl-C stops the server.
--background writes ${PIDFILE} and ${LOGFILE}.
--open opens the web UI with open(1).
--desktop launches the Fyne window. Alone, it does not start the server.
--server, --background, or --open with --desktop starts the server too.
GONETLOGO_LOG_LEVEL and GONETLOGO_LOG_FILE pass through. This script does not
add -log-level, -log-format, or -log-file; set those on the binary, or use the
environment variables, the same way ./startup.sh does.
EOF
}

# 10# forces decimal. Bash treats a leading zero, such as 08, as octal.
port_raw=$PORT
if [[ ! "$port_raw" =~ ^[0-9]+$ ]]; then
  echo "startup-mac.sh: invalid PORT: $port_raw" >&2
  exit 1
fi
PORT=$((10#$port_raw))
if (( PORT < 1 || PORT > 65535 )); then
  echo "startup-mac.sh: invalid PORT: $port_raw" >&2
  exit 1
fi

if [[ "$HOST" == *:* && "$HOST" != \[*\] ]]; then
  addr="[${HOST}]:${PORT}"
else
  addr="${HOST}:${PORT}"
fi

public_url() {
  local host="$HOST"
  case "$host" in
    0.0.0.0|127.0.0.1|::|::1) host="localhost" ;;
  esac
  if [[ "$host" == *:* && "$host" != \[*\] ]]; then
    printf 'http://[%s]:%s/\n' "$host" "$PORT"
  else
    printf 'http://%s:%s/\n' "$host" "$PORT"
  fi
}

curl_host() {
  case "$HOST" in
    0.0.0.0|::) printf '127.0.0.1\n' ;;
    *) printf '%s\n' "$HOST" ;;
  esac
}

graphql_url() {
  local host
  host=$(curl_host)
  if [[ "$host" == *:* ]]; then
    printf 'http://[%s]:%s/graphql\n' "$host" "$PORT"
  else
    printf 'http://%s:%s/graphql\n' "$host" "$PORT"
  fi
}

pid_running() {
  local pid="$1"
  [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null
}

read_pid() {
  [[ -f "$PIDFILE" ]] || return 1
  local pid
  pid=$(tr -d '[:space:]' < "$PIDFILE")
  [[ "$pid" =~ ^[0-9]+$ ]] || return 1
  printf '%s\n' "$pid"
}

server_cmdline() {
  local pid="$1"
  if [[ -z "$pid" ]]; then
    return 1
  fi
  ps -ww -p "$pid" -o args= 2>/dev/null
}

is_our_server() {
  local pid="$1" cmd
  cmd=$(server_cmdline "$pid" || true)
  [[ "$cmd" == *gonetlogo-server* ]]
}

lsof_bin() {
  if command -v lsof >/dev/null 2>&1; then
    command -v lsof
    return 0
  fi
  if [[ -x /usr/sbin/lsof ]]; then
    printf '%s\n' /usr/sbin/lsof
    return 0
  fi
  return 1
}

each_listener() {
  local lsof pid
  lsof=$(lsof_bin || true)
  [[ -n "$lsof" ]] || return 1
  while IFS= read -r pid; do
    [[ -n "$pid" ]] || continue
    printf '%s\n' "$pid"
  done < <("$lsof" -nP -t -iTCP:"$PORT" -sTCP:LISTEN 2>/dev/null || true)
}

find_server_on_port() {
  local pid
  while IFS= read -r pid; do
    if pid_running "$pid" && is_our_server "$pid"; then
      printf '%s\n' "$pid"
      return 0
    fi
  done < <(each_listener || true)
  return 1
}

foreign_listener() {
  local pid
  while IFS= read -r pid; do
    if is_our_server "$pid"; then
      continue
    fi
    printf '%s\n' "$pid"
    return 0
  done < <(each_listener || true)
  return 1
}

current_pid() {
  local pid
  pid=$(read_pid || true)
  if [[ -n "$pid" ]] && pid_running "$pid" && is_our_server "$pid"; then
    printf '%s\n' "$pid"
    return 0
  fi
  if [[ -n "$pid" ]]; then
    rm -f "$PIDFILE"
  fi
  pid=$(find_server_on_port || true)
  if [[ -n "$pid" ]]; then
    printf '%s\n' "$pid"
    return 0
  fi
  return 1
}

require_bin() {
  if [[ ! -x "$BIN" ]]; then
    echo "startup-mac.sh: $BIN is missing. Run ./build-mac.sh first." >&2
    exit 1
  fi
}

require_desktop() {
  if [[ -x "$DESKTOP_BIN" || -d "$APP_BUNDLE" ]]; then
    return 0
  fi
  echo "startup-mac.sh: desktop app is missing. Run ./build-mac.sh first." >&2
  exit 1
}

ensure_port_free() {
  local pid lsof
  if ! pid=$(foreign_listener); then
    return 0
  fi
  echo "startup-mac.sh: port ${PORT} is in use by pid ${pid}" >&2
  lsof=$(lsof_bin || true)
  if [[ -n "$lsof" ]]; then
    "$lsof" -nP -iTCP:"$PORT" -sTCP:LISTEN >&2 || true
  fi
  exit 1
}

stop_server() {
  local pid
  if ! pid=$(current_pid); then
    echo "GoNetLogo is not running"
    return 0
  fi
  echo "stopping pid ${pid}"
  kill "$pid" 2>/dev/null || true
  local _
  for _ in $(seq 1 50); do
    if ! pid_running "$pid"; then
      rm -f "$PIDFILE"
      echo "stopped"
      return 0
    fi
    sleep 0.1
  done
  echo "pid ${pid} did not exit; sending SIGKILL" >&2
  kill -9 "$pid" 2>/dev/null || true
  rm -f "$PIDFILE"
  echo "stopped"
}

status_server() {
  local pid
  if pid=$(current_pid); then
    echo "running pid ${pid}"
    public_url
    return 0
  fi
  echo "stopped"
  return 1
}

server_ready() {
  curl -sf -o /dev/null \
    -X POST -H 'content-type: application/json' \
    --data '{"query":"{ __typename }"}' \
    "$(graphql_url)"
}

wait_until_ready() {
  local _ pid
  for _ in $(seq 1 50); do
    if server_ready; then
      return 0
    fi
    pid=$(cat "$PIDFILE" 2>/dev/null || true)
    if [[ -n "$pid" ]] && ! pid_running "$pid"; then
      return 1
    fi
    sleep 0.1
  done
  return 1
}

open_url() {
  local url
  url=$(public_url)
  if ! command -v open >/dev/null 2>&1; then
    echo "startup-mac.sh: open is not available. Browse to ${url}" >&2
    return 0
  fi
  if ! open "$url"; then
    echo "startup-mac.sh: could not open ${url}" >&2
  fi
}

schedule_open() {
  local url
  url=$(public_url)
  (
    local _
    for _ in $(seq 1 50); do
      if server_ready; then
        if command -v open >/dev/null 2>&1; then
          open "$url" >/dev/null 2>&1 || true
        fi
        exit 0
      fi
      sleep 0.1
    done
  ) >/dev/null 2>&1 &
}

launch_desktop_background() {
  local pid
  if [[ -x "$DESKTOP_BIN" ]]; then
    mkdir -p "$RUNDIR"
    : >>"$DESKTOP_LOG"
    # macOS does not ship setsid. nohup keeps the window after this script exits.
    nohup "$DESKTOP_BIN" >>"$DESKTOP_LOG" 2>&1 </dev/null &
    pid=$!
    echo "desktop pid ${pid} (log: ${DESKTOP_LOG})"
    return 0
  fi
  open "$APP_BUNDLE"
  echo "opened ${APP_BUNDLE}"
}

launch_desktop_foreground() {
  if [[ -x "$DESKTOP_BIN" ]]; then
    echo "GoNetLogo desktop"
    exec "$DESKTOP_BIN"
  fi
  open "$APP_BUNDLE"
  echo "opened ${APP_BUNDLE}"
}

start_foreground() {
  if [[ "$do_open" -eq 1 ]]; then
    schedule_open
  fi
  echo "GoNetLogo at $(public_url)"
  echo "GraphQL at $(public_url)graphql"
  exec "$BIN" -addr "$addr"
}

start_background() {
  local pid
  mkdir -p "$RUNDIR"
  : >>"$LOGFILE"
  # macOS does not ship setsid. nohup keeps the server after this script exits.
  nohup "$BIN" -addr "$addr" >>"$LOGFILE" 2>&1 </dev/null &
  pid=$!
  echo "$pid" > "$PIDFILE"
  if command -v curl >/dev/null 2>&1 && wait_until_ready; then
    echo "GoNetLogo at $(public_url) (pid ${pid})"
    echo "log: $LOGFILE"
    if [[ "$do_open" -eq 1 ]]; then
      open_url
    fi
    return 0
  fi
  if pid_running "$pid"; then
    echo "GoNetLogo at $(public_url) (pid ${pid})"
    echo "log: $LOGFILE"
    if ! command -v curl >/dev/null 2>&1; then
      echo "startup-mac.sh: curl is not installed, so readiness was not checked" >&2
    fi
    if [[ "$do_open" -eq 1 ]]; then
      open_url
    fi
    return 0
  fi
  echo "startup-mac.sh: server exited. Last log lines:" >&2
  tail -n 40 "$LOGFILE" >&2 || true
  rm -f "$PIDFILE"
  exit 1
}

require_apple_silicon() {
  if [[ "$(uname -s)" != "Darwin" ]]; then
    echo "startup-mac.sh: this script supports macOS (found $(uname -s)). On Linux, use ./startup.sh." >&2
    exit 1
  fi
  if [[ "$(uname -m)" != "arm64" ]]; then
    echo "startup-mac.sh: this script targets Apple Silicon (arm64); this Mac is $(uname -m)." >&2
    exit 1
  fi
}

start_requested() {
  local start_server=1
  if [[ "$desktop" -eq 1 && "$server_flag" -eq 0 && "$background" -eq 0 && "$do_open" -eq 0 && "$saw_start_word" -eq 0 ]]; then
    start_server=0
  fi

  if [[ "$start_server" -eq 1 ]]; then
    require_bin
  fi
  if [[ "$desktop" -eq 1 ]]; then
    require_desktop
  fi

  local already=0 pid=""
  if [[ "$start_server" -eq 1 ]]; then
    if pid=$(current_pid); then
      already=1
    else
      ensure_port_free
    fi
  fi

  if [[ "$desktop" -eq 1 ]]; then
    if [[ "$start_server" -eq 1 ]]; then
      launch_desktop_background
    else
      launch_desktop_foreground
      return
    fi
  fi

  if [[ "$start_server" -eq 0 ]]; then
    return 0
  fi

  if [[ "$already" -eq 1 ]]; then
    if [[ "$background" -eq 1 || "$desktop" -eq 1 || "$do_open" -eq 1 ]]; then
      echo "already running (pid ${pid})"
      public_url
      if [[ "$do_open" -eq 1 ]]; then
        open_url
      fi
      return 0
    fi
    echo "startup-mac.sh: already running (pid ${pid}). ./startup-mac.sh stop" >&2
    exit 1
  fi

  if [[ "$background" -eq 1 ]]; then
    start_background
  else
    start_foreground
  fi
}

action=start
background=0
desktop=0
do_open=0
server_flag=0
saw_start_word=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    start) action=start; saw_start_word=1 ;;
    stop) action=stop ;;
    status) action=status ;;
    --background|-d|--daemon) background=1 ;;
    --open) do_open=1 ;;
    --desktop) desktop=1 ;;
    --server) server_flag=1 ;;
    -h|--help) usage; exit 0 ;;
    *)
      echo "startup-mac.sh: unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
  shift
done

if [[ "$action" != "start" && "$background" -eq 1 ]]; then
  echo "startup-mac.sh: --background applies to start" >&2
  exit 1
fi
if [[ "$action" != "start" && "$do_open" -eq 1 ]]; then
  echo "startup-mac.sh: --open applies to start" >&2
  exit 1
fi
if [[ "$action" != "start" && "$desktop" -eq 1 ]]; then
  echo "startup-mac.sh: --desktop applies to start" >&2
  exit 1
fi
if [[ "$action" != "start" && "$server_flag" -eq 1 ]]; then
  echo "startup-mac.sh: --server applies to start" >&2
  exit 1
fi

require_apple_silicon

case "$action" in
  start) start_requested ;;
  stop) stop_server ;;
  status) status_server ;;
esac
