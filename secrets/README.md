# Local credentials

This folder is for local NATS creds files and is gitignored.

Expected files for local compose:
- `platform.nk`: NATS user seed for platform service
- `agent.nk`: default local agent seed
- `agents/<agent-id>.nk`: per-agent seeds for individual VMs/agents

Generate seed keys and server config with:

`make bootstrap-nats-keys`

This updates:
- `secrets/platform.nk`
- `secrets/agent.nk`
- `nats/server.conf`

Generate a dedicated key for one agent:

`make bootstrap-agent-key AGENT=<agent-id>`

This creates `secrets/agents/<agent-id>.nk` and re-renders `nats/server.conf`.
