# Implementation Tasks: Remove Stale .gitconfig.lock Before Git Setup

- [ ] In `helix/desktop/shared/helix-workspace-setup.sh`, add `rm -f ~/.gitconfig.lock` immediately before the `# Git Configuration` comment block (~line 198)
- [ ] Verify setup succeeds when `~/.gitconfig.lock` exists (manually touch the file and re-run setup)
- [ ] Verify setup succeeds normally when `~/.gitconfig.lock` does not exist
