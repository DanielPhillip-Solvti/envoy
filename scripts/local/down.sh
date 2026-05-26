#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PID_FILE="$ROOT_DIR/var/dev-agent.pid"

cd "$ROOT_DIR"

if [[ -f "$PID_FILE" ]]; then
  agent_pid="$(cat "$PID_FILE")"
  if [[ -n "$agent_pid" ]] && kill -0 "$agent_pid" 2>/dev/null; then
    echo "Stopping local agent (pid=$agent_pid)..."
    kill "$agent_pid"
  fi
  rm -f "$PID_FILE"
else
  echo "No local agent pid file found."
fi

echo "Stopping NATS and platform via Docker Compose..."
docker compose down

echo "All local services stopped."
