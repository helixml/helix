package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/websearch"
)

// webSearchPrefix is the Exa-compatible API base URL path. Exa SDKs build
// URLs as base_url + "/search".
const webSearchPrefix = APIPrefix + "/websearch"

const maxWebSearchRequestBody = MEGABYTE

func (s *HelixAPIServer) registerWebSearchRoutes(authRouter *mux.Router) {
	authRouter.HandleFunc("/websearch/search", s.webSearchSearch).Methods(http.MethodPost)
	authRouter.HandleFunc("/websearch/contents", s.webSearchContents).Methods(http.MethodPost)
	authRouter.HandleFunc("/websearch/findSimilar", s.webSearchFindSimilar).Methods(http.MethodPost)
	authRouter.HandleFunc("/websearch/answer", s.webSearchAnswer).Methods(http.MethodPost)
}

// cleanWebSearchBasePath collapses the "//" an Exa SDK produces when its
// base_url ends in "/" (".../websearch/" + "/search"). Without it the router
// answers 301 to the cleaned path, which clients follow as a GET.
func cleanWebSearchBasePath(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rest, ok := strings.CutPrefix(r.URL.Path, webSearchPrefix+"//"); ok {
			r.URL.Path = webSearchPrefix + "/" + strings.TrimLeft(rest, "/")
			r.URL.RawPath = ""
		}
		next.ServeHTTP(w, r)
	})
}

// webSearchSearch godoc
// @Summary Search the web
// @Description Exa-compatible search. With stream=true and an outputSchema the response is a text/event-stream of websearch.SearchEvent ending with "data: [DONE]".
// @Tags    websearch
// @Accept  json
// @Produce json
// @Param   request body websearch.SearchRequest true "Search request"
// @Success 200 {object} websearch.SearchResponse
// @Failure 400 {object} websearch.ErrorResponse
// @Failure 500 {object} websearch.ErrorResponse
// @Router /api/v1/websearch/search [post]
// @Security BearerAuth
// @externalDocs.url https://docs.exa.ai/reference/search
func (s *HelixAPIServer) webSearchSearch(rw http.ResponseWriter, r *http.Request) {
	call, req, ok := startWebSearchCall[websearch.SearchRequest](rw, r)
	if !ok {
		return
	}
	if req.Stream && len(req.OutputSchema) > 0 {
		stream := newSSEStream(rw)
		err := s.webSearch.StreamSearch(call.ctx, req, func(ev websearch.SearchEvent) error {
			return stream.send(ev)
		})
		stream.finish(call, err, func(e *websearch.Error) any {
			return websearch.SearchEvent{Type: websearch.SearchEventError, Error: &websearch.EventError{Message: e.Message}}
		})
		return
	}
	resp, err := s.webSearch.Search(call.ctx, req)
	if err != nil {
		call.fail(err)
		return
	}
	resp.RequestID = call.requestID
	writeResponse(rw, resp, http.StatusOK)
}

// webSearchContents godoc
// @Summary Get page contents
// @Description Exa-compatible contents: text, highlights, summaries and extras for URLs or result ids.
// @Tags    websearch
// @Accept  json
// @Produce json
// @Param   request body websearch.ContentsRequest true "Contents request"
// @Success 200 {object} websearch.ContentsResponse
// @Failure 400 {object} websearch.ErrorResponse
// @Failure 500 {object} websearch.ErrorResponse
// @Router /api/v1/websearch/contents [post]
// @Security BearerAuth
// @externalDocs.url https://docs.exa.ai/reference/get-contents
func (s *HelixAPIServer) webSearchContents(rw http.ResponseWriter, r *http.Request) {
	call, req, ok := startWebSearchCall[websearch.ContentsRequest](rw, r)
	if !ok {
		return
	}
	resp, err := s.webSearch.Contents(call.ctx, req)
	if err != nil {
		call.fail(err)
		return
	}
	resp.RequestID = call.requestID
	writeResponse(rw, resp, http.StatusOK)
}

// webSearchFindSimilar godoc
// @Summary Find pages similar to a URL
// @Description Exa-compatible findSimilar.
// @Tags    websearch
// @Accept  json
// @Produce json
// @Param   request body websearch.FindSimilarRequest true "Find similar request"
// @Success 200 {object} websearch.FindSimilarResponse
// @Failure 400 {object} websearch.ErrorResponse
// @Failure 500 {object} websearch.ErrorResponse
// @Router /api/v1/websearch/findSimilar [post]
// @Security BearerAuth
// @externalDocs.url https://docs.exa.ai/reference/find-similar-links
func (s *HelixAPIServer) webSearchFindSimilar(rw http.ResponseWriter, r *http.Request) {
	call, req, ok := startWebSearchCall[websearch.FindSimilarRequest](rw, r)
	if !ok {
		return
	}
	resp, err := s.webSearch.FindSimilar(call.ctx, req)
	if err != nil {
		call.fail(err)
		return
	}
	resp.RequestID = call.requestID
	writeResponse(rw, resp, http.StatusOK)
}

// webSearchAnswer godoc
// @Summary Answer a question from web search results
// @Description Exa-compatible answer. With stream=true the response is a text/event-stream of websearch.AnswerEvent ending with "data: [DONE]".
// @Tags    websearch
// @Accept  json
// @Produce json
// @Param   request body websearch.AnswerRequest true "Answer request"
// @Success 200 {object} websearch.AnswerResponse
// @Failure 400 {object} websearch.ErrorResponse
// @Failure 500 {object} websearch.ErrorResponse
// @Router /api/v1/websearch/answer [post]
// @Security BearerAuth
// @externalDocs.url https://docs.exa.ai/reference/answer
func (s *HelixAPIServer) webSearchAnswer(rw http.ResponseWriter, r *http.Request) {
	call, req, ok := startWebSearchCall[websearch.AnswerRequest](rw, r)
	if !ok {
		return
	}
	if req.Stream {
		stream := newSSEStream(rw)
		err := s.webSearch.StreamAnswer(call.ctx, req, func(ev websearch.AnswerEvent) error {
			ev.RequestID = call.requestID
			return stream.send(ev)
		})
		stream.finish(call, err, func(e *websearch.Error) any {
			return websearch.AnswerErrorEvent{
				Tag:       websearch.AnswerErrorTag,
				Payload:   websearch.AnswerErrorPayload{Error: websearch.AnswerErrorDetail{Code: e.Status, Message: e.Message}},
				RequestID: call.requestID,
			}
		})
		return
	}
	resp, err := s.webSearch.Answer(call.ctx, req)
	if err != nil {
		call.fail(err)
		return
	}
	resp.RequestID = call.requestID
	writeResponse(rw, resp, http.StatusOK)
}

// webSearchCall is one request to the web search API. Its requestId is in
// the x-request-id header and in every response body.
type webSearchCall struct {
	ctx       context.Context
	requestID string
	rw        http.ResponseWriter
}

// webSearchRequest is a request body that checks itself against Exa's
// contract.
type webSearchRequest[T any] interface {
	*T
	Validate() error
}

// startWebSearchCall assigns the request id, attributes the request to the
// caller, and decodes and validates the body. An unknown field is a 400,
// never silently dropped.
func startWebSearchCall[T any, P webSearchRequest[T]](rw http.ResponseWriter, r *http.Request) (*webSearchCall, P, bool) {
	call := &webSearchCall{requestID: system.GenerateRequestID(), rw: rw}
	rw.Header().Set("x-request-id", call.requestID)

	user := getRequestUser(r)
	call.ctx = websearch.WithCaller(r.Context(), websearch.Caller{UserID: user.ID, OrgID: user.OrganizationID})

	dec := json.NewDecoder(http.MaxBytesReader(rw, r.Body, maxWebSearchRequestBody))
	dec.DisallowUnknownFields()
	req := P(new(T))
	if err := dec.Decode(req); err != nil {
		call.fail(websearch.InvalidRequest("invalid request body: %s", err))
		return nil, nil, false
	}
	if dec.More() {
		call.fail(websearch.InvalidRequest("invalid request body: unexpected data after the JSON object"))
		return nil, nil, false
	}
	if err := req.Validate(); err != nil {
		call.fail(err)
		return nil, nil, false
	}
	return call, req, true
}

func (c *webSearchCall) fail(err error) {
	e := websearch.AsError(err)
	if e.Status >= http.StatusInternalServerError {
		log.Error().Err(err).Str("request_id", c.requestID).Msg("web search request failed")
	}
	writeResponse(c.rw, websearch.ErrorResponse{RequestID: c.requestID, Error: e.Message, Tag: e.Tag}, e.Status)
}

// sseStream writes server-sent events. Headers are sent with the first
// event, so a request that fails before producing anything still gets a
// normal status code and error envelope.
type sseStream struct {
	rw      http.ResponseWriter
	started bool
}

func newSSEStream(rw http.ResponseWriter) *sseStream {
	return &sseStream{rw: rw}
}

func (s *sseStream) send(event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	return s.write(data)
}

func (s *sseStream) write(data []byte) error {
	if !s.started {
		s.rw.Header().Set("Content-Type", "text/event-stream")
		s.rw.Header().Set("Cache-Control", "no-cache")
		s.rw.Header().Set("Connection", "keep-alive")
		s.rw.WriteHeader(http.StatusOK)
		s.started = true
	}
	if _, err := fmt.Fprintf(s.rw, "data: %s\n\n", data); err != nil {
		return err
	}
	if f, ok := s.rw.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// finish ends the stream with [DONE]. A failure before the first event is a
// normal error response; after it, an in-band error event built by
// errorEvent.
func (s *sseStream) finish(call *webSearchCall, err error, errorEvent func(*websearch.Error) any) {
	if err != nil && !s.started {
		call.fail(err)
		return
	}
	if err != nil {
		log.Error().Err(err).Str("request_id", call.requestID).Msg("web search stream failed")
		if sendErr := s.send(errorEvent(websearch.AsError(err))); sendErr != nil {
			return
		}
	}
	if writeErr := s.write([]byte("[DONE]")); writeErr != nil {
		log.Debug().Err(writeErr).Str("request_id", call.requestID).Msg("web search stream closed by client")
	}
}
