# Inject project secrets into exploratory (human) desktop env

## Summary
Project secrets were being injected into spec task desktops but NOT into exploratory (human) desktops. This caused tools in the human desktop to lack access to secrets configured for the project.

## Changes
- Added `GetProjectSecretsAsEnvVars()` call to `startExploratorySession()` in both the restart path and new session path
- Matching the existing behavior in `spec_driven_task_service.go`

## Test Plan
- [ ] Create a project secret via API or UI
- [ ] Start an exploratory (human) desktop for the project
- [ ] Verify the secret is available as an environment variable: `env | grep SECRET_NAME`
