package server

import (
	"archive/tar"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

func (apiServer *HelixAPIServer) sessionLoaderWithID(req *http.Request, id string, writeMode bool) (*types.Session, *system.HTTPError) {
	if id == "" {
		return nil, system.NewHTTPError400("cannot load session without id")
	}
	ctx := req.Context()
	user := getRequestUser(req)

	session, err := apiServer.Store.GetSession(ctx, id)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	if session == nil {
		return nil, system.NewHTTPError404(fmt.Sprintf("no session found with id %s", id))
	}

	canSee := false

	if writeMode {
		canSee = canEditSession(user, session)
	} else {
		canSee = canSeeSession(user, session)
	}

	if !canSee {
		return nil, system.NewHTTPError403(fmt.Sprintf("access denied for session id %s", id))
	}
	return session, nil
}

func (apiServer *HelixAPIServer) sessionLoader(req *http.Request, writeMode bool) (*types.Session, *system.HTTPError) {
	return apiServer.sessionLoaderWithID(req, mux.Vars(req)["id"], writeMode)
}

func (apiServer *HelixAPIServer) getSession(res http.ResponseWriter, req *http.Request) (*types.Session, *system.HTTPError) {
	return apiServer.sessionLoader(req, false)
}

func (apiServer *HelixAPIServer) getSessionSummary(res http.ResponseWriter, req *http.Request) (*types.SessionSummary, *system.HTTPError) {
	session, err := apiServer.sessionLoader(req, false)
	if err != nil {
		return nil, err
	}
	return system.DefaultController(data.GetSessionSummary(session))
}

func (apiServer *HelixAPIServer) getSessions(res http.ResponseWriter, req *http.Request) (*types.SessionsList, error) {
	ctx := req.Context()
	user := getRequestUser(req)

	query := store.GetSessionsQuery{}
	query.Owner = user.ID
	query.OwnerType = user.Type

	// Extract offset and limit values from query parameters
	offsetStr := req.URL.Query().Get("offset")
	limitStr := req.URL.Query().Get("limit")

	// Convert offset and limit values to integers
	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		offset = 0 // Default value if offset is not provided or conversion fails
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		limit = 0 // Default value if limit is not provided or conversion fails
	}

	query.Offset = offset
	query.Limit = limit

	sessions, err := apiServer.Store.GetSessions(ctx, query)
	if err != nil {
		return nil, err
	}

	counter, err := apiServer.Store.GetSessionsCounter(ctx, query)
	if err != nil {
		return nil, err
	}

	sessionSummaries := []*types.SessionSummary{}
	for _, session := range sessions {
		summary, err := data.GetSessionSummary(session)
		if err != nil {
			return nil, err
		}
		sessionSummaries = append(sessionSummaries, summary)
	}

	return &types.SessionsList{
		Sessions: sessionSummaries,
		Counter:  counter,
	}, nil
}

func (apiServer *HelixAPIServer) createDataEntity(_ http.ResponseWriter, req *http.Request) (*types.DataEntity, error) {
	entity, err := apiServer.getDataEntityFromForm(req)
	if err != nil {
		return nil, err
	}

	return apiServer.Store.CreateDataEntity(req.Context(), entity)
}

func (apiServer *HelixAPIServer) updateSession(res http.ResponseWriter, req *http.Request) (*types.Session, *system.HTTPError) {
	session, httpError := apiServer.sessionLoader(req, true)
	if httpError != nil {
		return nil, httpError
	}

	user := getRequestUser(req)
	ctx := req.Context()

	// now upload any files that were included
	err := req.ParseMultipartForm(10 << 20)
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	userInteraction, err := apiServer.getUserInteractionFromForm(req, session.ID, session.Mode, "")
	if err != nil {
		return nil, system.NewHTTPError(err)
	}
	if userInteraction == nil {
		return nil, system.NewHTTPError404("no interaction found")
	}

	sessionData, err := apiServer.Controller.UpdateSession(ctx, user, types.UpdateSessionRequest{
		SessionID:       session.ID,
		UserInteraction: userInteraction,
		SessionMode:     session.Mode,
	})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to update session: %s", err))
	}

	return sessionData, nil
}

func (apiServer *HelixAPIServer) updateSessionConfig(res http.ResponseWriter, req *http.Request) (*types.SessionMetadata, *system.HTTPError) {
	session, httpError := apiServer.sessionLoader(req, true)
	if httpError != nil {
		return nil, httpError
	}

	var data *types.SessionMetadata

	// Decode the JSON from the request body
	err := json.NewDecoder(req.Body).Decode(&data)
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	result, err := apiServer.Controller.UpdateSessionMetadata(req.Context(), session, data)
	if err != nil {
		return nil, system.NewHTTPError(err)
	}

	return result, nil
}

func (apiServer *HelixAPIServer) getConfig() (types.ServerConfigForFrontend, error) {
	filestorePrefix := ""
	if apiServer.Cfg.WebServer.LocalFilestorePath != "" {
		filestorePrefix = fmt.Sprintf("%s%s/filestore/viewer", apiServer.Cfg.WebServer.URL, API_PREFIX)
	} else {
		return types.ServerConfigForFrontend{}, system.NewHTTPError500("we currently only support local filestore")
	}

	return types.ServerConfigForFrontend{
		FilestorePrefix:         filestorePrefix,
		StripeEnabled:           apiServer.Stripe.Enabled(),
		SentryDSNFrontend:       apiServer.Cfg.Janitor.SentryDsnFrontend,
		GoogleAnalyticsFrontend: apiServer.Cfg.Janitor.GoogleAnalyticsFrontend,
		EvalUserID:              apiServer.Cfg.WebServer.EvalUserID,
		RudderStackWriteKey:     apiServer.Cfg.Janitor.RudderStackWriteKey,
		RudderStackDataPlaneURL: apiServer.Cfg.Janitor.RudderStackDataPlaneURL,
		ToolsEnabled:            apiServer.Cfg.Tools.Enabled,
		AppsEnabled:             apiServer.Cfg.Apps.Enabled,
		Version:                 data.GetHelixVersion(),
	}, nil
}

func (apiServer *HelixAPIServer) config(res http.ResponseWriter, req *http.Request) (types.ServerConfigForFrontend, error) {
	return apiServer.getConfig()
}

// prints the config values as JavaScript values so we can block the rest of the frontend on
// initializing until we have these values (useful for things like Sentry without having to burn keys into frontend code)
func (apiServer *HelixAPIServer) configJS(res http.ResponseWriter, req *http.Request) {
	config, err := apiServer.getConfig()
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	res.Header().Set("Content-Type", "application/javascript")
	content := fmt.Sprintf(`
window.HELIX_SENTRY_DSN = "%s"
window.HELIX_GOOGLE_ANALYTICS = "%s"
window.RUDDERSTACK_WRITE_KEY = "%s"
window.RUDDERSTACK_DATA_PLANE_URL = "%s"
`,
		config.SentryDSNFrontend,
		config.GoogleAnalyticsFrontend,
		config.RudderStackWriteKey,
		config.RudderStackDataPlaneURL,
	)
	res.Write([]byte(content))
}

func (apiServer *HelixAPIServer) status(res http.ResponseWriter, req *http.Request) (types.UserStatus, error) {
	user := getRequestUser(req)
	ctx := req.Context()

	return apiServer.Controller.GetStatus(ctx, user)
}

func (apiServer *HelixAPIServer) filestoreConfig(res http.ResponseWriter, req *http.Request) (filestore.FilestoreConfig, error) {
	return apiServer.Controller.FilestoreConfig(getOwnerContext(req))
}

func (apiServer *HelixAPIServer) filestoreList(res http.ResponseWriter, req *http.Request) ([]filestore.FileStoreItem, error) {
	return apiServer.Controller.FilestoreList(getOwnerContext(req), req.URL.Query().Get("path"))
}

func (apiServer *HelixAPIServer) filestoreGet(res http.ResponseWriter, req *http.Request) (filestore.FileStoreItem, error) {
	return apiServer.Controller.FilestoreGet(getOwnerContext(req), req.URL.Query().Get("path"))
}

func (apiServer *HelixAPIServer) filestoreCreateFolder(res http.ResponseWriter, req *http.Request) (filestore.FileStoreItem, error) {
	return apiServer.Controller.FilestoreCreateFolder(getOwnerContext(req), req.URL.Query().Get("path"))
}

func (apiServer *HelixAPIServer) filestoreRename(res http.ResponseWriter, req *http.Request) (filestore.FileStoreItem, error) {
	return apiServer.Controller.FilestoreRename(getOwnerContext(req), req.URL.Query().Get("path"), req.URL.Query().Get("new_path"))
}

func (apiServer *HelixAPIServer) filestoreDelete(res http.ResponseWriter, req *http.Request) (string, error) {
	path := req.URL.Query().Get("path")
	err := apiServer.Controller.FilestoreDelete(getOwnerContext(req), path)
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
		_, err = apiServer.Controller.FilestoreUploadFile(getOwnerContext(req), filepath.Join(path, fileHeader.Filename), file)
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
		filePath, ownerContext, err := apiServer.convertFilestorePath(req.Context(), sessionid, filePath)
		if err != nil {
			return err
		}
		reader, err := apiServer.Controller.FilestoreDownloadFile(ownerContext, filePath)
		if err != nil {
			return err
		}
		defer reader.Close()

		// Set the appropriate mime-type headers
		res.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
		res.Header().Set("Content-Type", http.DetectContentType([]byte(filename)))

		// Write the file to the http.ResponseWriter
		_, err = io.Copy(res, reader)
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

	ownerContext := types.OwnerContext{
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

		item, err := apiServer.Controller.FilestoreUploadFile(ownerContext, filepath.Join(uploadFolder, fileHeader.Filename), file)
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

	ownerContext := types.OwnerContext{
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
			_, err := apiServer.Controller.FilestoreUploadFile(ownerContext, filepath.Join(uploadFolder, header.Name), vFile)
			if err != nil {
				return nil, fmt.Errorf("unable to upload file: %s", err.Error())
			}
		}
	}

	finalFolder, err := apiServer.Controller.FilestoreGet(ownerContext, uploadFolder)
	if err != nil {
		return nil, err
	}

	return &finalFolder, nil
}

func (apiServer *HelixAPIServer) restartSession(res http.ResponseWriter, req *http.Request) (*types.Session, *system.HTTPError) {
	session, err := apiServer.sessionLoader(req, true)
	if err != nil {
		return nil, err
	}
	// If it is a text inference session, then restart using the "new" openai controllers
	if session.Metadata.OriginalMode != types.SessionModeFinetune && session.Type == types.SessionTypeText && session.Mode == types.SessionModeInference {
		apiServer.restartChatSessionHandler(res, req)
		return session, nil
	}
	return system.DefaultController(apiServer.Controller.RestartSession(session))
}

func (apiServer *HelixAPIServer) retryTextFinetune(res http.ResponseWriter, req *http.Request) (*types.Session, *system.HTTPError) {
	session, err := apiServer.sessionLoader(req, true)
	if err != nil {
		return nil, err
	}
	go func() {
		apiServer.Controller.PrepareSession(session)
	}()
	return session, nil
}

func (apiServer *HelixAPIServer) cloneFinetuneInteraction(res http.ResponseWriter, req *http.Request) (*types.Session, *system.HTTPError) {
	vars := mux.Vars(req)

	user := getRequestUser(req)
	ctx := req.Context()

	// clone the session into the eval user account
	// only admins can do this
	cloneIntoEvalUser := req.URL.Query().Get("clone_into_eval_user")
	if cloneIntoEvalUser != "" && !apiServer.isAdmin(req) {
		return nil, system.NewHTTPError403("access denied")
	}

	// we only need to check for read only access here because
	// we are only reading the original session and then writing a new one
	// into our account
	session, httpError := apiServer.sessionLoader(req, false)
	if httpError != nil {
		return nil, httpError
	}

	// if we own the session then we don't need to copy all files
	copyAllFiles := true
	if doesOwnSession(user, session) {
		copyAllFiles = false
	}

	interaction, err := data.GetInteraction(session, vars["interaction"])
	if err != nil {
		return nil, system.NewHTTPError404(err.Error())
	}
	mode, err := types.ValidateCloneTextType(vars["mode"], false)
	if err != nil {
		return nil, system.NewHTTPError404(err.Error())
	}
	// switch the target user to be the eval user
	if cloneIntoEvalUser != "" {
		user.ID = apiServer.Cfg.WebServer.EvalUserID
	}
	return system.DefaultController(apiServer.Controller.CloneUntilInteraction(ctx, user, session, controller.CloneUntilInteractionRequest{
		InteractionID: interaction.ID,
		Mode:          mode,
		CopyAllFiles:  copyAllFiles,
	}))
}

func (apiServer *HelixAPIServer) finetuneAddDocuments(res http.ResponseWriter, req *http.Request) (*types.Session, *system.HTTPError) {
	session, httpError := apiServer.sessionLoader(req, true)
	if httpError != nil {
		return nil, httpError
	}

	// if this is set then it means we are adding files to the existing interaction
	interactionID := req.URL.Query().Get("interactionID")

	err := req.ParseMultipartForm(10 << 20)
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	// the user interaction is the request from the user
	newUserInteraction, err := apiServer.getUserInteractionFromForm(req, session.ID, types.SessionModeFinetune, interactionID)
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}
	if newUserInteraction == nil {
		return nil, system.NewHTTPError404("no user interaction found")
	}

	// this means we are adding the files to an existing interaction
	// rather than appending new interactions
	if interactionID != "" {
		return system.DefaultController(apiServer.Controller.AddDocumentsToInteraction(req.Context(), session, newUserInteraction.Files))
	} else {
		return system.DefaultController(apiServer.Controller.AddDocumentsToSession(req.Context(), session, newUserInteraction))
	}
}

func (apiServer *HelixAPIServer) getSessionFinetuneConversation(res http.ResponseWriter, req *http.Request) ([]types.DataPrepTextQuestion, *system.HTTPError) {
	vars := mux.Vars(req)
	session, httpError := apiServer.sessionLoader(req, false)
	if httpError != nil {
		return nil, httpError
	}

	interactionID := vars["interaction"]

	foundFile, err := data.GetInteractionFinetuneFile(session, interactionID)
	if err != nil {
		return nil, system.NewHTTPError(err)
	}
	return system.DefaultController(apiServer.Controller.ReadTextFineTuneQuestions(foundFile))
}

func (apiServer *HelixAPIServer) setSessionFinetuneConversation(res http.ResponseWriter, req *http.Request) ([]types.DataPrepTextQuestion, *system.HTTPError) {
	vars := mux.Vars(req)
	session, httpError := apiServer.sessionLoader(req, true)
	if httpError != nil {
		return nil, httpError
	}
	interactionID := vars["interaction"]
	foundFile, err := data.GetInteractionFinetuneFile(session, interactionID)
	if err != nil {
		return nil, system.NewHTTPError(err)
	}

	var data []types.DataPrepTextQuestion

	// Decode the JSON from the request body
	err = json.NewDecoder(req.Body).Decode(&data)
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	err = apiServer.Controller.WriteTextFineTuneQuestions(foundFile, data)
	if err != nil {
		return nil, system.NewHTTPError(err)
	}

	return data, nil
}

func (apiServer *HelixAPIServer) startSessionFinetune(res http.ResponseWriter, req *http.Request) (*types.Session, *system.HTTPError) {
	session, httpError := apiServer.sessionLoader(req, true)
	if httpError != nil {
		return nil, httpError
	}

	err := apiServer.Controller.BeginFineTune(session)

	if err != nil {
		return nil, system.NewHTTPError(err)
	}

	return session, nil
}

func (apiServer *HelixAPIServer) updateSessionMeta(res http.ResponseWriter, req *http.Request) (*types.Session, *system.HTTPError) {
	_, httpError := apiServer.sessionLoader(req, true)
	if httpError != nil {
		return nil, httpError
	}

	ctx := req.Context()

	update := &types.SessionMetaUpdate{}
	err := json.NewDecoder(req.Body).Decode(update)
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	return system.DefaultController(apiServer.Store.UpdateSessionMeta(ctx, *update))
}

func (apiServer *HelixAPIServer) isAdmin(req *http.Request) bool {
	user := getRequestUser(req)
	adminUserIDs := strings.Split(os.Getenv("ADMIN_USER_IDS"), ",")
	for _, a := range adminUserIDs {
		// development mode everyone is an admin
		if a == "all" {
			return true
		}
		if a == user.ID {
			return true
		}
	}
	return false
}

// admin is required by the auth middleware
func (apiServer *HelixAPIServer) dashboard(res http.ResponseWriter, req *http.Request) (*types.DashboardData, error) {
	return apiServer.Controller.GetDashboardData(req.Context())
}

func (apiServer *HelixAPIServer) deleteSession(res http.ResponseWriter, req *http.Request) (*types.Session, *system.HTTPError) {
	session, httpError := apiServer.sessionLoader(req, true)
	if httpError != nil {
		return nil, httpError
	}

	return system.DefaultController(apiServer.Store.DeleteSession(req.Context(), session.ID))
}

func (apiServer *HelixAPIServer) createAPIKey(res http.ResponseWriter, req *http.Request) (string, error) {
	newAPIKey := &types.APIKey{}
	name := req.URL.Query().Get("name")

	user := getRequestUser(req)
	ctx := req.Context()

	if name != "" {
		// if we are using the query string route then don't try to deode the body
		newAPIKey.Name = name
		newAPIKey.Type = types.APIKeyType_API
	} else {
		// For now we need to manually unmarshal the body because of the sql.NullString
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return "", err
		}
		var objmap map[string]json.RawMessage
		err = json.Unmarshal(body, &objmap)
		if err != nil {
			return "", err
		}
		var nameStr string
		err = json.Unmarshal(objmap["name"], &nameStr)
		if err != nil {
			return "", err
		}
		var typeStr string
		err = json.Unmarshal(objmap["type"], &typeStr)
		if err != nil {
			return "", err
		}
		var apiKeyStr string
		err = json.Unmarshal(objmap["app_id"], &apiKeyStr)
		if err != nil {
			return "", err
		}
		newAPIKey.Name = nameStr
		newAPIKey.Type = types.APIKeyType(typeStr)
		newAPIKey.AppID = &sql.NullString{String: apiKeyStr, Valid: true}
	}

	createdKey, err := apiServer.Controller.CreateAPIKey(ctx, user, newAPIKey)
	if err != nil {
		return "", err
	}
	return createdKey.Key, nil
}

func containsType(keyType string, typesParam string) bool {
	if typesParam == "" {
		return false
	}

	typesList := strings.Split(typesParam, ",")
	for _, t := range typesList {
		if t == keyType {
			return true
		}
	}
	return false
}

func (apiServer *HelixAPIServer) getAPIKeys(res http.ResponseWriter, req *http.Request) ([]*types.APIKey, error) {
	user := getRequestUser(req)
	ctx := req.Context()

	apiKeys, err := apiServer.Controller.GetAPIKeys(ctx, user)
	if err != nil {
		return nil, err
	}

	typesParam := req.URL.Query().Get("types")
	appIDParam := req.URL.Query().Get("app_id")

	includeAllTypes := false
	if typesParam == "all" {
		includeAllTypes = true
	}

	filteredAPIKeys := []*types.APIKey{}
	for _, key := range apiKeys {
		if !includeAllTypes && !containsType(string(key.Type), typesParam) {
			continue
		}
		if appIDParam != "" && (!key.AppID.Valid || key.AppID.String != appIDParam) {
			continue
		}
		filteredAPIKeys = append(filteredAPIKeys, key)
	}
	apiKeys = filteredAPIKeys
	return apiKeys, nil
}

func (apiServer *HelixAPIServer) deleteAPIKey(res http.ResponseWriter, req *http.Request) (string, error) {
	user := getRequestUser(req)
	ctx := req.Context()

	apiKey := req.URL.Query().Get("key")
	err := apiServer.Controller.DeleteAPIKey(ctx, user, apiKey)
	if err != nil {
		return "", err
	}
	return apiKey, nil
}

// TODO: verify if this is actually used
func (apiServer *HelixAPIServer) checkAPIKey(res http.ResponseWriter, req *http.Request) (*types.APIKey, error) {
	ctx := req.Context()

	apiKey := req.URL.Query().Get("key")
	key, err := apiServer.Controller.CheckAPIKey(ctx, apiKey)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (apiServer *HelixAPIServer) subscriptionCreate(res http.ResponseWriter, req *http.Request) (string, error) {
	user := getRequestUser(req)

	return apiServer.Stripe.GetCheckoutSessionURL(user.ID, user.Email)
}

func (apiServer *HelixAPIServer) subscriptionManage(res http.ResponseWriter, req *http.Request) (string, error) {
	user := getRequestUser(req)
	ctx := req.Context()

	userMeta, err := apiServer.Store.GetUserMeta(ctx, user.ID)
	if err != nil {
		return "", err
	}
	if userMeta == nil {
		return "", fmt.Errorf("no such user")
	}
	if userMeta.Config.StripeCustomerID == "" {
		return "", fmt.Errorf("no stripe customer id found")
	}
	return apiServer.Stripe.GetPortalSessionURL(userMeta.Config.StripeCustomerID)
}

func (apiServer *HelixAPIServer) subscriptionWebhook(res http.ResponseWriter, req *http.Request) {
	apiServer.Stripe.ProcessWebhook(res, req)
}
