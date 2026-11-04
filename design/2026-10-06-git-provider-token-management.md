# Saved Git-provider token management

Related issue: https://github.com/helixml/helix/issues/3367

## Granular scope

- Expose saved-connection management from the repository source list and saved-PAT browser, including after authentication errors.
- Reuse the existing PAT form to replace a token through `PUT /api/v1/git-provider-connections/{id}`. Preserve the connection ID, provider, instance URL, authentication username and creation time.
- Validate authentication and repository browsing with the provider before encrypting and persisting the replacement. A failed replacement leaves the existing credential untouched.
- Keep `Token` and `AuthUsername` excluded from responses. The replacement input starts empty and has no stored-token reveal control.
- Remove means disconnect the saved credential from Helix, not revoke it at the external provider. Linked repository credential copies are explicitly outside this change; both UI flows explain this limitation.

## Verification

Passed against the final code:

- `cd frontend && npx --no-install vitest run src/components/project/BrowseProvidersDialog.test.tsx src/utils/oauthProviders.test.ts`: 45 tests passed. Isolated React tests exercise management reachability, same-ID replacement followed by browsing/link selection, failed replacement retry, clearing an unsaved replacement and disconnect followed by creation.
- `cd frontend && npx --yes --package yarn@1.22.22 yarn build`: production build passed, with bundle-size warnings.
- `CGO_ENABLED=1 GOMAXPROCS=4 go test -p 2 ./api/pkg/server -run 'TestGitProviderConnectionSuite|TestRedactGitRepositor' -count=1`: passed. Production handlers call local mock provider HTTP servers; Store persistence is mocked. Cases include same-ID replacement followed by browsing, encryption/redaction, invalid tokens and missing repository access, authorization, storage failures, encrypted Bitbucket username preservation and disconnect without provider contact.
- `CGO_ENABLED=1 GOMAXPROCS=4 go build -p 2 ./api/pkg/server ./api/pkg/store ./api/pkg/types`: passed.
- `git diff --check`: passed. Swagger/OpenAPI JSON comparisons confirm only the new PUT operation and request schema changed.

**WARNING: NOT tested live in Helix/GitLab or against Postgres.** No Helix dev stack is running here; `localhost:8080` belongs to another application. Isolated component tests and mocked-provider HTTP tests are not end-to-end evidence. Live create/use/delete, actual database persistence and newly linked repository operations remain unverified. No existing services were stopped or real credentials revoked. Publish as a draft until a maintainer verifies these flows in a running Helix environment.
