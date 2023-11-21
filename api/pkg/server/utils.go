package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	stdlog "log"
	"net/http"
	"net/url"
	"strings"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// the sub path any API's are served over
const API_SUB_PATH = "/api/v1"

type ClientOptions struct {
	Host  string
	Token string
}

func URL(options ClientOptions, path string) string {
	return fmt.Sprintf("%s%s%s", options.Host, API_SUB_PATH, path)
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

func (apiServer *HelixAPIServer) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Access-Control-Allow-Origin", "*")
		next.ServeHTTP(res, req)
	})
}

func (apiServer *HelixAPIServer) getRequestContext(req *http.Request) types.RequestContext {
	return types.RequestContext{
		Ctx:       req.Context(),
		Owner:     getRequestUser(req),
		OwnerType: types.OwnerTypeUser,
	}
}

type httpWrapper[T any] func(res http.ResponseWriter, req *http.Request) (T, error)

type WrapperConfig struct {
	SilenceErrors bool
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
			if !config.SilenceErrors {
				log.Error().Msgf("error for route: %s", err.Error())
			}
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		} else {
			err = json.NewEncoder(res).Encode(data)
			if err != nil {
				log.Ctx(req.Context()).Error().Msgf("error for json encoding: %s", err.Error())
				http.Error(res, err.Error(), http.StatusInternalServerError)
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
	client := newRetryClient()
	parsedURL, err := url.Parse(URL(options, path))
	if err != nil {
		return nil, err
	}
	parsedURL.RawQuery = queryParams.Encode()
	req, err := retryablehttp.NewRequest("GET", parsedURL.String(), nil)
	if err != nil {
		return nil, err
	}
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
		return result, fmt.Errorf("THIS IS A JSON ERROR: %s", err.Error())
	}
	return PostRequestBuffer[ResultType](
		options,
		path,
		bytes.NewBuffer(dataBytes),
	)
}

func PostRequestBuffer[ResultType any](
	options ClientOptions,
	path string,
	data *bytes.Buffer,
) (ResultType, error) {
	var result ResultType
	client := newRetryClient()
	req, err := retryablehttp.NewRequest("POST", URL(options, path), data)
	if err != nil {
		return result, err
	}
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

	// parse body as json into result
	err = json.Unmarshal(body, &result)
	if err != nil {
		return result, err
	}

	return result, nil
}

func newRetryClient() *retryablehttp.Client {
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 10
	retryClient.Logger = stdlog.New(io.Discard, "", stdlog.LstdFlags)
	retryClient.RequestLogHook = func(_ retryablehttp.Logger, req *http.Request, attempt int) {
		switch {
		case req.Method == "POST":
			log.Debug().
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
	return retryClient
}
