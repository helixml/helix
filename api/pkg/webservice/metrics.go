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
	metricRecoveryDuration.DeleteLabelValues(projectID)
	metricRecoveryTotal.DeleteLabelValues(projectID, "success")
	metricRecoveryTotal.DeleteLabelValues(projectID, "failure")
}
