package server

import (
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"

	"github.com/helixml/helix/api/pkg/config"
)

func runnerNATSRouteRegistered(enabled bool) bool {
	apiServer := &HelixAPIServer{Cfg: &config.ServerConfig{}}
	apiServer.Cfg.WebServer.RunnerNATSProxyEnabled = enabled
	router := mux.NewRouter()
	apiServer.registerRunnerNATSProxy(router)
	var match mux.RouteMatch
	return router.Match(httptest.NewRequest("GET", "/runner/any-runner/ws", nil), &match)
}

func TestRunnerNATSProxy_NotRegisteredByDefault(t *testing.T) {
	if runnerNATSRouteRegistered(false) {
		t.Fatal("the NATS proxy route is registered although RUNNER_NATS_PROXY_ENABLED is off")
	}
}

func TestRunnerNATSProxy_RegisteredWhenEnabled(t *testing.T) {
	if !runnerNATSRouteRegistered(true) {
		t.Fatal("the NATS proxy route is missing although RUNNER_NATS_PROXY_ENABLED is on")
	}
}
