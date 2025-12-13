package server

import (
	"archive/tar"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	jsoniter "github.com/json-iterator/go"
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

// getSession godoc
// @Summary Get a session by ID
// @Description Get a session by ID
// @Tags    sessions
// @Success 200 {object} types.Session
// @Param id path string true "Session ID"
// @Router /api/v1/sessions/{id} [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getSession(_ http.ResponseWriter, req *http.Request) (*types.Session, *system.HTTPError) {
	id := mux.Vars(req)["id"]
	if id == "" {
		return nil, system.NewHTTPError400("cannot load session without id")
	}
	ctx := req.Context()
	user := getRequestUser(req)

	session, err := apiServer.Store.GetSession(ctx, id)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	canSee := canSeeSession(user, session)
	if !canSee {
		return nil, system.NewHTTPError403(fmt.Sprintf("access denied for session id %s", id))
	}

	// Load interactions
	interactions, _, err := apiServer.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    id,
		GenerationID: session.GenerationID,
		PerPage:      1000,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	session.Interactions = interactions

	return session, nil
}

// listSessions godoc
// @Summary List sessions
// @Description List sessions
// @Tags    sessions
// @Param   page            query    int     false  "Page number"
// @Param   page_size       query    int     false  "Page size"
// @Param   org_id				  query    string  false  "Organization slug or ID"
// @Param   question_set_id query    string  false  "Question set ID"
// @Param   question_set_execution_id query    string  false  "Question set execution ID"
// @Param   search          query    string  false  "Search sessions by name"
// @Success 200 {object} types.PaginatedSessionsList
// @Router /api/v1/sessions [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listSessions(_ http.ResponseWriter, req *http.Request) (*types.PaginatedSessionsList, error) {
	ctx := req.Context()
	user := getRequestUser(req)

	query := store.ListSessionsQuery{
		Search:                 req.URL.Query().Get("search"),
		QuestionSetID:          req.URL.Query().Get("question_set_id"),
		QuestionSetExecutionID: req.URL.Query().Get("question_set_execution_id"),
	}
	query.Owner = user.ID
	query.OwnerType = user.Type

	// Extract organization_id query parameter if present
	orgID := req.URL.Query().Get("org_id")
	if orgID != "" {
		// Lookup org
		org, err := apiServer.lookupOrg(ctx, orgID)
		if err != nil {
			return nil, system.NewHTTPError404(err.Error())
		}

		orgID = org.ID

		_, err = apiServer.authorizeOrgMember(ctx, user, orgID)
		if err != nil {
			return nil, system.NewHTTPError403(err.Error())
		}

		query.OrganizationID = orgID
	} else {
		// When no organization is specified, we only want personal sessions
		// Setting empty string explicitly ensures we only get sessions with no organization
		query.OrganizationID = ""
	}

	// Parse query parameters
	page, err := strconv.Atoi(req.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 0
	}

	pageSize, err := strconv.Atoi(req.URL.Query().Get("page_size"))
	if err != nil || pageSize < 1 {
		pageSize = 50 // Default page size
	}

	query.Page = page
	query.PerPage = pageSize

	sessions, totalCount, err := apiServer.Store.ListSessions(ctx, query)
	if err != nil {
		return nil, err
	}

	sessionSummaries := []*types.SessionSummary{}
	for _, session := range sessions {
		summary, err := data.GetSessionSummary(session)
		if err != nil {
			log.Error().Err(err).Str("session_id", session.ID).Msg("failed to get session summary")
			continue
		}
		sessionSummaries = append(sessionSummaries, summary)
	}

	return &types.PaginatedSessionsList{
		Sessions:   sessionSummaries,
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
		TotalPages: int(math.Ceil(float64(totalCount) / float64(pageSize))),
	}, nil
}

// getConfig godoc
// @Summary Get config
// @Description Get config
// @Tags    config
// @Success 200 {object} types.ServerConfigForFrontend
// @Router /api/v1/config [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getConfig(ctx context.Context) (types.ServerConfigForFrontend, error) {
	filestorePrefix := ""

	if apiServer.Cfg.FileStore.LocalFSPath != "" {
		filestorePrefix = fmt.Sprintf("%s%s/filestore/viewer", apiServer.Cfg.WebServer.URL, APIPrefix)
	} else {
		return types.ServerConfigForFrontend{}, system.NewHTTPError500("we currently only support local filestore")
	}

	currentVersion := data.GetHelixVersion()
	latestVersion := ""
	deploymentID := "unknown"
	if apiServer.pingService != nil {
		latestVersion = apiServer.pingService.GetLatestVersion()
		deploymentID = apiServer.pingService.GetDeploymentID()
	}

	// Add license information
	var licenseInfo *types.FrontendLicenseInfo
	if apiServer.pingService != nil {
		decodedLicense, err := apiServer.pingService.GetLicenseInfo(ctx)
		if err == nil && decodedLicense != nil {
			licenseInfo = &types.FrontendLicenseInfo{
				Valid:        decodedLicense.Valid && !decodedLicense.Expired(),
				Organization: decodedLicense.Organization,
				ValidUntil:   decodedLicense.ValidUntil,
				Features: struct {
					Users bool `json:"users"`
				}{
					Users: decodedLicense.Features.Users,
				},
				Limits: struct {
					Users    int64 `json:"users"`
					Machines int64 `json:"machines"`
				}{
					Users:    decodedLicense.Limits.Users,
					Machines: decodedLicense.Limits.Machines,
				},
			}
		} else {
			// if license is not valid, allow user to upload a new one
			deploymentID = "unknown"
		}
	}

	// Get Moonlight Web mode from environment
	moonlightWebMode := os.Getenv("MOONLIGHT_WEB_MODE")
	if moonlightWebMode == "" {
		moonlightWebMode = "single" // Default to single mode (session-persistence)
	}

	// Get streaming bitrate from environment (in Mbps)
	// Default: 10 Mbps - conservative for bandwidth-constrained networks (4G, etc.)
	// Can be overridden with STREAMING_BITRATE_MBPS env var
	// Frontend also allows user to adjust bitrate dynamically
	streamingBitrateMbps := 10 // Default: 10 Mbps (was 40, too high for low-bandwidth)
	if bitrateMbpsStr := os.Getenv("STREAMING_BITRATE_MBPS"); bitrateMbpsStr != "" {
		if bitrate, err := strconv.Atoi(bitrateMbpsStr); err == nil && bitrate > 0 {
			streamingBitrateMbps = bitrate
		}
	}

	config := types.ServerConfigForFrontend{
		RegistrationEnabled:                    apiServer.Cfg.Auth.RegistrationEnabled,
		AuthProvider:                           apiServer.Cfg.Auth.Provider,
		FilestorePrefix:                        filestorePrefix,
		StripeEnabled:                          apiServer.Stripe.Enabled(),
		BillingEnabled:                         apiServer.Cfg.Stripe.BillingEnabled,
		SentryDSNFrontend:                      apiServer.Cfg.Janitor.SentryDsnFrontend,
		GoogleAnalyticsFrontend:                apiServer.Cfg.Janitor.GoogleAnalyticsFrontend,
		EvalUserID:                             apiServer.Cfg.WebServer.EvalUserID,
		RudderStackWriteKey:                    apiServer.Cfg.Janitor.RudderStackWriteKey,
		RudderStackDataPlaneURL:                apiServer.Cfg.Janitor.RudderStackDataPlaneURL,
		ToolsEnabled:                           apiServer.Cfg.Tools.Enabled,
		AppsEnabled:                            apiServer.Cfg.Apps.Enabled,
		DisableLLMCallLogging:                  apiServer.Cfg.DisableLLMCallLogging,
		Version:                                currentVersion,
		LatestVersion:                          latestVersion,
		DeploymentID:                           deploymentID,
		License:                                licenseInfo,
		OrganizationsCreateEnabledForNonAdmins: apiServer.Cfg.Organizations.CreateEnabledForNonAdmins,
		ProvidersManagementEnabled:             apiServer.Cfg.ProvidersManagementEnabled,
		MoonlightWebMode:                       moonlightWebMode,
		StreamingBitrateMbps:                   streamingBitrateMbps,
	}

	return config, nil
}

func (apiServer *HelixAPIServer) config(_ http.ResponseWriter, req *http.Request) (types.ServerConfigForFrontend, error) {
	return apiServer.getConfig(req.Context())
}

// prints the config values as JavaScript values so we can block the rest of the frontend on
// initializing until we have these values (useful for things like Sentry without having to burn keys into frontend code)
func (apiServer *HelixAPIServer) configJS(res http.ResponseWriter, req *http.Request) {
	config, err := apiServer.getConfig(req.Context())
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	res.Header().Set("Content-Type", "application/javascript")
	content := fmt.Sprintf(`
window.DISABLE_LLM_CALL_LOGGING = %t
window.HELIX_SENTRY_DSN = "%s"
window.HELIX_GOOGLE_ANALYTICS = "%s"
window.RUDDERSTACK_WRITE_KEY = "%s"
window.RUDDERSTACK_DATA_PLANE_URL = "%s"
window.HELIX_VERSION = "%s"
window.HELIX_LATEST_VERSION = "%s"
window.ORGANIZATIONS_CREATE_ENABLED_FOR_NON_ADMINS = %t
`,
		config.DisableLLMCallLogging,
		config.SentryDSNFrontend,
		config.GoogleAnalyticsFrontend,
		config.RudderStackWriteKey,
		config.RudderStackDataPlaneURL,
		config.Version,
		config.LatestVersion,
		config.OrganizationsCreateEnabledForNonAdmins,
	)
	if _, err := res.Write([]byte(content)); err != nil {
		log.Error().Msgf("Failed to write response: %v", err)
	}
}

func (apiServer *HelixAPIServer) status(_ http.ResponseWriter, req *http.Request) (types.UserStatus, error) {
	user := getRequestUser(req)
	ctx := req.Context()

	return apiServer.Controller.GetStatus(ctx, user)
}

// filestoreConfig godoc
// @Summary Get filestore configuration
// @Description Get the filestore configuration including user prefix and available folders
// @Tags    filestore
// @Accept  json
// @Produce json
// @Success 200 {object} filestore.Config
// @Router /api/v1/filestore/config [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) filestoreConfig(_ http.ResponseWriter, req *http.Request) (filestore.Config, error) {
	return apiServer.Controller.FilestoreConfig(getOwnerContext(req))
}

// filestoreList godoc
// @Summary List filestore items
// @Description List files and folders in the specified path. Supports both user and app-scoped paths
// @Tags    filestore
// @Accept  json
// @Produce json
// @Param   path query string false "Path to list (e.g., 'documents', 'apps/app_id/folder')"
// @Success 200 {array} filestore.Item
// @Router /api/v1/filestore/list [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) filestoreList(_ http.ResponseWriter, req *http.Request) ([]filestore.Item, error) {
	path := req.URL.Query().Get("path")

	// Only admins can list the root apps directory to prevent app enumeration
	if path == "apps" {
		isAdmin := apiServer.isAdmin(req)
		if !isAdmin {
			return nil, fmt.Errorf("access denied: only administrators can list all apps")
		}
	}

	// Check if this is an app-scoped path
	if controller.IsAppPath(path) {
		appID, err := controller.ExtractAppID(path)
		if err != nil {
			return nil, fmt.Errorf("invalid app path format: %s", err)
		}

		// Check app filestore access for app-scoped paths
		hasAccess, _, err := apiServer.checkAppFilestoreAccess(req.Context(), path, req, types.ActionList)
		if err != nil {
			return nil, err
		}
		if !hasAccess {
			return nil, fmt.Errorf("access denied to app filestore path: %s", path)
		}

		// Extract the relative path within the app
		relativePath := path[len("apps/")+len(appID):]
		relativePath = strings.TrimPrefix(relativePath, "/")

		// Use the app-specific list method
		return apiServer.Controller.FilestoreAppList(appID, relativePath)
	}

	// Regular user path handling
	return apiServer.Controller.FilestoreList(getOwnerContext(req), path)
}

// filestoreGet godoc
// @Summary Get filestore item
// @Description Get information about a specific file or folder in the filestore
// @Tags    filestore
// @Accept  json
// @Produce json
// @Param   path query string true "Path to the file or folder (e.g., 'documents/file.pdf', 'apps/app_id/folder')"
// @Success 200 {object} filestore.Item
// @Router /api/v1/filestore/get [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) filestoreGet(_ http.ResponseWriter, req *http.Request) (filestore.Item, error) {
	path := req.URL.Query().Get("path")

	// Check if this is an app-scoped path
	if controller.IsAppPath(path) {
		appID, err := controller.ExtractAppID(path)
		if err != nil {
			return filestore.Item{}, fmt.Errorf("invalid app path format: %s", err)
		}

		// Check app filestore access for app-scoped paths
		hasAccess, _, err := apiServer.checkAppFilestoreAccess(req.Context(), path, req, types.ActionGet)
		if err != nil {
			return filestore.Item{}, err
		}
		if !hasAccess {
			return filestore.Item{}, fmt.Errorf("access denied to app filestore path: %s", path)
		}

		// Extract the relative path within the app
		relativePath := path[len("apps/")+len(appID):]
		relativePath = strings.TrimPrefix(relativePath, "/")

		// Use the app-specific get method
		return apiServer.Controller.FilestoreAppGet(appID, relativePath)
	}

	// Regular user path handling
	return apiServer.Controller.FilestoreGet(getOwnerContext(req), path)
}

// filestoreCreateFolder godoc
// @Summary Create filestore folder
// @Description Create a new folder in the filestore at the specified path
// @Tags    filestore
// @Accept  json
// @Produce json
// @Param   request body object{path=string} true "Request body with folder path"
// @Success 200 {object} filestore.Item
// @Router /api/v1/filestore/folder [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) filestoreCreateFolder(_ http.ResponseWriter, req *http.Request) (filestore.Item, error) {
	var request struct {
		Path string `json:"path"`
	}
	if err := jsoniter.NewDecoder(req.Body).Decode(&request); err != nil {
		return filestore.Item{}, err
	}

	// Check if this is an app-scoped path
	if controller.IsAppPath(request.Path) {
		appID, err := controller.ExtractAppID(request.Path)
		if err != nil {
			return filestore.Item{}, fmt.Errorf("invalid app path format: %s", err)
		}

		// Check app filestore access for app-scoped paths
		hasAccess, _, err := apiServer.checkAppFilestoreAccess(req.Context(), request.Path, req, types.ActionCreate)
		if err != nil {
			return filestore.Item{}, err
		}
		if !hasAccess {
			return filestore.Item{}, fmt.Errorf("access denied to app filestore path: %s", request.Path)
		}

		// Extract the relative path within the app
		relativePath := request.Path[len("apps/")+len(appID):]
		relativePath = strings.TrimPrefix(relativePath, "/")

		// Use the app-specific create folder method
		return apiServer.Controller.FilestoreAppCreateFolder(appID, relativePath)
	}

	// Regular user path handling
	return apiServer.Controller.FilestoreCreateFolder(getOwnerContext(req), request.Path)
}

// filestoreRename godoc
// @Summary Rename filestore item
// @Description Rename a file or folder in the filestore. Cannot rename between different scopes (user/app)
// @Tags    filestore
// @Accept  json
// @Produce json
// @Param   path query string true "Current path of the file or folder"
// @Param   new_path query string true "New path for the file or folder"
// @Success 200 {object} filestore.Item
// @Router /api/v1/filestore/rename [put]
// @Security BearerAuth
func (apiServer *HelixAPIServer) filestoreRename(_ http.ResponseWriter, req *http.Request) (filestore.Item, error) {
	path := req.URL.Query().Get("path")
	newPath := req.URL.Query().Get("new_path")

	// Prevent cross-scope renaming
	// Check if paths are in different scopes (user vs app or different apps)
	fromAppPath := controller.IsAppPath(path)
	toAppPath := controller.IsAppPath(newPath)

	// Don't allow moving between user space and app space
	if fromAppPath != toAppPath {
		return filestore.Item{}, fmt.Errorf("cannot rename files between different scopes (user/app)")
	}

	// For app paths, ensure it's the same app
	if fromAppPath && toAppPath {
		fromAppID, err := controller.ExtractAppID(path)
		if err != nil {
			return filestore.Item{}, fmt.Errorf("invalid source app path: %s", err)
		}

		toAppID, err := controller.ExtractAppID(newPath)
		if err != nil {
			return filestore.Item{}, fmt.Errorf("invalid destination app path: %s", err)
		}

		if fromAppID != toAppID {
			return filestore.Item{}, fmt.Errorf("cannot rename files between different apps")
		}
	}

	// Check app filestore access for app-scoped paths
	if fromAppPath {
		hasAccess, _, err := apiServer.checkAppFilestoreAccess(req.Context(), path, req, types.ActionUpdate)
		if err != nil {
			return filestore.Item{}, err
		}
		if !hasAccess {
			return filestore.Item{}, fmt.Errorf("access denied to app filestore path: %s", path)
		}
	}

	// Return to the controller which handles path sanitization for user context
	return apiServer.Controller.FilestoreRename(getOwnerContext(req), path, newPath)
}

// filestoreDelete godoc
// @Summary Delete filestore item
// @Description Delete a file or folder from the filestore
// @Tags    filestore
// @Accept  json
// @Produce json
// @Param   path query string true "Path to the file or folder to delete"
// @Success 200 {object} object{path=string} "Path of the deleted item"
// @Router /api/v1/filestore/delete [delete]
// @Security BearerAuth
func (apiServer *HelixAPIServer) filestoreDelete(_ http.ResponseWriter, req *http.Request) (string, error) {
	path := req.URL.Query().Get("path")

	// Check if this is an app-scoped path
	if controller.IsAppPath(path) {
		appID, err := controller.ExtractAppID(path)
		if err != nil {
			return "", fmt.Errorf("invalid app path format: %s", err)
		}

		// Check app filestore access for app-scoped paths
		hasAccess, _, err := apiServer.checkAppFilestoreAccess(req.Context(), path, req, types.ActionDelete)
		if err != nil {
			return "", err
		}
		if !hasAccess {
			return "", fmt.Errorf("access denied to app filestore path: %s", path)
		}

		// Extract the relative path within the app
		relativePath := path[len("apps/")+len(appID):]
		relativePath = strings.TrimPrefix(relativePath, "/")

		// Use the app-specific delete method
		err = apiServer.Controller.FilestoreAppDelete(appID, relativePath)
		return path, err
	}

	// Regular user path handling
	err := apiServer.Controller.FilestoreDelete(getOwnerContext(req), path)
	return path, err
}

// filestoreUpload godoc
// @Summary Upload files to filestore
// @Description Upload one or more files to the specified path in the filestore. Supports multipart form data with 'files' field
// @Tags    filestore
// @Accept  multipart/form-data
// @Produce json
// @Param   path query string true "Path where files should be uploaded (e.g., 'documents', 'apps/app_id/folder')"
// @Param   files formData file true "Files to upload (multipart form data)"
// @Success 200 {object} object{success=bool} "Upload success status"
// @Router /api/v1/filestore/upload [post]
// @Security BearerAuth
// TODO version of this which is session specific
func (apiServer *HelixAPIServer) filestoreUpload(_ http.ResponseWriter, req *http.Request) (bool, error) {
	path := req.URL.Query().Get("path")

	// Check if this is an app-scoped path
	if controller.IsAppPath(path) {
		appID, err := controller.ExtractAppID(path)
		if err != nil {
			log.Error().Err(err).Str("path", path).Msg("invalid app path format")
			return false, fmt.Errorf("invalid app path format: %s", err)
		}

		// Check app filestore access for app-scoped paths
		hasAccess, _, err := apiServer.checkAppFilestoreAccess(req.Context(), path, req, types.ActionCreate)
		if err != nil {
			log.Error().Err(err).Str("path", path).Msg("error checking app filestore access")
			return false, err
		}
		if !hasAccess {
			log.Error().Str("path", path).Msg("access denied to app filestore path")
			return false, fmt.Errorf("access denied to app filestore path: %s", path)
		}

		// Parse and handle file upload
		err = req.ParseMultipartForm(10 << 20)
		if err != nil {
			return false, err
		}

		if len(req.MultipartForm.File["files"]) == 0 {
			return false, fmt.Errorf("no files to upload")
		}

		files := req.MultipartForm.File["files"]
		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				return false, fmt.Errorf("unable to open file")
			}
			defer file.Close()

			// Extract the relative path within the app
			relativePath := path[len("apps/")+len(appID):]
			relativePath = strings.TrimPrefix(relativePath, "/")

			// Strip filename from path if it contains the filename to prevent duplication
			if strings.HasSuffix(relativePath, fileHeader.Filename) {
				relativePath = strings.TrimSuffix(relativePath, fileHeader.Filename)
				relativePath = strings.TrimSuffix(relativePath, "/")
			}

			// Use the app-specific upload method
			_, err = apiServer.Controller.FilestoreAppUploadFile(appID, filepath.Join(relativePath, fileHeader.Filename), file)
			if err != nil {
				return false, fmt.Errorf("unable to upload file: %s", err.Error())
			}
		}

		return true, nil
	}

	// Regular user path handling
	err := req.ParseMultipartForm(10 << 20)
	if err != nil {
		return false, err
	}

	if len(req.MultipartForm.File["files"]) == 0 {
		return false, fmt.Errorf("no files to upload")
	}

	files := req.MultipartForm.File["files"]
	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			return false, fmt.Errorf("unable to open file")
		}
		defer file.Close()

		// Strip filename from path if it contains the filename to prevent duplication
		uploadPath := path
		if strings.HasSuffix(uploadPath, fileHeader.Filename) {
			uploadPath = strings.TrimSuffix(uploadPath, fileHeader.Filename)
			uploadPath = strings.TrimSuffix(uploadPath, "/")
		}

		_, err = apiServer.Controller.FilestoreUploadFile(getOwnerContext(req), filepath.Join(uploadPath, fileHeader.Filename), file)
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
func (apiServer *HelixAPIServer) runnerSessionUploadFiles(_ http.ResponseWriter, req *http.Request) ([]filestore.Item, error) {
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

	result := []filestore.Item{}
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

func (apiServer *HelixAPIServer) runnerSessionUploadFolder(_ http.ResponseWriter, req *http.Request) (*filestore.Item, error) {
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

		if strings.Contains(header.Name, "..") {
			return nil, fmt.Errorf("invalid tar file: %s", header.Name)
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

func (apiServer *HelixAPIServer) isAdmin(req *http.Request) bool {
	auth := apiServer.authMiddleware

	switch auth.cfg.adminUserSrc {
	case config.AdminSrcTypeEnv:
		user := getRequestUser(req)
		return auth.isUserAdmin(user.ID)
	case config.AdminSrcTypeJWT:
		token := getRequestToken(req)
		if token == "" {
			return false
		}
		user, err := auth.authenticator.ValidateUserToken(context.Background(), token)
		if err != nil {
			return false
		}
		return user.Admin
	}
	return false
}

// dashboard godoc
// @Summary Get dashboard data
// @Description Get dashboard data
// @Tags    dashboard

// @Success 200 {object} types.DashboardData
// @Router /api/v1/dashboard [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) dashboard(_ http.ResponseWriter, req *http.Request) (*types.DashboardData, error) {
	data, err := apiServer.Controller.GetDashboardData(req.Context())
	if err != nil {
		return nil, err
	}

	return data, nil
}

// usersList godoc
// @Summary List users with pagination and filtering
// @Description List users with pagination support and optional filtering by email domain or username. Supports ILIKE matching for email domains (e.g., "hotmail.com" will find all users with @hotmail.com emails) and partial username matching.
// @Tags    users
// @Accept  json
// @Produce json
// @Param page query int false "Page number (default: 1)"
// @Param per_page query int false "Number of users per page (max: 200, default: 50)"
// @Param email query string false "Filter by email domain (e.g., 'hotmail.com') or exact email"
// @Param username query string false "Filter by username (partial match)"
// @Param admin query bool false "Filter by admin status"
// @Param type query string false "Filter by user type"
// @Param token_type query string false "Filter by token type"
// @Success 200 {object} types.PaginatedUsersList
// @Router /api/v1/users [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) usersList(_ http.ResponseWriter, req *http.Request) (*types.PaginatedUsersList, error) {
	ctx := req.Context()
	user := getRequestUser(req)

	// Only admins can list users
	if !user.Admin {
		return nil, system.NewHTTPError403("only admins can list users")
	}

	// Parse query parameters
	page := 1
	if p := req.URL.Query().Get("page"); p != "" {
		if parsedPage, err := strconv.Atoi(p); err == nil && parsedPage > 0 {
			page = parsedPage
		}
	}

	perPage := 50
	if pp := req.URL.Query().Get("per_page"); pp != "" {
		if parsedPerPage, err := strconv.Atoi(pp); err == nil && parsedPerPage > 0 {
			perPage = parsedPerPage
		}
	}

	// Build query
	query := &store.ListUsersQuery{
		Page:    page,
		PerPage: perPage,
		Order:   "created_at DESC",
	}

	// Add filters
	if email := req.URL.Query().Get("email"); email != "" {
		query.Email = email
	}
	if username := req.URL.Query().Get("username"); username != "" {
		query.Username = username
	}
	if admin := req.URL.Query().Get("admin"); admin != "" {
		if adminBool, err := strconv.ParseBool(admin); err == nil {
			query.Admin = adminBool
		}
	}
	if userType := req.URL.Query().Get("type"); userType != "" {
		query.Type = types.OwnerType(userType)
	}
	if tokenType := req.URL.Query().Get("token_type"); tokenType != "" {
		query.TokenType = types.TokenType(tokenType)
	}

	// Get users from store
	users, totalCount, err := apiServer.Store.ListUsers(ctx, query)
	if err != nil {
		return nil, err
	}

	// Calculate total pages
	totalPages := int((totalCount + int64(perPage) - 1) / int64(perPage))

	// Create response
	response := &types.PaginatedUsersList{
		Users:      users,
		Page:       page,
		PageSize:   perPage,
		TotalCount: totalCount,
		TotalPages: totalPages,
	}

	return response, nil
}

// createUser godoc
// @Summary Create a new user (Admin only)
// @Description Create a new user with the specified details. Only admins can create users.
// @Tags    users
// @Accept  json
// @Produce json
// @Param request body types.AdminCreateUserRequest true "User creation request"
// @Success 200 {object} types.User
// @Router /api/v1/users [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) createUser(_ http.ResponseWriter, req *http.Request) (*types.User, error) {
	ctx := req.Context()
	user := getRequestUser(req)

	if !user.Admin {
		return nil, system.NewHTTPError403("only admins can create users")
	}

	var request types.AdminCreateUserRequest

	if err := jsoniter.NewDecoder(req.Body).Decode(&request); err != nil {
		return nil, system.NewHTTPError400("failed to decode request: " + err.Error())
	}

	if request.Email == "" {
		return nil, system.NewHTTPError400("email is required")
	}

	if request.Password == "" {
		return nil, system.NewHTTPError400("password is required")
	}

	existingUser, err := apiServer.Store.GetUser(ctx, &store.GetUserQuery{
		Email: request.Email,
	})
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, system.NewHTTPError500("failed to check if user exists: " + err.Error())
	}
	if existingUser != nil {
		return nil, system.NewHTTPError400("email is already taken")
	}

	userID := system.GenerateUserID()
	newUser := &types.User{
		ID:           userID,
		Email:        request.Email,
		FullName:     request.FullName,
		Username:     request.Email,
		Password:     request.Password,
		Admin:        request.Admin,
		Type:         types.OwnerTypeUser,
		AuthProvider: types.AuthProviderRegular,
		CreatedAt:    time.Now(),
	}

	createdUser, err := apiServer.authenticator.CreateUser(ctx, newUser)
	if err != nil {
		return nil, system.NewHTTPError500("failed to create user: " + err.Error())
	}

	return createdUser, nil
}

// adminResetPassword godoc
// @Summary Reset a user's password (Admin only)
// @Description Reset the password for any user. Only admins can use this endpoint.
// @Tags    users
// @Accept  json
// @Produce json
// @Param id path string true "User ID"
// @Param request body types.AdminResetPasswordRequest true "New password"
// @Success 200 {object} types.User
// @Failure 400 {object} system.HTTPError "Invalid request"
// @Failure 403 {object} system.HTTPError "Not authorized"
// @Failure 404 {object} system.HTTPError "User not found"
// @Router /api/v1/admin/users/{id}/password [put]
// @Security BearerAuth
func (apiServer *HelixAPIServer) adminResetPassword(_ http.ResponseWriter, req *http.Request) (*types.User, error) {
	ctx := req.Context()
	adminUser := getRequestUser(req)

	if !adminUser.Admin {
		return nil, system.NewHTTPError403("only admins can reset user passwords")
	}

	targetUserID := mux.Vars(req)["id"]
	if targetUserID == "" {
		return nil, system.NewHTTPError400("user ID is required")
	}

	var request types.AdminResetPasswordRequest
	if err := jsoniter.NewDecoder(req.Body).Decode(&request); err != nil {
		return nil, system.NewHTTPError400("failed to decode request: " + err.Error())
	}

	if request.NewPassword == "" {
		return nil, system.NewHTTPError400("new password is required")
	}

	// Verify the target user exists
	targetUser, err := apiServer.Store.GetUser(ctx, &store.GetUserQuery{ID: targetUserID})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("user not found")
		}
		return nil, system.NewHTTPError500("failed to get user: " + err.Error())
	}

	// Update the password using the authenticator
	err = apiServer.authenticator.UpdatePassword(ctx, targetUserID, request.NewPassword)
	if err != nil {
		return nil, system.NewHTTPError400("failed to update password: " + err.Error())
	}

	log.Info().
		Str("admin_id", adminUser.ID).
		Str("admin_email", adminUser.Email).
		Str("target_user_id", targetUserID).
		Str("target_user_email", targetUser.Email).
		Msg("admin reset user password")

	// Return the updated user (without password hash)
	updatedUser, err := apiServer.Store.GetUser(ctx, &store.GetUserQuery{ID: targetUserID})
	if err != nil {
		return nil, system.NewHTTPError500("failed to get updated user: " + err.Error())
	}

	return updatedUser, nil
}

// getSchedulerHeartbeats godoc
// @Summary Get scheduler goroutine heartbeat status
// @Description Get the health status of all scheduler goroutines
// @Tags    dashboard
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/scheduler/heartbeats [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getSchedulerHeartbeats(_ http.ResponseWriter, req *http.Request) (interface{}, error) {
	return apiServer.Controller.GetSchedulerHeartbeats(req.Context())
}

// deleteSlot godoc
// @Summary Delete a slot from scheduler state
// @Description Delete a slot from the scheduler's desired state, allowing reconciliation to clean it up from the runner
// @Tags    dashboard
// @Param   slot_id path string true "Slot ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/slots/{slot_id} [delete]
// @Security BearerAuth
func (apiServer *HelixAPIServer) deleteSlot(_ http.ResponseWriter, req *http.Request) (interface{}, error) {
	vars := mux.Vars(req)
	slotID := vars["slot_id"]

	if slotID == "" {
		return nil, fmt.Errorf("slot_id is required")
	}

	// Parse slot ID as UUID
	slotUUID, err := uuid.Parse(slotID)
	if err != nil {
		return nil, fmt.Errorf("invalid slot_id format: %w", err)
	}
	// Delete the slot from scheduler's desired state
	err = apiServer.Controller.DeleteSlotFromScheduler(req.Context(), slotUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete slot: %w", err)
	}

	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Slot %s deleted from scheduler state", slotID),
	}, nil
}

// deleteSession godoc
// @Summary Delete a session by ID
// @Description Delete a session by ID
// @Tags    sessions
// @Success 200 {object} types.Session
// @Param id path string true "Session ID"
// @Router /api/v1/sessions/{id} [delete]
// @Security BearerAuth
func (apiServer *HelixAPIServer) deleteSession(_ http.ResponseWriter, req *http.Request) (*types.Session, *system.HTTPError) {
	session, httpError := apiServer.sessionLoader(req, true)
	if httpError != nil {
		return nil, httpError
	}

	return system.DefaultController(apiServer.Store.DeleteSession(req.Context(), session.ID))
}

// updateSession godoc
// @Summary Update a session by ID
// @Description Update a session by ID
// @Tags    sessions
// @Param id path string true "Session ID"
// @Param request body types.Session true "Session to update"
// @Success 200 {object} types.Session
// @Router /api/v1/sessions/{id} [put]
// @Security BearerAuth
func (apiServer *HelixAPIServer) updateSession(_ http.ResponseWriter, req *http.Request) (*types.Session, *system.HTTPError) {
	session, httpError := apiServer.sessionLoader(req, true)
	if httpError != nil {
		return nil, httpError
	}

	var update *types.Session

	err := json.NewDecoder(req.Body).Decode(&update)
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	session.Name = update.Name
	session.Provider = update.Provider
	session.ModelName = update.ModelName

	updated, err := apiServer.Store.UpdateSession(req.Context(), *session)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return updated, nil
}

// createAPIKey godoc
// @Summary Create a new API key
// @Description Create a new API key
// @Tags    api-keys
// @Param request body map[string]interface{} true "Request body with name and type"
// @Success 200 {string} string "API key"
// @Router /api/v1/api_keys [post]
func (apiServer *HelixAPIServer) createAPIKey(_ http.ResponseWriter, req *http.Request) (string, error) {
	newAPIKey := &types.ApiKey{}
	name := req.URL.Query().Get("name")

	user := getRequestUser(req)
	ctx := req.Context()

	if name != "" {
		// if we are using the query string route then don't try to deode the body
		newAPIKey.Name = name
		newAPIKey.Type = types.APIkeytypeAPI
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

// getAPIKeys godoc
// @Summary Get API keys
// @Description Get API keys
// @Tags    api-keys
// @Param types query string false "Filter by types (comma-separated list)"
// @Param app_id query string false "Filter by app ID"
// @Success 200 {array} types.ApiKey
// @Router /api/v1/api_keys [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getAPIKeys(_ http.ResponseWriter, req *http.Request) ([]*types.ApiKey, error) {
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

	filteredAPIKeys := []*types.ApiKey{}
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

	// If filter is missing, we are getting user keys. If we haven't got any. create a new one.
	if typesParam == "" && appIDParam == "" && len(apiKeys) == 0 {
		createdKey, err := apiServer.Controller.CreateAPIKey(ctx, user, &types.ApiKey{
			Created: time.Now(),
			Key:     uuid.New().String(),
			Name:    "API Key",
			Type:    types.APIkeytypeAPI,
			Owner:   user.ID,
		})
		if err != nil {
			return nil, err
		}
		apiKeys = append(apiKeys, createdKey)
		return apiKeys, nil
	}

	return apiKeys, nil
}

// deleteAPIKey godoc
// @Summary Delete an API key
// @Description Delete an API key
// @Tags    api-keys
// @Param key query string true "API key to delete"
// @Success 200 {string} string "API key"
// @Router /api/v1/api_keys [delete]
func (apiServer *HelixAPIServer) deleteAPIKey(_ http.ResponseWriter, req *http.Request) (string, error) {
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
func (apiServer *HelixAPIServer) checkAPIKey(_ http.ResponseWriter, req *http.Request) (*types.ApiKey, error) {
	ctx := req.Context()

	apiKey := req.URL.Query().Get("key")
	key, err := apiServer.Controller.CheckAPIKey(ctx, apiKey)
	if err != nil {
		return nil, err
	}
	return key, nil
}
