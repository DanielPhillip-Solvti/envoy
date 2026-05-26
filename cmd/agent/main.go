package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/example/staccato/internal/agent"
	"github.com/example/staccato/internal/config"
	"github.com/example/staccato/internal/manifest"
	"github.com/example/staccato/internal/queue"
)

func main() {
	cfg := config.AgentFromEnv()
	if cfg.NATSNKey == "" {
		cfg.NATSNKey = "secrets/agent.nk"
	}
	if _, err := os.Stat(cfg.NATSNKey); err != nil {
		log.Fatalf("STACCATO_NATS_NKEY seed file is required and must exist: %v", err)
	}
	mf, err := manifest.Load(cfg.ManifestPath)
	if err != nil {
		log.Fatalf("load manifest: %v", err)
	}

	bus, err := queue.Connect(cfg.NATSURL, cfg.NATSNKey)
	if err != nil {
		log.Fatalf("connect nats: %v", err)
	}
	defer bus.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runner := agent.NewRunner(mf, bus, cfg.ObjectDir)
	if err := runner.Run(ctx); err != nil {
		log.Fatalf("agent stopped: %v", err)
	}
}
