package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// filestoreViewerHandler godoc
// @Summary View filestore files
// @Description Serve files from the filestore with access control. Supports both user and app-scoped paths
// @Tags    filestore
// @Accept  json
// @Produce application/octet-stream
// @Param   path path string true "File path to view (e.g., 'dev/users/user_id/file.pdf', 'dev/apps/app_id/file.pdf')"
// @Param   redirect_urls query string false "Set to 'true' to redirect .url files to their target URLs"
// @Param   signature query string false "URL signature for public access"
// @Success 200 {file} file "File content"
// @Router /api/v1/filestore/viewer/{path} [get]
// @Security BearerAuth
func (s *HelixAPIServer) filestoreViewerHandler(w http.ResponseWriter, r *http.Request) {
	// if the session is "shared" then anyone can see the files inside the session
	// if the user is admin then can see anything
	// if the user is runner then can see anything
	// if the path is part of the user path then can see it
	// if path has presign URL
	// otherwise access denied
	canAccess, err := s.isFilestoreRouteAuthorized(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !canAccess {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	// read the query param called redirect_urls
	// if it's present and set to the string "true"
	// then assign a boolean
	shouldRedirectURLs := r.URL.Query().Get("redirect_urls") == "true"
	if shouldRedirectURLs && strings.HasSuffix(r.URL.Path, ".url") {
		url, err := s.Controller.FilestoreReadTextFile(r.URL.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		} else {
			http.Redirect(w, r, url, http.StatusFound)
		}
	} else {
		// Set cache control headers to prevent caching
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		s.fileServerHandler.ServeHTTP(w, r)
	}
}

// given a full filestore route (i.e. one that starts with /dev/users/XXX)
// this will tell you if the given http request is authorized to access it
func (s *HelixAPIServer) isFilestoreRouteAuthorized(req *http.Request) (bool, error) {
	logger := log.With().
		Str("path", req.URL.Path).
		Str("method", req.Method).
		Logger()

	logger.Debug().Msg("Checking filestore route authorization")

	if req.URL.Query().Get("signature") != "" {
		// Construct full URL
		u := fmt.Sprintf("http://api/%s%s?%s", req.URL.Host, req.URL.Path, req.URL.RawQuery)
		verified := s.Controller.VerifySignature(u)
		logger.Debug().Bool("signatureVerified", verified).Msg("Checking URL signature")
		return verified, nil
	}

	user := getRequestUser(req)
	logger.Debug().
		Str("user_id", user.ID).
		Str("username", user.Username).
		Str("user_type", string(user.Type)).
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

	// Check if this is an app path - ensure it matches the expected structure:
	// /api/v1/filestore/viewer/dev/apps/... - not just any path with "apps" in it
	appPath := extractAppPathFromViewerURL(req.URL.Path)
	logger.Debug().
		Str("original_path", req.URL.Path).
		Str("extracted_app_path", appPath).
		Bool("is_app_path", appPath != "" && strings.HasPrefix(appPath, "apps/")).
		Msg("App path extraction result")

	if appPath != "" && strings.HasPrefix(appPath, "apps/") {
		logger.Debug().Str("app_path", appPath).Msg("Path appears to be an app path")

		// Check app access using the same logic as in the API handlers
		hasAccess, appID, err := s.checkAppFilestoreAccess(req.Context(), appPath, req, types.ActionGet)
		if err != nil {
			logger.Error().Err(err).Str("app_path", appPath).Msg("Error checking app filestore access")
			return false, err
		}

		logger.Debug().
			Str("app_id", appID).
			Bool("has_access", hasAccess).
			Msg("App access check result")

		return hasAccess, nil
	}

	reqUser := getRequestUser(req)
	userID := reqUser.ID
	if userID == "" {
		logger.Debug().Msg("No user ID found in request, denying access")
		return false, nil
	}

	userPath, err := s.Controller.GetFilestoreUserPath(types.OwnerContext{
		Owner:     userID,
		OwnerType: types.OwnerTypeUser,
	}, "")
	if err != nil {
		logger.Error().Err(err).Msg("Error getting user filestore path")
		return false, err
	}

	logger.Info().
		Str("user_path", userPath).
		Str("url_path", req.URL.Path).
		Bool("path_match", strings.HasPrefix(req.URL.Path, userPath)).
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
	// In the stripped case, we're receiving paths like:
	// "{prefix}/apps/{app_id}/..." (after StripPrefix was applied)
	// e.g. "dev/apps/app_123/file.pdf"

	// Split the path into segments
	segments := strings.Split(urlPath, "/")

	// We need at least 2 segments and the second one should be "apps"
	if len(segments) > 1 && segments[1] == "apps" {
		// Return everything from "apps" onwards
		return strings.Join(segments[1:], "/")
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
func (s *HelixAPIServer) checkAppFilestoreAccess(ctx context.Context, path string, req *http.Request, requiredAction types.Action) (bool, string, error) {
	logger := log.With().
		Str("path", path).
		Str("required_action", string(requiredAction)).
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

	// Get the app to check permissions
	app, err := s.Store.GetApp(ctx, appID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			logger.Debug().Str("app_id", appID).Msg("App not found")
			return false, appID, nil
		}
		logger.Error().Err(err).Str("app_id", appID).Msg("Error retrieving app")
		return false, appID, err
	}

	user := getRequestUser(req)

	err = s.authorizeUserToApp(ctx, user, app, requiredAction)
	if err != nil {
		return false, appID, nil
	}

	return true, appID, nil
}
