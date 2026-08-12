# GitLab Per-User OAuth Attribution

## Customer Problem

GitLab repositories configured with a shared personal access token use that credential for upstream pushes and merge-request API calls. GitLab therefore shows the repository setup user as the push and merge-request actor, even when another Helix user approved the work.

Commit identity is separate. Helix can set the commit author's name and email in Git, but those fields do not choose the authenticated GitLab account used for transport or API requests. Correct customer-visible attribution requires the acting user's GitLab credential for both the push and merge-request creation.

## Existing Support and Gap

Helix already stores OAuth providers and per-user OAuth connections, including GitLab and custom/self-hosted providers. Repository-level GitLab OAuth or PAT credentials remain necessary for unattended repository operations.

The acting-user path was GitHub-only:

- OAuth validation required a per-user connection only for GitHub.
- Git credential lookup preferred a GitHub acting-user token but not a GitLab token.
- GitHub pull-request creation accepted the acting user, while GitLab merge-request creation always used repository credentials.
- Task UI OAuth recovery assumed every `oauth_required` response meant GitHub.

## Implemented Flow

- Map GitHub and GitLab repositories to their OAuth provider types and require a matching per-user OAuth connection for user-initiated planning, approval, push, and PR/MR actions.
- Prefer the acting user's OAuth token for upstream Git transport. GitLab uses username `oauth2` with that token.
- Pass the acting user into GitLab merge-request creation and construct the GitLab API client with that user's bearer token.
- Accept matching custom providers by provider name for self-hosted GitHub and GitLab.
- Preserve automated behavior: an empty acting-user ID skips per-user enforcement and falls back to the repository OAuth connection or PAT.
- Generalize frontend `oauth_required` handling across task planning, design approval, implementation approval, detail, board, and tab views. The backend `provider_type` selects GitHub or GitLab connection copy, provider, and scopes.
- Add a GitLab admin-provider template using GitLab authorization, token, and user-info endpoints.
- Request GitLab scopes `api`, `read_repository`, `write_repository`, and `read_user`. The `api` scope is required for merge-request API creation.

No schema change is required. The feature uses the existing OAuth provider, OAuth connection, and repository credential fields.

## Verification

```text
cd /Users/psamuel/helix/helix-worktrees/gitlab-oauth-attribution/api
go test ./pkg/services -run 'Test(GetCredentialsForRepo_GitLabUserOAuthTakesPrecedence|CreateGitLabMergeRequest_UserOAuthTakesPrecedence|GetGitLabClient_UserWithoutOAuthReturnsError|GetGitLabClient_AgentFallsBackToRepoCredentials|ValidateUserOAuth_GitLabRequiresMatchingConnection|ValidateUserOAuth_AcceptsCustomGitLabProvider|PushBranchToRemote_GitLabUserWithoutOAuthReturnsErrorBeforePush)' -count=1
ok github.com/helixml/helix/api/pkg/services 0.568s

cd /Users/psamuel/helix/helix-worktrees/gitlab-oauth-attribution/frontend
yarn test src/utils/oauthProviders.test.ts --run
Test Files 1 passed (1); Tests 39 passed (39)

yarn build
vite: 21709 modules transformed; build completed in 16.75s
```

The backend tests verify acting-token precedence for Git transport and GitLab API calls, missing-user rejection before push, custom GitLab provider matching, and empty-user repository-credential fallback. The frontend test verifies inclusion of the GitLab `api` scope.

## Remaining Live Verification

NOT live tested: user-attributed upstream push and user-attributed merge-request creation require completing a task against a GitLab repository with a connected user OAuth account.
