package server

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/bacalhau-project/lilypad/pkg/data"
	"github.com/bacalhau-project/lilysaas/api/pkg/job"
	"github.com/bacalhau-project/lilysaas/api/pkg/types"
)

func (apiServer *LilysaasAPIServer) status(res http.ResponseWriter, req *http.Request) (types.UserStatus, error) {
	return apiServer.Controller.GetStatus(apiServer.getRequestContext(req))
}

func (apiServer *LilysaasAPIServer) getJobs(res http.ResponseWriter, req *http.Request) ([]*types.Job, error) {
	return apiServer.Controller.GetJobs(apiServer.getRequestContext(req))
}

func (apiServer *LilysaasAPIServer) getModules(res http.ResponseWriter, req *http.Request) ([]types.Module, error) {
	return job.GetModules()
}

func (apiServer *LilysaasAPIServer) createJob(res http.ResponseWriter, req *http.Request) (data.JobOfferContainer, error) {
	request := types.JobSpec{}
	bs, err := io.ReadAll(req.Body)
	if err != nil {
		return data.JobOfferContainer{}, err
	}
	err = json.Unmarshal(bs, &request)
	if err != nil {
		return data.JobOfferContainer{}, err
	}
	return apiServer.Controller.CreateJob(apiServer.getRequestContext(req), request)
}
