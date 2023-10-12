package server

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/bacalhau-project/lilypad/pkg/data"
	"github.com/bacalhau-project/lilysaas/api/pkg/filestore"
	"github.com/bacalhau-project/lilysaas/api/pkg/job"
	"github.com/bacalhau-project/lilysaas/api/pkg/types"
)

func (apiServer *LilysaasAPIServer) status(res http.ResponseWriter, req *http.Request) (types.UserStatus, error) {
	return apiServer.Controller.GetStatus(apiServer.getRequestContext(req))
}

func (apiServer *LilysaasAPIServer) getJobs(res http.ResponseWriter, req *http.Request) ([]*types.Job, error) {
	return apiServer.Controller.GetJobs(apiServer.getRequestContext(req))
}

func (apiServer *LilysaasAPIServer) getTransactions(res http.ResponseWriter, req *http.Request) ([]*types.BalanceTransfer, error) {
	return apiServer.Controller.GetTransactions(apiServer.getRequestContext(req))
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

func (apiServer *LilysaasAPIServer) filestoreList(res http.ResponseWriter, req *http.Request) ([]filestore.FileStoreItem, error) {
	return apiServer.Controller.FilestoreList(apiServer.getRequestContext(req), req.URL.Query().Get("path"))
}

func (apiServer *LilysaasAPIServer) filestoreGet(res http.ResponseWriter, req *http.Request) (filestore.FileStoreItem, error) {
	return apiServer.Controller.FilestoreGet(apiServer.getRequestContext(req), req.URL.Query().Get("path"))
}

func (apiServer *LilysaasAPIServer) filestoreCreateFolder(res http.ResponseWriter, req *http.Request) (filestore.FileStoreItem, error) {
	return apiServer.Controller.FilestoreCreateFolder(apiServer.getRequestContext(req), req.URL.Query().Get("path"))
}

func (apiServer *LilysaasAPIServer) filestoreUpload(res http.ResponseWriter, req *http.Request) (filestore.FileStoreItem, error) {
	return apiServer.Controller.FilestoreUpload(apiServer.getRequestContext(req), req.URL.Query().Get("path"), req.Body)
}

func (apiServer *LilysaasAPIServer) filestoreRename(res http.ResponseWriter, req *http.Request) (filestore.FileStoreItem, error) {
	return apiServer.Controller.FilestoreRename(apiServer.getRequestContext(req), req.URL.Query().Get("path"), req.URL.Query().Get("new_path"))
}

func (apiServer *LilysaasAPIServer) filestoreDelete(res http.ResponseWriter, req *http.Request) (string, error) {
	path := req.URL.Query().Get("path")
	err := apiServer.Controller.FilestoreDelete(apiServer.getRequestContext(req), path)
	return path, err
}
