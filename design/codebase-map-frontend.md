# Frontend Codebase Map

## Overview

This document maps the Helix frontend codebase, identifying dead/unreachable code for removal.

**Analysis Tools Used:**
- `unimported` - Found 86 unimported files
- `ts-prune` - Found 832 unused exports
- `depcheck` - Found 31 unused dependencies
- `madge` - Found 54 circular dependencies

## Route Analysis

### All Routes (from `router.tsx`)

Routes are defined in two groups:
1. **Root routes** - accessible at `/`
2. **Org-prefixed routes** - accessible at `/org/:org_id/`

| Route Name | Path | Page Component | Navigation Access |
|------------|------|----------------|-------------------|
| `projects` | `/` | `Projects` | Main landing page |
| `chat` | `/chat` | `Home` | Sidebar link |
| `home` | `/home` | (redirect to projects) | Legacy redirect |
| `new` | `/new` | `Create` | "New Chat" button |
| `apps` | `/apps` | `Apps` | Projects page tab |
| `git-repos` | `/git-repos` | (redirect to projects) | Legacy redirect |
| `git-repo-detail` | `/git-repos/:repoId` | `GitRepoDetail` | Repository list click |
| `qa` | `/qa` | `QuestionSets` | Projects page tab |
| `providers` | `/providers` | `Providers` | Projects page tab |
| `tasks` | `/tasks` | `Tasks` | ❓ Unclear access |
| `spec-tasks` | `/spec-tasks` | `SpecTasksPage` | Projects page tab |
| `projects-legacy` | `/projects` | (redirect) | Legacy redirect |
| `project-specs` | `/projects/:id/specs` | `SpecTasksPage` | Project click |
| `project-task-detail` | `/projects/:id/tasks/:taskId` | `SpecTaskDetailPage` | Task click |
| `project-task-review` | `/projects/:id/tasks/:taskId/review/:reviewId` | `SpecTaskReviewPage` | Review click |
| `project-team-desktop` | `/projects/:id/desktop/:sessionId` | `TeamDesktopPage` | Desktop link |
| `project-settings` | `/projects/:id/settings` | `ProjectSettings` | Settings icon |
| `project-session` | `/projects/:id/session/:session_id` | `Session` | Session click |
| `app` | `/app/:app_id` | `App` | App list click |
| `new-agent` | `/new-agent` | `Apps` (redirect) | Legacy - wizard removed |
| `session` | `/session/:session_id` | `Session` | Session list click |
| `qa-results` | `/qa-results/:question_set_id/:execution_id` | `QuestionSetResults` | QA execution click |
| `import-agent` | `/import-agent` | `ImportAgent` | Import button |
| `orgs` | `/orgs` | `Orgs` | Org switcher |
| `org_settings` | `/orgs/:org_id/settings` | `OrgSettings` | Org menu |
| `org_people` | `/orgs/:org_id/people` | `OrgPeople` | Org menu |
| `org_teams` | `/orgs/:org_id/teams` | `OrgTeams` | Org menu |
| `org_billing` | `/orgs/:org_id/billing` | `OrgBilling` | Org menu |
| `team_people` | `/orgs/:org_id/teams/:team_id/people` | `TeamPeople` | Team click |
| `files` | `/files` | `Files` | Sidebar link |
| `secrets` | `/secrets` | `Secrets` | Account menu |
| `oauth-connections` | `/oauth-connections` | `OAuthConnectionsPage` | Account menu |
| `dashboard` | `/dashboard` | `Dashboard` | Admin menu |
| `account` | `/account` | `Account` | User menu |
| `api-reference` | `/api-reference` | `OpenAPI` | Footer/docs link |
| `password-reset` | `/password-reset` | `PasswordReset` | Login page |
| `password-reset-complete` | `/password-reset-complete` | `PasswordResetComplete` | Email link |
| `design-doc` | `/design-doc/:specTaskId/:reviewId` | `DesignDocPage` | Review link |

### Legacy/Redirect Routes (candidates for eventual removal)
- `home` → redirects to `projects`
- `git-repos` → redirects to `projects` with tab
- `projects-legacy` → redirects to `projects`
- `new-agent` → wizard removed, just shows Apps

---

## ❌ DEAD CODE: Unimported Files (86 files)

These files are NOT imported by any reachable code path:

### Components - App Examples (12 files) ❌ REMOVE
```
src/components/app/ApiIntegrations.tsx
src/components/app/ZapierIntegrations.tsx
src/components/app/examples/skillAtlassianApi.tsx
src/components/app/examples/skillConfluenceApi.tsx
src/components/app/examples/skillGmailApi.tsx
src/components/app/examples/skillGoogleApi.tsx
src/components/app/examples/skillGoogleCalendarApi.tsx
src/components/app/examples/skillGoogleDriveApi.tsx
src/components/app/examples/skillLinkedInApi.tsx
src/components/app/examples/skillMicrosoftApi.tsx
src/components/app/examples/skillOneDriveApi.tsx
src/components/app/examples/skillSlackApi.tsx
```

### Components - Create (6 files) ❌ REMOVE
```
src/components/create/ProviderEndpointPicker.tsx
src/components/create/SessionModeButton.tsx
src/components/create/SessionModeDropdown.tsx
src/components/create/SessionModeSwitch.tsx
src/components/create/SessionTypeSwitch.tsx
src/components/create/SessionTypeTabs.tsx
```

### Components - Finetune (6 files) ❌ REMOVE
```
src/components/finetune/AddDocumentsForm.tsx
src/components/finetune/AddImagesForm.tsx
src/components/finetune/FileList.tsx
src/components/finetune/FileUploadArea.tsx
src/components/finetune/LabelImagesForm.tsx
src/components/finetune/LabelImagesTextField.tsx
```

### Components - Session/Finetune Related (8 files) ❌ REMOVE
```
src/components/session/FineTuneCloneInteraction.tsx
src/components/session/FineTuneImageInputs.tsx
src/components/session/FineTuneImageLabel.tsx
src/components/session/FineTuneImageLabels.tsx
src/components/session/FineTuneTextInputs.tsx
src/components/session/FineTuneTextQuestionEditor.tsx
src/components/session/FineTuneTextQuestions.tsx
src/components/session/SchedulingDecisionSummary.tsx
```

### Components - Session Other (3 files) ❌ REMOVE
```
src/components/session/InputField.tsx
src/components/session/SessionButtons.tsx
src/components/session/ShareSessionOptions.tsx
```

### Components - DataGrid (3 files) ❌ REMOVE
```
src/components/datagrid/DataGridWithFilters.tsx
src/components/datagrid/FileStore.tsx
src/components/datagrid/ToolActions.tsx
```

### Components - Tasks (3 files) ❌ REMOVE
```
src/components/tasks/AgentKanbanBoard.tsx
src/components/tasks/DesignDocViewer.tsx
src/components/tasks/SpecTaskReviewPanel.tsx
```

### Components - Widgets (10 files) ❌ REMOVE
```
src/components/widgets/Caption.tsx
src/components/widgets/EditTextWindow.tsx
src/components/widgets/Embed.tsx
src/components/widgets/GeneralText.tsx
src/components/widgets/HeightDiv.tsx
src/components/widgets/ScrollingLoader.tsx
src/components/widgets/SelectOption.tsx
src/components/widgets/SimpleDeleteConfirmWindow.tsx
src/components/widgets/StringMapEditor.tsx
src/components/widgets/TextView.tsx
```

### Components - Other (10 files) ❌ REMOVE
```
src/components/common/PromptLibrarySidebar.tsx
src/components/common/PromptViewerModal.tsx
src/components/embed/EmbedWidget.tsx
src/components/external-agent/desktop-stream/types.ts
src/components/external-agent/index.ts
src/components/fleet/LiveAgentFleetDashboard.tsx
src/components/home/FeatureGrid.tsx
src/components/orgs/OrgSidebarMainLink.tsx
src/components/project/RepositoryBrowser.tsx
src/components/providers/logos/togetherai.tsx
src/components/settings/OAuthSettings.tsx
src/components/system/SidebarMainLink.tsx
src/components/tools/ToolDetail.tsx
```

### Hooks (8 files) ❌ REMOVE
```
src/hooks/useInteraction.ts
src/hooks/useInteractionQuestions.ts
src/hooks/useInterval.ts
src/hooks/useLoadingErrorHandler.ts
src/hooks/usePollingApiData.ts
src/hooks/usePromptLibraryShortcuts.ts
src/hooks/useSessionConfig.ts
src/hooks/useSpecTasks.ts
```

### Utils (5 files) ❌ REMOVE
```
src/utils/colors.ts
src/utils/debug.ts
src/utils/instanceId.ts
src/utils/sampleTypeUtils.ts
src/utils/windowPositioning.ts
```

### Lib/Other (7 files) ❌ REMOVE
```
src/fixtures.ts
src/lib/helix-stream/resources/index.ts
src/lib/helix-stream/workers/video-worker-protocol.ts
src/polyfills/path.js
src/polyfills/process.js
src/polyfills/url.js
src/styles/shared.ts
```

### Test Setup (1 file) ⚠️ REVIEW - may be needed for tests
```
src/setupTests.ts
```

---

## ❌ DEAD CODE: Unused Dependencies (31 packages)

From `depcheck` analysis - these packages are in `package.json` but not imported:

### Definitely Remove
```
@babel/core
@emotion/react
@emotion/styled
@helixml/chat-widget
@hookform/resolvers
@sentry/browser
core-js
mobx
pretty-bytes
puppeteer
react-copy-to-clipboard
react-markdown-editor-lite
react-to-pdf
react-virtualized-auto-sizer
react-window
recharts
redoc
rehype-sanitize
styled-components
three
three-orbitcontrols
yup
```

### Type Definitions (remove if corresponding package unused)
```
@types/bluebird
@types/chance
@types/hookrouter
@types/node
@types/react-copy-to-clipboard
@types/react-syntax-highlighter
@types/react-virtualized-auto-sizer
@types/react-window
@types/uuid
```

---

## ⚠️ Duplicate Directories

### `spec-tasks` vs `specTask`

Two directories with similar purpose:
- `src/components/spec-tasks/` 
- `src/components/specTask/`

**Analysis needed:** Determine which is actively used and consolidate.

---

## Circular Dependencies (54 found)

Most stem from `useRouter.ts` → `router.tsx` circular imports. This is a code quality issue but not dead code.

Key problematic pattern:
```
hooks/useRouter.ts → contexts/router.tsx → router.tsx → [various pages]
```

**Recommendation:** Refactor router context to break cycles (separate task).

---

## Summary

| Category | Count | Action |
|----------|-------|--------|
| Unimported files | 86 | ❌ REMOVE |
| Unused dependencies | 31 | ❌ REMOVE from package.json |
| Circular dependencies | 54 | ⚠️ Refactor later |
| Duplicate directories | 2 | ⚠️ Consolidate |

**Estimated lines of code to remove:** ~5,000-8,000 lines

---

## Files to Keep (actively used)

All page components in `src/pages/` are routed and should be kept:
- Account.tsx
- App.tsx
- Apps.tsx
- Create.tsx
- Dashboard.tsx
- DesignDocPage.tsx
- Files.tsx
- GitRepoDetail.tsx
- GitRepos.tsx (redirect only)
- Home.tsx
- ImportAgent.tsx
- Layout.tsx
- OAuthConnectionsPage.tsx
- OpenAPI.tsx
- OrgPeople.tsx
- OrgSettings.tsx
- OrgTeams.tsx
- Orgs.tsx
- PasswordReset.tsx
- PasswordResetComplete.tsx
- ProjectSettings.tsx
- Projects.tsx
- Providers.tsx
- QuestionSetResults.tsx
- QuestionSets.tsx
- Secrets.tsx
- Session.tsx
- SpecTaskDetailPage.tsx
- SpecTaskReviewPage.tsx
- SpecTasksPage.tsx
- Tasks.tsx
- TeamDesktopPage.tsx
- TeamPeople.tsx