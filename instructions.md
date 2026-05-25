
## Runbook: start queue + platform (control host)

These steps run only the control plane (NATS queue + web platform). Agents can be started later, on separate VMs.

1. Build binaries:

```bash
make build
```

2. Start NATS with JetStream:

```bash
make run-nats
```

If `nats-server` is not in `PATH`, install it first:

```bash
go install github.com/nats-io/nats-server/v2@latest
```

3. In a second terminal, start platform:

```bash
ENVOY_HTTP_ADDR=:8080 ENVOY_NATS_URL=nats://127.0.0.1:4222 go run ./cmd/platform
```

4. Verify platform is up:

```bash
curl -fsS http://127.0.0.1:8080/agents
```

Expected: an HTML page (initially with "No agents have registered yet.").

## Runbook: start an agent separately on a VM

Use this after the queue + platform are already running on the control host.

1. Copy agent binary to VM:

```bash
go build -o envoy-agent ./cmd/agent
```

Then copy `envoy-agent` to `/usr/local/bin/envoy-agent` on the VM.

2. Create agent manifest on VM at `/etc/envoy/agent.manifest.yaml`.

Use [examples/agent.manifest.yaml](examples/agent.manifest.yaml) as a template and update paths for that VM.

3. Set agent environment:

* `ENVOY_MANIFEST=/etc/envoy/agent.manifest.yaml`
* `ENVOY_NATS_URL=nats://<platform-host-or-ip>:4222`
* Optional: `ENVOY_NATS_NKEY=<path-to-creds-file>`

4. Start agent on VM (manual):

```bash
ENVOY_MANIFEST=/etc/envoy/agent.manifest.yaml \
ENVOY_NATS_URL=nats://<platform-host-or-ip>:4222 \
/usr/local/bin/envoy-agent
```

5. Or run as a service using [deployments/envoy-agent.service](deployments/envoy-agent.service):

```bash
sudo cp deployments/envoy-agent.service /etc/systemd/system/envoy-agent.service
sudo systemctl daemon-reload
sudo systemctl enable --now envoy-agent
sudo systemctl status envoy-agent
```

6. Verify registration from control host:

```bash
curl -fsS http://127.0.0.1:8080/agents | grep -i "agent"
```

## Stop services

* Stop platform and NATS with `Ctrl+C` in each terminal.
* For systemd-managed VM agent:

```bash
sudo systemctl stop envoy-agent
```

## Platform image artifact (GHCR)

The platform image is published by GitHub Actions from [platform-image.yml](.github/workflows/platform-image.yml).

Image name:

```text
ghcr.io/<owner>/envoy-platform
```

Example pull and run:

```bash
docker pull ghcr.io/<owner>/envoy-platform:latest
docker run --rm -p 8080:8080 \
	-e ENVOY_HTTP_ADDR=:8080 \
	-e ENVOY_NATS_URL=nats://<nats-host>:4222 \
	ghcr.io/<owner>/envoy-platform:latest
```