package server

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/sessionresources"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

const (
	sessionResourcesMaxPoints = 360
	sessionPlacementTTL       = time.Minute
)

func newSessionResourceStore(cfg *config.ServerConfig) *sessionresources.Store {
	return sessionresources.New(
		filepath.Join(cfg.FileStore.LocalFSPath, ".session-resources"),
		base64.RawURLEncoding.EncodeToString([]byte(cfg.HostHistoryNode)),
	)
}

// sessionPlacements caches which sandbox each session runs on, so heartbeats
// every 30 seconds don't each load every session. A session whose spec task or
// project has been deleted has no placement: its desktop may still be running,
// but its history must not come back.
var sessionPlacements = struct {
	sync.Mutex
	bySession map[string]sessionPlacement
}{bySession: map[string]sessionPlacement{}}

type sessionPlacement struct {
	sandboxID string
	checked   time.Time
}

func (apiServer *HelixAPIServer) sessionSandbox(ctx context.Context, sessionID string, now time.Time) (string, error) {
	sessionPlacements.Lock()
	cached, ok := sessionPlacements.bySession[sessionID]
	sessionPlacements.Unlock()
	if ok && now.Sub(cached.checked) < sessionPlacementTTL {
		return cached.sandboxID, nil
	}
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if id := session.Metadata.SpecTaskID; id != "" {
		if _, err := apiServer.Store.GetSpecTask(ctx, id); err != nil {
			return "", fmt.Errorf("spec task %s: %w", id, err)
		}
	}
	if id := session.ProjectID; id != "" {
		if _, err := apiServer.Store.GetProject(ctx, id); err != nil {
			return "", fmt.Errorf("project %s: %w", id, err)
		}
	}
	sessionPlacements.Lock()
	defer sessionPlacements.Unlock()
	for id, p := range sessionPlacements.bySession {
		if now.Sub(p.checked) >= sessionPlacementTTL {
			delete(sessionPlacements.bySession, id)
		}
	}
	sessionPlacements.bySession[sessionID] = sessionPlacement{sandboxID: session.SandboxID, checked: now}
	return session.SandboxID, nil
}

// recordSessionResources keeps the samples of sessions placed on the reporting
// sandbox. A runner cannot write history for a session running elsewhere.
func (apiServer *HelixAPIServer) recordSessionResources(ctx context.Context, sandboxID string, samples []types.SessionResourceSample, now time.Time) {
	if len(samples) == 0 {
		return
	}
	accepted := samples[:0:0]
	for _, sample := range samples {
		placed, err := apiServer.sessionSandbox(ctx, sample.SessionID, now)
		if err != nil || placed != sandboxID {
			log.Debug().Err(err).Str("sandbox_id", sandboxID).Str("session_id", sample.SessionID).Str("placed_on", placed).
				Msg("Dropping session resource sample from a sandbox the session is not placed on")
			continue
		}
		sample.Timestamp = now.UnixMilli()
		accepted = append(accepted, sample)
	}
	apiServer.sessionResources.Record(accepted)
}

// getSessionResources godoc
// @Summary Get a session's sandbox resource utilization
// @Description CPU, memory, I/O, pressure stalls, throttling and filesystem use of the session's desktop container, sampled every 30 seconds and kept for 30 days.
// @Tags    sessions
// @Success 200 {object} types.SessionResources
// @Param id path string true "Session ID"
// @Param hours query int false "Hours of history, 1 through 720 (default 24)"
// @Router /api/v1/sessions/{id}/resources [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getSessionResources(_ http.ResponseWriter, req *http.Request) (*types.SessionResources, *system.HTTPError) {
	ctx := req.Context()
	id := mux.Vars(req)["id"]
	hours := 24
	if v := req.URL.Query().Get("hours"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > sessionresources.Days*24 {
			return nil, system.NewHTTPError400("hours must be between 1 and 720")
		}
		hours = n
	}

	session, err := apiServer.Store.GetSession(ctx, id)
	if err != nil {
		return nil, system.NewHTTPError404(fmt.Sprintf("session %s not found", id))
	}
	if err := apiServer.authorizeUserToSession(ctx, getRequestUser(req), session, types.ActionGet); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	now := time.Now()
	samples, err := apiServer.sessionResources.Read(session.ID, now.Add(-time.Duration(hours)*time.Hour), now)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to read session resources: %s", err))
	}
	return sessionresources.Summarize(session.ID, samples, sessionResourcesMaxPoints), nil
}

// deleteSessionResources removes the resource history of sessions being
// deleted with their spec task, project or user. Archiving keeps it.
func (apiServer *HelixAPIServer) deleteSessionResources(sessionIDs []string) error {
	if len(sessionIDs) == 0 {
		return nil
	}
	sessionPlacements.Lock()
	for _, id := range sessionIDs {
		delete(sessionPlacements.bySession, id)
	}
	sessionPlacements.Unlock()
	return apiServer.sessionResources.Delete(sessionIDs)
}

// specTaskSessionIDs lists every session a spec task has run, including its
// planning session even when that row is already gone.
func (apiServer *HelixAPIServer) specTaskSessionIDs(ctx context.Context, task *types.SpecTask) ([]string, error) {
	sessions, _, err := apiServer.Store.ListSessions(ctx, store.ListSessionsQuery{
		SpecTaskID: task.ID, AnyOwner: true, IncludeExternalAgents: true,
	})
	if err != nil {
		return nil, err
	}
	ids := []string{}
	if task.PlanningSessionID != "" {
		ids = append(ids, task.PlanningSessionID)
	}
	for _, s := range sessions {
		if s.ID != task.PlanningSessionID {
			ids = append(ids, s.ID)
		}
	}
	return ids, nil
}
