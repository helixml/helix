package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog/log"
)

const sandboxOOMMetricRetention = time.Hour

var (
	sandboxOOMKills = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "helix_sandbox_oom_kills_total",
		Help: "Cgroup OOM kills observed for Helix sandbox containers.",
	}, []string{"runner_id", "runner_host", "sandbox_id", "session_id", "container_id", "project_id", "project_name", "organization_id", "organization_name"})
	sandboxOOMState = struct {
		sync.Mutex
		containers  map[string]sandboxOOMContainerState
		runnerHosts map[string]string
	}{containers: make(map[string]sandboxOOMContainerState), runnerHosts: make(map[string]string)}
)

type sandboxOOMContainerState struct {
	count            uint64
	labels           []string
	lastSeen         time.Time
	metadataComplete bool
}

func (apiServer *HelixAPIServer) updateSandboxOOMMetrics(ctx context.Context, runnerID string, reports []types.ContainerOOMKills, now time.Time) {
	runnerHost := ""
	if len(reports) > 0 {
		runnerHost = apiServer.sandboxOOMRunnerHost(ctx, runnerID)
	}
	for _, report := range reports {
		key := runnerID + "\x00" + report.ContainerID
		retryMetadata := false
		sandboxOOMState.Lock()
		previous, exists := sandboxOOMState.containers[key]
		if exists {
			if !previous.metadataComplete && previous.count == 0 && report.Count == 0 {
				retryMetadata = true
				sandboxOOMState.Unlock()
			} else {
				delta := report.Count
				if report.Count >= previous.count {
					delta = report.Count - previous.count
				}
				sandboxOOMKills.WithLabelValues(previous.labels...).Add(float64(delta))
				previous.count = report.Count
				previous.lastSeen = now
				sandboxOOMState.containers[key] = previous
				sandboxOOMState.Unlock()
				continue
			}
		} else {
			sandboxOOMState.Unlock()
		}

		labels, err := apiServer.sandboxOOMLabels(ctx, runnerID, runnerHost, report)
		if err != nil {
			log.Warn().Err(err).Str("runner_id", runnerID).Str("session_id", report.SessionID).
				Msg("Sandbox OOM metric has incomplete metadata")
		}

		sandboxOOMState.Lock()
		previous, exists = sandboxOOMState.containers[key]
		delta := report.Count
		if exists {
			if retryMetadata && err == nil {
				sandboxOOMKills.DeleteLabelValues(previous.labels...)
			} else {
				labels = previous.labels
				if report.Count >= previous.count {
					delta = report.Count - previous.count
				}
			}
		}
		sandboxOOMKills.WithLabelValues(labels...).Add(float64(delta))
		sandboxOOMState.containers[key] = sandboxOOMContainerState{
			count: report.Count, labels: labels, lastSeen: now, metadataComplete: err == nil,
		}
		sandboxOOMState.Unlock()
	}

	sandboxOOMState.Lock()
	defer sandboxOOMState.Unlock()
	for key, state := range sandboxOOMState.containers {
		if now.Sub(state.lastSeen) < sandboxOOMMetricRetention {
			continue
		}
		sandboxOOMKills.DeleteLabelValues(state.labels...)
		delete(sandboxOOMState.containers, key)
	}
}

func (apiServer *HelixAPIServer) sandboxOOMRunnerHost(ctx context.Context, runnerID string) string {
	if runnerID == "" {
		return ""
	}
	sandboxOOMState.Lock()
	host, ok := sandboxOOMState.runnerHosts[runnerID]
	sandboxOOMState.Unlock()
	if ok {
		return host
	}
	instance, err := apiServer.Store.GetSandboxInstance(ctx, runnerID)
	if err != nil {
		log.Warn().Err(err).Str("runner_id", runnerID).Msg("Sandbox OOM metric has no runner hostname")
		return ""
	}
	sandboxOOMState.Lock()
	sandboxOOMState.runnerHosts[runnerID] = instance.Hostname
	sandboxOOMState.Unlock()
	return instance.Hostname
}

func (apiServer *HelixAPIServer) sandboxOOMLabels(ctx context.Context, runnerID, runnerHost string, report types.ContainerOOMKills) ([]string, error) {
	sandboxID := ""
	var projectID, organizationID string
	var resolveErr error
	if strings.HasPrefix(report.SessionID, "sbx_") {
		sandboxID = report.SessionID
		sandbox, err := apiServer.Store.GetSandbox(ctx, report.SessionID)
		if err != nil {
			resolveErr = errors.Join(resolveErr, fmt.Errorf("get sandbox %s: %w", report.SessionID, err))
		} else {
			projectID = sandbox.ProjectID
			organizationID = sandbox.OrganizationID
		}
	} else if strings.HasPrefix(report.SessionID, "ses_") {
		session, err := apiServer.Store.GetSession(ctx, report.SessionID)
		if err != nil {
			resolveErr = errors.Join(resolveErr, fmt.Errorf("get session %s: %w", report.SessionID, err))
		} else {
			projectID = session.ProjectID
			organizationID = session.OrganizationID
		}
		sandbox, err := apiServer.Store.GetSandboxBySession(ctx, report.SessionID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			resolveErr = errors.Join(resolveErr, fmt.Errorf("get sandbox for session %s: %w", report.SessionID, err))
		} else if err == nil {
			sandboxID = sandbox.ID
		}
	} else {
		resolveErr = fmt.Errorf("unsupported helix.session_id %q", report.SessionID)
	}

	projectName := ""
	if projectID != "" {
		project, err := apiServer.Store.GetProject(ctx, projectID)
		if err != nil {
			resolveErr = errors.Join(resolveErr, fmt.Errorf("get project %s: %w", projectID, err))
		} else {
			projectName = project.Name
			organizationID = project.OrganizationID
		}
	}
	organizationName := "personal"
	if organizationID != "" {
		organization, err := apiServer.Store.GetOrganization(ctx, &store.GetOrganizationQuery{ID: organizationID})
		if err != nil {
			organizationName = ""
			resolveErr = errors.Join(resolveErr, fmt.Errorf("get organization %s: %w", organizationID, err))
		} else {
			organizationName = organization.Name
		}
	}
	return []string{runnerID, runnerHost, sandboxID, report.SessionID, report.ContainerID, projectID, projectName, organizationID, organizationName}, resolveErr
}
