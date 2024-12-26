package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func (apiServer *HelixAPIServer) startModelProxyServer(ctx context.Context) error {
	return apiServer.mcpServer.Run(ctx)
}

func modelContextProtocolHandler() http.Handler {
	u, err := url.Parse("http://localhost:21000")
	if err != nil {
		panic(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(u)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("XX path", r.URL.Path)
		fmt.Println("XX method", r.Method)

		u, p, ok := r.BasicAuth()
		fmt.Println("XX u", u)
		fmt.Println("XX p", p)
		fmt.Println("XX ok", ok)

		proxy.ServeHTTP(w, r)
	})
}
