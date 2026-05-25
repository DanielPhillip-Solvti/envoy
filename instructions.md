See makefile for local commands.
Usage example: make run-platform.

Start order is: NATS, then platform, then agent.

For local stack bring-up, run docker compose up (platform + NATS).

Using docker compose pulls the latest platform image, so local source changes may not be reflected.

## VM Agent Install Recommendation

Preferred approach: install only the agent binary on the VM, not the full envoy repo.

Why this is preferred:
- Smaller footprint and faster updates.
- Less source code and tooling on production VMs.
- Easier rollback by swapping one binary.

What the VM needs:
- envoy-agent binary.
- A manifest file (for example /etc/envoy/agent.manifest.yaml).
- Any scripts and files referenced by that manifest.
- Docker + docker compose plugin (if environments use compose).
- Network reachability to NATS.

When to pull a repo on the VM:
- Only if your manifest intentionally points at scripts/files that live in a checked-out repository path.
- If you do this, pin to a specific revision and keep paths stable.

### Binary-only install flow (recommended)

1. Build the agent binary:
	CGO_ENABLED=0 go build -o envoy-agent ./cmd/agent

2. Copy binary to VM:
	scp envoy-agent user@vm:/tmp/envoy-agent

3. Install binary and runtime dirs on VM:
	sudo install -m 0755 /tmp/envoy-agent /usr/local/bin/envoy-agent
	sudo mkdir -p /etc/envoy /var/lib/envoy

4. Place manifest and referenced scripts/files on VM:
	- Set ENVOY_MANIFEST to this manifest path.
	- Ensure all manifest paths exist on disk.

5. Set env vars and run as service:
	- Required: ENVOY_NATS_URL
	- Usually set: ENVOY_MANIFEST, optional ENVOY_OBJECT_DIR

6. Validate startup:
	- agent process is running
	- agent appears in platform UI
	- commands/logs/files round-trip from platform

Notes:
- The default manifest fallback in code is test/vm/agent.manifest.yaml; do not rely on that path for production VMs.
- Keep VM-specific manifests/scripts under infra/deployment management, separate from local test fixtures.
