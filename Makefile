.PHONY: test build compose-config run-nats run-platform run-agent

test:
	go test ./...

build:
	go build ./cmd/platform
	go build ./cmd/agent

compose-config:
	docker compose config

run-nats:
	nats-server -js -p 4222 -m 8222

run-platform:
	go run ./cmd/platform

run-agent:
	go run ./cmd/agent
