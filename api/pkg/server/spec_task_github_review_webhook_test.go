package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

const testWebhookSecret = "shh"

func signBody(t *testing.T, secret string, body []byte) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// newReviewPayload returns a pull_request_review event body for review 77 on
// owner/repo#42 with a body and reviewer.
func newReviewPayload(t *testing.T) []byte {
	t.Helper()
	payload := githubReviewWebhookPayload{
		Action: "submitted",
		Review: githubWebhookReview{
			ID:      77,
			State:   "changes_requested",
			User:    githubWebhookUser{Login: "alice", Type: "User"},
			Body:    "please fix the nil check",
			HTMLURL: "https://github.com/owner/repo/pull/42#pullrequestreview-77",
		},
		PullRequest: githubWebhookPullRequest{
			Number:  42,
			Title:   "add nil check",
			HTMLURL: "https://github.com/owner/repo/pull/42",
		},
	}
	payload.Repository.FullName = "owner/repo"
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestVerifyGitHubSignature(t *testing.T) {
	body := []byte(`{"action":"submitted"}`)

	sig := signBody(t, testWebhookSecret, body)
	if !verifyGitHubSignature(testWebhookSecret, body, sig) {
		t.Fatal("valid signature must verify")
	}
	if verifyGitHubSignature(testWebhookSecret, []byte(`{"action":"edited"}`), sig) {
		t.Fatal("tampered body must fail")
	}
	if verifyGitHubSignature(testWebhookSecret, body, "sha256=deadbeef") {
		t.Fatal("wrong digest must fail")
	}
	if verifyGitHubSignature(testWebhookSecret, body, "") {
		t.Fatal("missing header must fail")
	}
	if verifyGitHubSignature("", body, sig) {
		t.Fatal("missing secret must fail")
	}
	if verifyGitHubSignature(testWebhookSecret, body, "sha256=not-hex") {
		t.Fatal("malformed header must fail")
	}
}

func TestFormatGitHubReviewFeedback(t *testing.T) {
	var payload githubReviewWebhookPayload
	if err := json.Unmarshal(newReviewPayload(t), &payload); err != nil {
		t.Fatal(err)
	}

	comments := []*types.PRReviewComment{
		{ReviewID: 77, Author: "alice", Body: "this can nil-deref", Path: "api/pkg/foo.go", Line: 42, URL: "https://github.com/owner/repo/pull/42#discussion_r1"},
	}

	msg := formatGitHubReviewFeedback(&payload, comments)
	for _, want := range []string{
		`owner/repo#42 "add nil check"`,
		"@alice",
		"CHANGES REQUESTED",
		"please fix the nil check",
		"api/pkg/foo.go:42 @alice: this can nil-deref",
		"https://github.com/owner/repo/pull/42#pullrequestreview-77",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message missing %q:\n%s", want, msg)
		}
	}

	payload.Review.Body = ""
	bare := formatGitHubReviewFeedback(&payload, nil)
	if !strings.Contains(bare, "verdict: CHANGES REQUESTED") {
		t.Fatalf("bodyless review must still name the verdict:\n%s", bare)
	}
	if strings.Contains(bare, "Inline comments") {
		t.Fatalf("no comments must mean no comment section:\n%s", bare)
	}
}

// SpecTaskGitHubReviewWebhookSuite tests the full webhook HTTP path:
// signature validation → PR correlation → comment fetch → agent enqueue.
type SpecTaskGitHubReviewWebhookSuite struct {
	suite.Suite
	ctrl    *gomock.Controller
	store   *store.MockStore
	gitRepo *fakeGitRepoService
	server  *HelixAPIServer
}

func TestSpecTaskGitHubReviewWebhookSuite(t *testing.T) {
	suite.Run(t, new(SpecTaskGitHubReviewWebhookSuite))
}

func (s *SpecTaskGitHubReviewWebhookSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.gitRepo = &fakeGitRepoService{}
	s.server = &HelixAPIServer{
		Cfg: &config.ServerConfig{
			WebServer: config.WebServer{URL: "http://localhost:0", Host: "localhost", Port: 0, RunnerToken: "test"},
			GitHub:    config.GitHub{WebhookSecret: testWebhookSecret},
		},
		Store:                s.store,
		gitRepositoryService: s.gitRepo,
	}
}

func (s *SpecTaskGitHubReviewWebhookSuite) TearDownTest() {
	s.ctrl.Finish()
}

// post delivers a signed (or unsigned when signature is empty) request to the
// webhook endpoint.
func (s *SpecTaskGitHubReviewWebhookSuite) post(body []byte, event, signature string) *httptest.ResponseRecorder {
	return s.postWithOrg("", body, event, signature)
}

// postWithOrg delivers a request to the org-scoped route (mux vars set), or
// the unscoped route when org is empty.
func (s *SpecTaskGitHubReviewWebhookSuite) postWithOrg(org string, body []byte, event, signature string) *httptest.ResponseRecorder {
	path := "/api/v1/webhooks/github/reviews"
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	if org != "" {
		path += "/" + org
		req = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		req = mux.SetURLVars(req, map[string]string{"org": org})
	}
	req.Header.Set("X-GitHub-Event", event)
	req.Header.Set("Content-Type", "application/json")
	if signature != "" {
		req.Header.Set("X-Hub-Signature-256", signature)
	}
	rr := httptest.NewRecorder()
	s.server.specTaskGitHubReviewWebhook(rr, req)
	return rr
}

// trackingTask returns a task in the given status tracking owner/repo#42.
func (s *SpecTaskGitHubReviewWebhookSuite) trackingTask(id, status types.SpecTaskStatus) *types.SpecTask {
	return &types.SpecTask{
		ID:                string(id),
		Status:            status,
		PlanningSessionID: "ses_" + string(id),
		RepoPullRequests: []types.RepoPR{
			{
				RepositoryID:   "repo-1",
				RepositoryName: "owner/repo",
				PRID:           "42",
				PRNumber:       42,
				PRState:        "open",
			},
		},
	}
}

func (s *SpecTaskGitHubReviewWebhookSuite) TestEndToEndEnqueuesNormalizedFeedback() {
	task := s.trackingTask("spt_1", types.TaskStatusPullRequest)

	// Correlation query with the repo+PR filter.
	var capturedFilter *types.SpecTaskPRMatch
	s.store.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, filters *types.SpecTaskFilters) ([]*types.SpecTask, error) {
			capturedFilter = filters.PRMatch
			return []*types.SpecTask{task}, nil
		})

	// Inline comments fetched through the task's tracked repo credentials.
	var gotRepoID, gotPRID string
	var gotReviewID int64
	s.gitRepo.listReviewCommentsFunc = func(_ context.Context, repoID, prID string, reviewID int64) ([]*types.PRReviewComment, error) {
		gotRepoID, gotPRID, gotReviewID = repoID, prID, reviewID
		return []*types.PRReviewComment{
			{ReviewID: 77, Author: "alice", Body: "this can nil-deref", Path: "api/pkg/foo.go", Line: 42},
		}, nil
	}

	// The enqueue: a pending interrupt row keyed on the task's planning session.
	var captured *types.PromptHistoryEntry
	s.store.EXPECT().GetSession(gomock.Any(), "ses_spt_1").
		Return(&types.Session{ID: "ses_spt_1", Owner: "user-1"}, nil).AnyTimes()
	s.store.EXPECT().CreatePromptHistoryEntry(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, e *types.PromptHistoryEntry) error {
			captured = e
			return nil
		})
	// The background nudge lists the session's prompts; return none so the
	// poller bails immediately and the test stays deterministic.
	s.store.EXPECT().ListPromptHistoryBySession(gomock.Any(), "ses_spt_1").Return(nil, nil).AnyTimes()

	body := newReviewPayload(s.T())
	rr := s.post(body, "pull_request_review", signBody(s.T(), testWebhookSecret, body))
	s.Require().Equal(http.StatusOK, rr.Code, "body=%s", rr.Body.String())

	s.Require().NotNil(capturedFilter)
	s.Equal("owner/repo", capturedFilter.RepositoryName)
	s.Equal(42, capturedFilter.PRNumber)
	s.Equal("repo-1", gotRepoID)
	s.Equal("42", gotPRID)
	s.Equal(int64(77), gotReviewID)

	s.Require().NotNil(captured)
	s.Equal("ses_spt_1", captured.SessionID)
	s.Equal("spt_1", captured.SpecTaskID)
	s.True(captured.Interrupt, "review feedback interrupts like CI results")
	s.Equal("pending", captured.Status)
	for _, want := range []string{"owner/repo#42", "@alice", "CHANGES REQUESTED", "api/pkg/foo.go:42 @alice: this can nil-deref"} {
		s.Require().Contains(captured.Content, want)
	}

	// Let the background nudge run its single (AnyTimes) list call and exit
	// before the controller is finished.
	time.Sleep(100 * time.Millisecond)
}

func (s *SpecTaskGitHubReviewWebhookSuite) TestSkipsTasksPastFeedbackWindow() {
	// One eligible task, one done task — only the eligible one is notified.
	eligible := s.trackingTask("spt_live", types.TaskStatusPullRequest)
	done := s.trackingTask("spt_done", types.TaskStatusDone)

	s.store.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).Return([]*types.SpecTask{eligible, done}, nil)
	s.gitRepo.listReviewCommentsFunc = func(_ context.Context, _, _ string, _ int64) ([]*types.PRReviewComment, error) {
		return nil, nil
	}

	var enqueued int
	s.store.EXPECT().GetSession(gomock.Any(), gomock.Any()).Return(&types.Session{ID: "ses_spt_live", Owner: "u"}, nil).AnyTimes()
	s.store.EXPECT().CreatePromptHistoryEntry(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, e *types.PromptHistoryEntry) error {
			enqueued++
			s.Equal("spt_live", e.SpecTaskID)
			return nil
		})
	s.store.EXPECT().ListPromptHistoryBySession(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	body := newReviewPayload(s.T())
	rr := s.post(body, "pull_request_review", signBody(s.T(), testWebhookSecret, body))
	s.Require().Equal(http.StatusOK, rr.Code)
	s.Require().Equal(1, enqueued, "done tasks must not be notified")

	time.Sleep(100 * time.Millisecond)
}

func (s *SpecTaskGitHubReviewWebhookSuite) TestRejectsBadSignature() {
	body := newReviewPayload(s.T())
	rr := s.post(body, "pull_request_review", signBody(s.T(), "wrong-secret", body))
	s.Require().Equal(http.StatusUnauthorized, rr.Code)

	rr = s.post(body, "pull_request_review", "")
	s.Require().Equal(http.StatusUnauthorized, rr.Code)
}

func (s *SpecTaskGitHubReviewWebhookSuite) TestNotConfiguredFailsClosed() {
	s.server.Cfg.GitHub.WebhookSecret = ""
	body := newReviewPayload(s.T())
	rr := s.post(body, "pull_request_review", signBody(s.T(), testWebhookSecret, body))
	s.Require().Equal(http.StatusServiceUnavailable, rr.Code)
}

func (s *SpecTaskGitHubReviewWebhookSuite) TestAcksUninterestingEvents() {
	// ping and unrelated events ack 200 without touching the store; so do
	// edited/dismissed review actions.
	for _, tc := range []struct{ event, action string }{
		{"ping", ""},
		{"push", ""},
		{"pull_request_review", "edited"},
		{"pull_request_review", "dismissed"},
	} {
		body := []byte(`{"action":"edited"}`)
		if tc.action != "" {
			body, _ = json.Marshal(githubReviewWebhookPayload{Action: tc.action})
		}
		rr := s.post(body, tc.event, signBody(s.T(), testWebhookSecret, body))
		s.Require().Equal(http.StatusOK, rr.Code, "event=%s action=%s", tc.event, tc.action)
	}
}

func (s *SpecTaskGitHubReviewWebhookSuite) TestNoCorrelatedTaskAcks() {
	s.store.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).Return([]*types.SpecTask{}, nil)

	body := newReviewPayload(s.T())
	rr := s.post(body, "pull_request_review", signBody(s.T(), testWebhookSecret, body))
	s.Require().Equal(http.StatusOK, rr.Code, "no matching task = nothing to do, ack so GitHub stops retrying")
}

func (s *SpecTaskGitHubReviewWebhookSuite) TestOrgScopedCorrelation() {
	// Org route: the org segment is resolved to the canonical org id and
	// passed into the correlation filter alongside repo + PR.
	s.store.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, query *store.GetOrganizationQuery) (*types.Organization, error) {
			s.Equal("org_e2e", query.ID)
			return &types.Organization{ID: "org_e2e"}, nil
		})
	var capturedFilter *types.SpecTaskPRMatch
	s.store.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, filters *types.SpecTaskFilters) ([]*types.SpecTask, error) {
			capturedFilter = filters.PRMatch
			return []*types.SpecTask{}, nil
		})

	body := newReviewPayload(s.T())
	rr := s.postWithOrg("org_e2e", body, "pull_request_review", signBody(s.T(), testWebhookSecret, body))
	s.Require().Equal(http.StatusOK, rr.Code)
	s.Require().NotNil(capturedFilter)
	s.Equal("org_e2e", capturedFilter.OrganizationID)
	s.Equal("owner/repo", capturedFilter.RepositoryName)
	s.Equal(42, capturedFilter.PRNumber)
}

func (s *SpecTaskGitHubReviewWebhookSuite) TestOrgScopedUnknownOrgIs404() {
	s.store.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).
		Return(nil, store.ErrNotFound)

	body := newReviewPayload(s.T())
	rr := s.postWithOrg("org_ghost", body, "pull_request_review", signBody(s.T(), testWebhookSecret, body))
	s.Require().Equal(http.StatusNotFound, rr.Code)
}

func (s *SpecTaskGitHubReviewWebhookSuite) TestOrgScopedRejectsBadSignature() {
	// Signature check must gate before org resolution and correlation.
	body := newReviewPayload(s.T())
	rr := s.postWithOrg("org_e2e", body, "pull_request_review", "sha256=deadbeef")
	s.Require().Equal(http.StatusUnauthorized, rr.Code)
}
