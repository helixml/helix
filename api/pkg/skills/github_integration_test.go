package skills

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v57/github"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

const (
	// Test repository names
	testPublicRepo  = "helix-test-public"
	testPrivateRepo = "helix-test-private"

	// Test data
	testIssueTitle  = "Test Issue for Helix Integration"
	testIssueBody   = "This is a test issue created by Helix integration tests"
	testPRTitle     = "Test PR for Helix Integration"
	testPRBody      = "This is a test PR created by Helix integration tests"
	testReleaseTag  = "v1.0.0-test"
	testReleaseName = "Test Release v1.0.0"
	testReleaseBody = "Test release created by Helix integration tests"
)

// GitHubTestSetup manages GitHub test environment setup
type GitHubTestSetup struct {
	client   *github.Client
	username string
	ctx      context.Context
}

// NewGitHubTestSetup creates a new GitHub test setup manager
func NewGitHubTestSetup(ctx context.Context, token string) (*GitHubTestSetup, error) {
	if token == "" {
		return nil, fmt.Errorf("GitHub token is required for integration tests")
	}

	// Create OAuth2 token source
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	// Create GitHub client
	client := github.NewClient(tc)

	// Get authenticated user info
	user, _, err := client.Users.Get(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get authenticated user: %w", err)
	}

	log.Info().
		Str("username", user.GetLogin()).
		Str("user_id", fmt.Sprintf("%d", user.GetID())).
		Msg("GitHub test setup authenticated")

	return &GitHubTestSetup{
		client:   client,
		username: user.GetLogin(),
		ctx:      ctx,
	}, nil
}

// SetupTestRepositories ensures test repositories exist with required structure
func (gts *GitHubTestSetup) SetupTestRepositories() error {
	log.Info().Msg("Setting up GitHub test repositories")

	// Setup public repository
	if err := gts.setupRepository(testPublicRepo, false); err != nil {
		return fmt.Errorf("failed to setup public repository: %w", err)
	}

	// Setup private repository
	if err := gts.setupRepository(testPrivateRepo, true); err != nil {
		return fmt.Errorf("failed to setup private repository: %w", err)
	}

	log.Info().Msg("GitHub test repositories setup complete")
	return nil
}

// setupRepository creates or updates a repository with test content
func (gts *GitHubTestSetup) setupRepository(repoName string, private bool) error {
	log.Debug().
		Str("repo_name", repoName).
		Bool("private", private).
		Msg("Setting up repository")

	// Check if repository exists
	repo, resp, err := gts.client.Repositories.Get(gts.ctx, gts.username, repoName)
	if err != nil && resp.StatusCode != 404 {
		return fmt.Errorf("failed to check repository existence: %w", err)
	}

	// Create repository if it doesn't exist
	if resp.StatusCode == 404 {
		log.Info().
			Str("repo_name", repoName).
			Msg("Creating repository")

		repoReq := &github.Repository{
			Name:        github.String(repoName),
			Description: github.String(fmt.Sprintf("Test repository for Helix integration tests - %s", repoName)),
			Private:     github.Bool(private),
			AutoInit:    github.Bool(true),
		}

		repo, _, err = gts.client.Repositories.Create(gts.ctx, "", repoReq)
		if err != nil {
			return fmt.Errorf("failed to create repository: %w", err)
		}

		// Wait a moment for GitHub to initialize the repository
		time.Sleep(2 * time.Second)
	}

	// Setup repository content
	if err := gts.setupRepositoryContent(repo); err != nil {
		return fmt.Errorf("failed to setup repository content: %w", err)
	}

	return nil
}

// setupRepositoryContent adds test issues, PRs, and releases to the repository
func (gts *GitHubTestSetup) setupRepositoryContent(repo *github.Repository) error {
	repoName := repo.GetName()

	log.Debug().
		Str("repo_name", repoName).
		Msg("Setting up repository content")

	// Setup test issues
	if err := gts.setupTestIssues(repoName); err != nil {
		return fmt.Errorf("failed to setup test issues: %w", err)
	}

	// Setup test releases and tags
	if err := gts.setupTestReleases(repoName); err != nil {
		return fmt.Errorf("failed to setup test releases: %w", err)
	}

	// Setup test pull requests (only for public repo to avoid complexity)
	if repoName == testPublicRepo {
		if err := gts.setupTestPullRequests(repoName); err != nil {
			log.Warn().Err(err).Msg("Failed to setup test pull requests, continuing")
		}
	}

	return nil
}

// setupTestIssues creates test issues if they don't exist
func (gts *GitHubTestSetup) setupTestIssues(repoName string) error {
	log.Debug().
		Str("repo_name", repoName).
		Msg("Setting up test issues")

	// Check if test issue already exists
	issues, _, err := gts.client.Issues.ListByRepo(gts.ctx, gts.username, repoName, &github.IssueListByRepoOptions{
		State:       "all",
		ListOptions: github.ListOptions{PerPage: 100},
	})
	if err != nil {
		return fmt.Errorf("failed to list existing issues: %w", err)
	}

	// Check if our test issue already exists
	testIssueExists := false
	for _, issue := range issues {
		if strings.Contains(issue.GetTitle(), testIssueTitle) {
			testIssueExists = true
			break
		}
	}

	// Create test issue if it doesn't exist
	if !testIssueExists {
		log.Info().
			Str("repo_name", repoName).
			Str("issue_title", testIssueTitle).
			Msg("Creating test issue")

		issueReq := &github.IssueRequest{
			Title:  github.String(testIssueTitle),
			Body:   github.String(testIssueBody),
			Labels: &[]string{"test", "helix-integration"},
		}

		_, _, err := gts.client.Issues.Create(gts.ctx, gts.username, repoName, issueReq)
		if err != nil {
			return fmt.Errorf("failed to create test issue: %w", err)
		}
	}

	return nil
}

// setupTestReleases creates test releases and tags
func (gts *GitHubTestSetup) setupTestReleases(repoName string) error {
	log.Debug().
		Str("repo_name", repoName).
		Msg("Setting up test releases")

	// Check if test release already exists
	releases, _, err := gts.client.Repositories.ListReleases(gts.ctx, gts.username, repoName, &github.ListOptions{
		PerPage: 100,
	})
	if err != nil {
		return fmt.Errorf("failed to list existing releases: %w", err)
	}

	// Check if our test release already exists
	testReleaseExists := false
	for _, release := range releases {
		if release.GetTagName() == testReleaseTag {
			testReleaseExists = true
			break
		}
	}

	// Create test release if it doesn't exist
	if !testReleaseExists {
		log.Info().
			Str("repo_name", repoName).
			Str("release_tag", testReleaseTag).
			Msg("Creating test release")

		releaseReq := &github.RepositoryRelease{
			TagName:         github.String(testReleaseTag),
			Name:            github.String(testReleaseName),
			Body:            github.String(testReleaseBody),
			Draft:           github.Bool(false),
			Prerelease:      github.Bool(true), // Mark as prerelease to avoid confusion
			TargetCommitish: github.String("main"),
		}

		_, _, err := gts.client.Repositories.CreateRelease(gts.ctx, gts.username, repoName, releaseReq)
		if err != nil {
			return fmt.Errorf("failed to create test release: %w", err)
		}
	}

	return nil
}

// setupTestPullRequests creates test pull requests
func (gts *GitHubTestSetup) setupTestPullRequests(repoName string) error {
	log.Debug().
		Str("repo_name", repoName).
		Msg("Setting up test pull requests")

	// For simplicity, we'll skip PR creation as it requires creating branches
	// This can be expanded later if needed
	log.Debug().Msg("Skipping PR setup for now - requires branch management")

	return nil
}

// CleanupTestData removes test repositories (use with caution)
func (gts *GitHubTestSetup) CleanupTestData() error {
	log.Warn().Msg("Cleaning up GitHub test data")

	repos := []string{testPublicRepo, testPrivateRepo}
	for _, repoName := range repos {
		log.Debug().
			Str("repo_name", repoName).
			Msg("Deleting test repository")

		_, err := gts.client.Repositories.Delete(gts.ctx, gts.username, repoName)
		if err != nil {
			log.Error().Err(err).
				Str("repo_name", repoName).
				Msg("Failed to delete test repository")
		}
	}

	return nil
}

// Integration Tests

func TestGitHubSkillIntegration(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Get GitHub token from environment
	token := os.Getenv("GITHUB_TEST_TOKEN")
	if token == "" {
		t.Skip("GITHUB_TEST_TOKEN environment variable not set, skipping GitHub integration tests")
	}

	ctx := context.Background()

	// Setup GitHub test environment
	setup, err := NewGitHubTestSetup(ctx, token)
	require.NoError(t, err, "Failed to create GitHub test setup")

	// Setup test repositories and data
	err = setup.SetupTestRepositories()
	require.NoError(t, err, "Failed to setup test repositories")

	// Run the actual skill tests
	t.Run("ListRepositories", func(t *testing.T) {
		testGitHubSkillListRepositories(t, setup)
	})

	t.Run("GetRepository", func(t *testing.T) {
		testGitHubSkillGetRepository(t, setup)
	})

	t.Run("ListIssues", func(t *testing.T) {
		testGitHubSkillListIssues(t, setup)
	})

	t.Run("CreateIssue", func(t *testing.T) {
		testGitHubSkillCreateIssue(t, setup)
	})

	t.Run("ListReleases", func(t *testing.T) {
		testGitHubSkillListReleases(t, setup)
	})

	// Cleanup is optional - comment out to preserve test data for debugging
	// t.Cleanup(func() {
	//     _ = setup.CleanupTestData()
	// })
}

func testGitHubSkillListRepositories(t *testing.T, setup *GitHubTestSetup) {
	log.Info().Msg("Testing GitHub skill: List repositories")

	// List user repositories
	repos, _, err := setup.client.Repositories.List(setup.ctx, setup.username, &github.RepositoryListOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	})
	require.NoError(t, err, "Failed to list repositories")

	// Verify test repositories exist
	repoNames := make(map[string]bool)
	for _, repo := range repos {
		repoNames[repo.GetName()] = true
	}

	assert.True(t, repoNames[testPublicRepo], "Public test repository should exist")
	assert.True(t, repoNames[testPrivateRepo], "Private test repository should exist")

	log.Info().
		Int("repo_count", len(repos)).
		Msg("Successfully listed repositories")
}

func testGitHubSkillGetRepository(t *testing.T, setup *GitHubTestSetup) {
	log.Info().Msg("Testing GitHub skill: Get repository")

	// Get public repository
	repo, _, err := setup.client.Repositories.Get(setup.ctx, setup.username, testPublicRepo)
	require.NoError(t, err, "Failed to get public repository")

	assert.Equal(t, testPublicRepo, repo.GetName(), "Repository name should match")
	assert.False(t, repo.GetPrivate(), "Public repository should not be private")

	// Get private repository
	privateRepo, _, err := setup.client.Repositories.Get(setup.ctx, setup.username, testPrivateRepo)
	require.NoError(t, err, "Failed to get private repository")

	assert.Equal(t, testPrivateRepo, privateRepo.GetName(), "Repository name should match")
	assert.True(t, privateRepo.GetPrivate(), "Private repository should be private")

	log.Info().Msg("Successfully retrieved repositories")
}

func testGitHubSkillListIssues(t *testing.T, setup *GitHubTestSetup) {
	log.Info().Msg("Testing GitHub skill: List issues")

	// List issues from public repository
	issues, _, err := setup.client.Issues.ListByRepo(setup.ctx, setup.username, testPublicRepo, &github.IssueListByRepoOptions{
		State:       "all",
		ListOptions: github.ListOptions{PerPage: 100},
	})
	require.NoError(t, err, "Failed to list issues")

	// Verify test issue exists
	testIssueFound := false
	for _, issue := range issues {
		if strings.Contains(issue.GetTitle(), testIssueTitle) {
			testIssueFound = true
			assert.Equal(t, testIssueTitle, issue.GetTitle(), "Issue title should match")
			break
		}
	}

	assert.True(t, testIssueFound, "Test issue should exist")

	log.Info().
		Int("issue_count", len(issues)).
		Msg("Successfully listed issues")
}

func testGitHubSkillCreateIssue(t *testing.T, setup *GitHubTestSetup) {
	log.Info().Msg("Testing GitHub skill: Create issue")

	// Create a new test issue
	issueTitle := fmt.Sprintf("Integration Test Issue - %d", time.Now().Unix())
	issueBody := "This issue was created during integration testing"

	issueReq := &github.IssueRequest{
		Title:  github.String(issueTitle),
		Body:   github.String(issueBody),
		Labels: &[]string{"test", "integration", "auto-created"},
	}

	createdIssue, _, err := setup.client.Issues.Create(setup.ctx, setup.username, testPublicRepo, issueReq)
	require.NoError(t, err, "Failed to create test issue")

	assert.Equal(t, issueTitle, createdIssue.GetTitle(), "Created issue title should match")
	assert.Equal(t, issueBody, createdIssue.GetBody(), "Created issue body should match")
	assert.Greater(t, createdIssue.GetNumber(), 0, "Created issue should have a number")

	log.Info().
		Str("issue_title", createdIssue.GetTitle()).
		Int("issue_number", createdIssue.GetNumber()).
		Msg("Successfully created issue")

	// Close the test issue to keep things clean
	closeReq := &github.IssueRequest{
		State: github.String("closed"),
	}
	_, _, err = setup.client.Issues.Edit(setup.ctx, setup.username, testPublicRepo, createdIssue.GetNumber(), closeReq)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to close test issue")
	}
}

func testGitHubSkillListReleases(t *testing.T, setup *GitHubTestSetup) {
	log.Info().Msg("Testing GitHub skill: List releases")

	// List releases from public repository
	releases, _, err := setup.client.Repositories.ListReleases(setup.ctx, setup.username, testPublicRepo, &github.ListOptions{
		PerPage: 100,
	})
	require.NoError(t, err, "Failed to list releases")

	// Verify test release exists
	testReleaseFound := false
	for _, release := range releases {
		if release.GetTagName() == testReleaseTag {
			testReleaseFound = true
			assert.Equal(t, testReleaseName, release.GetName(), "Release name should match")
			assert.Equal(t, testReleaseTag, release.GetTagName(), "Release tag should match")
			break
		}
	}

	assert.True(t, testReleaseFound, "Test release should exist")

	log.Info().
		Int("release_count", len(releases)).
		Msg("Successfully listed releases")
}
