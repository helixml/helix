# Enforce organization scope for organization API keys

## Summary
Organization API keys now enforce the organization recorded on the credential, regardless of the permissions of the user who created the key. Organization-scoped credentials no longer inherit global administrator privileges, cannot access personal resources or resources in another organization, and default unfiltered project, session, repository, and agent listings to their organization. The restriction applies to existing organization keys without requiring migration or rotation.

## Testing
Added regression coverage proving that organization API keys do not inherit administrator access and cannot access personal or cross-organization resources. Focused server authentication, authorization, and listing tests pass. The complete server package was also run; only two pre-existing SQLite tests failed because the environment uses `CGO_ENABLED=0`, which causes the standard go-sqlite3 stub error.
