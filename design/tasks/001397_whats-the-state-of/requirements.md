# Requirements: Project Secrets Support

## Summary

Investigate the current state of project secret support and document what exists vs. what's needed for passing secrets (like Stripe dev API keys) as environment variables into desktop containers.

## Current State (Already Implemented ✅)

Based on code investigation, **project secrets are already fully implemented**:

### Backend
- **API endpoints exist**: `GET/POST /api/v1/projects/{id}/secrets` for listing and creating project secrets
- **Database storage**: Secrets stored in `secrets` table with `project_id` field
- **AES-256-GCM encryption**: Secrets encrypted at rest using `HELIX_ENCRYPTION_KEY`
- **Injection pipeline**: `GetProjectSecretsAsEnvVars()` decrypts and formats secrets as `NAME=value`
- **Container injection**: Both `StartSpecGeneration()` and `StartJustDoItMode()` inject secrets into `DesktopAgent.Env`

### Frontend
- **UI in Project Settings**: Secrets section with list display and "Add" button
- **Add Secret dialog**: Name/value input with visibility toggle
- **Delete functionality**: Per-secret delete buttons
- **Auto-uppercase**: Secret names auto-converted to `UPPER_SNAKE_CASE`

### Security
- Encrypted at rest (AES-256-GCM)
- Values never returned to frontend after creation
- Project-level authorization checks

## User Stories

### US-1: Add a secret to a project
**As a** developer  
**I want to** add a secret like `STRIPE_SECRET_KEY` to my project  
**So that** agents working on my project can access my Stripe dev account

**Status**: ✅ Already works

### US-2: Secrets available in container
**As a** developer  
**I want** my project secrets automatically available as env vars in desktop sessions  
**So that** my code and tools can use them without manual configuration

**Status**: ✅ Already works

### US-3: Delete a secret
**As a** developer  
**I want to** remove secrets I no longer need  
**So that** I can rotate or clean up credentials

**Status**: ✅ Already works

## Acceptance Criteria

All criteria are already met:

- [x] Secrets can be created via Project Settings UI
- [x] Secrets are encrypted before storage
- [x] Secret values are never exposed after creation
- [x] Secrets are injected as env vars when starting desktop sessions
- [x] Both spec-generation and just-do-it modes inject secrets
- [x] Secrets can be deleted
- [x] Authorization enforced (project access required)

## Potential Enhancements (Not Currently Requested)

If future improvements are needed:
- **Edit secret value**: Currently must delete and recreate
- **Secret descriptions**: Optional notes field
- **Last used timestamp**: Audit when secrets are accessed
- **Rotation reminders**: Alerts for old secrets