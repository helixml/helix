# Implementation Tasks

## Phase 1: Core Theme Infrastructure

- [x] Update `src/contexts/theme.tsx`: Initialize mode from localStorage, then OS preference (`prefers-color-scheme`), then default to `'dark'`
- [x] Update `src/contexts/theme.tsx`: Persist mode to localStorage in `toggleMode()`
- [x] Update `src/contexts/theme.tsx`: Make MUI component overrides (MuiMenu, MuiDialog, MuiPaper, MuiPopover) mode-conditional instead of hardcoded dark
- [x] Add light-mode scrollbar tokens to `src/themes.tsx` (`lightScrollbar`, `lightScrollbarThumb`, `lightScrollbarHover`)
- [x] Update scrollbar overrides in `src/contexts/theme.tsx` to use light scrollbar tokens when in light mode
- [x] Update `src/hooks/useLightTheme.tsx`: Add any missing light-mode return values needed by consumers (e.g., `panelColor`, `highlightColor`)
- [x] Add theme toggle button to the app bar (sun/moon icon using MUI `LightMode`/`DarkMode` icons), consuming `ThemeContext.toggleMode()`
- [x] Update `index.html` meta theme-color to be neutral or remove hardcoded dark value (already had `color-scheme: dark light`)

## Phase 2: Migrate Direct `themeConfig.dark*` Usage

- [~] Migrate `src/components/session/SessionToolbar.tsx` (10 direct dark accesses) to use `useLightTheme()`
- [~] Migrate `src/components/widgets/MonacoEditorImpl.tsx` — switch Monaco theme between `vs-dark` and `vs-light` based on mode
- [~] Migrate `src/pages/PasswordResetComplete.tsx` (15 dark accesses) to use `useLightTheme()`
- [~] Migrate `src/pages/ImportAgent.tsx` (18 dark accesses) to use `useLightTheme()`
- [~] Migrate diff viewer components (`DiffViewer.tsx`, `DiffFileList.tsx`, `DiffContent.tsx`) to use `useLightTheme()`
- [~] Migrate `src/components/datagrid/DataGridImpl.tsx` to use `useLightTheme()`
- [~] Migrate remaining files using `themeConfig.dark*` directly (~20 files) to use `useLightTheme()` or mode-conditional logic

## Phase 3: Replace Hardcoded Dark Colors

- [ ] Replace hardcoded dark hex values in `src/index.tsx` (ErrorBoundary fallback styling)
- [ ] Replace hardcoded colors in dialog components (`DarkDialog` pattern, `AddProviderDialog`, `AddApiSkillDialog`, `AddMcpSkillDialog`, `AddLocalMcpSkillDialog`)
- [ ] Replace hardcoded colors in skill components (`WebSearchSkill`, `BrowserSkill`, `EmailSkill`, `DroneCiSkill`, `AzureDevOpsSkill`, `ProjectManagerSkill`)
- [ ] Replace hardcoded colors in remaining components (`AccessManagement`, `UserOrgSelector`, `AgentSelector`, `Markdown`, `Citation`, `PdfRenderer`, `ToPDFImpl`)

## Phase 4: Visual QA & Polish

- [ ] Test high-traffic pages in light mode: session/chat, dashboard, create new session, app settings
- [ ] Test auth pages in light mode: login, register, password reset, onboarding
- [ ] Test admin pages in light mode: org billing, access management, model instances
- [ ] Verify charts and data visualizations are readable in light mode
- [ ] Verify sidebar navigation looks correct in light mode
- [ ] Run `yarn build` to confirm no TypeScript errors
