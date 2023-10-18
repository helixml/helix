package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"github.com/bacalhau-project/lilypad/pkg/data"
	"github.com/bacalhau-project/lilysaas/api/pkg/filestore"
	"github.com/bacalhau-project/lilysaas/api/pkg/job"
	"github.com/bacalhau-project/lilysaas/api/pkg/store"
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

// TODO version of this which is session specific
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
			return false, fmt.Errorf("unable to open file")
		}
		defer file.Close()
		_, err = apiServer.Controller.FilestoreUpload(apiServer.getRequestContext(req), filepath.Join(path, fileHeader.Filename), file)
		if err != nil {
			return false, fmt.Errorf("unable to upload file: %s", err.Error())
		}
	}

	return true, nil
}

func (apiServer *LilysaasAPIServer) getSession(res http.ResponseWriter, req *http.Request) (*types.Session, error) {
	id := req.URL.Query().Get("id")
	reqContext := apiServer.getRequestContext(req)
	session, err := apiServer.Store.GetSession(reqContext.Ctx, id)
	if err != nil {
		return nil, err
	}
	if session.OwnerType != reqContext.OwnerType || session.Owner != reqContext.Owner {
		return nil, fmt.Errorf("access denied")
	}
	return session, nil
}

func (apiServer *LilysaasAPIServer) getSessions(res http.ResponseWriter, req *http.Request) ([]*types.Session, error) {
	reqContext := apiServer.getRequestContext(req)
	query := store.GetSessionsQuery{}
	query.Owner = reqContext.Owner
	query.OwnerType = reqContext.OwnerType
	return apiServer.Store.GetSessions(reqContext.Ctx, query)
}

func (apiServer *LilysaasAPIServer) createSession(res http.ResponseWriter, req *http.Request) (*types.Session, error) {
	reqContext := apiServer.getRequestContext(req)
	request := types.Session{}
	bs, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(bs, &request)
	if err != nil {
		return nil, err
	}
	// only allow users to create their own sessions
	request.Owner = reqContext.Owner
	request.OwnerType = reqContext.OwnerType
	return apiServer.Store.CreateSession(reqContext.Ctx, request)
}

func (apiServer *LilysaasAPIServer) updateSession(res http.ResponseWriter, req *http.Request) (*types.Session, error) {
	reqContext := apiServer.getRequestContext(req)
	request := types.Session{}
	bs, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	// TODO: consider only allow updating certain fields
	err = json.Unmarshal(bs, &request)
	if err != nil {
		return nil, err
	}
	if request.ID == "" {
		return nil, fmt.Errorf("cannot update session without id")
	}
	if request.Owner != reqContext.Owner || request.OwnerType != reqContext.OwnerType {
		return nil, fmt.Errorf("access denied")
	}
	request.Updated = time.Now()
	return apiServer.Store.UpdateSession(reqContext.Ctx, request)
}

func (apiServer *LilysaasAPIServer) deleteSession(res http.ResponseWriter, req *http.Request) (*types.Session, error) {
	reqContext := apiServer.getRequestContext(req)
	id := req.URL.Query().Get("id")
	session, err := apiServer.Store.GetSession(reqContext.Ctx, id)
	if err != nil {
		return nil, err
	}
	if session.OwnerType != reqContext.OwnerType || session.Owner != reqContext.Owner {
		return nil, fmt.Errorf("access denied")
	}
	return apiServer.Store.DeleteSession(reqContext.Ctx, id)
}
