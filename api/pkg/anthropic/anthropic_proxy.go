package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"github.com/helixml/helix/api/pkg/openai/logger"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	anthropic "github.com/anthropics/anthropic-sdk-go" // imported as anthropic
	"github.com/rs/zerolog/log"
)

type providerEndpointContextKeyType string

var providerEndpointContextKey providerEndpointContextKeyType = "provider_endpoint"

func SetRequestProviderEndpoint(req *http.Request, endpoint *types.ProviderEndpoint) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), providerEndpointContextKey, endpoint))
}

func GetRequestProviderEndpoint(req *http.Request) *types.ProviderEndpoint {
	endpointIntf := req.Context().Value(providerEndpointContextKey)
	if endpointIntf == nil {
		return nil
	}
	return endpointIntf.(*types.ProviderEndpoint)
}

type Proxy struct {
	store                 store.Store
	anthropicReverseProxy *httputil.ReverseProxy
	logStores             []logger.LogStore
	wg                    sync.WaitGroup
}

func New(store store.Store, logStores ...logger.LogStore) *Proxy {
	p := &Proxy{
		store:                 store,
		anthropicReverseProxy: httputil.NewSingleHostReverseProxy(nil),
		logStores:             logStores,
		wg:                    sync.WaitGroup{},
	}

	p.anthropicReverseProxy.ModifyResponse = p.anthropicAPIProxyModifyResponse
	p.anthropicReverseProxy.Director = p.anthropicAPIProxyDirector

	return p
}

func (s *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.anthropicReverseProxy.ServeHTTP(w, r)
}

func (s *Proxy) anthropicAPIProxyDirector(r *http.Request) {
	endpoint := GetRequestProviderEndpoint(r)
	if endpoint == nil {
		log.Error().Msg("provider endpoint not found in context")
		return
	}

	u, err := url.Parse(endpoint.BaseURL)
	if err != nil {
		log.Error().Err(err).Msg("failed to parse provider endpoint URL")
		return
	}
	r.URL.Host = u.Host
	r.URL.Scheme = u.Scheme

	r.Host = u.Host

	for key, value := range endpoint.Headers {
		r.Header.Set(key, value)
	}

	// Remove authorization header
	r.Header.Del("Authorization")
}

// anthropicAPIProxyModifyResponse - parses the response
func (s *Proxy) anthropicAPIProxyModifyResponse(response *http.Response) error {

	// If content type is "text/event-stream", then process the stream
	if strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		pr, pw := io.Pipe()
		body := response.Body
		response.Body = pr

		// TODO: assemble the response body into a buffer
		go func() {
			defer pw.Close()
			reader := bufio.NewReader(body)
			for {
				line, err := reader.ReadBytes('\n')
				if err != nil {
					return
				}

				// TODO: read chunks and look for the usage
				fmt.Println(string(line))
				_, _ = pw.Write(line)
			}
		}()
		return nil
	}

	buf, err := io.ReadAll(response.Body)
	if err != nil {
		log.Error().Err(err).Msg("failed to read response body")
		return err
	}

	// Put the response buffer back as the response body
	response.Body = io.NopCloser(bytes.NewBuffer(buf))

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		// Parse usage from the usage buffer
		var resp anthropic.Message
		if err := resp.UnmarshalJSON(buf); err != nil {
			log.Error().Err(err).Msg("failed to parse anthropic response for usage")
		} else {
			// TODO: Extract and log usage information from resp
			log.Debug().Interface("usage", resp.Usage).Msg("anthropic usage information")
		}
		// Log LLM call
	}()

	return nil
}
