package services

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// Every pull request a spec task opens goes through a SpecTaskPRProposal: the
// agent proposes a head branch, base, title and body; a user approves (possibly
// after editing) or rejects; approval grants the agent push rights to exactly
// that branch and opens the PR as soon as the branch has commits that are not
// on the base. Helix never opens a spec-task PR on its own.

var (
	// ErrPRProposalInvalid marks a proposal or decision the caller can fix.
	ErrPRProposalInvalid = errors.New("invalid pull request proposal")
	// ErrPRProposalConflict marks a request that collides with existing state.
	ErrPRProposalConflict = errors.New("pull request proposal conflict")
)

func invalidProposal(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrPRProposalInvalid, fmt.Sprintf(format, args...))
}

func conflictingProposal(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrPRProposalConflict, fmt.Sprintf(format, args...))
}

// prProposalGit is the slice of GitRepositoryService the proposal flow uses.
type prProposalGit interface {
	WithRepoLock(key string, fn func() error) error
	PushBranchToRemote(ctx context.Context, repoID, branchName string, force bool, userID ...string) error
	ListPullRequests(ctx context.Context, repoID string) ([]*types.PullRequest, error)
	GetPullRequest(ctx context.Context, repoID, id string) (*types.PullRequest, error)
	CreatePullRequest(ctx context.Context, repoID, title, description, sourceBranch, targetBranch, userID string) (string, error)
	ValidateUserOAuth(ctx context.Context, repo *types.GitRepository, userID string) error
}

type PRProposalService struct {
	store         store.Store
	git           prProposalGit
	attention     *AttentionService
	enqueue       SpecTaskMessageEnqueuer
	serverBaseURL string
}

func NewPRProposalService(s store.Store, git prProposalGit, attention *AttentionService, serverBaseURL string) *PRProposalService {
	return &PRProposalService{store: s, git: git, attention: attention, serverBaseURL: serverBaseURL}
}

// SetMessageEnqueuer wires delivery of decision outcomes to the task's agent.
func (s *PRProposalService) SetMessageEnqueuer(fn SpecTaskMessageEnqueuer) {
	s.enqueue = fn
}

// ProposePRInput is what the agent asks for. Empty fields take defaults: the
// project's primary repository, the task's branch, and the task's target branch.
type ProposePRInput struct {
	RepositoryID string
	HeadBranch   string
	BaseBranch   string
	Title        string
	Body         string
	Reason       string
	SessionID    string
}

// proposableStatuses are the task statuses in which an agent may ask for a PR:
// once implementation has started, including after earlier PRs merged.
var proposableStatuses = map[types.SpecTaskStatus]bool{
	types.TaskStatusImplementation:       true,
	types.TaskStatusImplementationReview: true,
	types.TaskStatusPullRequest:          true,
	types.TaskStatusDone:                 true,
}

// Propose records a pending proposal and notifies the user. Re-proposing the
// same branch while the earlier proposal is still pending updates it in place.
func (s *PRProposalService) Propose(ctx context.Context, task *types.SpecTask, in ProposePRInput) (*types.SpecTaskPRProposal, error) {
	if !proposableStatuses[task.Status] {
		return nil, invalidProposal("the task is in %q; pull requests can be proposed once implementation has started", task.Status)
	}
	if strings.TrimSpace(in.Reason) == "" {
		return nil, invalidProposal("reason is required: tell the user what this pull request contains and why it should be opened now")
	}
	project, err := s.store.GetProject(ctx, task.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	repo, err := s.resolveRepository(ctx, task, project, in.RepositoryID)
	if err != nil {
		return nil, err
	}

	head := strings.TrimSpace(in.HeadBranch)
	if head == "" {
		head = task.BranchName
	}
	base := strings.TrimSpace(in.BaseBranch)
	if base == "" {
		base = TaskTargetBranch(repo, task, project.DefaultRepoID)
	}
	if err := s.validateBranches(ctx, task, repo, head, base, ""); err != nil {
		return nil, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = task.Name
	}

	existing, err := s.store.ListSpecTaskPRProposals(ctx, &types.SpecTaskPRProposalFilter{
		SpecTaskID:   task.ID,
		RepositoryID: repo.ID,
		HeadBranch:   head,
	})
	if err != nil {
		return nil, err
	}
	for _, p := range existing {
		switch p.Status {
		case types.PRProposalStatusPending:
			p.BaseBranch, p.Title, p.Body, p.Reason = base, title, in.Body, in.Reason
			if in.SessionID != "" {
				p.ProposedBySession = in.SessionID
			}
			ok, err := s.store.UpdateSpecTaskPRProposal(ctx, p, types.PRProposalStatusPending)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, conflictingProposal("proposal %s was decided while being updated; check its status", p.ID)
			}
			return p, nil
		case types.PRProposalStatusApproved, types.PRProposalStatusFailed:
			return nil, conflictingProposal("a proposal for %s is already approved (%s); push your commits to %s and Helix will open the pull request", head, p.ID, head)
		case types.PRProposalStatusOpened:
			if pr := findTaskPR(task, repo.ID, p.PRID); pr == nil || pr.PRState == "" ||
				pr.PRState == string(types.PullRequestStateOpen) || pr.PRState == string(types.PullRequestStateUnknown) {
				return nil, conflictingProposal("pull request #%d is already open from %s (%s); push more commits to %s to update it", p.PRNumber, head, p.PRURL, head)
			}
		}
	}

	p := &types.SpecTaskPRProposal{
		ID:                system.GeneratePRProposalID(),
		SpecTaskID:        task.ID,
		ProjectID:         task.ProjectID,
		RepositoryID:      repo.ID,
		RepositoryName:    repo.Name,
		HeadBranch:        head,
		BaseBranch:        base,
		Title:             title,
		Body:              in.Body,
		Reason:            in.Reason,
		Status:            types.PRProposalStatusPending,
		ProposedBySession: in.SessionID,
	}
	if err := s.store.CreateSpecTaskPRProposal(ctx, p); err != nil {
		return nil, err
	}
	s.emitAttention(task, types.AttentionEventPRProposal, p.ID, map[string]interface{}{
		"proposal_id": p.ID,
		"head_branch": p.HeadBranch,
		"base_branch": p.BaseBranch,
		"repository":  p.RepositoryName,
		"title":       p.Title,
	})
	log.Info().Str("task_id", task.ID).Str("proposal_id", p.ID).Str("repo", repo.Name).
		Str("head", head).Str("base", base).Msg("pr proposal: created")
	return p, nil
}

// Decide applies a user's decision. Approving grants push rights to the head
// branch and opens the PR if the branch already has commits; rejecting (also
// allowed for approved-but-unopened proposals) withdraws push rights.
func (s *PRProposalService) Decide(ctx context.Context, user *types.User, proposalID string, req *types.PRProposalDecisionRequest) (*types.SpecTaskPRProposal, error) {
	p, err := s.store.GetSpecTaskPRProposal(ctx, proposalID)
	if err != nil {
		return nil, err
	}
	task, err := s.store.GetSpecTask(ctx, p.SpecTaskID)
	if err != nil {
		return nil, fmt.Errorf("get spec task: %w", err)
	}
	now := time.Now()
	from := p.Status

	switch req.Decision {
	case types.PRProposalDecisionReject:
		if from == types.PRProposalStatusOpened || from == types.PRProposalStatusRejected {
			return nil, conflictingProposal("proposal is already %s", from)
		}
		p.Status = types.PRProposalStatusRejected
		p.DecidedBy, p.DecidedAt, p.DecisionComment = user.ID, &now, req.Comment
		ok, err := s.store.UpdateSpecTaskPRProposal(ctx, p, from)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, conflictingProposal("proposal changed while deciding; reload and try again")
		}
		s.dismissApprovalRequest(ctx, p)
		s.notifyAgent(ctx, task, p, user, nil)
		return p, nil

	case types.PRProposalDecisionApprove:
		if from != types.PRProposalStatusPending && from != types.PRProposalStatusFailed && from != types.PRProposalStatusApproved {
			return nil, conflictingProposal("proposal is already %s", from)
		}
		original := *p
		if from == types.PRProposalStatusPending {
			if v := strings.TrimSpace(req.HeadBranch); v != "" {
				p.HeadBranch = v
			}
			if v := strings.TrimSpace(req.BaseBranch); v != "" {
				p.BaseBranch = v
			}
			if v := strings.TrimSpace(req.Title); v != "" {
				p.Title = v
			}
			if req.Body != "" {
				p.Body = req.Body
			}
		}
		project, err := s.store.GetProject(ctx, task.ProjectID)
		if err != nil {
			return nil, fmt.Errorf("get project: %w", err)
		}
		repo, err := s.resolveRepository(ctx, task, project, p.RepositoryID)
		if err != nil {
			return nil, err
		}
		if err := s.validateBranches(ctx, task, repo, p.HeadBranch, p.BaseBranch, p.ID); err != nil {
			return nil, err
		}
		// The PR is opened with the approver's credentials; fail before
		// granting anything if they have not connected the provider.
		if err := s.git.ValidateUserOAuth(ctx, repo, user.ID); err != nil {
			var oauthErr *OAuthRequiredError
			if errors.As(err, &oauthErr) {
				return nil, err
			}
			log.Warn().Err(err).Str("proposal_id", p.ID).Msg("pr proposal: could not validate approver OAuth, continuing")
		}
		p.Status = types.PRProposalStatusApproved
		p.DecidedBy, p.DecidedAt, p.Error = user.ID, &now, ""
		if req.Comment != "" || from == types.PRProposalStatusPending {
			p.DecisionComment = req.Comment
		}
		ok, err := s.store.UpdateSpecTaskPRProposal(ctx, p, from)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, conflictingProposal("proposal changed while deciding; reload and try again")
		}
		s.dismissApprovalRequest(ctx, p)
		opened, err := s.tryOpen(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		s.notifyAgent(ctx, task, opened, user, &original)
		return opened, nil
	}
	return nil, invalidProposal("decision must be %q or %q", types.PRProposalDecisionApprove, types.PRProposalDecisionReject)
}

// OnBranchPushed opens the PRs of approved proposals waiting on this branch.
// Called from the git post-push hook for every pushed branch.
func (s *PRProposalService) OnBranchPushed(ctx context.Context, repoID, branch string) {
	if branch == SpecsBranchName {
		return
	}
	waiting, err := s.store.ListSpecTaskPRProposals(ctx, &types.SpecTaskPRProposalFilter{
		RepositoryID: repoID,
		HeadBranch:   branch,
		Statuses:     []types.SpecTaskPRProposalStatus{types.PRProposalStatusApproved},
	})
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Str("branch", branch).Msg("pr proposal: list approved proposals failed")
		return
	}
	for _, p := range waiting {
		s.openAndNotify(ctx, p.ID)
	}
}

// Reconcile retries approved proposals of a task and re-attaches opened PRs
// that a concurrent task write may have dropped from RepoPullRequests.
func (s *PRProposalService) Reconcile(ctx context.Context, taskID string) {
	proposals, err := s.store.ListSpecTaskPRProposals(ctx, &types.SpecTaskPRProposalFilter{
		SpecTaskID: taskID,
		Statuses:   []types.SpecTaskPRProposalStatus{types.PRProposalStatusApproved, types.PRProposalStatusOpened},
	})
	if err != nil {
		log.Debug().Err(err).Str("task_id", taskID).Msg("pr proposal: reconcile list failed")
		return
	}
	for _, p := range proposals {
		if p.Status == types.PRProposalStatusApproved {
			s.openAndNotify(ctx, p.ID)
			continue
		}
		if err := s.attachPR(ctx, p); err != nil {
			log.Warn().Err(err).Str("proposal_id", p.ID).Msg("pr proposal: re-attach failed")
		}
	}
}

// AllowedPushBranches lists the branches a task's agent may push to in repoID
// beyond its own task branch and helix-specs.
func (s *PRProposalService) AllowedPushBranches(ctx context.Context, taskID, repoID string) ([]string, error) {
	proposals, err := s.store.ListSpecTaskPRProposals(ctx, &types.SpecTaskPRProposalFilter{
		SpecTaskID:   taskID,
		RepositoryID: repoID,
	})
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range proposals {
		if p.GrantsPush() {
			out = append(out, p.HeadBranch)
		}
	}
	return out, nil
}

// HasOutstanding reports whether any proposal of the task still expects a PR,
// in which case the task must not be completed yet.
func (s *PRProposalService) HasOutstanding(ctx context.Context, taskID string) (bool, error) {
	proposals, err := s.store.ListSpecTaskPRProposals(ctx, &types.SpecTaskPRProposalFilter{SpecTaskID: taskID})
	if err != nil {
		return false, err
	}
	for _, p := range proposals {
		if p.AwaitsPR() {
			return true, nil
		}
	}
	return false, nil
}

func (s *PRProposalService) openAndNotify(ctx context.Context, proposalID string) {
	p, err := s.tryOpen(ctx, proposalID)
	if err != nil {
		log.Error().Err(err).Str("proposal_id", proposalID).Msg("pr proposal: open failed")
		return
	}
	if p.Status == types.PRProposalStatusApproved {
		return // still waiting for commits; nothing new to tell anyone
	}
	task, err := s.store.GetSpecTask(ctx, p.SpecTaskID)
	if err != nil {
		return
	}
	s.notifyAgent(ctx, task, p, nil, nil)
}

// tryOpen pushes an approved proposal's branch upstream and opens (or adopts)
// its PR. A branch with no commits beyond the base leaves the proposal approved
// so the next push retries. Provider failures mark it failed for a user retry.
func (s *PRProposalService) tryOpen(ctx context.Context, proposalID string) (*types.SpecTaskPRProposal, error) {
	var result *types.SpecTaskPRProposal
	err := s.git.WithRepoLock("pr-proposal:"+proposalID, func() error {
		p, err := s.store.GetSpecTaskPRProposal(ctx, proposalID)
		if err != nil {
			return err
		}
		result = p
		if p.Status != types.PRProposalStatusApproved {
			return nil
		}
		task, err := s.store.GetSpecTask(ctx, p.SpecTaskID)
		if err != nil {
			return fmt.Errorf("get spec task: %w", err)
		}
		repo, err := s.store.GetGitRepository(ctx, p.RepositoryID)
		if err != nil {
			return fmt.Errorf("get repository: %w", err)
		}
		if _, err := GetBranchCommitID(ctx, repo.LocalPath, p.HeadBranch); err != nil {
			return nil // agent has not pushed the branch yet
		}
		changed, err := BranchHasChanges(ctx, repo.LocalPath, p.BaseBranch, p.HeadBranch)
		if err != nil {
			return s.markFailed(ctx, p, err)
		}
		if !changed {
			return nil
		}

		pr, err := s.openOnProvider(ctx, p, task, repo)
		if err != nil {
			return s.markFailed(ctx, p, err)
		}
		p.Status = types.PRProposalStatusOpened
		p.PRID, p.PRNumber, p.PRURL, p.Error = pr.ID, pr.Number, pr.URL, ""
		if ok, err := s.store.UpdateSpecTaskPRProposal(ctx, p, types.PRProposalStatusApproved); err != nil || !ok {
			return fmt.Errorf("record opened pull request %s on proposal %s (updated=%v): %w", pr.ID, p.ID, ok, err)
		}
		if err := s.attachPR(ctx, p); err != nil {
			log.Error().Err(err).Str("proposal_id", p.ID).Msg("pr proposal: PR opened but attaching it to the task failed (reconcile will retry)")
		}
		s.emitAttention(task, types.AttentionEventPRReady, pr.ID, map[string]interface{}{
			"pr_id":  pr.ID,
			"pr_url": pr.URL,
		})
		log.Info().Str("task_id", task.ID).Str("proposal_id", p.ID).Str("pr_id", pr.ID).
			Str("pr_url", pr.URL).Msg("pr proposal: pull request opened")
		return nil
	})
	return result, err
}

func (s *PRProposalService) openOnProvider(ctx context.Context, p *types.SpecTaskPRProposal, task *types.SpecTask, repo *types.GitRepository) (*types.PullRequest, error) {
	var pr *types.PullRequest
	err := s.git.WithRepoLock(repo.ID, func() error {
		if err := s.git.PushBranchToRemote(ctx, repo.ID, p.HeadBranch, false, p.DecidedBy); err != nil {
			return fmt.Errorf("push %s to %s: %w", p.HeadBranch, repo.Name, err)
		}
		if existing := s.findOpenPR(ctx, repo.ID, p.HeadBranch, p.BaseBranch); existing != nil {
			pr = existing
			return nil
		}
		body := AppendPRFooter(p.Body, s.renderFooter(ctx, repo, task))
		id, err := s.git.CreatePullRequest(ctx, repo.ID, p.Title, body, p.HeadBranch, p.BaseBranch, p.DecidedBy)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				if existing := s.findOpenPR(ctx, repo.ID, p.HeadBranch, p.BaseBranch); existing != nil {
					pr = existing
					return nil
				}
			}
			return fmt.Errorf("create pull request: %w", err)
		}
		pr = &types.PullRequest{ID: id, State: types.PullRequestStateOpen}
		if fetched, err := s.git.GetPullRequest(ctx, repo.ID, id); err == nil && fetched != nil {
			pr = fetched
		} else if err != nil {
			log.Warn().Err(err).Str("pr_id", id).Msg("pr proposal: created PR but could not fetch its URL; the PR poller will fill it in")
		}
		return nil
	})
	return pr, err
}

func (s *PRProposalService) findOpenPR(ctx context.Context, repoID, head, base string) *types.PullRequest {
	prs, err := s.git.ListPullRequests(ctx, repoID)
	if err != nil {
		return nil
	}
	for _, pr := range prs {
		if pr.State == types.PullRequestStateOpen &&
			strings.TrimPrefix(pr.SourceBranch, "refs/heads/") == head &&
			(pr.TargetBranch == "" || strings.TrimPrefix(pr.TargetBranch, "refs/heads/") == base) {
			return pr
		}
	}
	return nil
}

func (s *PRProposalService) markFailed(ctx context.Context, p *types.SpecTaskPRProposal, cause error) error {
	p.Status = types.PRProposalStatusFailed
	p.Error = cause.Error()
	if _, err := s.store.UpdateSpecTaskPRProposal(ctx, p, types.PRProposalStatusApproved); err != nil {
		return err
	}
	log.Warn().Err(cause).Str("proposal_id", p.ID).Msg("pr proposal: opening pull request failed")
	return nil
}

// attachPR records an opened proposal's PR on its task and moves the task into
// pull_request, reopening it if earlier PRs had already completed it.
func (s *PRProposalService) attachPR(ctx context.Context, p *types.SpecTaskPRProposal) error {
	return s.git.WithRepoLock("task:"+p.SpecTaskID, func() error {
		task, err := s.store.GetSpecTask(ctx, p.SpecTaskID)
		if err != nil {
			return err
		}
		if findTaskPR(task, p.RepositoryID, p.PRID) != nil {
			return nil
		}
		task.RepoPullRequests = append(task.RepoPullRequests, types.RepoPR{
			RepositoryID:   p.RepositoryID,
			RepositoryName: p.RepositoryName,
			PRID:           p.PRID,
			PRNumber:       p.PRNumber,
			PRURL:          p.PRURL,
			PRState:        string(types.PullRequestStateOpen),
			HeadBranch:     p.HeadBranch,
			ProposalID:     p.ID,
		})
		now := time.Now()
		switch task.Status {
		case types.TaskStatusImplementation, types.TaskStatusImplementationReview, types.TaskStatusDone:
			task.Status = types.TaskStatusPullRequest
			task.StatusUpdatedAt = &now
			task.CompletedAt = nil
			task.MergedToMain = false
			task.MergedAt = nil
			task.MergeCommitHash = ""
		}
		if task.ImplementationApprovedBy == "" {
			task.ImplementationApprovedBy = p.DecidedBy
			task.ImplementationApprovedAt = &now
		}
		task.UpdatedAt = now
		return s.store.UpdateSpecTask(ctx, task)
	})
}

func findTaskPR(task *types.SpecTask, repoID, prID string) *types.RepoPR {
	if prID == "" {
		return nil
	}
	for i := range task.RepoPullRequests {
		if task.RepoPullRequests[i].RepositoryID == repoID && task.RepoPullRequests[i].PRID == prID {
			return &task.RepoPullRequests[i]
		}
	}
	return nil
}

func (s *PRProposalService) resolveRepository(ctx context.Context, task *types.SpecTask, project *types.Project, ref string) (*types.GitRepository, error) {
	repos, err := s.store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{ProjectID: task.ProjectID})
	if err != nil {
		return nil, fmt.Errorf("list project repositories: %w", err)
	}
	if ref == "" {
		ref = project.DefaultRepoID
	}
	var names []string
	for _, r := range repos {
		if r.ID == ref || r.Name == ref {
			if r.ExternalURL == "" {
				return nil, invalidProposal("repository %q is hosted by Helix and has no pull requests; its work lands when the user accepts the task", r.Name)
			}
			return r, nil
		}
		names = append(names, r.Name)
	}
	return nil, invalidProposal("repository %q is not part of this project (repositories: %s)", ref, strings.Join(names, ", "))
}

// branchNamePattern is a conservative subset of git-check-ref-format.
var branchNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

func validBranchName(name string) bool {
	return len(name) <= 200 && branchNamePattern.MatchString(name) &&
		!strings.Contains(name, "..") && !strings.Contains(name, "//") && !strings.Contains(name, "/.") &&
		!strings.HasSuffix(name, "/") && !strings.HasSuffix(name, ".") && !strings.HasSuffix(name, ".lock")
}

// validateBranches checks a head/base pair. Beyond naming rules, a head other
// than the task's own branch must not belong to anyone else: no other task's
// branch or proposal, and no pre-existing branch the task was never granted.
// selfID is the proposal being decided, whose own grant does not count against it.
func (s *PRProposalService) validateBranches(ctx context.Context, task *types.SpecTask, repo *types.GitRepository, head, base, selfID string) error {
	if head == "" {
		return invalidProposal("head_branch is required (the task has no branch yet)")
	}
	if !validBranchName(head) {
		return invalidProposal("head_branch %q is not a valid branch name", head)
	}
	if !validBranchName(base) {
		return invalidProposal("base_branch %q is not a valid branch name", base)
	}
	if head == base {
		return invalidProposal("head_branch and base_branch are both %q", head)
	}
	if head == SpecsBranchName || head == repo.DefaultBranch {
		return invalidProposal("cannot open a pull request from %q", head)
	}
	if _, err := GetBranchCommitID(ctx, repo.LocalPath, base); err != nil {
		return invalidProposal("base_branch %q does not exist in %s", base, repo.Name)
	}
	if head == task.BranchName {
		return nil
	}

	others, err := s.store.ListSpecTaskPRProposals(ctx, &types.SpecTaskPRProposalFilter{RepositoryID: repo.ID, HeadBranch: head})
	if err != nil {
		return err
	}
	grantedToTask := false
	for _, p := range others {
		if p.ID == selfID || p.Status == types.PRProposalStatusRejected {
			continue
		}
		if p.SpecTaskID != task.ID {
			return conflictingProposal("branch %q is already claimed by another task; choose a different name", head)
		}
		if p.GrantsPush() {
			grantedToTask = true
		}
	}
	tasks, err := s.store.ListSpecTasks(ctx, &types.SpecTaskFilters{ProjectID: task.ProjectID, BranchName: head, IncludeArchived: true})
	if err != nil {
		return fmt.Errorf("check branch ownership: %w", err)
	}
	for _, t := range tasks {
		if t.ID != task.ID {
			return conflictingProposal("branch %q is another task's working branch; choose a different name", head)
		}
	}
	if !grantedToTask {
		if _, err := GetBranchCommitID(ctx, repo.LocalPath, head); err == nil {
			return conflictingProposal("branch %q already exists in %s; propose a new branch name", head, repo.Name)
		}
	}
	return nil
}

func (s *PRProposalService) renderFooter(ctx context.Context, repo *types.GitRepository, task *types.SpecTask) string {
	tmpl := DefaultPRFooterTemplate
	if userID := taskPRUserID(task); userID != "" {
		if user, err := s.store.GetUser(ctx, &store.GetUserQuery{ID: userID}); err == nil {
			tmpl = UserPRFooterTemplate(user)
		}
	}
	orgName := ""
	if task.OrganizationID != "" {
		if org, err := s.store.GetOrganization(ctx, &store.GetOrganizationQuery{ID: task.OrganizationID}); err == nil && org != nil {
			orgName = org.Name
		}
	}
	footer, err := RenderPRFooter(tmpl, repo, task, orgName, s.serverBaseURL)
	if err != nil {
		log.Warn().Err(err).Str("task_id", task.ID).Msg("pr proposal: footer render failed; opening PR without it")
		return ""
	}
	return footer
}

func (s *PRProposalService) emitAttention(task *types.SpecTask, eventType types.AttentionEventType, qualifier string, metadata map[string]interface{}) {
	if s.attention == nil {
		return
	}
	go func() {
		if _, err := s.attention.EmitEvent(context.Background(), eventType, task, qualifier, metadata); err != nil {
			log.Warn().Err(err).Str("task_id", task.ID).Str("event_type", string(eventType)).Msg("pr proposal: attention event failed")
		}
	}()
}

// dismissApprovalRequest clears a decided proposal's Needs Attention entry.
func (s *PRProposalService) dismissApprovalRequest(ctx context.Context, p *types.SpecTaskPRProposal) {
	key := types.BuildAttentionEventIdempotencyKey(p.SpecTaskID, types.AttentionEventPRProposal, p.ID)
	if err := s.store.DismissAttentionEventByKey(ctx, key); err != nil {
		log.Warn().Err(err).Str("proposal_id", p.ID).Msg("pr proposal: failed to dismiss approval request")
	}
}

func (s *PRProposalService) notifyAgent(ctx context.Context, task *types.SpecTask, p *types.SpecTaskPRProposal, decidedBy *types.User, original *types.SpecTaskPRProposal) {
	if s.enqueue == nil {
		return
	}
	if decidedBy == nil && p.DecidedBy != "" {
		decidedBy, _ = s.store.GetUser(ctx, &store.GetUserQuery{ID: p.DecidedBy})
	}
	msg := BuildPRProposalOutcomePrompt(p, decidedBy, original)
	if msg == "" {
		return
	}
	if err := s.enqueue(ctx, task, msg, false, ""); err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Str("proposal_id", p.ID).Msg("pr proposal: failed to notify agent")
	}
}

// PullRequestProposalGuidance is the short pull-request section for prompts
// that do not carry the full implementation handoff (Just Do It tasks).
func PullRequestProposalGuidance(branchName string) string {
	return "**Pull requests:** Helix never opens a pull request on its own, and you must not create one with `gh`, the GitHub API or GitHub MCP tools. " +
		"When work is ready for review, push it and call `propose_pull_request` (title, body, reason); the user approves each proposal and the outcome arrives as a message in this session. " +
		"`head_branch` defaults to `" + branchName + "`. A task may ship as several pull requests: propose each further slice with its own new `head_branch` before pushing to it — " +
		"you can only push to your task branch and to branches the user has approved. Pushing more commits to the branch of an open pull request updates it."
}

// BuildPRProposalOutcomePrompt tells the agent what happened to its proposal.
// original is the proposal as the agent made it, so user edits can be called
// out — the agent must push to the approved branch name, not its own.
func BuildPRProposalOutcomePrompt(p *types.SpecTaskPRProposal, decidedBy *types.User, original *types.SpecTaskPRProposal) string {
	who := "The user"
	if decidedBy != nil {
		if decidedBy.Email != "" {
			who = decidedBy.Email
		} else if decidedBy.FullName != "" {
			who = decidedBy.FullName
		}
	}
	var b strings.Builder
	note := func() {
		if p.DecisionComment != "" {
			fmt.Fprintf(&b, "\n**Reviewer note:** %s\n", p.DecisionComment)
		}
	}
	edits := func() {
		if original == nil {
			return
		}
		var changes []string
		if original.HeadBranch != p.HeadBranch {
			changes = append(changes, fmt.Sprintf("- branch: `%s` → `%s` (push to the new name)", original.HeadBranch, p.HeadBranch))
		}
		if original.BaseBranch != p.BaseBranch {
			changes = append(changes, fmt.Sprintf("- base: `%s` → `%s`", original.BaseBranch, p.BaseBranch))
		}
		if original.Title != p.Title {
			changes = append(changes, fmt.Sprintf("- title: %q", p.Title))
		}
		if original.Body != p.Body {
			changes = append(changes, "- description was edited")
		}
		if len(changes) > 0 {
			fmt.Fprintf(&b, "\nThe reviewer changed your proposal before approving:\n%s\n", strings.Join(changes, "\n"))
		}
	}

	switch p.Status {
	case types.PRProposalStatusOpened:
		fmt.Fprintf(&b, "# Pull request opened\n\nSpeak English.\n\n")
		if decidedBy != nil {
			fmt.Fprintf(&b, "%s approved your pull request proposal.\n", who)
		}
		fmt.Fprintf(&b, "Pull request #%d is open in %s from `%s` into `%s`: %s\n", p.PRNumber, p.RepositoryName, p.HeadBranch, p.BaseBranch, p.PRURL)
		edits()
		note()
		fmt.Fprintf(&b, "\nPushing more commits to `%s` updates this pull request. To ship further work as a separate pull request, call `propose_pull_request` again with a new branch name.\n", p.HeadBranch)
	case types.PRProposalStatusApproved:
		fmt.Fprintf(&b, "# Pull request proposal approved\n\nSpeak English.\n\n%s approved pushing to `%s` in %s.\n", who, p.HeadBranch, p.RepositoryName)
		edits()
		note()
		fmt.Fprintf(&b, "\nCommit your work on `%s` and push it:\n\n```bash\ngit push origin %s\n```\n\nHelix opens the pull request into `%s` as soon as the branch has commits that are not on `%s`.\n", p.HeadBranch, p.HeadBranch, p.BaseBranch, p.BaseBranch)
	case types.PRProposalStatusFailed:
		fmt.Fprintf(&b, "# Pull request could not be opened\n\nSpeak English.\n\nYour proposal to open a pull request from `%s` into `%s` was approved, but opening it failed:\n\n```\n%s\n```\n", p.HeadBranch, p.BaseBranch, p.Error)
		note()
		b.WriteString("\nYou still have push rights to the branch. If the failure is caused by your branch (for example it has no commits beyond the base), fix it and push again; otherwise the user can retry from Helix. Do not propose the same pull request again.\n")
	case types.PRProposalStatusRejected:
		fmt.Fprintf(&b, "# Pull request proposal rejected\n\nSpeak English.\n\n%s rejected your proposal to open a pull request from `%s` into `%s`.\n", who, p.HeadBranch, p.BaseBranch)
		note()
		fmt.Fprintf(&b, "\nDo not push to `%s`. Address the feedback, then propose again if a pull request is still appropriate.\n", p.HeadBranch)
	default:
		return ""
	}
	return b.String()
}
