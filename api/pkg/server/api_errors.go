package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/inferencerouter"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

// jsonDecodeErrorMessage converts a JSON decode error into a client-facing
// message that names the offending JSON field without exposing Go type or
// struct names (e.g. "Go struct field SharePointSiteResolveRequest.site_url").
func jsonDecodeErrorMessage(err error) string {
	var typeErr *json.UnmarshalTypeError
	var syntaxErr *json.SyntaxError
	switch {
	case errors.As(err, &typeErr) && typeErr.Field != "":
		return fmt.Sprintf("Invalid request body: field %q has the wrong type", typeErr.Field)
	case errors.As(err, &syntaxErr), errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return "Invalid request body: must be a valid JSON object"
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		return "Invalid request body: unknown field " + strings.TrimPrefix(err.Error(), "json: unknown field ")
	default:
		return "Invalid request body"
	}
}

// jsonDecodeHTTPError is jsonDecodeErrorMessage as a 400 for handlers that
// return errors through system.Wrapper / system.DefaultWrapper.
func jsonDecodeHTTPError(err error) *system.HTTPError {
	return system.NewHTTPError400(jsonDecodeErrorMessage(err))
}

// storeLookupError maps a failed store lookup of one object to an HTTP error:
// 404 "<resource> not found" when the row does not exist, otherwise a generic
// 500 with the underlying error logged server-side only, so database error
// text never reaches the client.
func storeLookupError(err error, resource string) *system.HTTPError {
	if errors.Is(err, store.ErrNotFound) {
		return system.NewHTTPError404(resource + " not found")
	}
	log.Error().Err(err).Str("resource", resource).Msg("store lookup failed")
	return system.NewHTTPError500("failed to load " + resource)
}

// writeStoreLookupError is storeLookupError for handlers that write their own
// response.
func writeStoreLookupError(rw http.ResponseWriter, err error, resource string) {
	httpErr := storeLookupError(err, resource)
	http.Error(rw, httpErr.Message, httpErr.StatusCode)
}

// isModelResolutionError reports whether err means the requested model could
// not be routed to any provider: no model named, a model the default provider
// does not serve, no sandbox serving it, or an upstream "model not found".
// These are configuration problems the caller can fix, not server faults.
func isModelResolutionError(err error) bool {
	if errors.Is(err, inferencerouter.ErrNoRunner) ||
		errors.Is(err, oai.ErrMissingModel) ||
		errors.Is(err, controller.ErrModelNotConfigured) {
		return true
	}
	var apiErr *openai.APIError
	return errors.As(err, &apiErr) && apiErr.HTTPStatusCode == http.StatusNotFound
}

// withFilestoreErrors maps the errors a filestore handler returns to client
// answers that do not describe the server's storage: a missing file is a 404
// and a refused path a 400, both with a fixed message; any other storage
// failure is a generic 500, logged here. Errors the handler already shaped
// (*system.HTTPError) pass through.
func withFilestoreErrors[T any](handler func(http.ResponseWriter, *http.Request) (T, error)) func(http.ResponseWriter, *http.Request) (T, error) {
	return func(w http.ResponseWriter, r *http.Request) (T, error) {
		result, err := handler(w, r)
		return result, filestoreHTTPError(err)
	}
}

func filestoreHTTPError(err error) error {
	if err == nil {
		return nil
	}
	var httpErr *system.HTTPError
	switch {
	case errors.As(err, &httpErr):
		return err
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, storage.ErrObjectNotExist):
		return system.NewHTTPError404("file not found")
	case errors.Is(err, filestore.ErrInvalidPath):
		return system.NewHTTPError400("invalid path")
	default:
		log.Error().Err(err).Msg("filestore operation failed")
		return system.NewHTTPError500("filestore operation failed")
	}
}
