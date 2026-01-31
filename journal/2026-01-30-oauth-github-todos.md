# OAuth GitHub Integration TODOs - 2026-01-30

## Completed Today
- [x] OAuth flow now properly fetches auth_url and opens popup (was returning JSON instead of redirecting)
- [x] Upgrade prompt only shows when GitHub is selected as Repository Type
- [x] Implemented Option A: OAuth-first for GitHub in CreateProjectDialog
- [x] Inline repo browser replaces nested dialog (no more modal-on-modal)
- [x] Auto-refresh OAuth connections when popup closes
- [x] Pass `oauth_connection_id` when linking external repos

## Outstanding TODOs

### 1. ~~OAuth Providers Not Visible to Non-Admin Users~~ FIXED 2026-01-31
**Issue**: User sees "OAuth is not configured for GitHub" even though GitHub OAuth IS configured by admin.

**Root Cause (TWO BUGS)**:

**Bug A - OAuthProvidersTable.tsx icon click handler**: When clicking the "+" icon on a template card (instead of clicking the card itself), `handleOpenDialog(undefined)` was called instead of `handleOpenDialog(provider)`. This caused `provider?.type || 'custom'` to default to `'custom'`.

```typescript
// BUG: Was passing undefined for templates
handleOpenDialog(isTemplate ? undefined : provider);

// FIX: Always pass the provider
handleOpenDialog(provider);
```

**Bug B - Frontend type matching too strict**: Even if type was wrong, frontend should be more resilient by also checking provider name.

**Fixes Applied**:

1. **OAuthProvidersTable.tsx**: Fixed icon click handler to pass `provider` for both templates and existing providers (line 640)

2. **BrowseProvidersDialog.tsx**: Updated `getProviderIdForType()` and `getOAuthConnectionForProvider()` to check both `type` and `name`

3. **CreateProjectDialog.tsx**: Updated `githubConnection` and `githubProvider` useMemo hooks to check both `type` and `name`

4. **Database fix**: Corrected existing provider type:
   ```sql
   UPDATE o_auth_providers SET type = 'github' WHERE id = '6837fb5b-...' AND type = 'custom';
   ```

**Verification**:
- Database shows: `name=Github, type=github, enabled=t`
- Frontend build passes
- New providers created from templates will have correct type

### 2. Improve OAuth Callback Success Page (Task #1)
**Issue**: The OAuth callback page at `/api/v1/oauth/flow/callback` is plain - just a green checkmark.

**Desired**: Add Helix logo and branding, match app design aesthetic, consider dark mode, possibly auto-close.

**Location**: Backend serves this - likely `api/pkg/server/oauth.go`

### 3. Organization Repos Not Available via OAuth
**Issue**: When browsing repos via OAuth, only personal repositories are shown. Organization repositories are not listed.

**Root Cause**: The GitHub API endpoint being used likely only returns repos the user owns, not repos they have access to via organization membership.

**Potential Fix**: Use a different GitHub API endpoint or add pagination/filtering to include organization repos. May need to request additional OAuth scopes or use a different API call like `/user/repos?type=all` or query organization repos separately.

### 4. Test Full Flow End-to-End
Once OAuth providers issue is fixed:
- [ ] New user creates project with External GitHub repo via OAuth
- [ ] Spec task can push to upstream (the original issue that started this)
- [ ] Existing user with OAuth connection can browse repos in both:
  - Create Project dialog
  - Repositories tab "Connect & Browse Repositories"
- [ ] Verify organization repos are accessible (currently NOT working - see #3)

## Code Changes Summary (WIP commit 7875cfe79)

| File | Changes |
|------|---------|
| `CreateProjectDialog.tsx` | +515 lines - Inline repo browser, OAuth detection, popup tracking |
| `BrowseProvidersDialog.tsx` | +89 lines - Refresh on open, popup tracking, "Recommended" chip |
| `Projects.tsx` | +4 lines - Pass oauthConnectionId |
| `GitRepoDetail.tsx` | +4 lines - Pass oauthConnectionId |

## Related Context
- Original issue: Spec tasks can't push to GitHub - OAuth token lacks `repo` scope
- Luke's note: Scopes are requested by consumers at runtime, not stored on provider config
- Scopes needed: `repo,read:user,user:email`
