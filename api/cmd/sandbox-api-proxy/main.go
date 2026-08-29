package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	listenAddr := flag.String("listen", "", "address to listen on")
	upstream := flag.String("upstream", os.Getenv("HELIX_API_URL"), "Helix API URL")
	flag.Parse()

	if *listenAddr == "" {
		slog.Error("listen address is required")
		os.Exit(1)
	}
	handler, err := newAPIProxy(*upstream)
	if err != nil {
		slog.Error("invalid upstream", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              *listenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("proxy shutdown failed", "error", err)
		}
	}()

	slog.Info("starting sandbox API proxy", "listen", *listenAddr, "upstream", *upstream)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("sandbox API proxy failed", "error", err)
		os.Exit(1)
	}
}

func newAPIProxy(rawUpstream string) (http.Handler, error) {
	if rawUpstream == "" {
		return nil, fmt.Errorf("upstream URL is required")
	}
	target, err := url.Parse(rawUpstream)
	if err != nil {
		return nil, fmt.Errorf("parse upstream URL: %w", err)
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, fmt.Errorf("upstream scheme must be http or https")
	}
	if target.Host == "" {
		return nil, fmt.Errorf("upstream host is required")
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		director(req)
		req.Host = target.Host
	}
	proxy.FlushInterval = -1
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, proxyErr error) {
		slog.Warn("sandbox API proxy request failed", "method", r.Method, "path", r.URL.Path, "error", proxyErr)
		http.Error(w, "Helix API unavailable", http.StatusBadGateway)
	}
	return proxy, nil
}
