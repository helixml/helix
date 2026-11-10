package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// Helix's GitHub-compatible API: a GitHub REST v3 subset at
// /api/github/api/v3 for Helix-hosted repositories, so the go-github code path
// that drives GitHub repos drives Helix-hosted repos too.
//
// External callers authenticate with a Helix API key in the Authorization
// header (go-github sends PATs that way). Cookies are not accepted: this
// surface sits outside the CSRF middleware. Helix itself calls the same
// router in-process (newInternalCompatDispatch) with the acting user in the
// request context.
//
// Authorization matches the /api/v1/git/repositories handlers: reads need
// ActionGet on the repository, writes and merges need ActionUpdate.

const githubCompatPrefix = "/api/github/api/v3"

func (apiServer *HelixAPIServer) registerGitHubCompatRoutes(router *mux.Router) {
	compat := router.PathPrefix(githubCompatPrefix).Subrouter()
	compat.HandleFunc("/repos/{owner}/{repo}", apiServer.compatGetRepository).Methods(http.MethodGet)
	compat.HandleFunc("/repos/{owner}/{repo}/pulls", apiServer.compatListPullRequests).Methods(http.MethodGet)
	compat.HandleFunc("/repos/{owner}/{repo}/pulls", apiServer.compatCreatePullRequest).Methods(http.MethodPost)
	compat.HandleFunc("/repos/{owner}/{repo}/pulls/{number}", apiServer.compatGetPullRequest).Methods(http.MethodGet)
	compat.HandleFunc("/repos/{owner}/{repo}/pulls/{number}", apiServer.compatEditPullRequest).Methods(http.MethodPatch)
	compat.HandleFunc("/repos/{owner}/{repo}/pulls/{number}/files", apiServer.compatListPullRequestFiles).Methods(http.MethodGet)
	compat.HandleFunc("/repos/{owner}/{repo}/pulls/{number}/comments", apiServer.compatListPullRequestComments).Methods(http.MethodGet)
	compat.HandleFunc("/repos/{owner}/{repo}/pulls/{number}/merge", apiServer.compatMergePullRequest).Methods(http.MethodPut)
	compat.HandleFunc("/repos/{owner}/{repo}/hooks", apiServer.compatListHooks).Methods(http.MethodGet)
	compat.HandleFunc("/repos/{owner}/{repo}/hooks", apiServer.compatUpsertHook).Methods(http.MethodPost)
	compat.HandleFunc("/repos/{owner}/{repo}/hooks/{hookID}", apiServer.compatUpsertHook).Methods(http.MethodPatch)
	compat.HandleFunc("/repos/{owner}/{repo}/commits/{sha}/status", apiServer.compatCommitStatus).Methods(http.MethodGet)
	compat.HandleFunc("/repos/{owner}/{repo}/commits/{sha}/check-runs", apiServer.compatCheckRuns).Methods(http.MethodGet)
	compat.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		compatError(w, http.StatusNotFound, "Not Found")
	})
	apiServer.githubCompatRouter = compat
}

// newInternalCompatDispatch serves Helix's own go-github calls in-process.
// The router is read at call time, after registerRoutes has run.
func (apiServer *HelixAPIServer) newInternalCompatDispatch() services.InternalCompatDispatch {
	return func(ctx context.Context, req *http.Request, user types.User) *http.Response {
		recorder := httptest.NewRecorder()
		apiServer.githubCompatRouter.ServeHTTP(recorder, req.WithContext(setRequestUser(ctx, user)))
		return recorder.Result()
	}
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

type compatErrorJSON struct {
	Message          string `json:"message"`
	DocumentationURL string `json:"documentation_url"`
}

// httpStatusError carries an HTTP status for both the compat API and the
// /api/v1 pull request handlers.
type httpStatusError struct {
	status  int
	message string
}

func (e *httpStatusError) Error() string { return e.message }

func statusErr(status int, format string, args ...any) error {
	return &httpStatusError{status: status, message: fmt.Sprintf(format, args...)}
}

// internalPRErrorStatus maps an internal pull request error to an HTTP status
// and GitHub-style message.
func internalPRErrorStatus(err error) (int, string) {
	var se *httpStatusError
	switch {
	case errors.As(err, &se):
		return se.status, se.message
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound, "Not Found"
	case errors.Is(err, services.ErrInternalPRNotMergeable),
		errors.Is(err, services.ErrInternalPRMergeMethod),
		errors.Is(err, services.ErrInternalPRClosed):
		return http.StatusMethodNotAllowed, err.Error()
	default:
		log.Error().Err(err).Msg("internal pull request request failed")
		return http.StatusInternalServerError, "Internal Server Error"
	}
}

func compatError(w http.ResponseWriter, status int, message string) {
	writeCompatStatus(w, status, compatErrorJSON{Message: message, DocumentationURL: "https://docs.github.com/rest"})
}

func writeCompatError(w http.ResponseWriter, err error) {
	status, message := internalPRErrorStatus(err)
	compatError(w, status, message)
}

func writeCompat(w http.ResponseWriter, body any) { writeCompatStatus(w, http.StatusOK, body) }

func writeCompatStatus(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Error().Err(err).Msg("github-compat: encode response")
	}
}

// ---------------------------------------------------------------------------
// Auth and repository resolution
// ---------------------------------------------------------------------------

// compatRequest resolves the repository addressed by {owner}/{repo} and
// authorizes the caller for action.
func (apiServer *HelixAPIServer) compatRequest(r *http.Request, action types.Action) (*types.GitRepository, *types.User, error) {
	user := getRequestUser(r) // set only by the in-process dispatch
	if !hasUser(user) {
		token := getBearerToken(r)
		if token == "" {
			return nil, nil, statusErr(http.StatusUnauthorized, "Requires authentication")
		}
		var err error
		user, err = apiServer.authMiddleware.getUserFromToken(r.Context(), token)
		if err != nil || !hasUser(user) {
			return nil, nil, statusErr(http.StatusUnauthorized, "Bad credentials")
		}
	}
	vars := mux.Vars(r)
	repo, err := apiServer.resolveCompatRepo(r.Context(), vars["owner"], vars["repo"])
	if err != nil {
		return nil, nil, err
	}
	if err := apiServer.authorizeUserToRepository(r.Context(), user, repo, action); err != nil {
		// GitHub hides repositories the caller cannot see.
		if action == types.ActionGet {
			return nil, nil, statusErr(http.StatusNotFound, "Not Found")
		}
		return nil, nil, statusErr(http.StatusForbidden, "Must have push access to repository")
	}
	return repo, user, nil
}

func (apiServer *HelixAPIServer) resolveCompatRepo(ctx context.Context, owner, name string) (*types.GitRepository, error) {
	query := &types.ListGitRepositoriesRequest{}
	if userID, ok := strings.CutPrefix(owner, "u-"); ok {
		query.OwnerID = userID
	} else {
		org, err := apiServer.lookupOrg(ctx, owner)
		if err != nil {
			return nil, statusErr(http.StatusNotFound, "Not Found")
		}
		query.OrganizationID = org.ID
	}
	repos, err := apiServer.Store.ListGitRepositories(ctx, query)
	if err != nil {
		return nil, err
	}
	var match *types.GitRepository
	for _, r := range repos {
		if r.Name != name || r.ExternalURL != "" {
			continue
		}
		if query.OwnerID != "" && r.OrganizationID != "" {
			continue // org repos are addressed by org
		}
		if match != nil {
			return nil, statusErr(http.StatusConflict, "more than one Helix repository is named %q under %q", name, owner)
		}
		match = r
	}
	if match == nil {
		return nil, statusErr(http.StatusNotFound, "Not Found")
	}
	return match, nil
}

func compatNumber(r *http.Request) (int, error) {
	n, err := strconv.Atoi(mux.Vars(r)["number"])
	if err != nil {
		return 0, statusErr(http.StatusNotFound, "Not Found")
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func (apiServer *HelixAPIServer) compatGetRepository(w http.ResponseWriter, r *http.Request) {
	repo, user, err := apiServer.compatRequest(r, types.ActionGet)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	view, err := apiServer.newCompatView(r.Context(), repo)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	canPush := apiServer.authorizeUserToRepository(r.Context(), user, repo, types.ActionUpdate) == nil
	out := view.repo()
	out.Permissions = &compatPermissionsJSON{Pull: true, Push: canPush}
	writeCompat(w, out)
}

func (apiServer *HelixAPIServer) compatListPullRequests(w http.ResponseWriter, r *http.Request) {
	repo, _, err := apiServer.compatRequest(r, types.ActionGet)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	state := r.URL.Query().Get("state")
	if state == "" {
		state = "open"
	}
	prs, err := apiServer.listInternalPRs(r.Context(), repo, state)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	head := r.URL.Query().Get("head")
	if _, branch, ok := strings.Cut(head, ":"); ok {
		head = branch
	}
	base := r.URL.Query().Get("base")
	view, err := apiServer.newCompatView(r.Context(), repo)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	out := []*compatPullRequestJSON{}
	for _, pr := range prs {
		if (head == "" || pr.HeadBranch == head) && (base == "" || pr.BaseBranch == base) {
			out = append(out, view.pullRequest(pr, nil))
		}
	}
	writeCompat(w, out)
}

func (apiServer *HelixAPIServer) compatCreatePullRequest(w http.ResponseWriter, r *http.Request) {
	repo, user, err := apiServer.compatRequest(r, types.ActionUpdate)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	var req struct {
		Title string `json:"title"`
		Body  string `json:"body"`
		Head  string `json:"head"`
		Base  string `json:"base"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		compatError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if _, branch, ok := strings.Cut(req.Head, ":"); ok {
		req.Head = branch
	}
	pr, err := apiServer.createInternalPR(r.Context(), repo, user, req.Title, req.Body, req.Head, req.Base)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	view, err := apiServer.newCompatView(r.Context(), repo)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	writeCompatStatus(w, http.StatusCreated, view.pullRequest(pr, nil))
}

func (apiServer *HelixAPIServer) compatGetPullRequest(w http.ResponseWriter, r *http.Request) {
	repo, _, err := apiServer.compatRequest(r, types.ActionGet)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	number, err := compatNumber(r)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	pr, mergeable, err := apiServer.getInternalPR(r.Context(), repo, number, true)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	if strings.Contains(r.Header.Get("Accept"), "diff") {
		diff, err := apiServer.gitHTTPServer.InternalPullRequestDiff(r.Context(), repo, pr)
		if err != nil {
			writeCompatError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(diff))
		return
	}
	view, err := apiServer.newCompatView(r.Context(), repo)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	writeCompat(w, view.pullRequest(pr, mergeable))
}

func (apiServer *HelixAPIServer) compatEditPullRequest(w http.ResponseWriter, r *http.Request) {
	repo, _, err := apiServer.compatRequest(r, types.ActionUpdate)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	number, err := compatNumber(r)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	var update types.GitPullRequestUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		compatError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	pr, err := apiServer.updateInternalPR(r.Context(), repo, number, &update)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	view, err := apiServer.newCompatView(r.Context(), repo)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	writeCompat(w, view.pullRequest(pr, nil))
}

func (apiServer *HelixAPIServer) compatListPullRequestFiles(w http.ResponseWriter, r *http.Request) {
	repo, _, err := apiServer.compatRequest(r, types.ActionGet)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	number, err := compatNumber(r)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	pr, _, err := apiServer.getInternalPR(r.Context(), repo, number, false)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	files, err := apiServer.gitHTTPServer.InternalPRFileChanges(r.Context(), repo, pr)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	if files == nil {
		files = []*types.InternalPRFile{}
	}
	writeCompat(w, files)
}

// compatListPullRequestComments: internal PRs have no review comments by
// design; feedback goes through the task chat.
func (apiServer *HelixAPIServer) compatListPullRequestComments(w http.ResponseWriter, r *http.Request) {
	repo, _, err := apiServer.compatRequest(r, types.ActionGet)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	number, err := compatNumber(r)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	if _, err := apiServer.Store.GetGitPullRequest(r.Context(), repo.ID, number); err != nil {
		writeCompatError(w, err)
		return
	}
	writeCompat(w, []struct{}{})
}

func (apiServer *HelixAPIServer) compatMergePullRequest(w http.ResponseWriter, r *http.Request) {
	repo, user, err := apiServer.compatRequest(r, types.ActionUpdate)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	number, err := compatNumber(r)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	var req types.GitPullRequestMergeRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			compatError(w, http.StatusBadRequest, "Problems parsing JSON")
			return
		}
	}
	pr, err := apiServer.gitHTTPServer.MergeInternalPullRequest(r.Context(), repo, number, req.MergeMethod, req.CommitTitle, req.CommitMessage, user.ID)
	if err != nil {
		writeCompatError(w, err)
		return
	}
	writeCompat(w, compatMergeResultJSON{SHA: pr.MergeCommit, Merged: true, Message: "Pull Request successfully merged"})
}

// Hooks are accepted and dropped: merge state is internal, so nothing needs
// delivering. go-github's webhook upsert lists then creates or edits.
func (apiServer *HelixAPIServer) compatListHooks(w http.ResponseWriter, r *http.Request) {
	if _, _, err := apiServer.compatRequest(r, types.ActionGet); err != nil {
		writeCompatError(w, err)
		return
	}
	writeCompat(w, []compatHookJSON{})
}

func (apiServer *HelixAPIServer) compatUpsertHook(w http.ResponseWriter, r *http.Request) {
	if _, _, err := apiServer.compatRequest(r, types.ActionUpdate); err != nil {
		writeCompatError(w, err)
		return
	}
	var hook compatHookJSON
	if err := json.NewDecoder(r.Body).Decode(&hook); err != nil {
		compatError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	hook.ID = 1
	hook.Active = true
	status := http.StatusOK
	if r.Method == http.MethodPost {
		status = http.StatusCreated
	}
	writeCompatStatus(w, status, hook)
}

// Helix-hosted repositories have no external CI.
func (apiServer *HelixAPIServer) compatCommitStatus(w http.ResponseWriter, r *http.Request) {
	if _, _, err := apiServer.compatRequest(r, types.ActionGet); err != nil {
		writeCompatError(w, err)
		return
	}
	writeCompat(w, compatCombinedStatusJSON{SHA: mux.Vars(r)["sha"], State: "pending", Statuses: []struct{}{}})
}

func (apiServer *HelixAPIServer) compatCheckRuns(w http.ResponseWriter, r *http.Request) {
	if _, _, err := apiServer.compatRequest(r, types.ActionGet); err != nil {
		writeCompatError(w, err)
		return
	}
	writeCompat(w, compatCheckRunsJSON{CheckRuns: []struct{}{}})
}

// ---------------------------------------------------------------------------
// Rendering in the go-github shape
// ---------------------------------------------------------------------------

type compatUserJSON struct {
	Login string `json:"login"`
	ID    int64  `json:"id"`
	Type  string `json:"type"`
}

type compatPermissionsJSON struct {
	Pull bool `json:"pull"`
	Push bool `json:"push"`
}

type compatRepoJSON struct {
	ID            int64                  `json:"id"`
	Name          string                 `json:"name"`
	FullName      string                 `json:"full_name"`
	Private       bool                   `json:"private"`
	Owner         compatUserJSON         `json:"owner"`
	HTMLURL       string                 `json:"html_url"`
	URL           string                 `json:"url"`
	Description   string                 `json:"description,omitempty"`
	DefaultBranch string                 `json:"default_branch"`
	Permissions   *compatPermissionsJSON `json:"permissions,omitempty"`
}

type compatBranchJSON struct {
	Label string          `json:"label"`
	Ref   string          `json:"ref"`
	SHA   string          `json:"sha"`
	Repo  *compatRepoJSON `json:"repo"`
}

type compatPullRequestJSON struct {
	ID             int64            `json:"id"`
	Number         int              `json:"number"`
	State          string           `json:"state"`
	Title          string           `json:"title"`
	Body           string           `json:"body"`
	User           compatUserJSON   `json:"user"`
	URL            string           `json:"url"`
	HTMLURL        string           `json:"html_url"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	ClosedAt       *time.Time       `json:"closed_at"`
	MergedAt       *time.Time       `json:"merged_at"`
	Merged         bool             `json:"merged"`
	Mergeable      *bool            `json:"mergeable"`
	MergeCommitSHA string           `json:"merge_commit_sha,omitempty"`
	Head           compatBranchJSON `json:"head"`
	Base           compatBranchJSON `json:"base"`
}

type compatMergeResultJSON struct {
	SHA     string `json:"sha"`
	Merged  bool   `json:"merged"`
	Message string `json:"message"`
}

type compatHookJSON struct {
	ID     int64             `json:"id"`
	Name   string            `json:"name"`
	Active bool              `json:"active"`
	Events []string          `json:"events"`
	Config map[string]string `json:"config"`
}

type compatCombinedStatusJSON struct {
	SHA        string     `json:"sha"`
	State      string     `json:"state"`
	TotalCount int        `json:"total_count"`
	Statuses   []struct{} `json:"statuses"`
}

type compatCheckRunsJSON struct {
	TotalCount int        `json:"total_count"`
	CheckRuns  []struct{} `json:"check_runs"`
}

// compatView renders one repository's objects.
type compatView struct {
	baseURL string
	owner   string
	repoRow *types.GitRepository
}

func (apiServer *HelixAPIServer) newCompatView(ctx context.Context, repo *types.GitRepository) (*compatView, error) {
	owner, err := services.InternalCompatOwner(ctx, apiServer.Store, repo)
	if err != nil {
		return nil, err
	}
	return &compatView{baseURL: strings.TrimSuffix(apiServer.Cfg.WebServer.URL, "/"), owner: owner, repoRow: repo}, nil
}

func compatIntID(key string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return int64(h.Sum64() >> 1)
}

func (v *compatView) apiURL() string {
	return fmt.Sprintf("%s%s/repos/%s/%s", v.baseURL, githubCompatPrefix, v.owner, v.repoRow.Name)
}

// internalPRHTMLURL is the Helix UI page of a pull request (number > 0) or of
// the repository's pull request tab.
func internalPRHTMLURL(baseURL string, repo *types.GitRepository, owner string, number int) string {
	if repo.OrganizationID == "" {
		// Personal repositories have no UI page; link the API resource.
		if number > 0 {
			return fmt.Sprintf("%s%s/repos/%s/%s/pulls/%d", baseURL, githubCompatPrefix, owner, repo.Name, number)
		}
		return fmt.Sprintf("%s%s/repos/%s/%s", baseURL, githubCompatPrefix, owner, repo.Name)
	}
	u := fmt.Sprintf("%s/orgs/%s/git-repos/%s?tab=pull-requests", baseURL, owner, repo.ID)
	if number > 0 {
		u += "&pr=" + strconv.Itoa(number)
	}
	return u
}

func (v *compatView) repo() *compatRepoJSON {
	return &compatRepoJSON{
		ID:            compatIntID("repo:" + v.repoRow.ID),
		Name:          v.repoRow.Name,
		FullName:      v.owner + "/" + v.repoRow.Name,
		Private:       true,
		Owner:         compatUserJSON{Login: v.owner, ID: compatIntID("owner:" + v.owner), Type: "Organization"},
		HTMLURL:       internalPRHTMLURL(v.baseURL, v.repoRow, v.owner, 0),
		URL:           v.apiURL(),
		Description:   v.repoRow.Description,
		DefaultBranch: v.repoRow.DefaultBranch,
	}
}

func (v *compatView) pullRequest(pr *types.GitPullRequest, mergeable *bool) *compatPullRequestJSON {
	repo := v.repo()
	login := pr.AuthorLogin
	if login == "" {
		login = "helix"
	}
	return &compatPullRequestJSON{
		ID:             compatIntID("pr:" + pr.ID),
		Number:         pr.Number,
		State:          string(pr.State),
		Title:          pr.Title,
		Body:           pr.Body,
		User:           compatUserJSON{Login: login, ID: compatIntID("user:" + pr.AuthorID), Type: "User"},
		URL:            fmt.Sprintf("%s/pulls/%d", v.apiURL(), pr.Number),
		HTMLURL:        internalPRHTMLURL(v.baseURL, v.repoRow, v.owner, pr.Number),
		CreatedAt:      pr.CreatedAt,
		UpdatedAt:      pr.UpdatedAt,
		ClosedAt:       pr.ClosedAt,
		MergedAt:       pr.MergedAt,
		Merged:         pr.Merged,
		Mergeable:      mergeable,
		MergeCommitSHA: pr.MergeCommit,
		Head:           compatBranchJSON{Label: v.owner + ":" + pr.HeadBranch, Ref: pr.HeadBranch, SHA: pr.HeadSHA, Repo: repo},
		Base:           compatBranchJSON{Label: v.owner + ":" + pr.BaseBranch, Ref: pr.BaseBranch, SHA: pr.BaseSHA, Repo: repo},
	}
}
