# docs(org): add mint_credential backstop to worker-policy

## Summary

Adds a short "External-provider credentials" section to
`worker-policy.md` — the org-wide policy every AI Worker reads on
every activation. After #2586 (task 002092) removed the boot-time
`GH_TOKEN` `SecretInjector`, the mint-then-export contract was
documented only in `mint_credential`'s tool description and in
`owner_role.md`. Every other existing Role still describes
authenticated shell commands as if `GH_TOKEN` were present at boot,
so a Worker on one of those Roles silently burns its activation
when `gh`/`git`/auth-`curl` fails.

worker-policy.md is the right altitude for the rule — it reaches
every Worker on next activation without hand-editing five-plus
Roles. The cost asymmetry justifies the few extra lines for Roles
that don't use credentials: trivial unused context vs. a wasted
activation.

## Changes

- `api/pkg/org/application/agent/worker-policy.md` — new section
  between "Chain of command" and "Errors and exits" stating that
  the shell has no provider tokens by default, that
  `mint_credential` must be called before the first authenticated
  command (with the `export GH_TOKEN=$(...)` pattern), and that any
  401/403 means re-mint and retry once.
- `api/pkg/org/application/bootstrap/templates/owner_role.md` —
  one-paragraph cross-reference at the top of the existing
  "External-provider credentials: `mint_credential`" section,
  noting that worker-policy.md carries the baseline rule and that
  the Role-prompt paragraph below is the workflow specialisation
  an owner adds for Roles whose Workers actually run authenticated
  commands. The existing per-Role paragraph stays.

## Verification

- `go build ./pkg/org/...` — green (confirms the `//go:embed
  worker-policy.md` directive in `policy.go` still resolves).
- `go test ./pkg/org/application/agent/ -count=1` — green
  (`prompt_test.go` passes; no test asserts the literal `Policy`
  string, so the edit is text-only).
- `grep -n "mint_credential" api/pkg/org/application/agent/worker-policy.md`
  — three hits in the new section.

## Related

- Parent task: 002092 (`mint_credential` MCP tool +
  `SecretInjector` removal).
- Design doc: `helix-specs` branch,
  `design/tasks/002094_2-backstop-in-worker/`.
