package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type providerEndpointContextKeyType string

var providerEndpointContextKey providerEndpointContextKeyType = "provider_endpoint"

func setRequestProviderEndpoint(req *http.Request, endpoint *types.ProviderEndpoint) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), providerEndpointContextKey, endpoint))
}

func getRequestProviderEndpoint(req *http.Request) *types.ProviderEndpoint {
	endpointIntf := req.Context().Value(providerEndpointContextKey)
	if endpointIntf == nil {
		return nil
	}
	return endpointIntf.(*types.ProviderEndpoint)
}

func (s *HelixAPIServer) anthropicAPIProxyHandler(w http.ResponseWriter, r *http.Request) {
	endpoint, err := s.getProviderEndpoint(r)
	if err != nil {
		log.Error().Err(err).Msg("failed to get provider endpoint")

		http.Error(w, "Failed to get provider endpoint: "+err.Error(), http.StatusInternalServerError)
		return
	}
	r = setRequestProviderEndpoint(r, endpoint)

	s.anthropicReverseProxy.ServeHTTP(w, r)
}

func (s *HelixAPIServer) anthropicAPIProxyDirector(r *http.Request) {
	endpoint := getRequestProviderEndpoint(r)
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

func (s *HelixAPIServer) getProviderEndpoint(r *http.Request) (*types.ProviderEndpoint, error) {
	user := getRequestUser(r)
	if user == nil {
		return nil, fmt.Errorf("user is required")
	}

	provider := r.Header.Get("X-Provider")
	orgID := r.Header.Get("X-Org-ID")

	if provider == "" {
		return nil, fmt.Errorf("provider ID is required")
	}

	if orgID != "" {
		_, err := s.authorizeOrgMember(r.Context(), user, orgID)
		if err != nil {
			return nil, fmt.Errorf("error authorizing org member: %w", err)
		}

		q := &store.GetProviderEndpointsQuery{
			Owner: orgID,
		}

		if strings.HasPrefix(provider, system.ProviderEndpointPrefix) {
			q.ID = provider
		} else {
			q.Name = provider
		}

		return s.Store.GetProviderEndpoint(r.Context(), q)
	}

	q := &store.GetProviderEndpointsQuery{
		Owner: orgID,
	}

	if strings.HasPrefix(provider, system.ProviderEndpointPrefix) {
		q.ID = provider
	} else {
		q.Name = provider
	}

	return s.Store.GetProviderEndpoint(r.Context(), q)
}

// anthropicAPIProxyModifyResponse - parses the response
func (s *HelixAPIServer) anthropicAPIProxyModifyResponse(response *http.Response) error {

	// If content type is "text/event-stream", then process the stream
	if strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		pr, pw := io.Pipe()
		body := response.Body
		response.Body = pr
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
				pw.Write(line)
			}
		}()
		return nil
	}

	return nil
}
