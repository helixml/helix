package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"

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

func (apiServer *LilysaasAPIServer) filestoreConfig(res http.ResponseWriter, req *http.Request) (filestore.FilestoreConfig, error) {
	return apiServer.Controller.FilestoreConfig(apiServer.getRequestContext(req))
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

func (apiServer *LilysaasAPIServer) filestoreRename(res http.ResponseWriter, req *http.Request) (filestore.FileStoreItem, error) {
	return apiServer.Controller.FilestoreRename(apiServer.getRequestContext(req), req.URL.Query().Get("path"), req.URL.Query().Get("new_path"))
}

func (apiServer *LilysaasAPIServer) filestoreDelete(res http.ResponseWriter, req *http.Request) (string, error) {
	path := req.URL.Query().Get("path")
	err := apiServer.Controller.FilestoreDelete(apiServer.getRequestContext(req), path)
	return path, err
}

func (apiServer *LilysaasAPIServer) filestoreUpload(res http.ResponseWriter, req *http.Request) (bool, error) {
	path := req.URL.Query().Get("path")
	err := req.ParseMultipartForm(10 << 20)
	if err != nil {
		return false, err
	}

	files := req.MultipartForm.File["files"]
	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			return false, fmt.Errorf("Unable to open file")
		}
		defer file.Close()
		_, err = apiServer.Controller.FilestoreUpload(apiServer.getRequestContext(req), filepath.Join(path, fileHeader.Filename), file)
		if err != nil {
			return false, fmt.Errorf("Unable to upload file: %s", err.Error())
		}
	}

	return true, nil
}

func (apiServer *LilysaasAPIServer) createAPIKey(res http.ResponseWriter, req *http.Request) (string, error) {
	name := req.URL.Query().Get("name")
	apiKey, err := apiServer.Controller.CreateAPIKey(apiServer.getRequestContext(req), name)
	if err != nil {
		return "", err
	}
	return apiKey, nil
}

func (apiServer *LilysaasAPIServer) getAPIKeys(res http.ResponseWriter, req *http.Request) ([]*types.ApiKey, error) {
	apiKeys, err := apiServer.Controller.GetAPIKeys(apiServer.getRequestContext(req))
	if err != nil {
		return nil, err
	}
	return apiKeys, nil
}

func (apiServer *LilysaasAPIServer) deleteAPIKey(res http.ResponseWriter, req *http.Request) (string, error) {
	apiKey := req.URL.Query().Get("key")
	err := apiServer.Controller.DeleteAPIKey(apiServer.getRequestContext(req), apiKey)
	if err != nil {
		return "", err
	}
	return "", nil
}

func (apiServer *LilysaasAPIServer) checkAPIKey(res http.ResponseWriter, req *http.Request) (*types.ApiKey, error) {
	apiKey := req.URL.Query().Get("key")
	key, err := apiServer.Controller.CheckAPIKey(apiServer.getRequestContext(req).Ctx, apiKey)
	if err != nil {
		return nil, err
	}
	return key, nil
}
