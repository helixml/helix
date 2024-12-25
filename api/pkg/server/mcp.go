package server

import (
	"context"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/helixml/helix/api/pkg/mcp"
)

func (apiServer *HelixAPIServer) startModelProxyServer(ctx context.Context) error {
	mcpServer := mcp.NewServer()

	return mcpServer.Run(ctx)
}

func modelContextProtocolHandler() http.Handler {
	u, err := url.Parse("http://localhost:21000")
	if err != nil {
		panic(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(u)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	})
}
