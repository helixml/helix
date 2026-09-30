package server

import (
	"context"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
)

const metricsStoreTimeout = 2 * time.Second

func desktopHostAvailable(hosts []*types.SandboxInstance, now time.Time) float64 {
	cutoff := now.Add(-config.DefaultSandboxDispatchStaleThreshold)
	for _, host := range hosts {
		if host.Status == sandboxInstanceStatusOnline && host.LastSeen.After(cutoff) && host.CanHostDesktop() {
			return 1
		}
	}
	return 0
}

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
	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "helix_sandbox_desktop_host_available",
		Help: "Whether at least one online, recently heartbeating sandbox host can run streamed desktops.",
	}, func() float64 {
		queryCtx, cancel := context.WithTimeout(ctx, metricsStoreTimeout)
		defer cancel()
		hosts, err := apiServer.Store.ListSandboxInstances(queryCtx)
		if err != nil {
			log.Warn().Err(err).Msg("failed to collect sandbox desktop host availability metric")
			return 0
		}
		return desktopHostAvailable(hosts, time.Now())
	}))
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(prometheus.Gatherers{prometheus.DefaultGatherer, registry}, promhttp.HandlerOpts{}))
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
