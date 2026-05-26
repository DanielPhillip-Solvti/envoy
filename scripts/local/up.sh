#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PID_FILE="$ROOT_DIR/var/dev-agent.pid"
LOG_FILE="$ROOT_DIR/var/dev-agent.log"

cd "$ROOT_DIR"

mkdir -p var

if [[ -f "$PID_FILE" ]]; then
  existing_pid="$(cat "$PID_FILE")"
  if [[ -n "$existing_pid" ]] && kill -0 "$existing_pid" 2>/dev/null; then
    echo "Local agent is already running (pid=$existing_pid)."
  else
    rm -f "$PID_FILE"
  fi
fi

echo "Bootstrapping NATS keys and config..."
make bootstrap-nats-keys

echo "Starting NATS and platform via Docker Compose..."
docker compose pull
docker compose up -d nats platform

if [[ ! -f "$PID_FILE" ]]; then
  echo "Starting local agent process..."
  nohup make run-agent >"$LOG_FILE" 2>&1 &
  agent_pid=$!
  echo "$agent_pid" >"$PID_FILE"
  echo "Local agent started (pid=$agent_pid)."
else
  echo "Skipping agent start because it is already running."
fi

echo "All local services started."
echo "Platform UI: http://127.0.0.1:8080"
echo "Agent log: $LOG_FILE"
