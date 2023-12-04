package server

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"github.com/lukemarsden/helix/api/pkg/controller"
	"github.com/lukemarsden/helix/api/pkg/filestore"
	"github.com/lukemarsden/helix/api/pkg/model"
	"github.com/lukemarsden/helix/api/pkg/store"
	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

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

	modelName, err := model.GetModelNameForSession(sessionType)
	if err != nil {
		return nil, err
	}

	sessionID := system.GenerateUUID()

	// the user interaction is the request from the user
	userInteraction, err := apiServer.getUserInteractionFromForm(req, sessionID, sessionMode)
	if err != nil {
		return nil, err
	}
	if userInteraction == nil {
		return nil, fmt.Errorf("no interaction found")
	}

	sessionData, err := apiServer.Controller.CreateSession(req.Context(), types.CreateSessionRequest{
		SessionID:       sessionID,
		SessionMode:     sessionMode,
		SessionType:     sessionType,
		ModelName:       modelName,
		Owner:           reqContext.Owner,
		OwnerType:       reqContext.OwnerType,
		UserInteraction: *userInteraction,
	})

	return sessionData, nil
}

func (apiServer *HelixAPIServer) updateSession(res http.ResponseWriter, req *http.Request) (*types.Session, error) {
	reqContext := apiServer.getRequestContext(req)

	// now upload any files that were included
	err := req.ParseMultipartForm(10 << 20)
	if err != nil {
		return nil, err
	}

	vars := mux.Vars(req)
	sessionID := vars["id"]
	if sessionID == "" {
		return nil, fmt.Errorf("cannot update session without id")
	}

	session, err := apiServer.Store.GetSession(req.Context(), sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("no session found with id %v", sessionID)
	}

	canEdit := apiServer.canEditSession(reqContext, session)
	if !canEdit {
		return nil, fmt.Errorf("access dened for session id %s", session.ID)
	}

	userInteraction, err := apiServer.getUserInteractionFromForm(req, sessionID, session.Mode)
	if err != nil {
		return nil, err
	}
	if userInteraction == nil {
		return nil, fmt.Errorf("no interaction found")
	}

	sessionData, err := apiServer.Controller.UpdateSession(req.Context(), types.UpdateSessionRequest{
		SessionID:       sessionID,
		UserInteraction: *userInteraction,
	})

	return sessionData, nil
}

func (apiServer *HelixAPIServer) config(res http.ResponseWriter, req *http.Request) (types.ServerConfig, error) {
	if apiServer.Options.LocalFilestorePath != "" {
		return types.ServerConfig{
			FilestorePrefix: fmt.Sprintf("%s%s/filestore/viewer", apiServer.Options.URL, API_PREFIX),
		}, nil
	}

	// TODO: work out what to do for object storage here
	return types.ServerConfig{}, fmt.Errorf("we currently only support local filestore")
}

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

// in this case the path contains the full /dev/users/XXX/sessions/XXX path
// so we need to remove the /dev/users/XXX part and then we load the session based on it's ID
func (apiServer *HelixAPIServer) runnerSessionDownloadFile(res http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	sessionid := vars["sessionid"]
	filePath := req.URL.Query().Get("path")
	filename := filepath.Base(filePath)

	log.Debug().
		Msgf("🔵 download file: %s", filePath)

	err := func() error {
		filePath, requestContext, err := apiServer.convertFilestorePath(req.Context(), sessionid, filePath)
		if err != nil {
			return err
		}
		stream, err := apiServer.Controller.FilestoreDownload(requestContext, filePath)
		if err != nil {
			return err
		}

		// Set the appropriate mime-type headers
		res.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
		res.Header().Set("Content-Type", http.DetectContentType([]byte(filename)))

		// Write the file to the http.ResponseWriter
		_, err = io.Copy(res, stream)
		if err != nil {
			return err
		}

		return nil
	}()

	if err != nil {
		log.Error().Msgf("error for download file: %s", err.Error())
		http.Error(res, err.Error(), http.StatusInternalServerError)
	}
}

func (apiServer *HelixAPIServer) runnerSessionDownloadFolder(res http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	sessionid := vars["sessionid"]
	filePath := req.URL.Query().Get("path")
	filename := filepath.Base(filePath)

	log.Debug().
		Msgf("🔵 download folder: %s", filePath)

	err := func() error {
		filePath, requestContext, err := apiServer.convertFilestorePath(req.Context(), sessionid, filePath)
		if err != nil {
			return err
		}
		tarStream, err := apiServer.Controller.FilestoreDownloadFolder(requestContext, filePath)
		if err != nil {
			return err
		}

		// Set the appropriate mime-type headers
		res.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s,tar", filename))
		res.Header().Set("Content-Type", "application/x-tar")

		// Write the file to the http.ResponseWriter
		_, err = io.Copy(res, tarStream)
		if err != nil {
			return err
		}

		return nil
	}()

	if err != nil {
		log.Error().Msgf("error for download file: %s", err.Error())
		http.Error(res, err.Error(), http.StatusInternalServerError)
	}
}

// TODO: this need auth because right now it's an open filestore
func (apiServer *HelixAPIServer) runnerSessionUploadFiles(res http.ResponseWriter, req *http.Request) ([]filestore.FileStoreItem, error) {
	vars := mux.Vars(req)
	sessionid := vars["sessionid"]
	filePath := req.URL.Query().Get("path")

	err := req.ParseMultipartForm(10 << 20)
	if err != nil {
		return nil, err
	}

	session, err := apiServer.Store.GetSession(req.Context(), sessionid)
	if err != nil {
		return nil, err
	}

	uploadFolder := filepath.Join(controller.GetSessionFolder(session.ID), filePath)

	reqContext := types.RequestContext{
		Ctx:       req.Context(),
		Owner:     session.Owner,
		OwnerType: session.OwnerType,
	}

	result := []filestore.FileStoreItem{}
	files := req.MultipartForm.File["files"]

	for _, fileHeader := range files {
		// Handle non-tar files as before
		file, err := fileHeader.Open()
		if err != nil {
			return nil, fmt.Errorf("unable to open file")
		}
		defer file.Close()

		item, err := apiServer.Controller.FilestoreUpload(reqContext, filepath.Join(uploadFolder, fileHeader.Filename), file)
		if err != nil {
			return nil, fmt.Errorf("unable to upload file: %s", err.Error())
		}
		result = append(result, item)
	}

	return result, nil
}

func (apiServer *HelixAPIServer) runnerSessionUploadFolder(res http.ResponseWriter, req *http.Request) (*filestore.FileStoreItem, error) {
	vars := mux.Vars(req)
	sessionid := vars["sessionid"]
	filePath := req.URL.Query().Get("path")

	session, err := apiServer.Store.GetSession(req.Context(), sessionid)
	if err != nil {
		return nil, err
	}

	uploadFolder := filepath.Join(controller.GetSessionFolder(session.ID), filePath)

	reqContext := types.RequestContext{
		Ctx:       req.Context(),
		Owner:     session.Owner,
		OwnerType: session.OwnerType,
	}

	tarReader := tar.NewReader(req.Body)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading tar file: %s", err)
		}
		if header.Typeflag == tar.TypeReg {
			buffer := bytes.NewBuffer(nil)
			if _, err := io.Copy(buffer, tarReader); err != nil {
				return nil, fmt.Errorf("error reading file inside tar: %s", err)
			}

			// Create a virtual file from the buffer to upload
			vFile := bytes.NewReader(buffer.Bytes())
			_, err := apiServer.Controller.FilestoreUpload(reqContext, filepath.Join(uploadFolder, header.Name), vFile)
			if err != nil {
				return nil, fmt.Errorf("unable to upload file: %s", err.Error())
			}
		}
	}

	finalFolder, err := apiServer.Controller.FilestoreGet(reqContext, uploadFolder)
	if err != nil {
		return nil, err
	}

	return &finalFolder, nil
}

func (apiServer *HelixAPIServer) getSessionFromID(reqContext types.RequestContext, id string) (*types.Session, error) {
	session, err := apiServer.Store.GetSession(reqContext.Ctx, id)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("no session found with id %s", id)
	}
	canSee := apiServer.canSeeSession(reqContext, session)
	if !canSee {
		return nil, fmt.Errorf("access dened for session id %s", id)
	}
	return session, nil
}

func (apiServer *HelixAPIServer) getSession(res http.ResponseWriter, req *http.Request) (*types.Session, error) {
	vars := mux.Vars(req)
	id := vars["id"]
	reqContext := apiServer.getRequestContext(req)
	return apiServer.getSessionFromID(reqContext, id)
}

func (apiServer *HelixAPIServer) getSessions(res http.ResponseWriter, req *http.Request) ([]*types.SessionSummary, error) {
	reqContext := apiServer.getRequestContext(req)
	query := store.GetSessionsQuery{}
	query.Owner = reqContext.Owner
	query.OwnerType = reqContext.OwnerType
	sessions, err := apiServer.Store.GetSessions(reqContext.Ctx, query)
	if err != nil {
		return nil, err
	}
	ret := []*types.SessionSummary{}
	for _, session := range sessions {
		summary, err := model.GetSessionSummary(session)
		if err != nil {
			return nil, err
		}
		ret = append(ret, summary)
	}
	return ret, nil
}

func (apiServer *HelixAPIServer) retryTextFinetune(res http.ResponseWriter, req *http.Request) (*types.Session, error) {
	vars := mux.Vars(req)
	id := vars["id"]
	reqContext := apiServer.getRequestContext(req)
	session, err := apiServer.getSessionFromID(reqContext, id)
	if err != nil {
		return nil, err
	}
	go func() {
		apiServer.Controller.PrepareSession(session)
	}()
	return session, nil
}

func getSessionFinetuneFile(session *types.Session) (string, error) {
	userInteraction, err := model.GetUserInteraction(session)
	if err != nil {
		return "", err
	}
	if len(userInteraction.Files) == 0 {
		return "", fmt.Errorf("no files found")
	}
	foundFile := ""
	for _, filepath := range userInteraction.Files {
		if path.Base(filepath) == types.TEXT_DATA_PREP_QUESTIONS_FILE {
			foundFile = filepath
			break
		}
	}

	if foundFile == "" {
		return "", fmt.Errorf("file is not a jsonl file")
	}

	return foundFile, nil
}

func (apiServer *HelixAPIServer) getSessionFinetuneConversation(res http.ResponseWriter, req *http.Request) ([]types.DataPrepTextQuestion, error) {
	vars := mux.Vars(req)
	id := vars["id"]
	reqContext := apiServer.getRequestContext(req)

	session, err := apiServer.Store.GetSession(reqContext.Ctx, id)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("no session found with id %v", id)
	}
	if session.OwnerType != reqContext.OwnerType || session.Owner != reqContext.Owner {
		return nil, fmt.Errorf("access denied")
	}
	foundFile, err := getSessionFinetuneFile(session)
	if err != nil {
		return nil, err
	}
	return apiServer.Controller.ReadTextFineTuneQuestions(foundFile)
}

func (apiServer *HelixAPIServer) setSessionFinetuneConversation(res http.ResponseWriter, req *http.Request) ([]types.DataPrepTextQuestion, error) {
	vars := mux.Vars(req)
	id := vars["id"]
	reqContext := apiServer.getRequestContext(req)

	session, err := apiServer.Store.GetSession(reqContext.Ctx, id)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("no session found with id %v", id)
	}
	if session.OwnerType != reqContext.OwnerType || session.Owner != reqContext.Owner {
		return nil, fmt.Errorf("access denied")
	}

	foundFile, err := getSessionFinetuneFile(session)
	if err != nil {
		return nil, err
	}

	var data []types.DataPrepTextQuestion

	// Decode the JSON from the request body
	err = json.NewDecoder(req.Body).Decode(&data)
	if err != nil {
		return nil, err
	}

	err = apiServer.Controller.WriteTextFineTuneQuestions(foundFile, data)
	if err != nil {
		return nil, err
	}

	// now we switch the session into training mode
	err = apiServer.Controller.BeginTextFineTune(session)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (apiServer *HelixAPIServer) updateSessionMeta(res http.ResponseWriter, req *http.Request) (*types.Session, error) {
	vars := mux.Vars(req)
	sessionID := vars["id"]
	if sessionID == "" {
		return nil, fmt.Errorf("cannot update session without id")
	}

	reqContext := apiServer.getRequestContext(req)
	update := &types.SessionMetaUpdate{}
	err := json.NewDecoder(req.Body).Decode(update)
	if err != nil {
		return nil, err
	}

	session, err := apiServer.Store.GetSession(req.Context(), sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("no session found with id %v", sessionID)
	}

	canEdit := apiServer.canEditSession(reqContext, session)
	if !canEdit {
		return nil, fmt.Errorf("access dened for session id %s", session.ID)
	}

	return apiServer.Store.UpdateSessionMeta(reqContext.Ctx, *update)
}

func (apiServer *HelixAPIServer) isAdmin(req *http.Request) bool {
	user := getRequestUser(req)
	adminUserIDs := strings.Split(os.Getenv("ADMIN_USER_IDS"), ",")
	for _, a := range adminUserIDs {
		// development mode everyone is an admin
		if a == "*" {
			return true
		}
		if a == user {
			return true
		}
	}
	return false
}

// admin is required by the auth middleware
func (apiServer *HelixAPIServer) dashboard(res http.ResponseWriter, req *http.Request) (*types.DashboardData, error) {
	return apiServer.Controller.GetDashboardData(req.Context())
}

func (apiServer *HelixAPIServer) deleteSession(res http.ResponseWriter, req *http.Request) (*types.Session, error) {
	vars := mux.Vars(req)
	id := vars["id"]
	reqContext := apiServer.getRequestContext(req)

	session, err := apiServer.Store.GetSession(reqContext.Ctx, id)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("no session found with id %v", id)
	}
	canEdit := apiServer.canEditSession(reqContext, session)
	if !canEdit {
		return nil, fmt.Errorf("access dened for session id %s", session.ID)
	}
	return apiServer.Store.DeleteSession(reqContext.Ctx, id)
}

func (apiServer *HelixAPIServer) getNextRunnerSession(res http.ResponseWriter, req *http.Request) (*types.Session, error) {
	vars := mux.Vars(req)
	runnerID := vars["runnerid"]
	if runnerID == "" {
		return nil, fmt.Errorf("cannot get next session without runner id")
	}

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

	loraDir := req.URL.Query().Get("lora_dir")

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
	reject := []types.SessionFilterModel{}
	rejectPairs, ok := req.URL.Query()["reject"]

	if ok && len(rejectPairs) > 0 {
		for _, rejectPair := range rejectPairs {
			triple := strings.Split(rejectPair, ":")
			if len(triple) != 3 {
				return nil, fmt.Errorf("invalid reject pair: %s", rejectPair)
			}
			rejectModelName, err := types.ValidateModelName(triple[0], false)
			if err != nil {
				return nil, err
			}
			rejectModelMode, err := types.ValidateSessionMode(triple[1], false)
			if err != nil {
				return nil, err
			}
			rejectLoraDir := triple[2]
			reject = append(reject, types.SessionFilterModel{
				ModelName: rejectModelName,
				Mode:      rejectModelMode,
				LoraDir:   rejectLoraDir,
			})
		}
	}

	older := req.URL.Query().Get("older")

	var olderDuration time.Duration
	if older != "" {
		olderDuration, err = time.ParseDuration(older)
		if err != nil {
			return nil, err
		}
	}

	filter := types.SessionFilter{
		Mode:      sessionMode,
		Type:      sessionType,
		ModelName: modelName,
		Memory:    memory,
		Reject:    reject,
		LoraDir:   loraDir,
		Older:     types.Duration(olderDuration),
	}

	// alow the worker to filter what tasks it wants
	// if any of these values are defined then we will only consider those in the response
	nextSession, err := apiServer.Controller.ShiftSessionQueue(req.Context(), filter, runnerID)
	if err != nil {
		return nil, err
	}

	// if nextSession is nil - we will write null to the runner and it is setup
	// to regard that as an error (this means we don't need to write http error codes anymore)
	return nextSession, nil
}

func (apiServer *HelixAPIServer) handleRunnerResponse(res http.ResponseWriter, req *http.Request) (*types.RunnerTaskResponse, error) {
	taskResponse := &types.RunnerTaskResponse{}
	err := json.NewDecoder(req.Body).Decode(taskResponse)
	if err != nil {
		return nil, err
	}

	taskResponse, err = apiServer.Controller.HandleRunnerResponse(req.Context(), taskResponse)
	if err != nil {
		return nil, err
	}
	return taskResponse, nil
}

func (apiServer *HelixAPIServer) handleRunnerMetrics(res http.ResponseWriter, req *http.Request) (*types.RunnerState, error) {
	runnerState := &types.RunnerState{}
	err := json.NewDecoder(req.Body).Decode(runnerState)
	if err != nil {
		return nil, err
	}

	runnerState, err = apiServer.Controller.AddRunnerMetrics(req.Context(), runnerState)
	if err != nil {
		return nil, err
	}
	return runnerState, nil
}

func (apiServer *HelixAPIServer) createAPIKey(res http.ResponseWriter, req *http.Request) (string, error) {
	name := req.URL.Query().Get("name")
	apiKey, err := apiServer.Controller.CreateAPIKey(apiServer.getRequestContext(req), name)
	if err != nil {
		return "", err
	}
	return apiKey, nil
}

func (apiServer *HelixAPIServer) getAPIKeys(res http.ResponseWriter, req *http.Request) ([]*types.ApiKey, error) {
	apiKeys, err := apiServer.Controller.GetAPIKeys(apiServer.getRequestContext(req))
	if err != nil {
		return nil, err
	}
	return apiKeys, nil
}

func (apiServer *HelixAPIServer) deleteAPIKey(res http.ResponseWriter, req *http.Request) (string, error) {
	apiKey := req.URL.Query().Get("key")
	err := apiServer.Controller.DeleteAPIKey(apiServer.getRequestContext(req), apiKey)
	if err != nil {
		return "", err
	}
	return "", nil
}

func (apiServer *HelixAPIServer) checkAPIKey(res http.ResponseWriter, req *http.Request) (*types.ApiKey, error) {
	apiKey := req.URL.Query().Get("key")
	key, err := apiServer.Controller.CheckAPIKey(apiServer.getRequestContext(req).Ctx, apiKey)
	if err != nil {
		return nil, err
	}
	return key, nil
}
