# Trusted startup commit

## Problem

A repository can be attached to several projects (for example a shared PTaaS
kit). Agent keys may push `helix-specs`, and every sandbox runs
`helix-specs/.helix/startup.sh` at start, along with any file that script runs
(`engagement/lane_setup/fetch-secrets.sh` in the kit). An agent in project A
could therefore run code in project B's sandboxes, which hold B's keys and
secrets. Blocking agent writes under `.helix/` would not help: the script runs
other repo files.

## Rule

What runs at sandbox start comes only from a helix-specs commit a person vouched
for.

- `git_repositories.trusted_startup_sha` holds that commit. It moves only when:
  - a push that updates `helix-specs` is authenticated with a human credential
    (`!ApiKey.IsAgentBound()`: personal or org key);
  - a person (`User.IsHumanCredential()`) saves or creates the startup script
    through the UI/API (project create, project update, `helix apply`, sample
    projects).
- Agent pushes (spec-task, session, sandbox, project, app, embed, bot keys) are
  still accepted; they never move it.
- `UpdateGitRepository` omits the column; only `SetGitRepositoryTrustedStartupSHA`
  writes it.
- The API passes it to the desktop as `HELIX_TRUSTED_STARTUP_SHA`, set last and
  unconditionally, so `agent.Env` and project secrets cannot supply it.
- `helix-trusted-startup.sh` extracts that commit's tree (`git archive`) into a
  fresh read-only directory and runs `.helix/startup.sh` from there, so `$0` and
  every path the script resolves from it come from the trusted commit. CWD stays
  the primary code checkout, as before.
- No trusted commit and a `startup.sh` on helix-specs: nothing runs; the reason
  is printed and written to `/tmp/helix-startup.log`.
- Web-service deploys check out `$SPECS` at the trusted commit and refuse to
  deploy without one.

## Limits

- A human write endorses the whole helix-specs tree at that commit, including
  files an agent changed earlier. Review agent changes to helix-specs before
  pushing or saving the startup script.
- CWD is the primary code checkout, which agents of the same project can push
  (their task branch). Other projects' agents cannot push that branch.
- Repositories that existed before this change have no trusted commit; their
  startup script does not run until a person saves it in Project Settings or
  pushes helix-specs.
