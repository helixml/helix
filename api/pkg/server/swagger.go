package server

import (
	_ "embed"
	"net/http"
)

// @title HelixML API reference
// @version 0.1
// @description This is the HelixML API.
// @contact.name Helix support
// @contact.url https://app.helix.ml/
// @contact.email info@helix.ml
// @x-logo {"url": "https://avatars.githubusercontent.com/u/149581110?s=200&v=4", "altText": "Helix logo"}
// @host app.helix.ml
// @Schemes https
// @securityDefinitions.bearer BearerAuth
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization

//go:embed swagger.json
var swagger []byte

func (s *HelixAPIServer) swaggerHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(swagger); err != nil {
			http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		}
	})
}
