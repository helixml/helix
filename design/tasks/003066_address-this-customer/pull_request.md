# Suppress auto-remediation for pre-existing CI failures

## Summary
Verify the pull request base commit's CI status before prompting an agent to investigate and fix a failed pipeline. Helix now suppresses automatic remediation when base CI is failing, still running, unavailable, or otherwise not confirmed passing, preventing unrelated tasks from repeatedly modifying merge requests to repair known CI breakage.

The base commit and its cached CI status are tracked for GitHub, GitLab, and Azure DevOps pull requests so subsequent polling and agent pushes do not repeatedly query the same baseline.

## Testing
Ran `go test ./pkg/services ./pkg/types`; all tests passed. Added regression coverage confirming that failed head CI triggers remediation only when base CI passed, and that the cached base verdict is reused across subsequent polls and head commits.
