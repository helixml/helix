package server

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

// browserWebsocketUpgrader returns an upgrader for WebSockets that browsers
// open with the user's cookies. A page on another origin must not be able to
// open one: the handshake is a GET that carries the session cookie, and
// SameSite=Lax does not stop a same-site page (a sandbox preview served on a
// subdomain of this deployment) from sending it. The Origin header is
// therefore required to match the request host or the configured server URL.
// Requests without an Origin header come from non-browser clients, which
// cannot be made to act with a victim's cookies, and are allowed through.
func browserWebsocketUpgrader(serverURL string, bufferSize int) websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:  bufferSize,
		WriteBufferSize: bufferSize,
		CheckOrigin: func(r *http.Request) bool {
			return websocketOriginAllowed(r, serverURL)
		},
	}
}

// allowBrowserWebsocketOrigin applies the same Origin policy to handlers that
// hijack the connection themselves (the desktop input and video proxies)
// instead of going through an Upgrader. It answers 403 and returns false for
// a foreign Origin.
func (apiServer *HelixAPIServer) allowBrowserWebsocketOrigin(w http.ResponseWriter, r *http.Request) bool {
	var serverURL string
	if apiServer.Cfg != nil {
		serverURL = apiServer.Cfg.WebServer.URL
	}
	if websocketOriginAllowed(r, serverURL) {
		return true
	}
	http.Error(w, "websocket: request origin not allowed", http.StatusForbidden)
	return false
}

// websocketOriginAllowed reports whether r's Origin header, if any, names
// this deployment: the host the request was sent to, or the host of
// serverURL.
func websocketOriginAllowed(r *http.Request, serverURL string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	if strings.EqualFold(parsed.Host, r.Host) {
		return true
	}
	if serverURL == "" {
		return false
	}
	server, err := url.Parse(serverURL)
	if err != nil || server.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Host, server.Host)
}
