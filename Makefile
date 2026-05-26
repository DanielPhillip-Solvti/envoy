.PHONY: test test-e2e build compose-config bootstrap-nats-keys bootstrap-agent-key run-nats run-platform run-agent dev-up dev-down

NATS_SERVER ?= $(shell command -v nats-server 2>/dev/null || echo "$(HOME)/go/bin/nats-server")

test:
	go test ./...

test-e2e:
	go test ./test/e2e -v

build:
	go build ./cmd/platform
	go build ./cmd/agent

compose-config:
	docker compose config

bootstrap-nats-keys:
	go run ./cmd/nkeys-bootstrap init

bootstrap-agent-key:
	@if [ -z "$(AGENT)" ]; then \
		echo "usage: make bootstrap-agent-key AGENT=<agent-id>"; \
		exit 2; \
	fi
	go run ./cmd/nkeys-bootstrap add-agent $(AGENT)

run-nats:
	@if [ -x "$(NATS_SERVER)" ]; then \
		"$(NATS_SERVER)" -c nats/server.conf; \
	else \
		echo "nats-server not found in PATH or $(HOME)/go/bin."; \
		echo "Install it with: go install github.com/nats-io/nats-server/v2@latest"; \
		exit 127; \
	fi

run-platform:
	@set -a; \
	if [ -f .env ]; then . ./.env; fi; \
	case "$$STACCATO_NATS_NKEY" in /secrets/*) \
		echo "STACCATO_NATS_NKEY points to container path '$$STACCATO_NATS_NKEY'; switching to local secrets/$${STACCATO_NATS_NKEY##*/}"; \
		STACCATO_NATS_NKEY="secrets/$${STACCATO_NATS_NKEY##*/}"; \
		export STACCATO_NATS_NKEY; \
	esac; \
	if [ "$$STACCATO_NATS_URL" = "nats://nats:4222" ]; then \
		echo "STACCATO_NATS_URL points to docker host 'nats'; switching to nats://127.0.0.1:4222 for local run"; \
		STACCATO_NATS_URL="nats://127.0.0.1:4222"; \
		export STACCATO_NATS_URL; \
	fi; \
	set +a; \
	go run ./cmd/platform

run-agent:
	@set -a; \
	if [ -f .env ]; then . ./.env; fi; \
	if [ -f test/vm/.env ]; then . ./test/vm/.env; fi; \
	case "$$STACCATO_NATS_NKEY" in /secrets/*) \
		echo "STACCATO_NATS_NKEY points to container path '$$STACCATO_NATS_NKEY'; switching to local secrets/$${STACCATO_NATS_NKEY##*/}"; \
		STACCATO_NATS_NKEY="secrets/$${STACCATO_NATS_NKEY##*/}"; \
		export STACCATO_NATS_NKEY; \
	esac; \
	if [ "$$STACCATO_NATS_URL" = "nats://nats:4222" ]; then \
		echo "STACCATO_NATS_URL points to docker host 'nats'; switching to nats://127.0.0.1:4222 for local run"; \
		STACCATO_NATS_URL="nats://127.0.0.1:4222"; \
		export STACCATO_NATS_URL; \
	fi; \
	set +a; \
	go run ./cmd/agent

dev-up:
	./scripts/local/up.sh

dev-down:
	./scripts/local/down.sh
