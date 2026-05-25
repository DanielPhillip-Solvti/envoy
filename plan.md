# Envoy Implementation Plan

## Summary

Envoy is a Go platform plus VM agent connected through NATS. The MVP is intentionally database-free: platform views are reconstructed from agent registration, heartbeat, command events, and consumption metrics. JetStream can retain event history, but Envoy does not own a SQLite/Postgres-style operational store.

## Implementation Order

1. Build the Go project skeleton with `cmd/platform`, `cmd/agent`, and internal packages for config, manifests, queue payloads, agent runtime, platform state, and SSR UI.
2. Define the versioned YAML agent manifest and validate command/file capabilities at agent startup.
3. Define NATS subjects and JSON payloads for registration, heartbeats, commands, command events, logs, file requests, file responses, and consumption metrics.
4. Implement agent registration and heartbeats. Heartbeats include environment discovery and consumption counters.
5. Implement the platform state as an in-memory projection from queue events, not a durable database.
6. Implement command dispatch and agent-side command execution with manifest enforcement.
7. Add SSR + htmx views for agents, agent detail, environments, commands, logs, files, and consumption.
8. Add Docker Compose, Dockerfiles, an example manifest, test scripts, and a systemd unit.
9. Add GitHub OAuth and repo-based access checks after the queue loop is usable.
10. Harden path safety, timeouts, log redaction, object storage file transfer, stale heartbeat detection, and command cancellation.

## Current Scaffold

- `cmd/platform` starts the web platform and subscribes to queue events.
- `cmd/agent` loads a manifest, registers, sends heartbeats, and executes declared scripts.
- `internal/platform` keeps only an in-memory projection of the current world.
- `internal/queue` owns subject names and wire payloads.
- File requests copy manifest-approved files into the configured local object directory and publish queue responses.
- Log requests publish recent Docker Compose log output when an environment has a compose project.
- `examples/agent.manifest.yaml` and `test/vm` provide a local VM-like setup.

## Open Work

- Add JetStream durable consumers and NKey config.
- Replace local file-object storage with S3-compatible object storage.
- Add GitHub OAuth implementation.
- Add Docker Compose service status parsing.
- Add command cancellation and log streaming lifecycle controls.
