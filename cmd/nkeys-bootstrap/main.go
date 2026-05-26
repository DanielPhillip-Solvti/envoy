package main

import (
	"fmt"
	"os"

	"github.com/example/staccato/internal/natsauth"
)

func main() {
	mode := "init"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	switch mode {
	case "init":
		runInit()
	case "add-agent":
		if len(os.Args) < 3 {
			fatalf("usage: go run ./cmd/nkeys-bootstrap add-agent <agent-id>")
		}
		runAddAgent(os.Args[2])
	default:
		fatalf("unknown mode %q (supported: init, add-agent)", mode)
	}
}

func runInit() {
	generated, err := natsauth.EnsureBootstrap("secrets/platform.nk", "secrets/agent.nk", "secrets/agents", "nats/server.conf")
	if err != nil {
		fatalf("bootstrap nats auth: %v", err)
	}

	if generated {
		fmt.Println("Prepared NATS keys and server config:")
	} else {
		fmt.Println("Bootstrap completed; keys/config are up to date:")
	}
	fmt.Println("- secrets/platform.nk")
	fmt.Println("- secrets/agent.nk")
	fmt.Println("- secrets/agents/*.nk")
	fmt.Println("- nats/server.conf")
	fmt.Println("")
	fmt.Println("Next steps:")
	fmt.Println("1. Platform: STACCATO_NATS_NKEY=secrets/platform.nk")
	fmt.Println("2. Agent: STACCATO_NATS_NKEY=secrets/agents/<agent-id>.nk (or secrets/agent.nk for default local agent)")
	fmt.Println("3. Restart NATS after adding/removing agent keys.")
}

func runAddAgent(agentID string) {
	seedPath, created, err := natsauth.CreateAgentSeed(agentID, "secrets/agents")
	if err != nil {
		fatalf("create agent seed: %v", err)
	}
	_, err = natsauth.EnsureBootstrap("secrets/platform.nk", "secrets/agent.nk", "secrets/agents", "nats/server.conf")
	if err != nil {
		fatalf("render nats config: %v", err)
	}
	if created {
		fmt.Printf("Created agent key: %s\n", seedPath)
	} else {
		fmt.Printf("Agent key already exists: %s\n", seedPath)
	}
	fmt.Println("Updated nats/server.conf with current agent keys.")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
