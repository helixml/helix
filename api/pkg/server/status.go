package server

import (
	"net/http"

	"github.com/bacalhau-project/lilysaas/api/pkg/types"
)

type StatusResponse struct {
	User *types.User `json:"user"`
}

func (apiServer *LilysaasAPIServer) status(res http.ResponseWriter, req *http.Request) (StatusResponse, error) {
	user := getRequestUser(req.Context())
	return StatusResponse{
		User: user,
	}, nil
}
