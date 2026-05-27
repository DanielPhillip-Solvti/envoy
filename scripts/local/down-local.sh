#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
AGENT_PID_FILE="$ROOT_DIR/var/dev-agent.pid"
PLATFORM_PID_FILE="$ROOT_DIR/var/dev-platform.pid"

cd "$ROOT_DIR"

stop_pid_file_process() {
  local pid_file="$1"
  local label="$2"
  if [[ ! -f "$pid_file" ]]; then
    echo "No $label pid file found."
    return
  fi

  local pid
  pid="$(cat "$pid_file")"
  if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
    echo "Stopping $label (pid=$pid)..."
    kill "$pid"
  fi
  rm -f "$pid_file"
}

stop_pid_file_process "$PLATFORM_PID_FILE" "local platform"
stop_pid_file_process "$AGENT_PID_FILE" "local agent"

echo "Stopping NATS container..."
docker compose stop nats >/dev/null 2>&1 || true

echo "Local build-based stack stopped."
