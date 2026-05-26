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

# Naming
* agent: one instance of an agent usually on a VM
* environment: one docker compose folder in a configured env folder
* service: one running docker container
* event: a message passed on the queue
* logs: streamed logs from docker compose

## Local development
* `make test` runs the Go test suite
* `make build` builds the platform and agent binaries
* `make compose-config` validates the Docker Compose file
* `make run-nats` starts local NATS with JetStream
* `make run-platform` starts the web platform on `:8080`
* `make run-agent` starts the local test agent
