# Design: Add Credentials Backstop Paragraph to worker-policy.md

## 1. Problem recap

Task 002092 shipped `mint_credential` and (per its design.md §3.6)
deleted the `SecretInjector` mechanism that used to push `GH_TOKEN`
into a Worker's container at boot. The expected pattern is now
"mint → export → use; on 401/403, mint again."

That contract currently lives in **two** places only:

- `mint_credential`'s tool description (read when the agent inspects
  the tool — *after* it decides to look for credential help).
- `owner_role.md`'s new "External-provider credentials" section
  (read only by Workers filling the **owner** Role).

The other Roles in the org template set, and any Roles operators
created before #2586 against the `SecretInjector` contract, will
continue to instruct their Workers as if `GH_TOKEN` were present at
boot. The agent reads the Role prompt before reaching for any tool,
so the bad guidance wins.

The owner_role.md design pushed the credential prose down to
per-Role text on the theory that not every Role uses `gh`/`curl`. The
asymmetry that justifies overriding that choice **at the policy
altitude only**:

- A Role that doesn't use credentials gets ~3 lines of unused context
  per activation. Cost: trivial.
- A Role with stale guidance silently burns an entire activation
  reinventing the MCP-over-HTTP wheel, or worse, gives up on the task.
  Cost: a full turn of compute and (because activations are
  single-turn) a missed reply.

worker-policy.md is the right altitude for the **backstop rule**.
owner_role.md's section stays — it's still the right altitude for the
**hiring-manager guidance** (what an owner adds to a Role prompt when
creating one).

## 2. Where the text goes

`api/pkg/org/application/agent/worker-policy.md`.

Section order today:

1. *(intro)* — "You are an AI Worker..."
2. You are an AI, not a human
3. What every activation looks like
4. Speaking discipline — bias toward silence
5. AI-origin vs human-origin events
6. Direct address vs broadcast
7. Chain of command
8. Errors and exits
9. *(closing)* — "You may now act on the Trigger."

New section goes between **Chain of command** and **Errors and exits**.
Rationale: chain-of-command covers *who* a Worker can talk to, the new
section covers *what tokens* a Worker needs to talk to *external*
systems — same logical grouping (resolving live identifiers before
acting). Placing it just before "Errors and exits" also means
"if you hit a 401/403, that's an error pattern with a specific
remedy" reads in sequence with the generic error guidance that
follows.

## 3. The text

Proposed section (~12 lines, matches the file's existing voice):

```markdown
## External-provider credentials

Your shell has no provider tokens by default. Anything that needs to
authenticate to an external system — `gh`, `git push`/`git fetch`
against a private remote, `curl` to an authenticated endpoint — will
fail unless you mint a credential first.

- **Before** the first authenticated command in an activation, call
  `mint_credential` with the provider name (e.g. `"github"`) and
  `export` the returned token into your shell:
  `export GH_TOKEN=$(...)`. The minted token is short-lived (~1 hour).
- **On any 401 / 403** from a command that should have worked, assume
  the token has expired. Call `mint_credential` again, re-export,
  retry once. Do not abandon the task on a stale token; expired
  tokens are expected on any session that runs longer than ~1 hour.

Your Role describes *which* providers and *which* commands apply.
This section is the rule that holds even if the Role text predates
the `mint_credential` flow.
```

Design notes on the wording:

- **"Your shell has no provider tokens by default"** — this is the
  load-bearing first sentence. It contradicts the assumption baked
  into stale Role prompts head-on, so an agent reading both will see
  the conflict and (per the worker-policy.md > role.md precedence
  established in the file's intro) follow the policy.
- **"with the provider name (e.g. `"github"`)"** — gives a concrete
  arg without enumerating providers, so a future Slack provider
  doesn't require an edit here.
- **"`export GH_TOKEN=$(...)`"** — matches the exact pattern in
  `mint_credential`'s tool description (task 002092 design §3.3) and
  in `owner_role.md`'s paragraph, so the agent gets the same
  copy-pasteable shape from all three surfaces.
- **"401 / 403"** — exact same trigger language as
  `mint_credential`'s description.
- **"retry once"** — the agent gets one explicit retry. If the second
  attempt also fails, that escalates into the generic "Errors and
  exits" guidance immediately below: say so once, briefly, and exit.
  No retry-loop temptation.
- **Last paragraph** — sets the contract between this section and the
  Role text: rule lives here, workflow lives in the Role. Prevents
  the next Role author from re-litigating *whether* to mint.

## 4. owner_role.md: delete the section (revised after review)

**Original plan:** keep `owner_role.md`'s existing "External-provider
credentials: `mint_credential`" section as hiring-manager guidance,
and prepend a cross-reference paragraph pointing at worker-policy.md.

**Revised after review feedback:** delete the entire section.

The reviewer's argument: with the rule in worker-policy.md, the
hiring-manager guidance is redundant. Owners creating new Roles do
not need to write a credential paragraph into the Role prompt —
worker-policy.md already tells every Worker what to do. Keeping the
hiring-manager paragraph would push owners to duplicate the same
instruction into every Role that touches `gh`/`git`/auth-`curl`,
which is the exact pattern the backstop was added to eliminate.

The two pieces of owner-specific information being removed:

- "`mint_credential` is in the baseline tool set, you do not need to
  add it to a Role's `tools` list." — discoverable from
  `BaseReadTools`' doc comment (added in task 002092).
- "You can call `mint_credential` yourself; it returns
  `{token, expires_at, usage}`." — discoverable by the owner calling
  the tool and reading the schema, or by reading
  `mint_credential`'s tool description.

Both are minor losses and worth taking in exchange for one source of
truth. If a future operator-facing surface needs to document the
shape, it can do so without owners-Role-prompt duplication.

The remaining concern (a future editor "deduplicating" by removing
the backstop instead of the Role-level guidance) is now moot —
there is no Role-level guidance to deduplicate against.

## 5. Verification

This is a pure text change to an embedded file. Verification is:

1. **Build green.** `cd api && go build ./pkg/org/...` — the file is
   `//go:embed`'d into `policy.go`, so an empty file or a missing
   file would break the build. The build passing confirms the embed
   resolved.
2. **String present.** `grep -n "mint_credential" api/pkg/org/application/agent/worker-policy.md`
   returns the new section. (Used as a smoke check; no test
   asserts literal `Policy` contents, see AC6.)
3. **Inner-Helix manual check.** Hire a Worker on a Role with no
   credential text in its prompt. On the next activation, inspect the
   pushed `.context/worker-policy.md` in the Worker's repo and
   confirm the new section is there. (Confirms the runtime
   serialiser picks up the embed-time change after rebuild.)
4. **No regressions in existing tests.** `cd api && go test ./pkg/org/application/agent/ -count=1`
   passes. The only test in that package
   (`prompt_test.go`) tests `BuildPrompt`/`renderTrigger`, not
   `Policy` — so the run validates we haven't broken the package
   compile.

No new tests. The change asserts a documentation invariant, and the
load-bearing behaviour (the agent reading and acting on the rule) is
not deterministically testable in code — it's tested by AC4's
reviewer-judgement check and by the inner-Helix observation in §5.3.

## 6. Anti-scope (explicit)

- **No code changes outside `worker-policy.md` and `owner_role.md`.**
  The Go embed pipeline already covers reload; nothing in `policy.go`
  or `prompt.go` needs to change.
- **No edits to stale Roles.** Per the requirements: those get fixed
  when next touched. The point of the backstop is to make that lazy
  cleanup safe.
- **No new provider enumeration.** worker-policy.md must not list
  "github (today), slack (soon)" — that's exactly the kind of
  reference that rots. `mint_credential`'s own error message
  ("unknown provider %q; available: %s") is the live source of truth
  for what's registered.
- **No tool-surface edits.** `mint_credential` is already part of
  `BaseReadTools` (task 002092 design §4). This task does not add or
  remove tools from any Role.

## 7. Learnings to record

For future agents touching org-wide prompt text:

- **`worker-policy.md` is the embed-time source of truth.** It is
  shipped into the binary via `//go:embed` in
  `api/pkg/org/application/agent/policy.go`, then pushed by the
  Helix runtime to each Worker's `.context/worker-policy.md`. A
  binary rebuild + new Worker activation is the full propagation
  path — there is no separate config to deploy.
- **The "altitude" question is real.** When a new contract spans
  *all* Workers (credentials, speaking discipline, escalation
  protocol), worker-policy.md is the right home. When it's
  *role-specific* (the owner's hiring flow, an engineer's build
  steps), Role text is the right home. Cost-of-omission asymmetry,
  not "cleanliness of separation", decides which.
- **Backstop prose should contradict the stale assumption head-on.**
  If a Role prompt is going to lie ("you have GH_TOKEN"), the
  backstop must start by negating the lie ("your shell has no
  provider tokens by default"). A neutrally-worded note ("here's
  how to mint credentials") loses the precedence fight.
