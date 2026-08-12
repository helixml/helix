package webservice

import (
	"context"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// GitSyncer is the slice of the git-repository service the CD watcher needs:
// pull an external repo's branch into the local mirror, then read its HEAD.
type GitSyncer interface {
	SyncBaseBranch(ctx context.Context, repoID, branch string) error
	GetLocalBranchSHA(ctx context.Context, repoID, branch string) (string, error)
}

// Redeployer is the slice of *Controller the watcher needs (mockable in tests).
type Redeployer interface {
	Redeploy(ctx context.Context, req DeployRequest) (*types.WebServiceDeploy, error)
}

// GitHubDeployWatcher gives agent-created apps continuous delivery: when a
// project's GitHub default branch advances (e.g. a PR is merged), it redeploys
// that project's web service.
//
// Helix's own git server fires onDefaultBranchPush for direct pushes, but
// GitHub-hosted repos only reach Helix via a pull/sync — so a PR merged on
// GitHub never triggers the push hook. This watcher closes that gap by polling
// the external default branch and redeploying when it moves. It acts ONLY on
// external repos; Helix-hosted repos already auto-deploy via the post-receive
// hook and are skipped to avoid double-deploys.
type GitHubDeployWatcher struct {
	store      store.Store
	controller Redeployer
	git        GitSyncer

	interval time.Duration

	mu       sync.Mutex
	lastSeen map[string]string // projectID -> last default-branch SHA deployed/baselined
}

// NewGitHubDeployWatcher polls every 60s — fast enough that "merge a PR → site
// updates" feels immediate next to a build/deploy, without hammering GitHub.
func NewGitHubDeployWatcher(s store.Store, c Redeployer, g GitSyncer) *GitHubDeployWatcher {
	return &GitHubDeployWatcher{
		store:      s,
		controller: c,
		git:        g,
		interval:   60 * time.Second,
		lastSeen:   map[string]string{},
	}
}

// Start runs the watcher on a ticker until ctx is cancelled.
func (w *GitHubDeployWatcher) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	log.Info().Dur("interval", w.interval).Msg("web-service GitHub CD watcher started")
	for {
		w.runOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *GitHubDeployWatcher) runOnce(ctx context.Context) {
	states, err := w.store.ListActiveWebServices(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("github-cd: list active web services failed")
		return
	}
	for _, st := range states {
		w.checkOne(ctx, st.ProjectID)
	}
}

func (w *GitHubDeployWatcher) checkOne(ctx context.Context, projectID string) {
	project, err := w.store.GetProject(ctx, projectID)
	if err != nil || project.DefaultRepoID == "" {
		return
	}
	repo, err := w.store.GetGitRepository(ctx, project.DefaultRepoID)
	if err != nil {
		return
	}
	// Helix-hosted repos auto-deploy via the git post-receive hook; only
	// external (GitHub/GitLab/…) repos need polling.
	if !repo.IsExternal || repo.DefaultBranch == "" {
		return
	}
	branch := repo.DefaultBranch

	// Pull the latest default branch from the external remote into the mirror.
	if err := w.git.SyncBaseBranch(ctx, repo.ID, branch); err != nil {
		log.Debug().Err(err).Str("repo_id", repo.ID).Msg("github-cd: sync failed, will retry next tick")
		return
	}
	head, err := w.git.GetLocalBranchSHA(ctx, repo.ID, branch)
	if err != nil || head == "" {
		return
	}

	baseline := w.baseline(ctx, projectID, head)
	if head == baseline {
		return
	}

	log.Info().Str("project_id", projectID).
		Str("from", shortSHA(baseline)).Str("to", shortSHA(head)).
		Msg("github-cd: default branch advanced — redeploying web service")
	if _, err := w.controller.Redeploy(ctx, DeployRequest{
		ProjectID: projectID,
		Owner:     project.UserID,
		CommitSHA: head,
	}); err != nil {
		log.Error().Err(err).Str("project_id", projectID).Msg("github-cd: redeploy failed")
		return
	}
	w.setSeen(projectID, head)
}

// baseline returns the SHA to compare HEAD against. On first sight it seeds from
// the last deployed commit (so a merge that landed while the API was down is
// still caught), falling back to the current HEAD when nothing was recorded —
// so we never redeploy an app that's already on HEAD (e.g. right after a manual
// deploy, which records no SHA).
func (w *GitHubDeployWatcher) baseline(ctx context.Context, projectID, head string) string {
	w.mu.Lock()
	if s, ok := w.lastSeen[projectID]; ok {
		w.mu.Unlock()
		return s
	}
	w.mu.Unlock()

	seed := w.lastDeployedSHA(ctx, projectID)
	if seed == "" {
		seed = head
	}
	w.setSeen(projectID, seed)
	return seed
}

func (w *GitHubDeployWatcher) setSeen(projectID, sha string) {
	w.mu.Lock()
	w.lastSeen[projectID] = sha
	w.mu.Unlock()
}

// lastDeployedSHA returns the commit SHA of the most recent live/superseded
// deploy that recorded one (watcher-driven deploys always do), or "".
func (w *GitHubDeployWatcher) lastDeployedSHA(ctx context.Context, projectID string) string {
	deploys, err := w.store.ListWebServiceDeploys(ctx, projectID, 20)
	if err != nil {
		return ""
	}
	for _, d := range deploys {
		if d.CommitSHA != "" &&
			(d.Status == types.WebServiceDeployStatusLive || d.Status == types.WebServiceDeployStatusSuperseded) {
			return d.CommitSHA
		}
	}
	return ""
}

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
