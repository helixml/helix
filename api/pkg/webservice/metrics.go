package webservice

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus metrics for hosted web-service reliability. Scraped from the API's
// /metrics endpoint so a service going down — or recovery looping — pages an
// operator. (Nothing alerted when find-ai went down mid customer-demo; this
// closes that gap.) Cardinality is bounded by the number of hosted web services
// (a handful), and stale project label sets are dropped in HealthMonitor.gc.
var (
	metricUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "helix_webservice_up",
		Help: "1 if the project's hosted web service last probed healthy, 0 if it is failing.",
	}, []string{"project_id"})

	metricRecoveryTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "helix_webservice_recovery_total",
		Help: "Total hosted web-service auto-recovery attempts, by result.",
	}, []string{"project_id", "result"})

	metricConsecutiveRecoveryFailures = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "helix_webservice_consecutive_recovery_failures",
		Help: "Consecutive failed auto-recoveries for a project (0 when healthy); a high value means recovery is looping and needs an operator.",
	}, []string{"project_id"})

	// metricUnhealthySince is THE signal to page on. helix_webservice_up is not:
	// it is set to 1 whenever a deploy is in flight, and auto-recovery redeploys
	// a broken service every ~11 minutes, so a permanently-503 site produces a
	// square wave rather than a flat 0. During the we-find.ai outage (2026-07-28)
	// that made an Alertmanager rule on `up == 0` fire and RESOLVE on every
	// recovery attempt for five days — the operator saw "down, then resolved" and
	// reasonably read it as a self-healing blip.
	//
	// This gauge is monotonic while a service is down: it records when the
	// current down-streak began and is cleared ONLY by a genuinely successful
	// probe. A recovery attempt is an attempt to fix, not evidence of health, so
	// an in-flight deploy does not clear it. Alert on
	// `time() - helix_webservice_unhealthy_since_seconds > <threshold>`.
	metricUnhealthySince = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "helix_webservice_unhealthy_since_seconds",
		Help: "Unix timestamp when this web service's current down-streak began; absent when healthy. Alert on time() minus this value — do NOT page on helix_webservice_up, which flaps during recovery redeploys.",
	}, []string{"project_id"})

	// metricUpstreamErrors counts requests a customer actually made that we could
	// not serve. Everything else here infers health from the platform's own state
	// machine; this is the one signal that cannot be masked by a bug in it.
	metricUpstreamErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "helix_webservice_upstream_errors_total",
		Help: "Requests to a hosted web service that could not be proxied to its upstream (holding page served).",
	}, []string{"project_id"})

	metricRecoveryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "helix_webservice_recovery_duration_seconds",
		Help:    "Wall-clock duration of hosted web-service auto-recovery attempts.",
		Buckets: []float64{5, 15, 30, 60, 120, 300, 600},
	}, []string{"project_id"})
)

// forgetProjectMetrics drops all metric series for a project that is no longer
// an active web service, so the label cardinality stays bounded.
func forgetProjectMetrics(projectID string) {
	metricUp.DeleteLabelValues(projectID)
	metricConsecutiveRecoveryFailures.DeleteLabelValues(projectID)
	metricUnhealthySince.DeleteLabelValues(projectID)
	metricUpstreamErrors.DeleteLabelValues(projectID)
	metricRecoveryDuration.DeleteLabelValues(projectID)
	metricRecoveryTotal.DeleteLabelValues(projectID, "success")
	metricRecoveryTotal.DeleteLabelValues(projectID, "failure")
}

// RecordUpstreamError counts a request that could not be proxied to a project's
// hosted web service. Called from the vhost proxy's error handler — the point
// where a real customer request actually failed.
func RecordUpstreamError(projectID string) {
	if projectID == "" {
		return
	}
	metricUpstreamErrors.WithLabelValues(projectID).Inc()
}
