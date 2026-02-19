# Design: Onboarding Resolution Picker

## Architecture

The resolution picker will be added to the existing `Onboarding.tsx` component, appearing within the agent creation section (when `agentMode === 'create'`).

## Key Decisions

1. **Placement**: Add resolution picker directly in `Onboarding.tsx` after the `CodingAgentForm` component, not inside the form itself. This keeps the CodingAgentForm focused on agent runtime/model selection.

2. **UI style**: Use a **dropdown (Select)** rather than cards — this is more compact and consistent with CodingAgentForm which uses dropdowns for runtime/model selection. Avoids crowding the UI.

3. **State management**: Add local state for resolution in Onboarding.tsx. When creating the agent, update the agent's `external_agent_config` via a subsequent API call after agent creation.

4. **Simpler approach**: Don't modify `CodingAgentForm` or `ICreateAgentParams`. Instead, after `codingAgentFormRef.current?.handleCreateAgent()` succeeds, immediately call `apps.updateApp()` to set the `external_agent_config` with resolution and zoom.

## Implementation Details

### New State in Onboarding.tsx
```typescript
const [desktopResolution, setDesktopResolution] = useState<'1080p' | '4k'>('1080p')
```

### UI Component (inline in renderStepContent)
A compact dropdown with helper text, placed after CodingAgentForm:

```tsx
<Typography sx={{ color: 'rgba(255,255,255,0.4)', fontSize: '0.75rem', mb: 1 }}>
  Desktop Resolution
</Typography>
<FormControl fullWidth size="small" sx={{ mb: 2 }}>
  <Select
    value={desktopResolution}
    onChange={(e) => setDesktopResolution(e.target.value as '1080p' | '4k')}
  >
    <MenuItem value="1080p">
      <Box>
        <Typography variant="body2">1080p</Typography>
        <Typography variant="caption" color="text.secondary">
          Run more agents in parallel
        </Typography>
      </Box>
    </MenuItem>
    <MenuItem value="4k">
      <Box>
        <Typography variant="body2">4K (2x scaling)</Typography>
        <Typography variant="caption" color="text.secondary">
          Sharper display quality
        </Typography>
      </Box>
    </MenuItem>
  </Select>
</FormControl>
```

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
- Onboarding uses dropdowns for secondary selections (e.g., Claude org picker, agent selector)