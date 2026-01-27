# Implementation Tasks

- [ ] Modify `helix-specs/.helix/startup.sh` to add renaming logic at the start
  - Rename `helix-[0-9]*` directories to `helix`
  - Rename `zed-[0-9]*` directories to `zed`
  - Rename `qwen-code-[0-9]*` directories to `qwen-code`
  - Make renaming idempotent (skip if canonical name already exists)
- [ ] Test startup script handles already-correct naming
- [ ] Test startup script handles mixed state (some renamed, some not)