# GitHub OAuth Integration Testing

## Setup

1. **Create GitHub OAuth App** (if not done already):
   - Go to GitHub Settings → Developer settings → OAuth Apps → New OAuth App
   - Homepage URL: `http://localhost:8080`
   - Authorization callback URL: `http://localhost:8080/api/v1/oauth/callback/github`

2. **Create Test GitHub Account**:
   - Create account like `helix-test-github`
   - **DO NOT enable 2FA** (for CI automation)
   - Generate a Personal Access Token with scopes: `repo`, `user:read`, `user:email`

3. **Set Environment Variable**:
   ```bash
   export GITHUB_TEST_TOKEN="ghp_your_personal_access_token_here"
   ```

## Running Tests

```bash
# Run just the GitHub integration test
go test ./pkg/skills/ -v -run TestGitHubSkillIntegration

# Run all skills tests including integration
go test ./pkg/skills/ -v

# Skip integration tests (default in CI)
go test ./pkg/skills/ -v -short
```

## What the Test Does

1. **Setup Phase**:
   - Authenticates with GitHub using your token
   - Creates test repositories: `helix-test-public` and `helix-test-private`
   - Adds test issues, releases, and tags to each repo

2. **Test Phase**:
   - Tests listing repositories
   - Tests getting specific repositories  
   - Tests listing and creating issues
   - Tests listing releases
   - Validates all GitHub API operations work

3. **Cleanup** (optional):
   - Uncomment the cleanup code to remove test repositories after testing

## Expected Output

```
=== RUN   TestGitHubSkillIntegration
{"level":"info","username":"helix-test-github","user_id":"12345","message":"GitHub test setup authenticated"}
{"level":"info","message":"Setting up GitHub test repositories"}
{"level":"info","repo_name":"helix-test-public","message":"Creating repository"}
{"level":"info","repo_name":"helix-test-private","message":"Creating repository"}
=== RUN   TestGitHubSkillIntegration/ListRepositories
{"level":"info","message":"Testing GitHub skill: List repositories"}
=== RUN   TestGitHubSkillIntegration/GetRepository
{"level":"info","message":"Testing GitHub skill: Get repository"}
=== RUN   TestGitHubSkillIntegration/ListIssues
{"level":"info","message":"Testing GitHub skill: List issues"}
=== RUN   TestGitHubSkillIntegration/CreateIssue
{"level":"info","message":"Testing GitHub skill: Create issue"}
=== RUN   TestGitHubSkillIntegration/ListReleases
{"level":"info","message":"Testing GitHub skill: List releases"}
--- PASS: TestGitHubSkillIntegration (5.23s)
```

## Troubleshooting

- **Token Issues**: Make sure your PAT has the right scopes (`repo`, `user:read`, `user:email`)
- **Rate Limiting**: GitHub API has rate limits - wait a few minutes if you hit them
- **Repository Exists**: Test will reuse existing test repositories if they exist
- **Cleanup**: Manually delete `helix-test-public` and `helix-test-private` repos if needed 