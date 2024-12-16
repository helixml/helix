package server

import (
	_ "embed"
	"net/http"
)

// @title HelixML API reference
// @version 0.1
// @description This is a HelixML AI API.

// @contact.name Helix support
// @contact.url https://app.tryhelix.ai/
// @contact.email info@helix.ml
// @x-logo {"url": "https://avatars.githubusercontent.com/u/149581110?s=200&v=4", "altText": "Helix logo"}
// @host app.tryhelix.ai
// @Schemes https

// @securityDefinitions.bearer BearerAuth

//go:embed swagger.json
var swagger []byte

func (s *HelixAPIServer) swaggerHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(swagger); err != nil {
			http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		}
	})
}
