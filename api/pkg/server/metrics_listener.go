package server

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
)

// startMetricsListener starts a dedicated Prometheus /metrics HTTP server on
// Cfg.WebServer.MetricsListen, if configured. It is intentionally kept OFF the
// main API/vhost router so metrics are never reachable on the public app port —
// restrict access with a firewall to your Prometheus scraper. No-op when unset.
//
// It serves the default Prometheus registry, which includes the Go runtime and
// process collectors plus everything registered via promauto (e.g. the
// helix_webservice_* reliability metrics).
func (apiServer *HelixAPIServer) startMetricsListener(ctx context.Context) {
	addr := apiServer.Cfg.WebServer.MetricsListen
	if addr == "" {
		return
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	go func() {
		log.Info().Str("addr", addr).Msg("starting Prometheus /metrics listener")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Str("addr", addr).Msg("metrics listener failed")
		}
	}()
}
