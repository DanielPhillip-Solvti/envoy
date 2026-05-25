package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/envoy/internal/config"
	"github.com/example/envoy/internal/platform"
	"github.com/example/envoy/internal/queue"
	"github.com/example/envoy/internal/web"
)

func main() {
	cfg := config.PlatformFromEnv()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	bus, err := queue.Connect(cfg.NATSURL, cfg.NATSNKey)
	if err != nil {
		log.Fatalf("connect nats: %v", err)
	}
	defer bus.Close()

	state := platform.NewMemoryState(time.Now)
	if err := platform.Subscribe(ctx, bus, state); err != nil {
		log.Fatalf("subscribe platform events: %v", err)
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           web.NewServer(bus, state),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("platform listening on %s", cfg.HTTPAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("platform server: %v", err)
	}
}
