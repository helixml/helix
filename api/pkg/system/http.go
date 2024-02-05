package system

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdlog "log"
	"net/http"
	"net/url"
	"strings"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog/log"
)

// the sub path any API's are served over
const API_SUB_PATH = "/api/v1"

type ClientOptions struct {
	Host  string
	Token string
}

func URL(options ClientOptions, path string) string {
	return fmt.Sprintf("%s%s", options.Host, path)
}

func GetApiPath(path string) string {
	return fmt.Sprintf("%s%s", API_SUB_PATH, path)
}

func WSURL(options ClientOptions, path string) string {
	url := URL(options, path)
	// replace http:// with ws://
	// and https:// with wss://
	if strings.HasPrefix(url, "http://") {
		return "ws" + url[4:]
	} else {
		return "wss" + url[5:]
	}
}

type HTTPError struct {
	StatusCode int
	Message    string
	Req        *http.Request
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

func NewHTTPError400(tmpl string, format ...interface{}) *HTTPError {
	return &HTTPError{
		StatusCode: http.StatusBadRequest,
		Message:    fmt.Sprintf(tmpl, format...),
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

func NewHTTPError500(tmpl string, format ...interface{}) *HTTPError {
	return &HTTPError{
		StatusCode: http.StatusInternalServerError,
		Message:    fmt.Sprintf(tmpl, format...),
	}
}

type httpErrorHandler func(err *HTTPError, req *http.Request)
type errorHandler func(err error, req *http.Request)

var HTTP_ERROR_HANDLER httpErrorHandler
var ERROR_HANDLER errorHandler

// functions that understand they need to return a http error
type httpWrapper[T any] func(res http.ResponseWriter, req *http.Request) (T, *HTTPError)

// normal functions that return just an error
// which will be tranalated into a 500
type defaultWrapper[T any] func(res http.ResponseWriter, req *http.Request) (T, error)

type WrapperConfig struct {
	SilenceErrors bool
}

func SetHTTPErrorHandler(handler httpErrorHandler) {
	HTTP_ERROR_HANDLER = handler
}

func SetErrorHandler(handler errorHandler) {
	ERROR_HANDLER = handler
}

// wrap a http handler with some error handling
// so if it returns an error we handle it
func Wrapper[T any](handler httpWrapper[T]) func(res http.ResponseWriter, req *http.Request) {
	return WrapperWithConfig[T](handler, WrapperConfig{})
}

func WrapperWithConfig[T any](handler httpWrapper[T], config WrapperConfig) func(res http.ResponseWriter, req *http.Request) {
	ret := func(res http.ResponseWriter, req *http.Request) {
		data, err := handler(res, req)
		if err != nil {
			if HTTP_ERROR_HANDLER != nil {
				HTTP_ERROR_HANDLER(err, req)
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
		} else {
			res.Header().Set("Content-Type", "application/json")
			jsonError := json.NewEncoder(res).Encode(data)
			if jsonError != nil {
				log.Ctx(req.Context()).Error().Msgf("error for json encoding: %s", err.Error())
				http.Error(res, jsonError.Error(), http.StatusInternalServerError)
				return
			}
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
	return DefaultWrapperWithConfig[T](handler, WrapperConfig{})
}

func DefaultWrapperWithConfig[T any](handler defaultWrapper[T], config WrapperConfig) func(res http.ResponseWriter, req *http.Request) {
	ret := func(res http.ResponseWriter, req *http.Request) {
		data, err := handler(res, req)
		if err != nil {
			if ERROR_HANDLER != nil {
				ERROR_HANDLER(err, req)
			}
			if !config.SilenceErrors {
				log.Error().Msgf("error for route: %s", err.Error())
			}
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		} else {
			res.Header().Set("Content-Type", "application/json")
			jsonError := json.NewEncoder(res).Encode(data)
			if jsonError != nil {
				log.Ctx(req.Context()).Error().Msgf("error for json encoding: %s", err.Error())
				http.Error(res, jsonError.Error(), http.StatusInternalServerError)
				return
			}
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

func GetRequest[ResultType any](
	options ClientOptions,
	path string,
	queryParams map[string]string,
) (ResultType, error) {
	var result ResultType
	buf, err := GetRequestBuffer(
		options,
		path,
		queryParams,
	)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(buf.Bytes(), &result)
	if err != nil {
		return result, err
	}

	return result, nil
}

func GetRequestBuffer(
	options ClientOptions,
	path string,
	queryParams map[string]string,
) (*bytes.Buffer, error) {
	urlValues := url.Values{}
	for key, value := range queryParams {
		urlValues.Add(key, value)
	}
	return GetRequestBufferWithQuery(options, path, urlValues)
}

func GetRequestBufferWithQuery(
	options ClientOptions,
	path string,
	queryParams url.Values,
) (*bytes.Buffer, error) {
	client := NewRetryClient()
	parsedURL, err := url.Parse(URL(options, path))
	if err != nil {
		return nil, err
	}
	parsedURL.RawQuery = queryParams.Encode()
	req, err := retryablehttp.NewRequest("GET", parsedURL.String(), nil)
	if err != nil {
		return nil, err
	}
	log.Trace().
		Str(req.Method, req.URL.String()).
		Msgf("")
	AddAuthHeadersRetryable(req, options.Token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("error response from server: %s %s", resp.Status, parsedURL.String())
	}

	var buf bytes.Buffer
	_, err = io.Copy(&buf, resp.Body)
	if err != nil {
		return nil, err
	}

	return &buf, nil
}

func PostRequest[RequestType any, ResultType any](
	options ClientOptions,
	path string,
	data RequestType,
) (ResultType, error) {
	var result ResultType
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return result, fmt.Errorf("error parsing JSON: %s", err.Error())
	}
	return PostRequestBuffer[ResultType](
		options,
		path,
		bytes.NewBuffer(dataBytes),
		"application/json",
	)
}

func PostRequestBuffer[ResultType any](
	options ClientOptions,
	path string,
	data *bytes.Buffer,
	contentType string,
) (ResultType, error) {
	var result ResultType
	client := NewRetryClient()
	req, err := retryablehttp.NewRequest("POST", URL(options, path), data)
	if err != nil {
		return result, err
	}
	log.Trace().
		Str(req.Method, req.URL.String()).
		Msgf("")
	req.Header.Add("Content-type", contentType)
	err = AddAuthHeadersRetryable(req, options.Token)
	if err != nil {
		return result, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, err
	}

	if resp.StatusCode >= 400 {
		return result, fmt.Errorf("error response from server: %s %s %s", resp.Status, URL(options, path), string(body))
	}

	// parse body as json into result
	err = json.Unmarshal(body, &result)
	if err != nil {
		return result, err
	}

	return result, nil
}

func NewRetryClient() *retryablehttp.Client {
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 5
	retryClient.Logger = stdlog.New(io.Discard, "", stdlog.LstdFlags)
	retryClient.RequestLogHook = func(_ retryablehttp.Logger, req *http.Request, attempt int) {
		switch {
		case req.Method == "POST":
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
	retryClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
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
