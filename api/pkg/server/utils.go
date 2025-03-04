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
	if req.URL.Query().Get("signature") != "" {
		// Construct full URL
		u := fmt.Sprintf("http://api/%s%s?%s", req.URL.Host, req.URL.Path, req.URL.RawQuery)
		return apiServer.Controller.VerifySignature(u), nil
	}

	// if the session is "shared" then anyone can see it's files
	sessionID := extractSessionID(req.URL.Path)
	if sessionID != "" {
		session, err := apiServer.Store.GetSession(req.Context(), sessionID)
		if err != nil {
			return false, err
		}
		if session.Metadata.Shared {
			return true, nil
		}
	}

	user := getRequestUser(req)

	// a runner can see all files
	if isRunner(user) {
		return true, nil
	}

	// an admin user can see all files
	if isAdmin(user) {
		return true, nil
	}

	reqUser := getRequestUser(req)
	userID := reqUser.ID
	if userID == "" {
		return false, nil
	}
	userPath, err := apiServer.Controller.GetFilestoreUserPath(types.OwnerContext{
		Owner:     userID,
		OwnerType: types.OwnerTypeUser,
	}, "")
	if err != nil {
		return false, err
	}
	if strings.HasPrefix(req.URL.Path, userPath) {
		return true, nil
	}
	return false, nil
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
	// If the path doesn't start with "apps/", it's not app-scoped
	if !strings.HasPrefix(path, "apps/") {
		return false, "", nil
	}

	// Extract the app ID from the path (apps/:app_id/...)
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 {
		return false, "", fmt.Errorf("invalid app filestore path format: %s", path)
	}

	appID := parts[1]

	// Get the app to check permissions
	app, err := apiServer.Store.GetApp(ctx, appID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return false, appID, nil
		}
		return false, appID, err
	}

	// Get the user from the request
	user := getRequestUser(req)

	// Admin users have access to all apps
	if isAdmin(user) {
		return true, appID, nil
	}

	// If the user is the owner of the app, they have access
	if user.ID == app.Owner && app.OwnerType == types.OwnerTypeUser {
		return true, appID, nil
	}

	// Check if the app is public/global
	if app.Global {
		// For global apps, read access is allowed for everyone
		if requiredAction == types.ActionGet || requiredAction == types.ActionList {
			return true, appID, nil
		}
		// Only admins and owners can modify global apps
		return false, appID, nil
	}

	// Check if the app is shared
	if app.Shared {
		// For shared apps, read access is allowed for everyone
		if requiredAction == types.ActionGet || requiredAction == types.ActionList {
			return true, appID, nil
		}
		// Only admins and owners can modify shared apps
		return false, appID, nil
	}

	// Now check RBAC permissions through access grants
	accessGrants, err := apiServer.Store.ListAccessGrants(ctx, &store.ListAccessGrantsQuery{
		ResourceType: types.ResourceApplication,
		ResourceID:   appID,
		UserID:       user.ID,
	})
	if err != nil {
		return false, appID, err
	}

	// If no access grants found, the user doesn't have access
	if len(accessGrants) == 0 {
		return false, appID, nil
	}

	// Check if any of the roles in the access grants allow the required action
	for _, accessGrant := range accessGrants {
		for _, role := range accessGrant.Roles {
			for _, rule := range role.Config.Rules {
				// Check if the rule applies to application resources
				resourceMatch := false
				for _, resource := range rule.Resources {
					if resource == types.ResourceApplication || resource == "*" {
						resourceMatch = true
						break
					}
				}

				if !resourceMatch {
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
					return true, appID, nil
				}
			}
		}
	}

	// No matching rules found, access denied
	return false, appID, nil
}
