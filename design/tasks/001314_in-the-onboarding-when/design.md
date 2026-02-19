# Design: Onboarding Resolution Picker

## Architecture

The resolution picker will be added to the existing `Onboarding.tsx` component, appearing within the agent creation section (when `agentMode === 'create'`).

## Key Decisions

1. **Placement**: Add resolution picker directly in `Onboarding.tsx` after the `CodingAgentForm` component, not inside the form itself. This keeps the CodingAgentForm focused on agent runtime/model selection.

2. **State management**: Add local state for resolution and zoom in Onboarding.tsx. When creating the agent, update the agent's `external_agent_config` via a subsequent API call after agent creation.

3. **Simpler approach**: Don't modify `CodingAgentForm` or `ICreateAgentParams`. Instead, after `codingAgentFormRef.current?.handleCreateAgent()` succeeds, immediately call `apps.updateApp()` to set the `external_agent_config` with resolution and zoom.

## Implementation Details

### New State in Onboarding.tsx
```typescript
const [desktopResolution, setDesktopResolution] = useState<'1080p' | '4k'>('1080p')
```

### UI Component (inline in renderStepContent)
Simple two-button toggle styled consistently with existing onboarding cards:
- 1080p card: "Standard HD — run more agents in parallel"
- 4K card: "Ultra HD — sharper display (2x scaling)"

### Agent Config Update
After agent creation in `handleCreateProject`:
```typescript
if (agentMode === 'create' && agentId) {
  await apps.updateApp(agentId, {
    config: {
      helix: {
        external_agent_config: {
          resolution: desktopResolution,
          zoom_level: desktopResolution === '4k' ? 200 : 100,
        }
      }
    }
  })
}
```

## Existing Patterns Found

- Resolution picker UI exists in `AppSettings.tsx` (lines 845-905) with similar MenuItem structure
- `ExternalAgentConfig` type defined in `api/pkg/types/types.go` with `resolution` and `zoom_level` fields
- `apps.updateApp()` available via `useApps()` hook for post-creation config updates