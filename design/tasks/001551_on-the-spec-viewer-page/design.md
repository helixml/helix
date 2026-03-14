# Design: Hide Git Metadata on Mobile in Spec Viewer

## Location

`/home/retro/work/helix/frontend/src/components/spec-tasks/DesignReviewContent.tsx`

Lines ~1038–1055 contain the "Git info and actions on the right" section:

```tsx
{/* Git info and actions on the right */}
<Box display="flex" alignItems="center" gap={1.5} pr={2}>
  <Tooltip title={`Commit: ${review.git_commit_hash}`}>
    <Chip
      icon={<GitBranch size={14} />}
      label={`${review.git_branch} @ ${review.git_commit_hash.substring(0, 7)}`}
      ...
    />
  </Tooltip>
  <Typography variant="caption" color="text.secondary" sx={{ whiteSpace: "nowrap" }}>
    {new Date(review.git_pushed_at).toLocaleString()}
  </Typography>
  {/* share + comment log buttons follow */}
</Box>
```

## Approach

Use MUI's responsive `sx` `display` shorthand to hide the two elements on small screens:

```tsx
// On the Tooltip wrapping the Chip:
<Tooltip ...>
  <Box sx={{ display: { xs: 'none', sm: 'flex' } }}>
    <Chip ... />
  </Box>
</Tooltip>

// On the Typography:
<Typography ... sx={{ whiteSpace: "nowrap", display: { xs: 'none', sm: 'block' } }}>
  ...
</Typography>
```

`xs: 'none'` hides below MUI `sm` breakpoint (600px). This is simpler than adding another `useMediaQuery` call since MUI's `sx` responsive syntax handles it inline.

The share icon and comment log buttons stay in the same `Box` and are unaffected.

## Pattern Note

This codebase uses MUI `sx` responsive display (`display: { xs: 'none', md: 'flex' }`) in other components (e.g. `CodeIntelligenceTab.tsx`). No Tailwind — all responsive work goes through MUI `sx` or `useMediaQuery`.
