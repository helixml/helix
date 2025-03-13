package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type LoggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func NewLoggingResponseWriter(w http.ResponseWriter) *LoggingResponseWriter {
	return &LoggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (lrw *LoggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

// Hijack lets the caller take over the connection.
// Implement this method to support websockets.
func (lrw *LoggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := lrw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("the ResponseWriter does not support Hijack")
	}
	return hijacker.Hijack()
}

func ErrorLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wrap the ResponseWriter
		lrw := NewLoggingResponseWriter(w)

		// Create a custom ResponseWriter that supports flushing
		flushWriter := &flushResponseWriter{lrw}

		// Call the next handler, which can be another middleware in the chain, or the final handler.
		start := time.Now()
		next.ServeHTTP(flushWriter, r)
		log.Trace().Str("method", r.Method).Str("path", r.URL.Path).Dur("duration_ms", time.Since(start)).Msg("request")

		switch lrw.statusCode {
		case http.StatusForbidden:
			log.Warn().Msgf("unauthorized - method: %s, path: %s, status: %d\n", r.Method, r.URL.Path, lrw.statusCode)
		default:
			if lrw.statusCode >= 400 {
				log.Warn().Str("method", r.Method).Str("path", r.URL.Path).Int("status", lrw.statusCode).Msg("response")
			}
		}
	})
}

type flushResponseWriter struct {
	*LoggingResponseWriter
}

func (frw *flushResponseWriter) Flush() {
	if f, ok := frw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// create a new data entity from the uploaded files
func (apiServer *HelixAPIServer) getDataEntityFromForm(
	req *http.Request,
) (*types.DataEntity, error) {
	ID := system.GenerateUUID()

	user := getRequestUser(req)

	err := req.ParseMultipartForm(10 << 20)
	if err != nil {
		return nil, err
	}

	files, okFiles := req.MultipartForm.File["files"]
	inputPath := controller.GetDataEntityFolder(ID)

	metadata := map[string]string{}

	if okFiles {
		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				return nil, fmt.Errorf("unable to open file")
			}
			defer file.Close()

			filePath := filepath.Join(inputPath, fileHeader.Filename)

			log.Debug().Msgf("uploading file %s", filePath)
			imageItem, err := apiServer.Controller.FilestoreUploadFile(getOwnerContext(req), filePath, file)
			if err != nil {
				return nil, fmt.Errorf("unable to upload file: %s", err.Error())
			}
			log.Debug().Msgf("success uploading file: %s", imageItem.Path)

			// let's see if there is a single form field named after the filename
			// this is for labelling images for fine tuning
			labelValues, ok := req.MultipartForm.Value[fileHeader.Filename]

			if ok && len(labelValues) > 0 {
				filenameParts := strings.Split(fileHeader.Filename, ".")
				filenameParts[len(filenameParts)-1] = "txt"
				labelFilename := strings.Join(filenameParts, ".")
				labelFilepath := filepath.Join(inputPath, labelFilename)
				label := labelValues[0]

				metadata[fileHeader.Filename] = label

				_, err := apiServer.Controller.FilestoreUploadFile(getOwnerContext(req), labelFilepath, strings.NewReader(label))
				if err != nil {
					return nil, fmt.Errorf("unable to create label: %s", err.Error())
				}
				log.Debug().Msgf("success uploading file: %s", fileHeader.Filename)
			}
		}
		log.Debug().Msgf("success uploading files")
	}

	return &types.DataEntity{
		ID:        ID,
		Created:   time.Now(),
		Updated:   time.Now(),
		Type:      types.DataEntityTypeUploadedDocuments,
		Owner:     user.ID,
		OwnerType: user.Type,
		Config: types.DataEntityConfig{
			FilestorePath: inputPath,
		},
	}, nil
}

// based on a multi-part form that has message and files
// return a user interaction we can add to a session
// if we are uploading files for a fine-tuning session for images
// then the form data will have a field named after each of the filenames
// this is the label for the file and we should create a text file
// in the session folder that is named after the file and contains the label
func (apiServer *HelixAPIServer) getUserInteractionFromForm(
	req *http.Request,
	sessionID string,
	sessionMode types.SessionMode,
	interactionID string,
) (*types.Interaction, error) {
	message := req.FormValue("input")

	if sessionMode == types.SessionModeInference && message == "" {
		return nil, fmt.Errorf("inference sessions require a message")
	}

	if interactionID == "" {
		interactionID = system.GenerateUUID()
	}

	filePaths := []string{}
	files, okFiles := req.MultipartForm.File["files"]
	inputPath := controller.GetInteractionInputsFolder(sessionID, interactionID)

	metadata := map[string]string{}

	if okFiles {
		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				return nil, fmt.Errorf("unable to open file")
			}
			defer file.Close()

			filePath := filepath.Join(inputPath, fileHeader.Filename)

			log.Debug().Msgf("uploading file %s", filePath)
			imageItem, err := apiServer.Controller.FilestoreUploadFile(getOwnerContext(req), filePath, file)
			if err != nil {
				return nil, fmt.Errorf("unable to upload file: %s", err.Error())
			}
			log.Debug().Msgf("success uploading file: %s", imageItem.Path)
			filePaths = append(filePaths, imageItem.Path)

			// let's see if there is a single form field named after the filename
			// this is for labelling images for fine tuning
			labelValues, ok := req.MultipartForm.Value[fileHeader.Filename]

			if ok && len(labelValues) > 0 {
				filenameParts := strings.Split(fileHeader.Filename, ".")
				filenameParts[len(filenameParts)-1] = "txt"
				labelFilename := strings.Join(filenameParts, ".")
				labelFilepath := filepath.Join(inputPath, labelFilename)
				label := labelValues[0]

				metadata[fileHeader.Filename] = label

				labelItem, err := apiServer.Controller.FilestoreUploadFile(getOwnerContext(req), labelFilepath, strings.NewReader(label))
				if err != nil {
					return nil, fmt.Errorf("unable to create label: %s", err.Error())
				}
				log.Debug().Msgf("success uploading file: %s", fileHeader.Filename)
				filePaths = append(filePaths, labelItem.Path)
			}
		}
		log.Debug().Msgf("success uploading files")
	}

	return &types.Interaction{
		ID:             interactionID,
		Created:        time.Now(),
		Updated:        time.Now(),
		Scheduled:      time.Now(),
		Completed:      time.Now(),
		Creator:        types.CreatorTypeUser,
		Mode:           sessionMode,
		Message:        message,
		Files:          filePaths,
		State:          types.InteractionStateComplete,
		Finished:       true,
		Metadata:       metadata,
		DataPrepChunks: map[string][]types.DataPrepChunk{},
	}, nil
}

// given a data entity that is the uploaded files from a user
// return a user interaction that is the old style of interaction
// that has files inside
// TODO: we won't need this once we have data entity pipelines
func (apiServer *HelixAPIServer) getUserInteractionFromDataEntity(
	dataEntity *types.DataEntity,
	ownerContext types.OwnerContext,
) (*types.Interaction, error) {
	filePaths := []string{}
	dataEntityPath := controller.GetDataEntityFolder(dataEntity.ID)
	files, err := apiServer.Controller.FilestoreList(ownerContext, dataEntityPath)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		filePaths = append(filePaths, file.Path)
	}

	return &types.Interaction{
		ID:             system.GenerateUUID(),
		Created:        time.Now(),
		Updated:        time.Now(),
		Scheduled:      time.Now(),
		Completed:      time.Now(),
		Creator:        types.CreatorTypeUser,
		Mode:           types.SessionModeFinetune,
		Files:          filePaths,
		State:          types.InteractionStateComplete,
		Finished:       true,
		Metadata:       map[string]string{},
		DataPrepChunks: map[string][]types.DataPrepChunk{},
	}, nil
}

func (apiServer *HelixAPIServer) convertFilestorePath(ctx context.Context, sessionID string, filePath string) (string, types.OwnerContext, error) {
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return "", types.OwnerContext{}, err
	}

	if session == nil {
		return "", types.OwnerContext{}, fmt.Errorf("no session found with id %v", sessionID)
	}

	ownerContext := types.OwnerContext{
		Owner:     session.Owner,
		OwnerType: session.OwnerType,
	}
	// let's remove the /dev/users/XXX part of the path if it's there
	userPath, err := apiServer.Controller.GetFilestoreUserPath(ownerContext, "")
	if err != nil {
		return "", types.OwnerContext{}, err
	}

	// NOTE(milosgajdos): no need for if check
	// https://pkg.go.dev/strings#TrimPrefix
	filePath = strings.TrimPrefix(filePath, userPath)

	return filePath, ownerContext, nil
}

func extractSessionID(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "sessions" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// given a full filestore route (i.e. one that starts with /dev/users/XXX)
// this will tell you if the given http request is authorized to access it
func (apiServer *HelixAPIServer) isFilestoreRouteAuthorized(req *http.Request) (bool, error) {
	logger := log.With().
		Str("path", req.URL.Path).
		Str("method", req.Method).
		Logger()

	logger.Debug().Msg("Checking filestore route authorization")

	if req.URL.Query().Get("signature") != "" {
		// Construct full URL
		u := fmt.Sprintf("http://api/%s%s?%s", req.URL.Host, req.URL.Path, req.URL.RawQuery)
		verified := apiServer.Controller.VerifySignature(u)
		logger.Debug().Bool("signatureVerified", verified).Msg("Checking URL signature")
		return verified, nil
	}

	// if the session is "shared" then anyone can see it's files
	sessionID := extractSessionID(req.URL.Path)
	if sessionID != "" {
		logger.Debug().Str("sessionID", sessionID).Msg("Found session ID in path")
		session, err := apiServer.Store.GetSession(req.Context(), sessionID)
		if err != nil {
			logger.Error().Err(err).Str("sessionID", sessionID).Msg("Error retrieving session")
			return false, err
		}
		if session.Metadata.Shared {
			logger.Debug().Str("sessionID", sessionID).Msg("Session is shared, allowing access")
			return true, nil
		}
		logger.Debug().Str("sessionID", sessionID).Bool("isShared", false).Msg("Session is not shared")
	}

	user := getRequestUser(req)
	logger.Debug().
		Str("userID", user.ID).
		Str("username", user.Username).
		Str("userType", string(user.Type)).
		Msg("User information from request")

	// a runner can see all files
	if isRunner(user) {
		logger.Debug().Msg("User is a runner, allowing access")
		return true, nil
	}

	// an admin user can see all files
	if isAdmin(user) {
		logger.Debug().Msg("User is an admin, allowing access")
		return true, nil
	}

	// Check if this is an app path
	if strings.Contains(req.URL.Path, "/apps/") {
		appPath := extractAppPathFromViewerURL(req.URL.Path)
		if appPath != "" {
			logger.Debug().Str("appPath", appPath).Msg("Path appears to be an app path")

			// Check app access using the same logic as in the API handlers
			hasAccess, appID, err := apiServer.checkAppFilestoreAccess(req.Context(), appPath, req, types.ActionGet)
			if err != nil {
				logger.Error().Err(err).Str("appPath", appPath).Msg("Error checking app filestore access")
				return false, err
			}

			logger.Debug().
				Str("appID", appID).
				Bool("hasAccess", hasAccess).
				Msg("App access check result")

			return hasAccess, nil
		}
	}

	reqUser := getRequestUser(req)
	userID := reqUser.ID
	if userID == "" {
		logger.Debug().Msg("No user ID found in request, denying access")
		return false, nil
	}

	userPath, err := apiServer.Controller.GetFilestoreUserPath(types.OwnerContext{
		Owner:     userID,
		OwnerType: types.OwnerTypeUser,
	}, "")
	if err != nil {
		logger.Error().Err(err).Msg("Error getting user filestore path")
		return false, err
	}

	logger.Debug().
		Str("userPath", userPath).
		Bool("pathMatch", strings.HasPrefix(req.URL.Path, userPath)).
		Msg("Checking if path is in user's filestore")

	if strings.HasPrefix(req.URL.Path, userPath) {
		logger.Debug().Msg("Path is in user's filestore, allowing access")
		return true, nil
	}

	logger.Debug().Msg("No access rules matched, denying access")
	return false, nil
}

// Helper function to extract app path from a viewer URL
func extractAppPathFromViewerURL(urlPath string) string {
	// Example: /api/v1/filestore/viewer/dev/apps/app_123/file.pdf
	parts := strings.Split(urlPath, "/")
	for i, part := range parts {
		if part == "apps" && i+1 < len(parts) {
			return strings.Join(parts[i:], "/")
		}
	}
	return ""
}

// this means our local filestore viewer will not list directories
type neuteredFileSystem struct {
	fs http.FileSystem
}

func (nfs neuteredFileSystem) Open(path string) (http.File, error) {
	f, err := nfs.fs.Open(path)
	if err != nil {
		return nil, err
	}

	s, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if s.IsDir() {
		return nil, errors.New("directory access is denied")
	}

	return f, nil
}

// checkAppFilestoreAccess checks if the user has access to app-scoped filestore paths
// This enforces the same RBAC controls as for apps themselves
func (apiServer *HelixAPIServer) checkAppFilestoreAccess(ctx context.Context, path string, req *http.Request, requiredAction types.Action) (bool, string, error) {
	logger := log.With().
		Str("path", path).
		Str("requiredAction", string(requiredAction)).
		Logger()

	logger.Debug().Msg("Starting filestore access check")

	// If the path doesn't start with "apps/", it's not app-scoped
	if !controller.IsAppPath(path) {
		logger.Debug().Msg("Path is not an app path")
		return false, "", nil
	}

	// Extract the app ID from the path (apps/:app_id/...)
	appID, err := controller.ExtractAppID(path)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to extract app ID from path")
		return false, "", fmt.Errorf("invalid app filestore path format: %s", path)
	}

	logger.Debug().Str("appID", appID).Msg("Extracted app ID from path")

	// Get the app to check permissions
	app, err := apiServer.Store.GetApp(ctx, appID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			logger.Debug().Str("appID", appID).Msg("App not found")
			return false, appID, nil
		}
		logger.Error().Err(err).Str("appID", appID).Msg("Error retrieving app")
		return false, appID, err
	}

	logger.Debug().
		Str("appID", appID).
		Str("appOwner", app.Owner).
		Str("ownerType", string(app.OwnerType)).
		Bool("isGlobal", app.Global).
		Bool("isShared", app.Shared).
		Msg("Retrieved app information")

	// Get the user from the request
	user := getRequestUser(req)
	logger.Debug().
		Str("userID", user.ID).
		Str("username", user.Username).
		Bool("isAuthenticated", user.ID != "").
		Msg("User information from request")

	// Admin users have access to all apps
	if isAdmin(user) {
		logger.Debug().Msg("User is admin, granting access")
		return true, appID, nil
	}

	// If the user is the owner of the app, they have access
	if user.ID == app.Owner && app.OwnerType == types.OwnerTypeUser {
		logger.Debug().Msg("User is the app owner, granting access")
		return true, appID, nil
	}

	// Log ownership mismatch if user is not empty and not the owner
	if user.ID != "" && user.ID != app.Owner {
		logger.Debug().
			Str("userID", user.ID).
			Str("appOwner", app.Owner).
			Msg("User is not the app owner")
	}

	// Check if the app is public/global
	if app.Global {
		// For global apps, read access is allowed for everyone
		if requiredAction == types.ActionGet || requiredAction == types.ActionList {
			logger.Debug().Msg("App is global, granting read access")
			return true, appID, nil
		}
		// Only admins and owners can modify global apps
		logger.Debug().Msg("App is global but user requires write access, denying access")
		return false, appID, nil
	}

	// Check if the app is shared
	if app.Shared {
		// For shared apps, read access is allowed for everyone
		if requiredAction == types.ActionGet || requiredAction == types.ActionList {
			logger.Debug().Msg("App is shared, granting read access")
			return true, appID, nil
		}
		// Only admins and owners can modify shared apps
		logger.Debug().Msg("App is shared but user requires write access, denying access")
		return false, appID, nil
	}

	// Now check RBAC permissions through access grants
	accessGrants, err := apiServer.Store.ListAccessGrants(ctx, &store.ListAccessGrantsQuery{
		ResourceType: types.ResourceApplication,
		ResourceID:   appID,
		UserID:       user.ID,
	})
	if err != nil {
		logger.Error().Err(err).Msg("Error querying access grants")
		return false, appID, err
	}

	logger.Debug().Int("numAccessGrants", len(accessGrants)).Msg("Retrieved access grants")

	// If no access grants found, the user doesn't have access
	if len(accessGrants) == 0 {
		logger.Debug().Msg("No access grants found for user, denying access")
		return false, appID, nil
	}

	// Check if any of the roles in the access grants allow the required action
	for _, accessGrant := range accessGrants {
		logger := logger.With().
			Str("grantID", accessGrant.ID).
			Int("numRoles", len(accessGrant.Roles)).
			Logger()

		logger.Debug().Msg("Checking access grant")

		for i, role := range accessGrant.Roles {
			logger := logger.With().
				Int("roleIndex", i).
				Str("roleName", role.Name).
				Int("numRules", len(role.Config.Rules)).
				Logger()

			logger.Debug().Msg("Checking role")

			for j, rule := range role.Config.Rules {
				// Convert []Resource to []string for logging
				resources := make([]string, len(rule.Resources))
				for k, r := range rule.Resources {
					resources[k] = string(r)
				}

				// Convert []Action to []string for logging
				actions := make([]string, len(rule.Actions))
				for k, a := range rule.Actions {
					actions[k] = string(a)
				}

				logger := logger.With().
					Int("ruleIndex", j).
					Strs("resources", resources).
					Strs("actions", actions).
					Str("effect", string(rule.Effect)).
					Logger()

				logger.Debug().Msg("Checking rule")

				// Check if the rule applies to application resources
				resourceMatch := false
				for _, resource := range rule.Resources {
					if resource == types.ResourceApplication || resource == "*" {
						resourceMatch = true
						break
					}
				}

				if !resourceMatch {
					logger.Debug().Msg("Rule does not apply to application resources")
					continue
				}

				// Check if the action is allowed
				actionMatch := false
				for _, action := range rule.Actions {
					if action == requiredAction || action == "*" {
						actionMatch = true
						break
					}
				}

				if actionMatch && rule.Effect == types.EffectAllow {
					logger.Debug().Msg("Found matching rule with allow effect, granting access")
					return true, appID, nil
				}

				if actionMatch {
					logger.Debug().Str("effect", string(rule.Effect)).Msg("Action matched but effect is not 'allow'")
				}
			}
		}
	}

	// No matching rules found, access denied
	logger.Debug().Msg("No matching rules found in access grants, denying access")
	return false, appID, nil
}
