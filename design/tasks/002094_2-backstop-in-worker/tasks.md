# Implementation Tasks: Add Credentials Backstop Paragraph to worker-policy.md

- [x] Open `api/pkg/org/application/agent/worker-policy.md` and add the
      new "External-provider credentials" section between the
      "Chain of command" and "Errors and exits" sections, using the
      proposed text in design.md §3 verbatim (or trivially edited for
      house style).
- [x] Open `api/pkg/org/application/bootstrap/templates/owner_role.md`
      and prepend the cross-reference sentence from design.md §4 to
      the existing "External-provider credentials: `mint_credential`"
      section. Do not remove the existing per-Role paragraph.
- [x] Run `cd api && go build ./pkg/org/...` to confirm the
      `//go:embed worker-policy.md` directive still resolves and the
      package compiles.
- [x] Run `cd api && go test ./pkg/org/application/agent/ -count=1`
      to confirm `prompt_test.go` still passes (no test asserts
      literal Policy contents, but the package must still compile and
      run cleanly).
- [x] Smoke-check the embedded text: `grep -n "mint_credential" api/pkg/org/application/agent/worker-policy.md`
      should return the new section.
- [x] Commit on a feature branch with a conventional-commit message,
      e.g. `docs(org): add mint_credential backstop to worker-policy`.
      Single commit; do not bundle unrelated Role edits.
- [x] Push the branch. The Helix platform creates the GitHub PR
      automatically when the user clicks "Open PR" in the UI — do
      NOT run `gh pr create`.
- [ ] Watch CI after the user opens the PR. Expected: green —
      text-only change to an embedded markdown file.
- [ ] (Optional, post-merge) In the inner Helix at `localhost:8080`,
      hire a Worker on a Role with no credential paragraph in its
      prompt, activate it, and confirm `.context/worker-policy.md` in
      the Worker's repo contains the new section. Documents the
      end-to-end propagation path for the next agent.
