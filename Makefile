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
	go run ./cmd/platform

run-agent:
	go run ./cmd/agent
