package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// GitHub PR review webhook: closes the feedback loop between GitHub PR
// reviews (human or bot) and the spec task whose sandbox opened the PR.
//
// Flow: GitHub POSTs a signed pull_request_review delivery → we validate the
// HMAC signature → correlate repository.full_name + PR number to spec tasks
// via their tracked RepoPullRequests → fetch the review's inline comments
// through the GitRepositoryService (webhook payloads don't include them) →
// enqueue one normalized message to each matching task's agent via the same
// prompt-queue path CI results use (interrupt=true).
//
// Provisioning: when the feature is configured (GITHUB_INTEGRATION_WEBHOOK_SECRET),
// every GitHub PR creation installs a pull_request_review webhook on the repo
// (services.ensureGitHubReviewWebhook). Secret unset = endpoint disabled.

// githubWebhookUser is the minimal actor shape of a webhook payload.
type githubWebhookUser struct {
	Login string `json:"login"`
	Type  string `json:"type"` // "User" or "Bot"
}

// githubWebhookReview is the minimal shape of the review object in a
// pull_request_review delivery.
type githubWebhookReview struct {
	ID          int64             `json:"id"`
	User        githubWebhookUser `json:"user"`
	Body        string            `json:"body"`
	State       string            `json:"state"` // approved | changes_requested | commented | dismissed
	HTMLURL     string            `json:"html_url"`
	SubmittedAt *time.Time        `json:"submitted_at"`
}

// githubWebhookPullRequest is the minimal PR shape in a pull_request_review delivery.
type githubWebhookPullRequest struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	HTMLURL string `json:"html_url"`
}

// githubReviewWebhookPayload is the minimal pull_request_review event payload.
// AGENTS.md rule: structs for API responses, not map[string]interface{}.
type githubReviewWebhookPayload struct {
	Action      string                   `json:"action"` // submitted | edited | dismissed
	Review      githubWebhookReview      `json:"review"`
	PullRequest githubWebhookPullRequest `json:"pull_request"`
	Repository  struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

// verifyGitHubSignature validates the X-Hub-Signature-256 header against the
// raw request body (HMAC-SHA256, constant-time compare).
func verifyGitHubSignature(secret string, body []byte, signature string) bool {
	if secret == "" || signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	sig, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return false
	}
	return hmac.Equal(expected, sig)
}

// specTaskReceivesPRFeedback reports whether a task in this status should be
// notified of PR review feedback. Tasks past PR stage (done/merged) and
// pre-implementation tasks have nothing actionable.
func specTaskReceivesPRFeedback(status types.SpecTaskStatus) bool {
	switch status {
	case types.TaskStatusImplementation,
		types.TaskStatusImplementationReview,
		types.TaskStatusPullRequest:
		return true
	}
	return false
}

// findRepoPR locates the tracked RepoPR matching the webhook's repo + PR
// number, so the comment fetch can reuse the task's own repository credentials.
func findRepoPR(tasks []*types.SpecTask, repoName string, prNumber int) *types.RepoPR {
	for _, task := range tasks {
		for i := range task.RepoPullRequests {
			repoPR := &task.RepoPullRequests[i]
			if repoPR.RepositoryName == repoName && repoPR.PRNumber == prNumber {
				return repoPR
			}
		}
	}
	return nil
}

// formatGitHubReviewFeedback renders one review as the normalized message the
// spec task agent receives. One message per review submission, review body
// first, inline comments after.
func formatGitHubReviewFeedback(payload *githubReviewWebhookPayload, comments []*types.PRReviewComment) string {
	actor := payload.Review.User.Login
	if actor == "" {
		actor = "unknown"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "GitHub PR review on %s#%d \"%s\"\n",
		payload.Repository.FullName, payload.PullRequest.Number, payload.PullRequest.Title)
	fmt.Fprintf(&b, "Reviewer: @%s — verdict: %s\n",
		actor, strings.ToUpper(strings.ReplaceAll(payload.Review.State, "_", " ")))
	if payload.Review.Body != "" {
		fmt.Fprintf(&b, "\n%s\n", payload.Review.Body)
	}
	if len(comments) > 0 {
		b.WriteString("\nInline comments:\n")
		for _, c := range comments {
			fmt.Fprintf(&b, "- %s:%d @%s: %s\n", c.Path, c.Line, c.Author, c.Body)
		}
	}
	fmt.Fprintf(&b, "\nSource: %s", payload.Review.HTMLURL)
	return b.String()
}

// specTaskGitHubReviewWebhook is the HTTP entry point registered on the
// insecure router:
//   - /api/v1/webhooks/github/reviews            (deployment-scoped: personal repos)
//   - /api/v1/webhooks/github/reviews/{org}      (org-scoped: org repos)
//
// Signature is the auth; the org segment scopes correlation to that org's
// tasks, so a leaked repo webhook secret can't forge events into other orgs
// on a shared deployment. Deliveries are installed with the org's canonical
// org id in the URL, resolved here via lookupOrg (slug tolerance is free).
func (s *HelixAPIServer) specTaskGitHubReviewWebhook(w http.ResponseWriter, r *http.Request) {
	secret := s.Cfg.GitHub.WebhookSecret
	if secret == "" {
		// Feature not configured — fail closed rather than accepting unsigned deliveries.
		http.Error(w, "github review webhook not configured", http.StatusServiceUnavailable)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	if !verifyGitHubSignature(secret, body, r.Header.Get("X-Hub-Signature-256")) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	// Org-scoped installs carry the org in the URL. Resolve it (id or slug)
	// to the canonical org before any store lookup; unknown orgs are 404s.
	// Deliberately AFTER signature validation: garbage requests never touch
	// the store.
	orgID := ""
	if vars := mux.Vars(r); vars["org"] != "" {
		org, err := s.lookupOrg(r.Context(), vars["org"])
		if err != nil {
			http.Error(w, "unknown organization", http.StatusNotFound)
			return
		}
		orgID = org.ID
	}

	switch event := r.Header.Get("X-GitHub-Event"); event {
	case "ping":
		writeResponse(w, nil, http.StatusOK)
		return
	case "pull_request_review":
		// handled below
	default:
		// Not an event we consume — ack so GitHub stops retrying.
		writeResponse(w, nil, http.StatusOK)
		return
	}

	var payload githubReviewWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	// Only fresh submissions are feedback. edited/dismissed are noise for the
	// agent, and non-PR payloads can't be correlated.
	if payload.Action != "submitted" || payload.Repository.FullName == "" || payload.PullRequest.Number == 0 {
		writeResponse(w, nil, http.StatusOK)
		return
	}

	if err := s.handleGitHubPRReview(r.Context(), orgID, &payload); err != nil {
		log.Error().Err(err).
			Str("repository", payload.Repository.FullName).
			Int("pr_number", payload.PullRequest.Number).
			Int64("review_id", payload.Review.ID).
			Msg("failed to process GitHub PR review webhook")
		http.Error(w, "failed to process review", http.StatusInternalServerError)
		return
	}

	writeResponse(w, nil, http.StatusOK)
}

// handleGitHubPRReview correlates the review to spec tasks tracking this PR
// and enqueues the normalized feedback to each eligible task's agent. Mirrors
// the CI notifier path (interrupt=true, best-effort per task). orgID scopes
// correlation to one org's tasks; empty matches deployment-wide (personal repos).
func (s *HelixAPIServer) handleGitHubPRReview(ctx context.Context, orgID string, payload *githubReviewWebhookPayload) error {
	tasks, err := s.Store.ListSpecTasks(ctx, &types.SpecTaskFilters{
		PRMatch: &types.SpecTaskPRMatch{
			OrganizationID: orgID,
			RepositoryName: payload.Repository.FullName,
			PRNumber:       payload.PullRequest.Number,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to find spec tasks for PR: %w", err)
	}

	// Fetch the review's inline comments once, using the first tracked RepoPR's
	// repository (all matching tasks share the same repo + PR by definition of
	// the correlation filter).
	var comments []*types.PRReviewComment
	if repoPR := findRepoPR(tasks, payload.Repository.FullName, payload.PullRequest.Number); repoPR != nil {
		comments, err = s.gitRepositoryService.ListPullRequestReviewComments(
			ctx, repoPR.RepositoryID, repoPR.PRID, payload.Review.ID)
		if err != nil {
			return fmt.Errorf("failed to fetch review comments: %w", err)
		}
	}

	message := formatGitHubReviewFeedback(payload, comments)

	delivered := 0
	for _, task := range tasks {
		if !specTaskReceivesPRFeedback(task.Status) {
			continue
		}
		// interrupt=true like CI results: a review verdict is worth surfacing
		// immediately even mid-turn; delivery is queued, so offline agents
		// still receive it on reconnect.
		if err := s.enqueueSpecTaskAgentMessage(ctx, task, message, true, ""); err != nil {
			log.Error().Err(err).
				Str("spec_task_id", task.ID).
				Int64("review_id", payload.Review.ID).
				Msg("failed to enqueue GitHub review feedback for spec task")
			continue
		}
		delivered++
	}

	log.Info().
		Str("repository", payload.Repository.FullName).
		Int("pr_number", payload.PullRequest.Number).
		Int64("review_id", payload.Review.ID).
		Str("reviewer", payload.Review.User.Login).
		Str("state", payload.Review.State).
		Int("tasks_matched", len(tasks)).
		Int("tasks_notified", delivered).
		Msg("processed GitHub PR review webhook")

	return nil
}
