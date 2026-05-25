package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/example/envoy/internal/agent"
	"github.com/example/envoy/internal/config"
	"github.com/example/envoy/internal/manifest"
	"github.com/example/envoy/internal/queue"
)

func main() {
	cfg := config.AgentFromEnv()
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
