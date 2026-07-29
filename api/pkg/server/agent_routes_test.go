package server

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

func TestAgentAndAppRoutesMatch(t *testing.T) {
	authRouter := mux.NewRouter()
	insecureRouter := mux.NewRouter()
	new(HelixAPIServer).registerAgentRoutes(authRouter, insecureRouter)

	collect := func(router *mux.Router, resource string) []string {
		var routes []string
		if err := router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
			path, err := route.GetPathTemplate()
			if err != nil || !strings.HasPrefix(path, "/"+resource) {
				return nil
			}
			methods, err := route.GetMethods()
			if err != nil {
				return err
			}
			routes = append(routes, strings.TrimPrefix(path, "/"+resource)+" "+strings.Join(methods, ","))
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		sort.Strings(routes)
		return routes
	}

	for _, router := range []*mux.Router{authRouter, insecureRouter} {
		if agents, apps := collect(router, "agents"), collect(router, "apps"); !reflect.DeepEqual(agents, apps) {
			t.Fatalf("agent routes do not match app aliases:\nagents: %v\napps: %v", agents, apps)
		}
	}
}
