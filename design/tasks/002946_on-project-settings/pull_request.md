# Mask local MCP environment variables in project settings

## Summary
Mask local MCP environment variable values by default in project settings so credentials and other sensitive configuration are not exposed during screen sharing or in screenshots. Add an accessible per-variable visibility control so users can explicitly reveal a value when needed.

## Testing
- Added a regression test confirming existing environment variable values render as password fields and can be explicitly revealed.
- Ran `yarn test AddLocalMcpSkillDialog.test.tsx` successfully.
- Ran `yarn tsc` successfully.
