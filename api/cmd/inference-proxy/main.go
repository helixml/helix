// Command inference-proxy is a long-running HTTP server that lives inside
// the Helix sandbox and serves OpenAI-compatible inference requests by
// reading the `model` field from each request body and forwarding to the
// matching container in the inner dockerd via Docker DNS.
//
// It rebuilds its routing table when the active compose YAML changes
// (signalled by SIGHUP or a poll against /etc/helix/active.yaml mtime).
package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/inferenceproxy"
)

func main() {
	var (
		listenAddr = flag.String("listen", envOr("HELIX_INFERENCE_PROXY_LISTEN", "0.0.0.0:8090"), "address to listen on")
		composePath = flag.String("compose", envOr("HELIX_RUNNER_ACTIVE_YAML", "/etc/helix/active.yaml"), "path to active compose YAML")
	)
	flag.Parse()

	lookup := inferenceproxy.Empty()
	reload := func() {
		data, err := os.ReadFile(*composePath)
		if err != nil {
			if !os.IsNotExist(err) {
				log.Warn().Err(err).Str("path", *composePath).Msg("inference-proxy: read compose")
			}
			lookup.Replace(inferenceproxy.Empty())
			return
		}
		next, err := inferenceproxy.NewLookup(string(data))
		if err != nil {
			log.Warn().Err(err).Msg("inference-proxy: parse compose")
			return
		}
		lookup.Replace(next)
		log.Info().Strs("models", next.Models()).Msg("inference-proxy: routing table updated")
	}
	reload() // initial load

	// SIGHUP triggers immediate reload (compose-manager sends one after
	// every successful Apply).
	hupCh := make(chan os.Signal, 1)
	signal.Notify(hupCh, syscall.SIGHUP)
	go func() {
		for range hupCh {
			reload()
		}
	}()

	// Belt-and-braces poll on file mtime so a missed signal still
	// converges within ~30s.
	go func() {
		var lastMod time.Time
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for range t.C {
			fi, err := os.Stat(*composePath)
			if err != nil {
				continue
			}
			if fi.ModTime() != lastMod {
				lastMod = fi.ModTime()
				reload()
			}
		}
	}()

	srv := &http.Server{
		Addr:              *listenAddr,
		Handler:           inferenceproxy.Handler(lookup),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		log.Info().Msg("inference-proxy: shutting down")
		shutdownCtx, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		_ = srv.Shutdown(shutdownCtx)
		cancel()
	}()

	log.Info().Str("listen", *listenAddr).Msg("inference-proxy: ready")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal().Err(err).Msg("inference-proxy: serve")
	}
	<-ctx.Done()
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
