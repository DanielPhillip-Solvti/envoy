package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/staccato/internal/config"
	"github.com/example/staccato/internal/natsauth"
	"github.com/example/staccato/internal/platform"
	"github.com/example/staccato/internal/queue"
	"github.com/example/staccato/internal/web"
)

func main() {
	cfg := config.PlatformFromEnv()
	if cfg.NATSNKey == "" {
		cfg.NATSNKey = "secrets/platform.nk"
	}
	if _, err := os.Stat(cfg.NATSNKey); err != nil {
		if os.IsNotExist(err) {
			generated, bootstrapErr := natsauth.EnsureBootstrap(cfg.NATSNKey, "secrets/agent.nk", "secrets/agents", "nats/server.conf")
			if bootstrapErr != nil {
				log.Fatalf("bootstrap nats auth: %v", bootstrapErr)
			}
			if generated {
				log.Printf("generated local NATS auth material at %s and nats/server.conf", cfg.NATSNKey)
			}
		} else {
			log.Fatalf("read STACCATO_NATS_NKEY: %v", err)
		}
	}
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

	webSrv, webHandler := web.NewServer(bus, state, cfg)
	webSrv.StartTokenRotation(ctx)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           webHandler,
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
