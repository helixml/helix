package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/prompts"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// Pull requests on Helix-hosted repositories. The GitHub-compatible API
// (github_compat.go) and the /api/v1 handlers below share this core.

// internalPRCreateLocks serializes PR creation per repository, so the
// duplicate check and the number allocation see each other's writes. It is
// deliberately not the repository git lock: PR proposals call create while
// holding that lock.
var internalPRCreateLocks sync.Map // repo id -> *sync.Mutex

func (apiServer *HelixAPIServer) createInternalPR(ctx context.Context, repo *types.GitRepository, user *types.User, title, body, head, base string) (*types.GitPullRequest, error) {
	if title == "" || head == "" || base == "" {
		return nil, statusErr(http.StatusUnprocessableEntity, "Validation Failed: title, head and base are required")
	}
	headSHA, err := services.GetBranchCommitID(ctx, repo.LocalPath, head)
	if err != nil {
		return nil, statusErr(http.StatusUnprocessableEntity, "Validation Failed: head branch %q does not exist", head)
	}
	baseSHA, err := services.GetBranchCommitID(ctx, repo.LocalPath, base)
	if err != nil {
		return nil, statusErr(http.StatusUnprocessableEntity, "Validation Failed: base branch %q does not exist", base)
	}
	changed, err := services.BranchHasChanges(ctx, repo.LocalPath, base, head)
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, statusErr(http.StatusUnprocessableEntity, "Validation Failed: No commits between %s and %s", base, head)
	}

	lock, _ := internalPRCreateLocks.LoadOrStore(repo.ID, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()

	open, err := apiServer.Store.ListGitPullRequests(ctx, repo.ID, string(types.GitPullRequestStateOpen))
	if err != nil {
		return nil, err
	}
	for _, pr := range open {
		if pr.HeadBranch == head && pr.BaseBranch == base {
			return nil, statusErr(http.StatusUnprocessableEntity, "Validation Failed: A pull request already exists for %s.", head)
		}
	}

	pr := &types.GitPullRequest{
		ID:           system.GenerateUUID(),
		RepositoryID: repo.ID,
		Title:        title,
		Body:         body,
		HeadBranch:   head,
		BaseBranch:   base,
		HeadSHA:      headSHA,
		BaseSHA:      baseSHA,
		State:        types.GitPullRequestStateOpen,
		AuthorID:     user.ID,
		AuthorLogin:  pullRequestLogin(user),
	}
	if err := apiServer.Store.CreateGitPullRequest(ctx, pr); err != nil {
		return nil, err
	}
	log.Info().Str("repo_id", repo.ID).Str("pr", pr.String()).Str("head", head).Str("base", base).Msg("internal pull request opened")
	return pr, nil
}

func pullRequestLogin(user *types.User) string {
	if user.Username != "" {
		return user.Username
	}
	if local, _, ok := strings.Cut(user.Email, "@"); ok && local != "" {
		return local
	}
	return user.ID
}

// listInternalPRs lists PRs newest first. state is open, closed or all. Open
// PRs are refreshed first, so ones that landed by another route show closed.
func (apiServer *HelixAPIServer) listInternalPRs(ctx context.Context, repo *types.GitRepository, state string) ([]*types.GitPullRequest, error) {
	switch state {
	case "open", "closed":
	case "all":
		state = ""
	default:
		return nil, statusErr(http.StatusUnprocessableEntity, "Validation Failed: state must be open, closed or all")
	}
	all, err := apiServer.Store.ListGitPullRequests(ctx, repo.ID, "")
	if err != nil {
		return nil, err
	}
	out := make([]*types.GitPullRequest, 0, len(all))
	for _, pr := range all {
		if _, err := apiServer.gitHTTPServer.RefreshInternalPR(ctx, repo, pr, false); err != nil {
			return nil, err
		}
		if state == "" || string(pr.State) == state {
			out = append(out, pr)
		}
	}
	return out, nil
}

func (apiServer *HelixAPIServer) getInternalPR(ctx context.Context, repo *types.GitRepository, number int, withMergeability bool) (*types.GitPullRequest, *bool, error) {
	pr, err := apiServer.Store.GetGitPullRequest(ctx, repo.ID, number)
	if err != nil {
		return nil, nil, err
	}
	mergeable, err := apiServer.gitHTTPServer.RefreshInternalPR(ctx, repo, pr, withMergeability)
	if err != nil {
		return nil, nil, err
	}
	return pr, mergeable, nil
}

func (apiServer *HelixAPIServer) updateInternalPR(ctx context.Context, repo *types.GitRepository, number int, update *types.GitPullRequestUpdate) (*types.GitPullRequest, error) {
	// Refresh first: it may record that the PR already landed.
	if _, _, err := apiServer.getInternalPR(ctx, repo, number, false); err != nil {
		return nil, err
	}
	var updated *types.GitPullRequest
	err := apiServer.gitRepositoryService.WithRepoLock(repo.ID, func() error {
		pr, err := apiServer.Store.GetGitPullRequest(ctx, repo.ID, number)
		if err != nil {
			return err
		}
		if err := apiServer.applyInternalPRUpdate(ctx, repo, pr, update); err != nil {
			return err
		}
		updated = pr
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// applyInternalPRUpdate edits and saves a PR. The caller holds the repository
// lock, so a concurrent merge cannot be overwritten by this whole-row save.
func (apiServer *HelixAPIServer) applyInternalPRUpdate(ctx context.Context, repo *types.GitRepository, pr *types.GitPullRequest, update *types.GitPullRequestUpdate) error {
	if update.Title != nil {
		pr.Title = *update.Title
	}
	if update.Body != nil {
		pr.Body = *update.Body
	}
	if update.State != nil {
		switch *update.State {
		case "open":
			if pr.Merged {
				return statusErr(http.StatusUnprocessableEntity, "Validation Failed: a merged pull request cannot be reopened")
			}
			if pr.State != types.GitPullRequestStateOpen {
				pr.State = types.GitPullRequestStateOpen
				pr.ClosedAt = nil
			}
		case "closed":
			if pr.State == types.GitPullRequestStateOpen {
				apiServer.gitHTTPServer.SettleInternalPRSHAs(ctx, repo, pr)
				now := time.Now()
				pr.State = types.GitPullRequestStateClosed
				pr.ClosedAt = &now
			}
		default:
			return statusErr(http.StatusUnprocessableEntity, "Validation Failed: state must be open or closed")
		}
	}
	return apiServer.Store.UpdateGitPullRequest(ctx, pr)
}

// ---------------------------------------------------------------------------
// /api/v1 handlers for the Helix UI
// ---------------------------------------------------------------------------

func writeInternalPRError(w http.ResponseWriter, err error) {
	status, message := internalPRErrorStatus(err)
	writeErrResponse(w, errors.New(message), status)
}

// internalPRRepo loads and authorizes the Helix-hosted repository of a v1 request.
func (apiServer *HelixAPIServer) internalPRRepo(r *http.Request, action types.Action) (*types.GitRepository, *types.User, error) {
	user := getRequestUser(r)
	repo, err := apiServer.Store.GetGitRepository(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, statusErr(http.StatusNotFound, "repository not found")
		}
		return nil, nil, err
	}
	if err := apiServer.authorizeUserToRepository(r.Context(), user, repo, action); err != nil {
		return nil, nil, statusErr(http.StatusForbidden, "%s", err.Error())
	}
	if repo.ExternalURL != "" {
		return nil, nil, statusErr(http.StatusBadRequest, "repository is external; its pull requests live on %s", repo.ExternalType)
	}
	return repo, user, nil
}

func pullRequestNumber(r *http.Request) (int, error) {
	n, err := strconv.Atoi(mux.Vars(r)["number"])
	if err != nil || n <= 0 {
		return 0, statusErr(http.StatusBadRequest, "invalid pull request number")
	}
	return n, nil
}

// listGitRepositoryPulls godoc
// @Summary List Helix pull requests
// @Description List the pull requests of a Helix-hosted repository
// @ID listGitRepositoryPulls
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Param state query string false "open (default), closed or all"
// @Success 200 {array} types.GitPullRequest
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Router /api/v1/git/repositories/{id}/pulls [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listGitRepositoryPulls(w http.ResponseWriter, r *http.Request) {
	repo, _, err := apiServer.internalPRRepo(r, types.ActionGet)
	if err != nil {
		writeInternalPRError(w, err)
		return
	}
	state := r.URL.Query().Get("state")
	if state == "" {
		state = "open"
	}
	prs, err := apiServer.listInternalPRs(r.Context(), repo, state)
	if err != nil {
		writeInternalPRError(w, err)
		return
	}
	writeResponse(w, prs, http.StatusOK)
}

// getGitRepositoryPull godoc
// @Summary Get a Helix pull request
// @Description Get a pull request of a Helix-hosted repository with its changed files and mergeability
// @ID getGitRepositoryPull
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Param number path int true "Pull request number"
// @Success 200 {object} types.GitPullRequestDetail
// @Failure 404 {object} types.APIError
// @Router /api/v1/git/repositories/{id}/pulls/{number} [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getGitRepositoryPull(w http.ResponseWriter, r *http.Request) {
	repo, _, err := apiServer.internalPRRepo(r, types.ActionGet)
	if err != nil {
		writeInternalPRError(w, err)
		return
	}
	number, err := pullRequestNumber(r)
	if err != nil {
		writeInternalPRError(w, err)
		return
	}
	detail, err := apiServer.internalPRDetail(r.Context(), repo, number)
	if err != nil {
		writeInternalPRError(w, err)
		return
	}
	writeResponse(w, detail, http.StatusOK)
}

func (apiServer *HelixAPIServer) internalPRDetail(ctx context.Context, repo *types.GitRepository, number int) (*types.GitPullRequestDetail, error) {
	pr, mergeable, err := apiServer.getInternalPR(ctx, repo, number, true)
	if err != nil {
		return nil, err
	}
	files, err := apiServer.gitHTTPServer.InternalPRFileChanges(ctx, repo, pr)
	if err != nil {
		return nil, err
	}
	if files == nil {
		files = []*types.InternalPRFile{}
	}
	owner, err := services.InternalCompatOwner(ctx, apiServer.Store, repo)
	if err != nil {
		return nil, err
	}
	detail := &types.GitPullRequestDetail{
		PullRequest: pr,
		HTMLURL:     internalPRHTMLURL(strings.TrimSuffix(apiServer.Cfg.WebServer.URL, "/"), repo, owner, pr.Number),
		Mergeable:   mergeable,
		Files:       files,
	}
	if task, err := apiServer.specTaskForInternalPR(ctx, repo, pr); err != nil {
		return nil, err
	} else if task != nil {
		detail.SpecTaskID = task.ID
		detail.SpecTaskName = task.Name
		detail.SpecTaskProjectID = task.ProjectID
	}
	return detail, nil
}

func (apiServer *HelixAPIServer) specTaskForInternalPR(ctx context.Context, repo *types.GitRepository, pr *types.GitPullRequest) (*types.SpecTask, error) {
	tasks, err := apiServer.Store.ListSpecTasks(ctx, &types.SpecTaskFilters{
		PRMatch:         &types.SpecTaskPRMatch{RepositoryID: repo.ID, PRNumber: pr.Number},
		IncludeArchived: true,
		Limit:           1,
	})
	if err != nil {
		return nil, fmt.Errorf("find spec task for %s: %w", pr, err)
	}
	if len(tasks) == 0 {
		return nil, nil
	}
	return tasks[0], nil
}

// updateGitRepositoryPull godoc
// @Summary Update a Helix pull request
// @Description Edit the title or body of a Helix pull request, or close or reopen it
// @ID updateGitRepositoryPull
// @Tags git-repositories
// @Accept json
// @Produce json
// @Param id path string true "Repository ID"
// @Param number path int true "Pull request number"
// @Param request body types.GitPullRequestUpdate true "Update"
// @Success 200 {object} types.GitPullRequest
// @Failure 404 {object} types.APIError
// @Failure 422 {object} types.APIError
// @Router /api/v1/git/repositories/{id}/pulls/{number} [patch]
// @Security BearerAuth
func (apiServer *HelixAPIServer) updateGitRepositoryPull(w http.ResponseWriter, r *http.Request) {
	repo, _, err := apiServer.internalPRRepo(r, types.ActionUpdate)
	if err != nil {
		writeInternalPRError(w, err)
		return
	}
	number, err := pullRequestNumber(r)
	if err != nil {
		writeInternalPRError(w, err)
		return
	}
	var update types.GitPullRequestUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeErrResponse(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}
	pr, err := apiServer.updateInternalPR(r.Context(), repo, number, &update)
	if err != nil {
		writeInternalPRError(w, err)
		return
	}
	writeResponse(w, pr, http.StatusOK)
}

// mergeGitRepositoryPull godoc
// @Summary Merge a Helix pull request
// @Description Merge a Helix pull request: fast-forward when possible, otherwise a merge commit. 405 when it conflicts.
// @ID mergeGitRepositoryPull
// @Tags git-repositories
// @Accept json
// @Produce json
// @Param id path string true "Repository ID"
// @Param number path int true "Pull request number"
// @Param request body types.GitPullRequestMergeRequest false "Merge options"
// @Success 200 {object} types.GitPullRequest
// @Failure 404 {object} types.APIError
// @Failure 405 {object} types.APIError
// @Router /api/v1/git/repositories/{id}/pulls/{number}/merge [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) mergeGitRepositoryPull(w http.ResponseWriter, r *http.Request) {
	repo, user, err := apiServer.internalPRRepo(r, types.ActionUpdate)
	if err != nil {
		writeInternalPRError(w, err)
		return
	}
	number, err := pullRequestNumber(r)
	if err != nil {
		writeInternalPRError(w, err)
		return
	}
	var req types.GitPullRequestMergeRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErrResponse(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
			return
		}
	}
	pr, err := apiServer.gitHTTPServer.MergeInternalPullRequest(r.Context(), repo, number, req.MergeMethod, req.CommitTitle, req.CommitMessage, user.ID)
	if err != nil {
		writeInternalPRError(w, err)
		return
	}
	writeResponse(w, pr, http.StatusOK)
}

// requestGitRepositoryPullRebase godoc
// @Summary Ask the agent to rebase a Helix pull request
// @Description Sends the spec task that owns the pull request an instruction to rebase its head branch onto the base branch
// @ID requestGitRepositoryPullRebase
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Param number path int true "Pull request number"
// @Success 200 {object} types.GitPullRequestDetail
// @Failure 404 {object} types.APIError
// @Failure 409 {object} types.APIError
// @Router /api/v1/git/repositories/{id}/pulls/{number}/request-rebase [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) requestGitRepositoryPullRebase(w http.ResponseWriter, r *http.Request) {
	repo, user, err := apiServer.internalPRRepo(r, types.ActionUpdate)
	if err != nil {
		writeInternalPRError(w, err)
		return
	}
	number, err := pullRequestNumber(r)
	if err != nil {
		writeInternalPRError(w, err)
		return
	}
	pr, err := apiServer.Store.GetGitPullRequest(r.Context(), repo.ID, number)
	if err != nil {
		writeInternalPRError(w, err)
		return
	}
	if pr.State != types.GitPullRequestStateOpen {
		writeInternalPRError(w, statusErr(http.StatusConflict, "pull request is not open"))
		return
	}
	task, err := apiServer.specTaskForInternalPR(r.Context(), repo, pr)
	if err != nil {
		writeInternalPRError(w, err)
		return
	}
	if task == nil {
		writeInternalPRError(w, statusErr(http.StatusConflict, "no spec task owns this pull request"))
		return
	}
	message, err := prompts.RebaseRequiredInstruction(pr.HeadBranch, pr.BaseBranch)
	if err != nil {
		writeInternalPRError(w, err)
		return
	}
	if err := apiServer.enqueueSpecTaskAgentMessage(r.Context(), task, message, false, user.ID); err != nil {
		writeInternalPRError(w, err)
		return
	}
	detail, err := apiServer.internalPRDetail(r.Context(), repo, number)
	if err != nil {
		writeInternalPRError(w, err)
		return
	}
	writeResponse(w, detail, http.StatusOK)
}
