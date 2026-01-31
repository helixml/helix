package system

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	stdlog "log"
	"net/http"
	"strings"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog/log"
)

// the sub path any API's are served over
const APISubPath = "/api/v1"

type ClientOptions struct {
	Host  string
	Token string
}

func URL(options ClientOptions, path string) string {
	return fmt.Sprintf("%s%s", options.Host, path)
}

func GetAPIPath(path string) string {
	return fmt.Sprintf("%s%s", APISubPath, path)
}

func WSURL(options ClientOptions, path string) string {
	url := URL(options, path)
	// replace http:// with ws://
	// and https:// with wss://
	if strings.HasPrefix(url, "http://") {
		return "ws" + url[4:]
	}
	return "wss" + url[5:]
}

type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return e.Message
}

func NewHTTPError(err error) *HTTPError {
	return &HTTPError{
		StatusCode: http.StatusInternalServerError,
		Message:    err.Error(),
	}
}

func NewHTTPError400(message string) *HTTPError {
	return &HTTPError{
		StatusCode: http.StatusBadRequest,
		Message:    message,
	}
}

func NewHTTPError401(message string) *HTTPError {
	return &HTTPError{
		StatusCode: http.StatusUnauthorized,
		Message:    message,
	}
}

func NewHTTPError403(message string) *HTTPError {
	return &HTTPError{
		StatusCode: http.StatusForbidden,
		Message:    message,
	}
}

func NewHTTPError404(message string) *HTTPError {
	return &HTTPError{
		StatusCode: http.StatusNotFound,
		Message:    message,
	}
}

func NewHTTPError500(message string) *HTTPError {
	return &HTTPError{
		StatusCode: http.StatusInternalServerError,
		Message:    message,
	}
}

type httpErrorHandler func(err *HTTPError, req *http.Request)
type errorHandler func(err error, req *http.Request)

var HTTPErrorHandler httpErrorHandler
var ErrorHandler errorHandler

// functions that understand they need to return a http error
type httpWrapper[T any] func(res http.ResponseWriter, req *http.Request) (T, *HTTPError)

// normal functions that return just an error
// which will be tranalated into a 500
type defaultWrapper[T any] func(res http.ResponseWriter, req *http.Request) (T, error)

type WrapperConfig struct {
	SilenceErrors bool
}

func SetHTTPErrorHandler(handler httpErrorHandler) {
	HTTPErrorHandler = handler
}

func SetErrorHandler(handler errorHandler) {
	ErrorHandler = handler
}

// wrap a http handler with some error handling
// so if it returns an error we handle it
func Wrapper[T any](handler httpWrapper[T]) func(res http.ResponseWriter, req *http.Request) {
	return WrapperWithConfig(handler, WrapperConfig{})
}

func WrapperWithConfig[T any](handler httpWrapper[T], config WrapperConfig) func(res http.ResponseWriter, req *http.Request) {
	ret := func(res http.ResponseWriter, req *http.Request) {
		data, err := handler(res, req)
		if err != nil {
			if HTTPErrorHandler != nil {
				HTTPErrorHandler(err, req)
			}
			if !config.SilenceErrors {
				log.Error().Msgf("error for route: %s", err.Error())
			}
			statusCode := err.StatusCode
			if statusCode == 0 {
				statusCode = http.StatusInternalServerError
			}
			http.Error(res, err.Error(), statusCode)
			return
		}
		res.Header().Set("Content-Type", "application/json")
		jsonError := json.NewEncoder(res).Encode(data)
		if jsonError != nil {
			log.Ctx(req.Context()).Error().Msgf("error for json encoding: %s", err.Error())
			http.Error(res, jsonError.Error(), http.StatusInternalServerError)
			return
		}
	}
	return ret
}

// this is a wrapper for any function that just returns some data and a normal error
// it is used because we want control over http status codes but lots of controller
// functions just return a normal error - so, if we get one of these we just do a 500
// it's up to the server handlers to decide they want to care about the http status code
// and if they do then they should be handling the error themselves
// this method is used for controllers that just return a result and an error
func DefaultController[T any](result T, err error) (T, *HTTPError) {
	if err != nil {
		return result, NewHTTPError500(err.Error())
	}
	return result, nil
}

func DefaultWrapper[T any](handler defaultWrapper[T]) func(res http.ResponseWriter, req *http.Request) {
	return DefaultWrapperWithConfig(handler, WrapperConfig{})
}

func DefaultWrapperWithConfig[T any](handler defaultWrapper[T], config WrapperConfig) func(res http.ResponseWriter, req *http.Request) {
	ret := func(res http.ResponseWriter, req *http.Request) {
		data, err := handler(res, req)
		if err != nil {
			if ErrorHandler != nil {
				ErrorHandler(err, req)
			}
			if !config.SilenceErrors {
				log.Error().Msgf("error for route: %s", err.Error())
			}
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
		res.Header().Set("Content-Type", "application/json")
		jsonError := json.NewEncoder(res).Encode(data)
		if jsonError != nil {
			log.Ctx(req.Context()).Error().Msgf("error for json encoding: %s", jsonError.Error())
			http.Error(res, jsonError.Error(), http.StatusInternalServerError)
			return
		}
	}
	return ret
}

func AddAuthHeadersRetryable(
	req *retryablehttp.Request,
	token string,
) error {
	if token != "" {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	}
	return nil
}

func AddAutheaders(
	req *http.Request,
	token string,
) error {
	if token != "" {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	}
	return nil
}

func NewRetryClient(retryMax int, tlsSkipVerify bool) *retryablehttp.Client {
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = retryMax

	if tlsSkipVerify {
		retryClient.HTTPClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	retryClient.Logger = stdlog.New(io.Discard, "", stdlog.LstdFlags)
	retryClient.RequestLogHook = func(_ retryablehttp.Logger, req *http.Request, attempt int) {
		switch {
		case req.Method == http.MethodPost:
			log.Trace().
				Str(req.Method, req.URL.String()).
				Int("attempt", attempt).
				Msgf("")
		default:
			// GET, PUT, DELETE, etc.
			log.Trace().
				Str(req.Method, req.URL.String()).
				Int("attempt", attempt).
				Msgf("")
		}
	}
	retryClient.CheckRetry = func(_ context.Context, resp *http.Response, err error) (bool, error) {
		if resp == nil {
			return true, err
		}
		log.Trace().
			Str(resp.Request.Method, resp.Request.URL.String()).
			Int("code", resp.StatusCode).
			Msgf("")
		// don't retry for auth errors
		return resp.StatusCode >= 500, nil
	}
	return retryClient
}
