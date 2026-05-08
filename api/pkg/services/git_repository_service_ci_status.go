package services

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	azuredevops "github.com/helixml/helix/api/pkg/agent/skill/azure_devops"
	"github.com/helixml/helix/api/pkg/agent/skill/github"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
)

// GetCIStatus fetches the CI verdict for a PR's current head SHA. The
// returned types.CIStatus.State is one of CIStatusRunning / CIStatusPassed /
// CIStatusFailed / CIStatusNone. Empty headSHA → returns "none" without
// hitting any provider.
//
// The provider is determined by the repository's ExternalType. Bitbucket
// always returns "none" in v1 (CI integration not yet implemented).
//
// Errors here are non-fatal for the caller — the PR poll loop should log
// and continue rather than abort the whole loop.
func (s *GitRepositoryService) GetCIStatus(ctx context.Context, repoID, prID, headSHA string) (*types.CIStatus, error) {
	if headSHA == "" {
		return &types.CIStatus{State: CIStatusNone}, nil
	}

	repo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("repository not found: %w", err)
	}
	if repo.ExternalURL == "" {
		return nil, fmt.Errorf("repository is not external")
	}

	switch repo.ExternalType {
	case types.ExternalRepositoryTypeGitHub:
		return s.getGitHubCIStatus(ctx, repo, prID, headSHA)
	case types.ExternalRepositoryTypeGitLab:
		return s.getGitLabCIStatus(ctx, repo, headSHA)
	case types.ExternalRepositoryTypeADO:
		return s.getAzureDevOpsCIStatus(ctx, repo, headSHA)
	case types.ExternalRepositoryTypeBitbucket:
		// Stub in v1 — see Bitbucket client's GetCIStatus.
		return &types.CIStatus{State: CIStatusNone, HeadSHA: headSHA}, nil
	default:
		return &types.CIStatus{State: CIStatusNone, HeadSHA: headSHA}, nil
	}
}

func (s *GitRepositoryService) getGitHubCIStatus(ctx context.Context, repo *types.GitRepository, prID, headSHA string) (*types.CIStatus, error) {
	client, err := s.getGitHubClient(ctx, repo, "")
	if err != nil {
		return nil, err
	}
	owner, repoName, err := github.ParseGitHubURL(repo.ExternalURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GitHub URL: %w", err)
	}
	res, err := client.GetCIStatus(ctx, owner, repoName, headSHA)
	if err != nil {
		return nil, err
	}
	state := NormalizeCIStatus("github", res.Status)
	url := ""
	if state != CIStatusNone {
		// Link to the PR's "Checks" tab — directly clickable for users.
		if prNum, err := strconv.Atoi(prID); err == nil && prNum > 0 {
			url = fmt.Sprintf("https://github.com/%s/%s/pull/%d/checks", owner, repoName, prNum)
		}
	}
	return &types.CIStatus{State: state, URL: url, HeadSHA: headSHA}, nil
}

func (s *GitRepositoryService) getGitLabCIStatus(ctx context.Context, repo *types.GitRepository, headSHA string) (*types.CIStatus, error) {
	client, err := s.getGitLabClient(ctx, repo)
	if err != nil {
		return nil, err
	}
	projectID, err := s.getGitLabProjectID(ctx, client, repo)
	if err != nil {
		return nil, err
	}
	res, err := client.GetCIStatus(ctx, projectID, headSHA)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return &types.CIStatus{State: CIStatusNone, HeadSHA: headSHA}, nil
	}
	return &types.CIStatus{
		State:   NormalizeCIStatus("gitlab", res.Status),
		URL:     res.URL,
		HeadSHA: headSHA,
	}, nil
}

func (s *GitRepositoryService) getAzureDevOpsCIStatus(ctx context.Context, repo *types.GitRepository, headSHA string) (*types.CIStatus, error) {
	client, err := s.getAzureDevOpsClient(ctx, repo)
	if err != nil {
		return nil, err
	}
	project, err := s.getAzureDevOpsProject(repo)
	if err != nil {
		return nil, err
	}
	repoName, err := s.getAzureDevOpsRepositoryName(repo)
	if err != nil {
		return nil, err
	}
	res, err := client.GetCIStatus(ctx, project, repoName, headSHA)
	if err != nil {
		// PAT missing the Build Read scope: gracefully degrade to "none"
		// so existing connections without vso.build don't break the UI.
		// Log once at warn level — admins should re-issue the PAT.
		if errors.Is(err, azuredevops.ErrCIScopeMissing) {
			log.Warn().
				Str("repo_id", repo.ID).
				Msg("Azure DevOps PAT missing vso.build scope; CI status hidden for this repo")
			return &types.CIStatus{State: CIStatusNone, HeadSHA: headSHA}, nil
		}
		return nil, err
	}
	if res == nil {
		return &types.CIStatus{State: CIStatusNone, HeadSHA: headSHA}, nil
	}
	return &types.CIStatus{
		State:   NormalizeCIStatus("azure_devops", res.Status),
		URL:     res.URL,
		HeadSHA: headSHA,
	}, nil
}
