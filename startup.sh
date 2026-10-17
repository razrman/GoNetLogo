#!/usr/bin/env bash
# Start the native GoNetLogo server (web UI and GraphQL on one port).
# Foreground is the default. Background mode records a pid and a log.
#
#   ./startup.sh                      # foreground, http://localhost:8080
#   PORT=9090 ./startup.sh            # another port
#   HOST=0.0.0.0 ./startup.sh         # listen beyond localhost
#   ./startup.sh --background         # detach; pid and log under ./run
#   ./startup.sh stop
#   ./startup.sh status
set -euo pipefail

root=$(cd "$(dirname "$0")" && pwd)
cd "$root"

HOST="${HOST:-127.0.0.1}"
PORT="${PORT:-8080}"
BIN="${BIN:-$root/bin/gonetlogo-server}"
RUNDIR="${RUNDIR:-$root/run}"
PIDFILE="${PIDFILE:-$RUNDIR/gonetlogo-server.pid}"
LOGFILE="${LOGFILE:-$RUNDIR/gonetlogo-server.log}"

usage() {
  cat <<EOF
usage: ./startup.sh [start] [--background|-d]
       ./startup.sh stop
       ./startup.sh status

  HOST  bind address (default 127.0.0.1)
  PORT  bind port (default 8080)
  BIN   server binary (default ./bin/gonetlogo-server)

Foreground is the default. Ctrl-C stops it.
--background writes ${PIDFILE} and ${LOGFILE}.
EOF
}

if [[ ! "$PORT" =~ ^[0-9]+$ ]] || (( PORT < 1 || PORT > 65535 )); then
  echo "startup.sh: invalid PORT: $PORT" >&2
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
  if [[ -r "/proc/${pid}/cmdline" ]]; then
    tr '\0' ' ' < "/proc/${pid}/cmdline"
    return 0
  fi
  return 1
}

is_our_server() {
  local pid="$1" cmd
  cmd=$(server_cmdline "$pid" || true)
  [[ "$cmd" == *gonetlogo-server* ]]
}

current_pid() {
  local pid
  pid=$(read_pid || true)
  if [[ -z "$pid" ]]; then
    return 1
  fi
  if pid_running "$pid" && is_our_server "$pid"; then
    printf '%s\n' "$pid"
    return 0
  fi
  rm -f "$PIDFILE"
  return 1
}

require_bin() {
  if [[ ! -x "$BIN" ]]; then
    echo "startup.sh: $BIN is missing. Run ./build.sh first." >&2
    exit 1
  fi
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
    echo "$(public_url)"
    return 0
  fi
  echo "stopped"
  return 1
}

wait_until_ready() {
  local host url
  host=$(curl_host)
  if [[ "$host" == *:* ]]; then
    url="http://[${host}]:${PORT}/graphql"
  else
    url="http://${host}:${PORT}/graphql"
  fi
  local _
  for _ in $(seq 1 50); do
    if curl -sf -o /dev/null \
      -X POST -H 'content-type: application/json' \
      --data '{"query":"{ __typename }"}' \
      "$url"; then
      return 0
    fi
    local pid
    pid=$(cat "$PIDFILE" 2>/dev/null || true)
    if [[ -n "$pid" ]] && ! pid_running "$pid"; then
      return 1
    fi
    sleep 0.1
  done
  return 1
}

start_foreground() {
  require_bin
  local pid
  if pid=$(current_pid); then
    echo "startup.sh: already running (pid ${pid}). ./startup.sh stop" >&2
    exit 1
  fi
  echo "GoNetLogo at $(public_url)"
  echo "GraphQL at $(public_url)graphql"
  exec "$BIN" -addr "$addr"
}

start_background() {
  require_bin
  local pid
  if pid=$(current_pid); then
    echo "already running (pid ${pid})"
    echo "$(public_url)"
    return 0
  fi
  mkdir -p "$RUNDIR"
  : >>"$LOGFILE"
  setsid "$BIN" -addr "$addr" >>"$LOGFILE" 2>&1 </dev/null &
  pid=$!
  echo "$pid" > "$PIDFILE"
  if command -v curl >/dev/null 2>&1 && wait_until_ready; then
    echo "GoNetLogo at $(public_url) (pid ${pid})"
    echo "log: $LOGFILE"
    return 0
  fi
  if pid_running "$pid"; then
    echo "GoNetLogo at $(public_url) (pid ${pid})"
    echo "log: $LOGFILE"
    if ! command -v curl >/dev/null 2>&1; then
      echo "startup.sh: curl is not installed, so readiness was not checked" >&2
    fi
    return 0
  fi
  echo "startup.sh: server exited. Last log lines:" >&2
  tail -n 40 "$LOGFILE" >&2 || true
  rm -f "$PIDFILE"
  exit 1
}

action=start
background=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    start) action=start ;;
    stop) action=stop ;;
    status) action=status ;;
    --background|-d|--daemon) background=1 ;;
    -h|--help) usage; exit 0 ;;
    *)
      echo "startup.sh: unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
  shift
done

if [[ "$action" != "start" && "$background" -eq 1 ]]; then
  echo "startup.sh: --background applies to start" >&2
  exit 1
fi

case "$action" in
  start)
    if [[ "$background" -eq 1 ]]; then
      start_background
    else
      start_foreground
    fi
    ;;
  stop) stop_server ;;
  status) status_server ;;
esac
