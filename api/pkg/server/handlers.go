package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/lukemarsden/helix/api/pkg/filestore"
	"github.com/lukemarsden/helix/api/pkg/model"
	"github.com/lukemarsden/helix/api/pkg/store"
	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/lukemarsden/helix/api/pkg/types"
)

func (apiServer *HelixAPIServer) status(res http.ResponseWriter, req *http.Request) (types.UserStatus, error) {
	return apiServer.Controller.GetStatus(apiServer.getRequestContext(req))
}

func (apiServer *HelixAPIServer) getTransactions(res http.ResponseWriter, req *http.Request) ([]*types.BalanceTransfer, error) {
	return apiServer.Controller.GetTransactions(apiServer.getRequestContext(req))
}

func (apiServer *HelixAPIServer) filestoreConfig(res http.ResponseWriter, req *http.Request) (filestore.FilestoreConfig, error) {
	return apiServer.Controller.FilestoreConfig(apiServer.getRequestContext(req))
}

func (apiServer *HelixAPIServer) filestoreList(res http.ResponseWriter, req *http.Request) ([]filestore.FileStoreItem, error) {
	return apiServer.Controller.FilestoreList(apiServer.getRequestContext(req), req.URL.Query().Get("path"))
}

func (apiServer *HelixAPIServer) filestoreGet(res http.ResponseWriter, req *http.Request) (filestore.FileStoreItem, error) {
	return apiServer.Controller.FilestoreGet(apiServer.getRequestContext(req), req.URL.Query().Get("path"))
}

func (apiServer *HelixAPIServer) filestoreCreateFolder(res http.ResponseWriter, req *http.Request) (filestore.FileStoreItem, error) {
	return apiServer.Controller.FilestoreCreateFolder(apiServer.getRequestContext(req), req.URL.Query().Get("path"))
}

func (apiServer *HelixAPIServer) filestoreRename(res http.ResponseWriter, req *http.Request) (filestore.FileStoreItem, error) {
	return apiServer.Controller.FilestoreRename(apiServer.getRequestContext(req), req.URL.Query().Get("path"), req.URL.Query().Get("new_path"))
}

func (apiServer *HelixAPIServer) filestoreDelete(res http.ResponseWriter, req *http.Request) (string, error) {
	path := req.URL.Query().Get("path")
	err := apiServer.Controller.FilestoreDelete(apiServer.getRequestContext(req), path)
	return path, err
}

// TODO version of this which is session specific
func (apiServer *HelixAPIServer) filestoreUpload(res http.ResponseWriter, req *http.Request) (bool, error) {
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

func (apiServer *HelixAPIServer) getSession(res http.ResponseWriter, req *http.Request) (*types.Session, error) {
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

func (apiServer *HelixAPIServer) getSessions(res http.ResponseWriter, req *http.Request) ([]*types.Session, error) {
	reqContext := apiServer.getRequestContext(req)
	query := store.GetSessionsQuery{}
	query.Owner = reqContext.Owner
	query.OwnerType = reqContext.OwnerType
	return apiServer.Store.GetSessions(reqContext.Ctx, query)
}

func (apiServer *HelixAPIServer) createSession(res http.ResponseWriter, req *http.Request) (*types.Session, error) {
	reqContext := apiServer.getRequestContext(req)

	// now upload any files that were included
	err := req.ParseMultipartForm(10 << 20)
	if err != nil {
		return nil, err
	}

	sessionMode, err := types.ValidateSessionMode(req.FormValue("mode"), false)
	if err != nil {
		return nil, err
	}

	sessionType, err := types.ValidateSessionType(req.FormValue("type"), false)
	if err != nil {
		return nil, err
	}

	session := types.Session{
		ID:   system.GenerateUUID(),
		Name: system.GenerateAmusingName(),
		Type: sessionType,
		Mode: sessionMode,
	}

	modelName, err := model.GetModelNameForSession(reqContext.Ctx, &session)
	if err != nil {
		return nil, err
	}

	session.ModelName = modelName

	// only allow users to create their own sessions
	session.Owner = reqContext.Owner
	session.OwnerType = reqContext.OwnerType

	paths := []string{}
	files := req.MultipartForm.File["files"]
	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			return nil, fmt.Errorf("unable to open file")
		}
		defer file.Close()
		path := fmt.Sprintf("/sessions/%s", session.ID)
		paths = append(paths, filepath.Join(path, fileHeader.Filename))
		log.Printf("uploading file %s/%s", path, fileHeader.Filename)
		_, err = apiServer.Controller.FilestoreUpload(apiServer.getRequestContext(req), filepath.Join(path, fileHeader.Filename), file)
		if err != nil {
			return nil, fmt.Errorf("unable to upload file: %s", err.Error())
		}
		log.Printf("success!")
	}

	// so far it's a chat with one message and some uploads
	session.Interactions = []types.Interaction{
		{
			Creator:  types.CreatorTypeUser,
			Message:  req.FormValue("input"),
			Files:    paths,
			Finished: true,
		},
	}

	// create session in database
	sessionData, err := apiServer.Store.CreateSession(reqContext.Ctx, session)
	if err != nil {
		return nil, err
	}

	// add the session to the controller queue
	err = apiServer.Controller.PushSessionQueue(reqContext.Ctx, sessionData)
	if err != nil {
		return nil, err
	}

	return sessionData, nil
}

func (apiServer *HelixAPIServer) updateSession(res http.ResponseWriter, req *http.Request) (*types.Session, error) {
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

	id := mux.Vars(req)["id"]
	if id != request.ID {
		return nil, fmt.Errorf("id mismatch")
	}
	sessionData, err := apiServer.Store.UpdateSession(reqContext.Ctx, request)

	// add the session to the controller queue
	err = apiServer.Controller.PushSessionQueue(reqContext.Ctx, sessionData)
	if err != nil {
		return nil, err
	}

	return sessionData, nil
}

func (apiServer *HelixAPIServer) deleteSession(res http.ResponseWriter, req *http.Request) (*types.Session, error) {
	reqContext := apiServer.getRequestContext(req)
	id := req.URL.Query().Get("id")
	session, err := apiServer.Store.GetSession(reqContext.Ctx, id)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("no session found with id %v", id)
	}
	log.Printf("session %+v %+v", session, reqContext)
	if session.OwnerType != reqContext.OwnerType || session.Owner != reqContext.Owner {
		return nil, fmt.Errorf("access denied")
	}
	return apiServer.Store.DeleteSession(reqContext.Ctx, id)
}

func (apiServer *HelixAPIServer) getWorkerSession(res http.ResponseWriter, req *http.Request) (*types.Session, error) {
	sessionMode, err := types.ValidateSessionMode(req.URL.Query().Get("mode"), true)
	if err != nil {
		return nil, err
	}

	sessionType, err := types.ValidateSessionType(req.URL.Query().Get("type"), true)
	if err != nil {
		return nil, err
	}

	modelName, err := types.ValidateModelName(req.URL.Query().Get("model_name"), true)
	if err != nil {
		return nil, err
	}

	memory := uint64(0)
	memoryString := req.URL.Query().Get("memory")
	if memoryString != "" {
		memory, err = strconv.ParseUint(memoryString, 10, 64)
		if err != nil {
			return nil, err
		}
	}

	// there are multiple entries for this param all of the format:
	// model_name:mode
	deprioritize := []types.SessionFilterDeprioritize{}
	deprioritizePairs, ok := req.URL.Query()["deprioritize"]

	if ok && len(deprioritizePairs) > 0 {
		for _, deprioritizePair := range deprioritizePairs {
			pair := strings.Split(deprioritizePair, ":")
			if len(pair) != 2 {
				return nil, fmt.Errorf("invalid deprioritize pair: %s", deprioritizePair)
			}
			deprioritizeModelName, err := types.ValidateModelName(pair[0], false)
			if err != nil {
				return nil, err
			}
			deprioritizeModelMode, err := types.ValidateSessionMode(pair[1], false)
			if err != nil {
				return nil, err
			}
			deprioritize = append(deprioritize, types.SessionFilterDeprioritize{
				ModelName: deprioritizeModelName,
				Mode:      deprioritizeModelMode,
			})
		}
	}

	// alow the worker to filter what tasks it wants
	// if any of these values are defined then we will only consider those in the response
	nextSession, err := apiServer.Controller.ShiftSessionQueue(req.Context(), types.SessionFilter{
		Mode:         sessionMode,
		Type:         sessionType,
		ModelName:    modelName,
		Memory:       memory,
		Deprioritize: deprioritize,
	})
	if err != nil {
		return nil, err
	}
	// IMPORTANT: we need to throw an error here (i.e. non 200 http code) because
	// that is how the workers will know to wait before asking again
	if nextSession == nil {
		return nil, fmt.Errorf("no task found")
	}

	return nextSession, nil
}

func (apiServer *HelixAPIServer) respondWorkerSession(res http.ResponseWriter, req *http.Request) (*types.WorkerTaskResponse, error) {
	taskResponse := &types.WorkerTaskResponse{}
	err := json.NewDecoder(req.Body).Decode(taskResponse)
	if err != nil {
		return nil, err
	}
	taskResponse, err = apiServer.Controller.HandleWorkerResponse(req.Context(), taskResponse)
	if err != nil {
		return nil, err
	}
	return taskResponse, nil
}
