# Implementation Tasks

## Backend - Data Model

- [ ] Add `SecretFile` struct to `api/pkg/types/types.go` (ID, ProjectID, RepositoryID, Path, Content, Value, Owner, OwnerType, Created, Updated)
- [ ] Add `CreateSecretFileRequest` struct (RepositoryID, Path, Content)

## Backend - Store Layer

- [ ] Add `SecretFile` to AutoMigrate in `api/pkg/store/postgres.go`
- [ ] Add store interface methods: `CreateSecretFile`, `GetSecretFile`, `ListProjectSecretFiles`, `UpdateSecretFile`, `DeleteSecretFile`
- [ ] Implement store methods in `api/pkg/store/postgres.go`

## Backend - API Handlers

- [ ] Create `api/pkg/server/secret_files_handlers.go` with handlers:
  - [ ] `listProjectSecretFiles` - GET `/api/v1/projects/{id}/secret-files`
  - [ ] `createProjectSecretFile` - POST `/api/v1/projects/{id}/secret-files`
  - [ ] `getProjectSecretFile` - GET `/api/v1/projects/{id}/secret-files/{file_id}` (returns decrypted)
  - [ ] `updateProjectSecretFile` - PUT `/api/v1/projects/{id}/secret-files/{file_id}`
  - [ ] `deleteProjectSecretFile` - DELETE `/api/v1/projects/{id}/secret-files/{file_id}`
- [ ] Register routes in `api/pkg/server/server.go`
- [ ] Add swagger annotations and run `./stack update_openapi`

## Backend - Session Injection

- [ ] Add `GetProjectSecretFiles` callback type to `api/pkg/services/spec_driven_task_service.go`
- [ ] Implement `GetProjectSecretFilesDecrypted` in `api/pkg/server/secret_files_handlers.go`
- [ ] Wire callback in `api/pkg/server/server.go` (like `GetProjectSecrets`)
- [ ] Add `writeSecretFilesToWorkspace()` function in `api/pkg/external-agent/hydra_executor.go`
- [ ] Call `writeSecretFilesToWorkspace()` in `StartDesktop()` after workspace created
- [ ] Add `updateGitignore()` helper to append secret file paths to `.gitignore`

## Frontend

- [ ] Add "Secret Files" section to `frontend/src/pages/ProjectSettings.tsx` (below Secrets)
- [ ] Add query for `v1ProjectsSecretFilesDetail` (list files)
- [ ] Add query for project repositories (for dropdown)
- [ ] Add mutation for create, update, delete
- [ ] Add "Add Secret File" dialog with repository dropdown + path input + multiline content editor
- [ ] Add "Edit Secret File" dialog (loads decrypted content via `getProjectSecretFile`)
- [ ] List display: repository name, file path, "encrypted" chip, edit/delete buttons

## Testing

- [ ] Verify `go build ./pkg/server/ ./pkg/store/ ./pkg/types/`
- [ ] Verify `cd frontend && yarn build`
- [ ] Manual test: add secret file in UI, start session, verify file exists in workspace
- [ ] Manual test: verify secret file path appears in `.gitignore`
