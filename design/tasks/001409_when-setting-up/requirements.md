# Requirements: Onboarding API Keys Not Working in Zed Sessions

## Problem Statement

When users set up Anthropic API Keys through the onboarding flow, the resulting Zed sessions fail to authenticate with the LLM provider. However, setting `ANTHROPIC_API_KEY` as an environment variable on the server works correctly.

## Root Cause

The onboarding flow creates provider endpoints at the **user level** (with empty `owner`), but spec tasks/sessions run in **organization context**. The provider endpoint lookup filters by organization ID, so user-level providers are never found.

**Flow breakdown:**
1. User goes through onboarding, creates org, adds Anthropic API key
2. Provider endpoint created with `owner: ""` (user-level, not org-level)
3. User creates a project (which belongs to the org)
4. User starts a spec task → session created with `OrganizationID: "org_xxx"`
5. Zed agent makes LLM request → proxy looks up provider with `Owner: "org_xxx"`
6. No match found (provider has `Owner: ""`)
7. Falls back to env var `ANTHROPIC_API_KEY`, which may not be set
8. Request fails

## User Stories

### US-1: Onboarding API Key Setup
**As a** new user going through onboarding  
**I want** to add my Anthropic API key  
**So that** my Zed sessions can make LLM calls without any additional configuration

### US-2: Organization Provider Inheritance
**As an** organization member  
**I want** my user-level providers to work in org context  
**So that** I don't need to configure providers separately for each org

## Acceptance Criteria

### AC-1: Provider Lookup Falls Back to User Level
- [ ] When an org-level provider is not found, the system checks for user-level providers
- [ ] User-level providers work in organization context (sessions, spec tasks)
- [ ] Existing org-level providers take precedence over user-level providers

### AC-2: Onboarding Creates Org-Level Providers (Alternative Fix)
- [ ] When onboarding is in org context, provider is created with org as owner
- [ ] The `orgId` parameter is passed correctly from onboarding to AddProviderDialog

### AC-3: End-to-End Verification
- [ ] User can complete onboarding with Anthropic API key
- [ ] User can create a spec task that successfully makes LLM calls
- [ ] No `ANTHROPIC_API_KEY` environment variable is required on the server

## Out of Scope

- Changes to the built-in provider fallback mechanism (env var)
- Provider management UI changes (beyond the fix)
- Claude subscription OAuth flow (separate credential type)