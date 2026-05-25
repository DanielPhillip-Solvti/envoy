.PHONY: test build compose-config run-nats run-platform run-agent

NATS_SERVER ?= $(shell command -v nats-server 2>/dev/null || echo "$(HOME)/go/bin/nats-server")

test:
	go test ./...

build:
	go build ./cmd/platform
	go build ./cmd/agent

compose-config:
	docker compose config

run-nats:
	@if [ -x "$(NATS_SERVER)" ]; then \
		"$(NATS_SERVER)" -js -p 4222 -m 8222; \
	else \
		echo "nats-server not found in PATH or $(HOME)/go/bin."; \
		echo "Install it with: go install github.com/nats-io/nats-server/v2@latest"; \
		exit 127; \
	fi

run-platform:
	@set -a; \
	if [ -f .env ]; then . ./.env; fi; \
	if [ "$$ENVOY_NATS_URL" = "nats://nats:4222" ]; then \
		echo "ENVOY_NATS_URL points to docker host 'nats'; switching to nats://127.0.0.1:4222 for local run"; \
		ENVOY_NATS_URL="nats://127.0.0.1:4222"; \
		export ENVOY_NATS_URL; \
	fi; \
	set +a; \
	go run ./cmd/platform

run-agent:
	go run ./cmd/agent
