package server

import (
	"net/http"
)

type statusResponse struct {
	User    string `json:"user"`
	Credits int    `json:"credits"`
}

func (apiServer *LilysaasAPIServer) status(res http.ResponseWriter, req *http.Request) (statusResponse, error) {
	return statusResponse{
		User:    getRequestUser(req),
		Credits: 100,
	}, nil
}
