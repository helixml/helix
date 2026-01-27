# Implementation Tasks

- [x] Modify `helix-specs/.helix/startup.sh` to add renaming logic at the start
  - Rename `helix-[0-9]*` directories to `helix`
  - Rename `zed-[0-9]*` directories to `zed`
  - Rename `qwen-code-[0-9]*` directories to `qwen-code`
  - Create symlinks from old names to new names (e.g., `zed-1` → `zed`) for API compatibility on container restart
  - Make renaming idempotent (skip if canonical name already exists)
- [~] Modify `helix-3/desktop/shared/helix-workspace-setup.sh` to resolve symlinks when building Zed folders list
  - Use `readlink -f` on repo paths so Zed shows canonical names (e.g., `zed` instead of `zed-1`)
- [ ] Test startup script handles already-correct naming
- [ ] Test startup script handles container restart (symlinks allow helix-workspace-setup.sh to find repos)
- [ ] Test Zed sidebar shows canonical names after symlink resolution