package server

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/lukemarsden/helix/api/pkg/controller"
	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

func (apiServer *HelixAPIServer) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Access-Control-Allow-Origin", "*")
		next.ServeHTTP(res, req)
	})
}

func (apiServer *HelixAPIServer) getRequestContext(req *http.Request) types.RequestContext {
	user := getRequestUser(req)
	return types.RequestContext{
		Ctx:       req.Context(),
		Owner:     user,
		OwnerType: types.OwnerTypeUser,
		Admin:     apiServer.adminAuth.isUserAdmin(user),
	}
}

func (apiServer *HelixAPIServer) canSeeSession(reqContext types.RequestContext, session *types.Session) bool {
	if session.OwnerType == reqContext.OwnerType && session.Owner == reqContext.Owner {
		return true
	}
	return apiServer.adminAuth.isUserAdmin(reqContext.Owner)
}

func (apiServer *HelixAPIServer) canEditSession(reqContext types.RequestContext, session *types.Session) bool {
	return apiServer.canSeeSession(reqContext, session)
}

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

func errorLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wrap the ResponseWriter
		lrw := NewLoggingResponseWriter(w)

		// Call the next handler, which can be another middleware in the chain, or the final handler.
		next.ServeHTTP(lrw, r)

		if lrw.statusCode >= 400 {
			log.Error().Msgf("Method: %s, Path: %s, Status: %d\n", r.Method, r.URL.Path, lrw.statusCode)
		}
	})
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
) (*types.Interaction, error) {
	message := req.FormValue("input")

	if sessionMode == types.SessionModeInference && message == "" {
		return nil, fmt.Errorf("inference sessions require a message")
	}

	filePaths := []string{}
	files, okFiles := req.MultipartForm.File["files"]
	inputPath := controller.GetSessionInputsFolder(sessionID)

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
			imageItem, err := apiServer.Controller.FilestoreUpload(apiServer.getRequestContext(req), filePath, file)
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

				labelItem, err := apiServer.Controller.FilestoreUpload(apiServer.getRequestContext(req), labelFilepath, strings.NewReader(label))
				if err != nil {
					return nil, fmt.Errorf("unable to create label: %s", err.Error())
				}
				log.Debug().Msgf("success uploading file: %s", fileHeader.Filename)
				filePaths = append(filePaths, labelItem.Path)
			}
		}
		log.Debug().Msgf("success uploading files")
	}

	if sessionMode == types.SessionModeFinetune && len(filePaths) == 0 {
		return nil, fmt.Errorf("finetune sessions require some files")
	}

	return &types.Interaction{
		ID:        system.GenerateUUID(),
		Created:   time.Now(),
		Updated:   time.Now(),
		Scheduled: time.Now(),
		Completed: time.Now(),
		Creator:   types.CreatorTypeUser,
		Message:   message,
		Files:     filePaths,
		State:     types.InteractionStateComplete,
		Finished:  true,
		Metadata:  metadata,
	}, nil
}

func (apiServer *HelixAPIServer) convertFilestorePath(ctx context.Context, sessionID string, filePath string) (string, types.RequestContext, error) {
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return "", types.RequestContext{}, err
	}

	if session == nil {
		return "", types.RequestContext{}, fmt.Errorf("no session found with id %v", sessionID)
	}

	requestContext := types.RequestContext{
		Ctx:       ctx,
		Owner:     session.Owner,
		OwnerType: session.OwnerType,
	}
	// let's remove the /dev/users/XXX part of the path if it's there
	userPath, err := apiServer.Controller.GetFilestoreUserPath(requestContext, "")
	if err != nil {
		return "", types.RequestContext{}, err
	}

	if strings.HasPrefix(filePath, userPath) {
		filePath = strings.TrimPrefix(filePath, userPath)
	}

	return filePath, requestContext, nil
}

func (apiServer *HelixAPIServer) requireAdmin(req *http.Request) error {
	isAdmin := apiServer.isAdmin(req)
	if !isAdmin {
		return fmt.Errorf("access denied")
	} else {
		return nil
	}
}
