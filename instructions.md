See makefile for local commands.
Usage example: make run-platform.

Start order is: NATS, then platform, then agent.

For local stack bring-up, run docker compose up (platform + NATS).

Using docker compose pulls the latest platform image, so local source changes may not be reflected.

## Environment Variables

Yes, this repo now includes [example.env](example.env) as a template.

Local/dev setup:
1. Copy template: cp example.env .env
2. Generate local NATS seeds and auth config:
	- make bootstrap-nats-keys
	- make bootstrap-agent-key AGENT=<agent-id> (repeat per VM/agent)
3. Fill real values for OAuth fields:
	- STACCATO_GITHUB_CLIENT_ID
	- STACCATO_GITHUB_CLIENT_SECRET
	- STACCATO_GITHUB_CALLBACK_URL
	- STACCATO_NATS_NKEY (path to platform NATS seed file)
	- If you need to create/configure a GitHub token, use: https://github.com/settings/personal-access-tokens
4. Keep STACCATO_SESSION_SECURE=false for local HTTP.
5. Start platform with those env vars loaded (for compose, make sure .env is present).

Do not commit real credentials.

## VM Agent Install Recommendation

Preferred approach: install only the agent binary on the VM, not the full staccato repo.

Why this is preferred:
- Smaller footprint and faster updates.
- Less source code and tooling on production VMs.
- Easier rollback by swapping one binary.

What the VM needs:
- staccato-agent binary.
- A manifest file (for example /etc/staccato/agent.manifest.yaml).
- Any scripts and files referenced by that manifest.
- Docker + docker compose plugin (if environments use compose).
- Network reachability to NATS.

When to pull a repo on the VM:
- Only if your manifest intentionally points at scripts/files that live in a checked-out repository path.
- If you do this, pin to a specific revision and keep paths stable.

### Binary-only install flow (recommended)

1. Build the agent binary:
	CGO_ENABLED=0 go build -o staccato-agent ./cmd/agent

2. Copy binary to VM:
	scp staccato-agent user@vm:/tmp/staccato-agent

3. Install binary and runtime dirs on VM:
	sudo install -m 0755 /tmp/staccato-agent /usr/local/bin/staccato-agent
	sudo mkdir -p /etc/staccato /var/lib/staccato

4. Place manifest and referenced scripts/files on VM:
	- Set STACCATO_MANIFEST to this manifest path.
	- Ensure all manifest paths exist on disk.

5. Set env vars and run as service:
	- Required: STACCATO_NATS_URL
	- Required: STACCATO_NATS_NKEY (path to agent NATS seed file)
	- Usually set: STACCATO_MANIFEST, optional STACCATO_OBJECT_DIR
	- For local `make run-agent`, use `test/vm/.env` for agent-only overrides (for example `STACCATO_NATS_NKEY=secrets/agents/local-test-vm.nk`)

6. Validate startup:
	- agent process is running
	- agent appears in platform UI
	- activate the agent in the platform UI before issuing commands
	- commands/logs/files round-trip from platform

Notes:
- The default manifest fallback in code is test/vm/agent.manifest.yaml; do not rely on that path for production VMs.
- Keep VM-specific manifests/scripts under infra/deployment management, separate from local test fixtures.
