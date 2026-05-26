# Staccato - Environment orchestrator platform

## Staccato components
* Platform - Hosts UI for interacting with agents and issuing commands
* Agent - Go binary deployed on VMs, pub/sub with event queue, strict capabilities determined by manifest, option to link a single github repo
* Event queue - The platform issues commands and the VMs post responses

## Platform
* Go
* Github OAuth
* SSR templates, uses composition with htmx for upserting/removing elements
* Shows agents based on repos accessible by user based on github OAuth
* Shows vm level scripts
* Shows environments active on vm per agent, these correspond to branches, shows docker service status per env
* Shows environment level scripts
* agents as tile elements-> agent page, nav by environment, tabs: events, logs, files, commands

## Agent
* optionally linked to a single github repo which will limit access at the platform
* manifest defines repo, envs folder, vm level scripts, env level scripts, accessible files
* commands correspond to scripts, ie. deploy: script: home/scripts/deploy.sh, args : ["branch"]
* listens to queue, executes commands, publishes result
* able to browse and stream docker compose logs
* able to send files by object store if listed in accessible files
* no application database for operational state; platform state is reconstructed from queue events, heartbeats, and consumption metrics

## Queue
* NATS and NKey, asymmetric keys
* single NATS server should be deployed alongside the platform
* Only the platform and agents are allowed to publish to the queue
* jetstream for events
* core nats for log streaming
* platform and agent now require `STACCATO_NATS_NKEY` (credentials file path) at startup
* newly discovered agents must be activated in the platform UI before commands/logs/files are enabled

# Naming
* agent: one instance of an agent usually on a VM
* environment: one docker compose folder in a configured env folder
* service: one running docker container
* event: a message passed on the queue
* logs: streamed logs from docker compose

## Local development
* `make test` runs the Go test suite
* `make test-e2e` runs local end-to-end security tests (requires `nats-server` in `PATH`)
* `make build` builds the platform and agent binaries
* `make compose-config` validates the Docker Compose file
* `make bootstrap-nats-keys` initializes/updates platform key + default local agent key and writes `nats/server.conf`
* `make bootstrap-agent-key AGENT=<agent-id>` creates a dedicated agent key in `secrets/agents/` and updates `nats/server.conf`
* `make run-nats` starts local authenticated NATS using `nats/server.conf`
* `make run-platform` starts the web platform on `:8080`
* `make run-agent` starts the local test agent
* `make dev-up` starts NATS + platform (Docker Compose) and local agent process
* `make dev-down` stops local agent process and tears down Docker Compose services

## Agent activation flow
* agents appear in the UI after registration/heartbeat
* operators must confirm activation on the agent page
* until activation, commands/log requests/file requests are blocked

## VM Agent Install Recommendation

Use this exact sequence to install one agent on one VM.

### 1. Build agent binary on your workstation (repo root)

```bash
cd /path/to/staccato
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/staccato-agent ./cmd/agent
```

### 2. Generate NATS keys and server config on your workstation

```bash
cd /path/to/staccato
make bootstrap-nats-keys
make bootstrap-agent-key AGENT=vm-prod-01
```

This creates:
- `secrets/platform.nk`
- `secrets/agent.nk`
- `secrets/agents/vm-prod-01.nk`
- `nats/server.conf`

### 3. Copy binary and config templates to VM

Replace `vm-user@vm-host` with your target.

```bash
scp dist/staccato-agent vm-user@vm-host:/tmp/staccato-agent
scp test/vm/agent.manifest.yaml vm-user@vm-host:/tmp/agent.manifest.yaml
scp example.env vm-user@vm-host:/tmp/agent.env
scp secrets/agents/vm-prod-01.nk vm-user@vm-host:/tmp/agent.nk
```

### 4. Install files on VM

```bash
ssh vm-user@vm-host '
	set -eu
	sudo install -m 0755 /tmp/staccato-agent /usr/local/bin/staccato-agent
	sudo mkdir -p /etc/staccato /etc/staccato/nats /var/lib/staccato
	sudo install -m 0644 /tmp/agent.manifest.yaml /etc/staccato/agent.manifest.yaml
	sudo install -m 0600 /tmp/agent.nk /etc/staccato/nats/agent.nk
	sudo install -m 0644 /tmp/agent.env /etc/staccato/agent.env
'
```

### 5. Edit `/etc/staccato/agent.env` on VM

```bash
ssh vm-user@vm-host 'sudo sh -c "cat > /etc/staccato/agent.env <<EOF
STACCATO_NATS_URL=nats://<nats-host>:4222
STACCATO_NATS_NKEY=/etc/staccato/nats/agent.nk
STACCATO_MANIFEST=/etc/staccato/agent.manifest.yaml
STACCATO_OBJECT_DIR=/var/lib/staccato
EOF"'
```

### 6. Create systemd service on VM

```bash
ssh vm-user@vm-host 'sudo sh -c "cat > /etc/systemd/system/staccato-agent.service <<EOF
[Unit]
Description=Staccato Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/staccato/agent.env
ExecStart=/usr/local/bin/staccato-agent
Restart=always
RestartSec=2
User=root
WorkingDirectory=/var/lib/staccato

[Install]
WantedBy=multi-user.target
EOF"'
```

### 7. Start and verify service

```bash
ssh vm-user@vm-host '
	sudo systemctl daemon-reload
	sudo systemctl enable --now staccato-agent
	sudo systemctl status --no-pager staccato-agent
'
```

### 8. Final validation in platform UI

1. Agent appears in the platform.
2. Click Activate on the agent page.
3. Run a command and confirm events/logs/files are visible.
