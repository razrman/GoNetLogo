#!/usr/bin/env bash
# Start the GraphQL API and the web UI together.
# UI:      http://127.0.0.1:43132
# GraphQL: http://127.0.0.1:43131/graphql
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

if [[ ! -d frontend/node_modules ]]; then
  npm install --prefix frontend
fi

go run ./backend/cmd/server -addr 0.0.0.0:43131 &
api=$!
cleanup() {
  kill "$api" 2>/dev/null || true
  wait "$api" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

for _ in $(seq 1 50); do
  if curl -sf -o /dev/null \
    -X POST -H 'content-type: application/json' \
    --data '{"query":"{ models }"}' \
    http://127.0.0.1:43131/graphql; then
    break
  fi
  sleep 0.2
done

cd frontend
exec npm run dev
