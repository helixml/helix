package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/websearch"
)

type WebSearchHandlersSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	provider *websearch.MockProvider
	server   *HelixAPIServer
	handler  http.Handler
}

func TestWebSearchHandlersSuite(t *testing.T) {
	suite.Run(t, new(WebSearchHandlersSuite))
}

func (s *WebSearchHandlersSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.provider = websearch.NewMockProvider(s.ctrl)
	s.server = &HelixAPIServer{webSearch: s.provider}

	router := mux.NewRouter()
	s.server.registerWebSearchRoutes(router.PathPrefix(APIPrefix).Subrouter())
	s.handler = cleanWebSearchBasePath(router)
}

func (s *WebSearchHandlersSuite) post(path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req = req.WithContext(setRequestUser(context.Background(), types.User{ID: "user_1", OrganizationID: "org_1"}))
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

func (s *WebSearchHandlersSuite) decodeError(rec *httptest.ResponseRecorder) websearch.ErrorResponse {
	var body websearch.ErrorResponse
	s.Require().NoError(json.Unmarshal(rec.Body.Bytes(), &body))
	return body
}

func (s *WebSearchHandlersSuite) TestSearchReturnsJSONWithRequestID() {
	s.provider.EXPECT().Search(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req *websearch.SearchRequest) (*websearch.SearchResponse, error) {
			s.Equal("golang", req.Query)
			s.Equal(3, req.NumResults)
			s.Equal(websearch.Caller{UserID: "user_1", OrgID: "org_1"}, websearch.CallerFrom(ctx))
			return &websearch.SearchResponse{RequestID: "upstream", Results: []websearch.Result{{URL: "https://go.dev/", Title: "Go"}}}, nil
		})

	rec := s.post("/api/v1/websearch/search", `{"query":"golang","numResults":3}`)

	s.Equal(http.StatusOK, rec.Code)
	requestID := rec.Header().Get("x-request-id")
	s.NotEmpty(requestID)
	var resp websearch.SearchResponse
	s.Require().NoError(json.Unmarshal(rec.Body.Bytes(), &resp))
	s.Equal(requestID, resp.RequestID)
	s.Equal("https://go.dev/", resp.Results[0].URL)
}

func (s *WebSearchHandlersSuite) TestTrailingSlashBaseURL() {
	s.provider.EXPECT().Contents(gomock.Any(), gomock.Any()).Return(&websearch.ContentsResponse{}, nil)

	rec := s.post("/api/v1/websearch//contents", `{"urls":["https://go.dev/"]}`)

	s.Equal(http.StatusOK, rec.Code)
}

func (s *WebSearchHandlersSuite) TestProviderErrorUsesEnvelope() {
	s.provider.EXPECT().FindSimilar(gomock.Any(), gomock.Any()).
		Return(nil, websearch.FeatureDisabled("excludeSourceDomain", "not available"))

	rec := s.post("/api/v1/websearch/findSimilar", `{"url":"https://go.dev/"}`)

	s.Equal(http.StatusBadRequest, rec.Code)
	body := s.decodeError(rec)
	s.Equal(websearch.TagFeatureDisabled, body.Tag)
	s.Equal(rec.Header().Get("x-request-id"), body.RequestID)
	s.Contains(body.Error, "excludeSourceDomain")
}

func (s *WebSearchHandlersSuite) TestUnexpectedErrorIs500() {
	s.provider.EXPECT().Answer(gomock.Any(), gomock.Any()).Return(nil, errors.New("boom"))

	rec := s.post("/api/v1/websearch/answer", `{"query":"why"}`)

	s.Equal(http.StatusInternalServerError, rec.Code)
	s.Equal(websearch.TagInternalError, s.decodeError(rec).Tag)
}

func (s *WebSearchHandlersSuite) TestUnknownFieldIs400() {
	rec := s.post("/api/v1/websearch/search", `{"query":"golang","numResultz":3}`)

	s.Equal(http.StatusBadRequest, rec.Code)
	body := s.decodeError(rec)
	s.Equal(websearch.TagInvalidRequest, body.Tag)
	s.Contains(body.Error, "numResultz")
	s.NotEmpty(rec.Header().Get("x-request-id"))
}

func (s *WebSearchHandlersSuite) TestContractViolationIs400() {
	rec := s.post("/api/v1/websearch/search", `{"query":""}`)

	s.Equal(http.StatusBadRequest, rec.Code)
	s.Equal(websearch.TagInvalidRequest, s.decodeError(rec).Tag)
}

func (s *WebSearchHandlersSuite) TestStreamSearchSSE() {
	s.provider.EXPECT().StreamSearch(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ *websearch.SearchRequest, emit func(websearch.SearchEvent) error) error {
			s.Require().NoError(emit(websearch.SearchEvent{Type: websearch.SearchEventTextDelta, Delta: "Hi"}))
			return emit(websearch.SearchEvent{Type: websearch.SearchEventDone})
		})

	rec := s.post("/api/v1/websearch/search", `{"query":"q","stream":true,"outputSchema":{"type":"text"}}`)

	s.Equal(http.StatusOK, rec.Code)
	s.Equal("text/event-stream", rec.Header().Get("Content-Type"))
	s.NotEmpty(rec.Header().Get("x-request-id"))
	s.Equal(`data: {"type":"text-delta","delta":"Hi"}`+"\n\n"+
		`data: {"type":"done"}`+"\n\n"+
		"data: [DONE]\n\n", rec.Body.String())
}

func (s *WebSearchHandlersSuite) TestStreamSearchErrorAfterStart() {
	s.provider.EXPECT().StreamSearch(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ *websearch.SearchRequest, emit func(websearch.SearchEvent) error) error {
			s.Require().NoError(emit(websearch.SearchEvent{Type: websearch.SearchEventTextDelta, Delta: "Hi"}))
			return errors.New("llm went away")
		})

	rec := s.post("/api/v1/websearch/search", `{"query":"q","stream":true,"outputSchema":{"type":"text"}}`)

	s.Equal(http.StatusOK, rec.Code)
	s.Equal(`data: {"type":"text-delta","delta":"Hi"}`+"\n\n"+
		`data: {"type":"error","error":{"message":"llm went away"}}`+"\n\n"+
		"data: [DONE]\n\n", rec.Body.String())
}

func (s *WebSearchHandlersSuite) TestStreamErrorBeforeStartIsEnvelope() {
	s.provider.EXPECT().StreamSearch(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(websearch.FeatureDisabled("outputSchema", "no LLM configured"))

	rec := s.post("/api/v1/websearch/search", `{"query":"q","stream":true,"outputSchema":{"type":"text"}}`)

	s.Equal(http.StatusBadRequest, rec.Code)
	s.Equal("application/json", rec.Header().Get("Content-Type"))
	s.Equal(websearch.TagFeatureDisabled, s.decodeError(rec).Tag)
}

func (s *WebSearchHandlersSuite) TestStreamWithoutOutputSchemaIsJSON() {
	s.provider.EXPECT().Search(gomock.Any(), gomock.Any()).Return(&websearch.SearchResponse{}, nil)

	rec := s.post("/api/v1/websearch/search", `{"query":"q","stream":true}`)

	s.Equal(http.StatusOK, rec.Code)
	s.Equal("application/json", rec.Header().Get("Content-Type"))
}

func (s *WebSearchHandlersSuite) TestStreamAnswerSSE() {
	s.provider.EXPECT().StreamAnswer(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ *websearch.AnswerRequest, emit func(websearch.AnswerEvent) error) error {
			return emit(websearch.AnswerEvent{Choices: []websearch.AnswerChoice{{Delta: websearch.AnswerDelta{Content: "42"}}}})
		})

	rec := s.post("/api/v1/websearch/answer", `{"query":"q","stream":true}`)

	requestID := rec.Header().Get("x-request-id")
	s.Equal(http.StatusOK, rec.Code)
	s.Equal("text/event-stream", rec.Header().Get("Content-Type"))
	s.Equal(`data: {"choices":[{"index":0,"delta":{"content":"42"},"finish_reason":null}],"requestId":"`+requestID+`"}`+"\n\n"+
		"data: [DONE]\n\n", rec.Body.String())
}

func (s *WebSearchHandlersSuite) TestStreamAnswerErrorAfterStart() {
	s.provider.EXPECT().StreamAnswer(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ *websearch.AnswerRequest, emit func(websearch.AnswerEvent) error) error {
			s.Require().NoError(emit(websearch.AnswerEvent{Choices: []websearch.AnswerChoice{{Delta: websearch.AnswerDelta{Content: "4"}}}}))
			return &websearch.Error{Status: http.StatusServiceUnavailable, Tag: websearch.TagUpstreamError, Message: "searxng down"}
		})

	rec := s.post("/api/v1/websearch/answer", `{"query":"q","stream":true}`)

	requestID := rec.Header().Get("x-request-id")
	events := strings.Split(strings.TrimSuffix(rec.Body.String(), "\n\n"), "\n\n")
	s.Require().Len(events, 3)
	s.Equal(`data: {"tag":"ERROR","payload":{"error":{"code":503,"message":"searxng down"}},"requestId":"`+requestID+`"}`, events[1])
	s.Equal("data: [DONE]", events[2])
}
