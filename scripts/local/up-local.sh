#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
AGENT_PID_FILE="$ROOT_DIR/var/dev-agent.pid"
PLATFORM_PID_FILE="$ROOT_DIR/var/dev-platform.pid"
AGENT_LOG_FILE="$ROOT_DIR/var/dev-agent.log"
PLATFORM_LOG_FILE="$ROOT_DIR/var/dev-platform.log"

cd "$ROOT_DIR"
mkdir -p var

stop_stale_pid_file() {
  local pid_file="$1"
  if [[ -f "$pid_file" ]]; then
    local existing_pid
    existing_pid="$(cat "$pid_file")"
    if [[ -n "$existing_pid" ]] && kill -0 "$existing_pid" 2>/dev/null; then
      return 0
    fi
    rm -f "$pid_file"
  fi
  return 1
}

echo "Building local binaries..."
make build

echo "Bootstrapping NATS keys and config..."
make bootstrap-nats-keys

echo "Starting NATS only via Docker Compose..."
docker compose up -d nats

echo "Ensuring pull-based platform container is not running..."
docker compose stop platform >/dev/null 2>&1 || true

if stop_stale_pid_file "$PLATFORM_PID_FILE"; then
  echo "Local platform is already running (pid=$(cat "$PLATFORM_PID_FILE"))."
else
  echo "Starting local platform binary..."
  nohup env -i PATH="$PATH" HOME="$HOME" \
    bash -lc '
      set -a
      if [[ -f .env ]]; then . ./.env; fi
      if [[ "${STACCATO_NATS_NKEY:-}" == /secrets/* ]]; then
        STACCATO_NATS_NKEY="secrets/${STACCATO_NATS_NKEY##*/}"
        export STACCATO_NATS_NKEY
      fi
      if [[ "${STACCATO_NATS_URL:-}" == "nats://nats:4222" ]]; then
        STACCATO_NATS_URL="nats://127.0.0.1:4222"
        export STACCATO_NATS_URL
      fi
      set +a
      exec ./platform
    ' >"$PLATFORM_LOG_FILE" 2>&1 &
  platform_pid=$!
  echo "$platform_pid" >"$PLATFORM_PID_FILE"
  echo "Local platform started (pid=$platform_pid)."
fi

if stop_stale_pid_file "$AGENT_PID_FILE"; then
  echo "Local agent is already running (pid=$(cat "$AGENT_PID_FILE"))."
else
  echo "Starting local agent binary..."
  nohup env -i PATH="$PATH" HOME="$HOME" \
    bash -lc '
      set -a
      if [[ -f .env ]]; then . ./.env; fi
      if [[ -f test/vm/.env ]]; then . ./test/vm/.env; fi
      if [[ "${STACCATO_NATS_NKEY:-}" == /secrets/* ]]; then
        STACCATO_NATS_NKEY="secrets/${STACCATO_NATS_NKEY##*/}"
        export STACCATO_NATS_NKEY
      fi
      if [[ "${STACCATO_NATS_URL:-}" == "nats://nats:4222" ]]; then
        STACCATO_NATS_URL="nats://127.0.0.1:4222"
        export STACCATO_NATS_URL
      fi
      set +a
      exec ./agent
    ' >"$AGENT_LOG_FILE" 2>&1 &
  agent_pid=$!
  echo "$agent_pid" >"$AGENT_PID_FILE"
  echo "Local agent started (pid=$agent_pid)."
fi

echo "Local build-based stack started."
echo "Platform UI: http://127.0.0.1:8080"
echo "Platform log: $PLATFORM_LOG_FILE"
echo "Agent log: $AGENT_LOG_FILE"
