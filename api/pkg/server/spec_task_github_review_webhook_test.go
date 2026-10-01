package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

const (
	testWebhookSecret   = "repo-secret-abc123"
	testWebhookRepoID   = "repo-e2e"
	testReviewBotUserID = int64(291906607)
)

func signBody(t *testing.T, secret string, body []byte) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// newReviewPayload returns a pull_request_review event body for review 77 on
// owner/repo#42 with a body and reviewer (association COLLABORATOR = passes
// the trust gate).
func newReviewPayload(t *testing.T) []byte {
	t.Helper()
	payload := githubReviewWebhookPayload{
		Action: "submitted",
		Review: githubWebhookReview{
			ID:                77,
			State:             "changes_requested",
			User:              githubWebhookUser{Login: "alice", Type: "User"},
			Body:              "please fix the nil check",
			HTMLURL:           "https://github.com/owner/repo/pull/42#pullrequestreview-77",
			AuthorAssociation: "COLLABORATOR",
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
	// Metadata outside the fence (Helix-generated, non-attacker-controlled).
	for _, want := range []string{
		"PR #42 on owner/repo",
		"reviewer @alice",
		"verdict: CHANGES REQUESTED",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message missing %q:\n%s", want, msg)
		}
	}
	// Fenced untrusted content: title, body, comments all inside the markers.
	begin := strings.Index(msg, "--- BEGIN UNTRUSTED GITHUB REVIEW CONTENT ---")
	end := strings.Index(msg, "--- END UNTRUSTED GITHUB REVIEW CONTENT ---")
	if begin < 0 || end < 0 || end < begin {
		t.Fatalf("fence markers missing/misordered:\n%s", msg)
	}
	inside := msg[begin:end]
	for _, want := range []string{
		"PR title: add nil check",
		"please fix the nil check",
		"api/pkg/foo.go:42 @alice: this can nil-deref",
	} {
		if !strings.Contains(inside, want) {
			t.Fatalf("fenced content missing %q:\n%s", want, inside)
		}
	}
	// Title must NOT appear outside the fence (it is attacker text).
	outside := msg[:begin] + msg[end:]
	if strings.Contains(outside, "add nil check") {
		t.Fatalf("attacker-controlled title leaked outside the fence:\n%s", outside)
	}
	if !strings.Contains(msg, "Review link: https://github.com/owner/repo/pull/42#pullrequestreview-77") {
		t.Fatalf("review link missing:\n%s", msg)
	}

	// Bodyless review with no comments still names the verdict inside a fence.
	payload.Review.Body = ""
	bare := formatGitHubReviewFeedback(&payload, nil)
	if !strings.Contains(bare, "verdict: CHANGES REQUESTED") {
		t.Fatalf("bodyless review must still name the verdict:\n%s", bare)
	}
	if strings.Contains(bare, "Inline comments") {
		t.Fatalf("no comments must mean no comment section:\n%s", bare)
	}
}

func TestGitHubRepoFullName(t *testing.T) {
	// ParseGitHubURL is host-agnostic by design (GHES support).
	cases := []struct{ url, want string }{
		{"https://github.com/owner/repo", "owner/repo"},
		{"https://github.com/owner/repo.git", "owner/repo"},
		{"https://ghes.corp.com/owner/repo", "owner/repo"},
		{"git@github.com:owner/repo.git", "owner/repo"},
		{"https://gitlab.com/owner/repo", "owner/repo"},
		// Mixed-case row: delivery matching must be case-insensitive, so the
		// parse result preserving row casing is expected here.
		{"https://github.com/Owner/Repo", "Owner/Repo"},
	}
	for _, tc := range cases {
		got, err := githubRepoFullName(tc.url)
		if err != nil || got != tc.want {
			t.Fatalf("githubRepoFullName(%q) = %q, %v; want %q", tc.url, got, err, tc.want)
		}
	}
}

// SpecTaskGitHubReviewWebhookSuite tests the full webhook HTTP path:
// repo lookup → per-repo signature validation → PR correlation → comment
// fetch → agent enqueue.
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
			GitHub:    config.GitHub{ReviewWebhooks: true},
		},
		Store:                s.store,
		gitRepositoryService: s.gitRepo,
	}
}

func (s *SpecTaskGitHubReviewWebhookSuite) TearDownTest() {
	s.ctrl.Finish()
}

// webhookRepo is the repo row the endpoint loads for signature validation.
func (s *SpecTaskGitHubReviewWebhookSuite) webhookRepo() *types.GitRepository {
	return &types.GitRepository{
		ID:             testWebhookRepoID,
		Name:           "repo",
		OrganizationID: "org_e2e",
		ExternalURL:    "https://github.com/owner/repo",
		ExternalType:   types.ExternalRepositoryTypeGitHub,
		GitHub: &types.GitHub{
			WebhookSecret:   testWebhookSecret,
			ReviewBotUserID: testReviewBotUserID,
		},
	}
}

// expectRepoLoad mocks the repo row lookup the endpoint does first. AnyTimes:
// some tests issue several deliveries against one repo.
func (s *SpecTaskGitHubReviewWebhookSuite) expectRepoLoad(repoID string, repo *types.GitRepository, err error) {
	s.store.EXPECT().GetGitRepository(gomock.Any(), repoID).Return(repo, err).AnyTimes()
}

// post delivers a signed (or unsigned when signature is empty) request to the
// repo-scoped webhook endpoint.
func (s *SpecTaskGitHubReviewWebhookSuite) post(repoID string, body []byte, event, signature string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/reviews/"+repoID, bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"repo_id": repoID})
	req.Header.Set("X-GitHub-Event", event)
	req.Header.Set("Content-Type", "application/json")
	if signature != "" {
		req.Header.Set("X-Hub-Signature-256", signature)
	}
	rr := httptest.NewRecorder()
	s.server.specTaskGitHubReviewWebhook(rr, req)
	return rr
}

// trackingTask returns a task in the given status tracking the webhook repo's PR #42.
func (s *SpecTaskGitHubReviewWebhookSuite) trackingTask(id string, status types.SpecTaskStatus) *types.SpecTask {
	return &types.SpecTask{
		ID:                id,
		Status:            status,
		PlanningSessionID: "ses_" + id,
		RepoPullRequests: []types.RepoPR{
			{
				RepositoryID:   testWebhookRepoID,
				RepositoryName: "owner/repo",
				PRID:           "42",
				PRNumber:       42,
				PRState:        "open",
			},
		},
	}
}

func (s *SpecTaskGitHubReviewWebhookSuite) TestEndToEndEnqueuesNormalizedFeedback() {
	s.expectRepoLoad(testWebhookRepoID, s.webhookRepo(), nil)

	// Correlation query with the repo+PR filter.
	var capturedFilter *types.SpecTaskPRMatch
	s.store.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, filters *types.SpecTaskFilters) ([]*types.SpecTask, error) {
			capturedFilter = filters.PRMatch
			return []*types.SpecTask{s.trackingTask("spt_1", types.TaskStatusPullRequest)}, nil
		})

	// Inline comments fetched through the repo's own credentials.
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

	var payload githubReviewWebhookPayload
	s.Require().NoError(json.Unmarshal(newReviewPayload(s.T()), &payload))
	payload.Review.State = "commented"
	payload.Review.AuthorAssociation = "NONE"
	payload.Review.User = githubWebhookUser{ID: testReviewBotUserID, Login: "helixml-bot[bot]", Type: "Bot"}
	body, err := json.Marshal(payload)
	s.Require().NoError(err)
	rr := s.post(testWebhookRepoID, body, "pull_request_review", signBody(s.T(), testWebhookSecret, body))
	s.Require().Equal(http.StatusOK, rr.Code, "body=%s", rr.Body.String())

	s.Require().NotNil(capturedFilter)
	s.Equal(testWebhookRepoID, capturedFilter.RepositoryID)
	s.Equal(42, capturedFilter.PRNumber)
	s.Equal(testWebhookRepoID, gotRepoID)
	s.Equal("42", gotPRID)
	s.Equal(int64(77), gotReviewID)

	s.Require().NotNil(captured)
	s.Equal("ses_spt_1", captured.SessionID)
	s.Equal("spt_1", captured.SpecTaskID)
	s.True(captured.Interrupt, "review feedback interrupts like CI results")
	s.Equal("pending", captured.Status)
	for _, want := range []string{
		"PR #42 on owner/repo",
		"reviewer @helixml-bot[bot]",
		"verdict: COMMENTED",
		"--- BEGIN UNTRUSTED GITHUB REVIEW CONTENT ---",
		"api/pkg/foo.go:42 @alice: this can nil-deref",
		"--- END UNTRUSTED GITHUB REVIEW CONTENT ---",
	} {
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

	s.expectRepoLoad(testWebhookRepoID, s.webhookRepo(), nil)
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
	rr := s.post(testWebhookRepoID, body, "pull_request_review", signBody(s.T(), testWebhookSecret, body))
	s.Require().Equal(http.StatusOK, rr.Code)
	s.Require().Equal(1, enqueued, "done tasks must not be notified")

	time.Sleep(100 * time.Millisecond)
}

func (s *SpecTaskGitHubReviewWebhookSuite) TestRejectsWrongRepoSecret() {
	// The isolation property: a secret from ANOTHER repo's webhook must not
	// validate deliveries to this repo's endpoint.
	otherRepoSecret := "other-repos-secret"
	s.expectRepoLoad(testWebhookRepoID, s.webhookRepo(), nil)
	s.expectRepoLoad(testWebhookRepoID, s.webhookRepo(), nil)

	body := newReviewPayload(s.T())
	rr := s.post(testWebhookRepoID, body, "pull_request_review", signBody(s.T(), otherRepoSecret, body))
	s.Require().Equal(http.StatusUnauthorized, rr.Code)

	rr = s.post(testWebhookRepoID, body, "pull_request_review", "")
	s.Require().Equal(http.StatusUnauthorized, rr.Code)
}

func (s *SpecTaskGitHubReviewWebhookSuite) TestRepoWithoutSecretFailsClosed() {
	repo := s.webhookRepo()
	repo.GitHub = nil
	s.expectRepoLoad(testWebhookRepoID, repo, nil)

	body := newReviewPayload(s.T())
	rr := s.post(testWebhookRepoID, body, "pull_request_review", signBody(s.T(), testWebhookSecret, body))
	s.Require().Equal(http.StatusUnauthorized, rr.Code)
}

func (s *SpecTaskGitHubReviewWebhookSuite) TestUnknownRepoIs404() {
	s.expectRepoLoad("repo_ghost", nil, store.ErrNotFound)

	body := newReviewPayload(s.T())
	rr := s.post("repo_ghost", body, "pull_request_review", signBody(s.T(), testWebhookSecret, body))
	s.Require().Equal(http.StatusNotFound, rr.Code)
}

func (s *SpecTaskGitHubReviewWebhookSuite) TestPayloadRepoMismatchIsDropped() {
	// Valid signature but the payload claims a different repo — misrouting;
	// must ack without correlating.
	s.expectRepoLoad(testWebhookRepoID, s.webhookRepo(), nil)

	body := newReviewPayload(s.T())
	var payload githubReviewWebhookPayload
	s.Require().NoError(json.Unmarshal(body, &payload))
	payload.Repository.FullName = "other/tenant"
	mutated, err := json.Marshal(payload)
	s.Require().NoError(err)

	s.store.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).Times(0)
	rr := s.post(testWebhookRepoID, mutated, "pull_request_review", signBody(s.T(), testWebhookSecret, mutated))
	s.Require().Equal(http.StatusOK, rr.Code)
}

func (s *SpecTaskGitHubReviewWebhookSuite) TestAcksUninterestingEvents() {
	for _, tc := range []struct{ event, action string }{
		{"ping", ""},
		{"push", ""},
		{"pull_request_review", "edited"},
		{"pull_request_review", "dismissed"},
	} {
		s.expectRepoLoad(testWebhookRepoID, s.webhookRepo(), nil)
		var body []byte
		switch {
		case tc.event == "ping":
			body = []byte(`{}`)
		case tc.action != "":
			body, _ = json.Marshal(githubReviewWebhookPayload{Action: tc.action})
		default:
			body = []byte(`{"action":"pushed"}`)
		}
		rr := s.post(testWebhookRepoID, body, tc.event, signBody(s.T(), testWebhookSecret, body))
		s.Require().Equal(http.StatusOK, rr.Code, "event=%s action=%s", tc.event, tc.action)
	}
}

func (s *SpecTaskGitHubReviewWebhookSuite) TestNoCorrelatedTaskAcks() {
	s.expectRepoLoad(testWebhookRepoID, s.webhookRepo(), nil)
	s.store.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).Return([]*types.SpecTask{}, nil)
	s.gitRepo.listReviewCommentsFunc = func(_ context.Context, _, _ string, _ int64) ([]*types.PRReviewComment, error) {
		return nil, nil
	}

	body := newReviewPayload(s.T())
	rr := s.post(testWebhookRepoID, body, "pull_request_review", signBody(s.T(), testWebhookSecret, body))
	s.Require().Equal(http.StatusOK, rr.Code, "no matching task = nothing to do, ack so GitHub stops retrying")
}

func (s *SpecTaskGitHubReviewWebhookSuite) TestFeatureOffIsKillSwitch() {
	// The route stays registered when the feature is disabled; flipping the
	// flag off must stop validating already-provisioned repo hooks immediately.
	s.server.Cfg.GitHub.ReviewWebhooks = false
	s.store.EXPECT().GetGitRepository(gomock.Any(), gomock.Any()).Times(0)

	body := newReviewPayload(s.T())
	rr := s.post(testWebhookRepoID, body, "pull_request_review", signBody(s.T(), testWebhookSecret, body))
	s.Require().Equal(http.StatusNotFound, rr.Code)
}

func (s *SpecTaskGitHubReviewWebhookSuite) TestTrustGateDropsOutsiders() {
	// Any GitHub user can review a public repo; only OWNER/MEMBER/COLLABORATOR
	// reviews may reach the task agent. Signed deliveries from others are
	// acked without correlating.
	s.expectRepoLoad(testWebhookRepoID, s.webhookRepo(), nil)
	s.store.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).Times(0)
	s.gitRepo.listReviewCommentsFunc = func(_ context.Context, _, _ string, _ int64) ([]*types.PRReviewComment, error) {
		s.Fail("comment fetch must not run for untrusted reviews")
		return nil, nil
	}

	for _, association := range []string{"NONE", "CONTRIBUTOR", "FIRST_TIME_CONTRIBUTOR", "MANNEQUIN", ""} {
		var payload githubReviewWebhookPayload
		s.Require().NoError(json.Unmarshal(newReviewPayload(s.T()), &payload))
		payload.Review.AuthorAssociation = association
		body, err := json.Marshal(payload)
		s.Require().NoError(err)

		rr := s.post(testWebhookRepoID, body, "pull_request_review", signBody(s.T(), testWebhookSecret, body))
		s.Require().Equal(http.StatusOK, rr.Code, "association=%q must ack-drop", association)
	}
}

func (s *SpecTaskGitHubReviewWebhookSuite) TestTrustGateDropsUntrustedBots() {
	s.store.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).Times(0)
	s.gitRepo.listReviewCommentsFunc = func(_ context.Context, _, _ string, _ int64) ([]*types.PRReviewComment, error) {
		s.Fail("comment fetch must not run for untrusted bot reviews")
		return nil, nil
	}

	for i, tc := range []struct {
		name              string
		configuredID      int64
		reviewerID        int64
		reviewerType      string
		authorAssociation string
	}{
		{name: "wrong ID", configuredID: testReviewBotUserID, reviewerID: testReviewBotUserID + 1, reviewerType: "Bot", authorAssociation: "COLLABORATOR"},
		{name: "right ID user", configuredID: testReviewBotUserID, reviewerID: testReviewBotUserID, reviewerType: "User", authorAssociation: "NONE"},
		{name: "wrong type", configuredID: testReviewBotUserID, reviewerID: testReviewBotUserID, reviewerType: "Robot", authorAssociation: "COLLABORATOR"},
		{name: "empty type", configuredID: testReviewBotUserID, reviewerID: testReviewBotUserID, authorAssociation: "COLLABORATOR"},
		{name: "unconfigured", reviewerID: testReviewBotUserID, reviewerType: "Bot", authorAssociation: "COLLABORATOR"},
	} {
		repoID := fmt.Sprintf("%s-%d", testWebhookRepoID, i)
		repo := s.webhookRepo()
		repo.ID = repoID
		repo.GitHub.ReviewBotUserID = tc.configuredID
		s.expectRepoLoad(repoID, repo, nil)

		var payload githubReviewWebhookPayload
		s.Require().NoError(json.Unmarshal(newReviewPayload(s.T()), &payload))
		payload.Review.AuthorAssociation = tc.authorAssociation
		payload.Review.User = githubWebhookUser{ID: tc.reviewerID, Login: "helixml-bot[bot]", Type: tc.reviewerType}
		body, err := json.Marshal(payload)
		s.Require().NoError(err)

		rr := s.post(repoID, body, "pull_request_review", signBody(s.T(), testWebhookSecret, body))
		s.Require().Equal(http.StatusOK, rr.Code, tc.name)
	}
}

func (s *SpecTaskGitHubReviewWebhookSuite) TestTransientStoreErrorIsRetryable() {
	// A DB blip must 500 so GitHub retries the delivery, not 404 (drop).
	s.expectRepoLoad(testWebhookRepoID, nil, errors.New("connection reset"))

	body := newReviewPayload(s.T())
	rr := s.post(testWebhookRepoID, body, "pull_request_review", signBody(s.T(), testWebhookSecret, body))
	s.Require().Equal(http.StatusInternalServerError, rr.Code)
}

func (s *SpecTaskGitHubReviewWebhookSuite) TestPayloadRepoMatchIsCaseInsensitive() {
	// Row saved as github.com/Owner/Repo must match GitHub's canonical
	// owner/repo casing — a strict compare silently kills the feature.
	repo := s.webhookRepo()
	repo.ExternalURL = "https://github.com/Owner/Repo"
	s.expectRepoLoad(testWebhookRepoID, repo, nil)
	s.store.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).Return([]*types.SpecTask{}, nil)
	s.gitRepo.listReviewCommentsFunc = func(_ context.Context, _, _ string, _ int64) ([]*types.PRReviewComment, error) {
		return nil, nil
	}

	body := newReviewPayload(s.T())
	rr := s.post(testWebhookRepoID, body, "pull_request_review", signBody(s.T(), testWebhookSecret, body))
	s.Require().Equal(http.StatusOK, rr.Code, "mixed-case repo row must still correlate")
}
