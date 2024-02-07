package server

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/helixml/helix/api/pkg/types"
)

// startSessionHandler can be used to start or continue a session with the Helix API.
func (apiServer *HelixAPIServer) startSessionHandler(res http.ResponseWriter, req *http.Request) {

	var startSessionRequest types.StartSessionRequest
	err := json.NewDecoder(io.LimitReader(req.Body, 10*MEGABYTE)).Decode(&startSessionRequest)
	if err != nil {
		http.Error(res, "invalid request body", http.StatusBadRequest)
		return
	}

	userContext := apiServer.getRequestContext(req)

	status, err := apiServer.Controller.GetStatus(userContext)
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	var (
		session *types.Session
		err     error
	)

	if startSessionRequest.SessionID != "" {

	} else {

	}

}
