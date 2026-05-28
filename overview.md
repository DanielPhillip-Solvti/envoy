# Staccato: Appliance-Mode Odoo Orchestration

Staccato is a lightweight, distributed platform designed to manage Odoo instances as managed appliances. It replaces complex script-based deployments with a robust Go-based agent that internalizes Odoo orchestration logic (Blue/Green, DB Management, automated GHCR access).

## Architecture & Stack
- **Platform (Go/HTMX)**: A central web server serving a rich, reactive UI via HTML templates and HTMX. It manages agent registrations, identity (GitHub OAuth), and orchestration.
- **Agent (Go)**: A stateless-first binary running on target VMs. It performs localized orchestration (Docker, Git, Databases) and communicates solely via NATS.
- **Queue (NATS)**: The secure backbone. Uses NKEY authentication for zero-trust communication. Supports command requests (request/reply), events (PubSub), and streaming logs.

## Core Specification

### 1. Project Structure (Improved)
- `/cmd`: Entry points for `platform` and `agent`.
- `/internal/platform`: State machine, shared types, and NATS event handlers.
- `/internal/agent`: Agent-specific runner logic and internalized Odoo orchestration commands.
- `/internal/web`: Server-side rendering (Go templates), HTMX partials, and GitHub App/OAuth integration.
- `/internal/testrunner`: Isolated logic for executing `go test` and parsing results into live documentation.
- `/internal/manifest`: YAML parser for `agent.manifest.yaml`, defining agent identity and allowed capabilities.

### 2. The Identity System (GitHub App Flow)
- **Primary Auth**: Users sign in via GitHub OAuth to access the dashboard.
- **Agent Auth (Automated)**: The platform acts as a GitHub App.
    - Installation tokens are generated using the App's Private Key.
    - **Insight**: Tokens are dynamically pushed to agents over NATS every 5-60 minutes.
    - **Outcome**: Agents can pull private repos and GHCR images without manual PAT management.

### 3. Agent Lifecycle & "Appliance" Model
- **Wizard**: Generates a self-contained ZIP (`staccato-agent`, `agent.nk`, `agent.env`, `start.sh`, `staccato-agent.service`).
- **Registration**: Agents heartbeat on startup; platform must "Activate" them before they accept commands or receive tokens.
- **Internalized Logic**: Commands like `deploy`, `backup`, `restore`, `update`, `health`, `disk`, `memory`, `cpu`, `logs`, `shell` and `download` are built-in Go functions, not external scripts.
    - **Blue/Green Deployment**: New project is built in parallel (`docker compose -p project_timestamp`), health-checked, then the old one is swapped/stopped.
    - **Database Logic**: Native `pg_dump`/`psql` orchestration with automated anonymization for staging restores.

### 4. Security Model
- **NATS NKEYs**: Every agent has a unique NKEY. NATS permissions restrict agents to their own `staccato.*.<agentID>` subjects.
- **Manifest Restrictions**: The local `agent.manifest.yaml` defines `allowed_builtins` and specific `files` or `scripts` the agent is permitted to touch.
- **Token Sandboxing**: GitHub App tokens are installation-scoped and short-lived.

## User Interface Experience
The UI is a single-entry Go web app optimized for speed and realtime feedback, using **HTMX** to avoid full page reloads.

- **Agent Dashboard**: A grid-based overview of the entire fleet. Displays agent health, repository links, and quick-access metrics.
- **Navigation Structure**: Utilizes an outer navigation bar for VM-level actions, logs, and system commands, alongside an inner tabbed interface for environment-specific (branch) settings and controls.
- **The Agent Hub (Detail View)**: A tabbed interface for deep orchestration:
    - **Environments**: Manage multiple branches/deployments on a single VM. Supports one-click deployments and branch switching.
    - **Live Logs**: Real-time streaming of Docker container logs directly in the browser.
    - **Command Center**: Trigger built-in commands (Deploy, Backup, etc.) and view live output streams from the agent process.
    - **Commit Integration**: Views the GitHub commit history; allows checking out specific SHAs with a single click.
    - **File Manager**: Request and download database dumps or system files securely via the NATS object transport.
- **Installation Wizard**: A step-by-step modal that simplifies VM onboarding. It automatically detects GitHub App status and generates a production-ready ZIP bundle tailored to the specific agent. The zip bundle should include bash scripts to start and stop the agent as a service, and to run the agent in the foreground for debugging. It should also include the agent.nk file and the manifest based on the wizard inputs. The wizard should also provide instructions on how to install the agent on the VM.

## Development Process & Live Documentation
Staccato follows a strictly **Test-Driven Development (TDD)** workflow to maintain the reliability of the appliance model:
1. **Scoping**: Define precise requirements for a new capability.
2. **Red**: Write failing unit and integration tests (e.g., NATS message round-trips).
3. **Green**: Implement the minimal logic in the `Runner` or `Server`.
4. **Refactor**: Clean up the implementation once tests pass.

- **Live Documentation (`/test`)**: The platform includes a dedicated UI endpoint at `/test`. This view invokes the `internal/testrunner` package to execute the test suite and display results in real-time. This provides living documentation of project features where "Passing" is the ultimate guarantee of correctness. The logic is isolated in its own package to keep the web server clean and allow for build-tag based exclusion in production.

## Implementation Insights
- **HTMX for Orchestration**: The UI reflects realtime agent state by swapping HTML partials on NATS events (e.g., command logs streaming).
- **Service-First**: The agent is designed to run as a systemd service, but can be run as a standalone binary for debugging.
- **Env-Isolation**: Each environment on an agent VM is isolated via Docker Compose project names, allowing multiple versions of Odoo to coexist during transitions.