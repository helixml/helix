# Silent CI: how a 10-second webhook timeout cost us builds, and how WebhookRelay fixed it

Date: 2026-08-23
Repo: helixml/helix — CI: self-hosted Drone at drone.lukemarsden.net

NOTE: all tokens in this document are redacted. Do not paste real
`sk-whrm-…` / Drone tokens into a published post.

## 1. The symptom

https://github.com/helixml/helix/pull/3098 had no CI checks. Not failing — absent.

    gh pr view 3098 --json statusCheckRollup
    → {"statusCheckRollup": []}

    gh api repos/helixml/helix/commits/42aa22853ea572c5cba55980e053d11cba74db46/status
    → {"state":"pending","total_count":0}

The parent commit was fine:

    gh api repos/helixml/helix/commits/0c9294065/status
    → {"state":"success","total_count":1,
       "context":"continuous-integration/drone/push",
       "target_url":"https://drone.lukemarsden.net/helixml/helix/3856"}

Drone had built the branch twice — 3855 (`a34dce12`, failure) and 3856
(`0c929406`, success) — but had never heard of `42aa2285`, the PR head.
GitHub attaches commit statuses per SHA, so the PR showed nothing.

## 2. The red herring

The webhook looked healthy:

    gh api repos/helixml/helix/hooks --jq '.[] | {id,url:.config.url,active,last_response}'
    → {"id":603552912,"url":"https://drone.lukemarsden.net/hook","active":true,
       "last_response":{"code":200,"message":"OK","status":"active"}}

`last_response` only reflects the *most recent* delivery. A hook that drops
one delivery in fifty still reads 200/OK forever. Deliveries are where the
truth is.

## 3. The actual error

    gh api "repos/helixml/helix/hooks/603552912/deliveries?per_page=60" \
      --jq '.[] | select(.status_code==500) | "\(.delivered_at) \(.event) \(.status)"'

    2026-08-21T20:52:55.855Z  push  POST https://drone.lukemarsden.net/hook giving up after 1 attempt(s): Post "https://drone.lukemarsden.net/hook": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
    2026-08-21T15:52:38.568Z  push  (same)
    2026-08-21T15:50:43.191Z  push  (same)

Keywords for search: **"giving up after 1 attempt(s)"**, **"context deadline
exceeded"**, **"Client.Timeout exceeded while awaiting headers"**,
**status_code 500**, **GitHub webhook not retried**.

Two facts collide:

- GitHub allows a webhook **10 seconds** to respond.
- GitHub **does not retry** a failed delivery — `giving up after 1 attempt(s)`.

Drone's `/hook` handler does real synchronous work: signature validation, repo
lookup, fetching `.drone.yml` from the GitHub API, build creation. When that
crossed 10s, the delivery was gone permanently, and with it the build.

There was no alert, no failed build, no red X. Just absence — the worst CI
failure mode, because absence looks like "not started yet".

A `pull_request synchronize` hook 8 seconds earlier returned 200, but
`.drone.yml` triggers only on `push`/`tag`, so it was inert.

    trigger:
      event:
        - push
        - tag

(Confirmed across 125 builds of history: 124 `push`, 1 `tag`, zero `pull_request`.)

Fetching an individual delivery body needs an extra scope:

    gh: This API operation needs the "admin:repo_hook" scope.

## 4. The fix: put a relay in front

Before — Drone's latency sits on GitHub's critical path:

    GitHub --(10s limit, 0 retries)--> Drone     # slow Drone = lost build

After — the relay answers instantly and owns delivery:

    GitHub --(~0.5s 200)--> WebhookRelay --(60s, 8 attempts, 24h queue)--> Drone

The headline win is not the retries. It is **decoupling the acknowledgement
from the delivery**. GitHub now talks to something that is always fast, so
GitHub's 10s limit stops being a constraint on Drone at all. Even with retries
disabled this removes the failure mode.

### MCP setup

    claude mcp add --transport http webhookrelay https://my.webhookrelay.com/v1/mcp \
      --header "Authorization: Bearer sk-whrm-REDACTED"

Tools used: `create_bucket`, `create_output`, `update_output`,
`list_webhook_logs`, `get_webhook_log`, `list_buckets`, `delete_bucket`.

### Bucket

    create_bucket {
      "name": "helix-drone",
      "destination": "https://drone.lukemarsden.net/hook",
      "internal": false
    }
    → endpoint_url: https://01m0nspf19eykaba8qp6crg5zd.hooks.webhookrelay.com

### Output tuning — every value chosen against a specific failure

    update_output {
      "lock_path":         true,   # always POST exactly /hook, never /hook/
      "retries":           5,      # +5 on top of the first 3 attempts = 8
      "timeout_seconds":   60,     # default 20 could clip the same slow path
      "tls_verification":  true,   # DEFAULT IS false — surprising, fix it
      "durability": { "enabled": true, "schedule": "medium",
                      "handoff_after": "15m", "deadline": "24h" }
    }

Deliberately NOT set: `response_from_output`. That makes the input reply with
the destination's response — which would re-couple GitHub's 10s timeout to
Drone's latency and undo the entire point. Keep the static fast 200.

## 5. The part that could have broken everything: HMAC

Drone validates GitHub's `X-Hub-Signature-256`, an HMAC-SHA256 over the **raw
request body**. Any relay that re-serializes JSON — reorders keys, changes
whitespace — silently invalidates every signature. You would trade lost builds
for rejected builds.

### Proof 1 — manual probe

    request_body:  {"zen":"Keep it logically awesome.","hook_id":603552912,...}
    Content-Length: 99
    X-Hub-Signature-256: sha256=7ea7fb87…      ← forwarded
    X-Github-Event: ping                        ← forwarded
    X-Github-Delivery: probe-0001-relay-passthrough
    → Drone responded 200 in 77ms

### Proof 2 — the real one

Created a throwaway GitHub webhook with a secret we control
(`probe-secret-abc123`), fired `POST /repos/{o}/{r}/hooks/{id}/pings`, then
recomputed the HMAC over the body **as the relay delivered it**:

    sig sent    : sha256=d8acea04077b98470cc10d1823851dc5625474d999adcbd7ec16cdd5998dcafe
    sig w/known : sha256=d8acea04077b98470cc10d1823851dc5625474d999adcbd7ec16cdd5998dcafe
    MATCH       : True

GitHub signed the *original* payload and it still validated after the hop.
That is byte-exact passthrough, proven rather than assumed.

## 6. The trap worth the whole blog post

Repointing a GitHub webhook looks like a one-liner. One of the two obvious
ways to do it **destroys the webhook secret**.

### DESTRUCTIVE — `PATCH /repos/{owner}/{repo}/hooks/{id}`

    gh api -X PATCH repos/helixml/helix/hooks/669228346 -f config[url]=https://new.example/hook

    before: {"content_type":"json","insecure_ssl":"0","secret":"********","url":"…"}
    after:  {"content_type":"form","insecure_ssl":"0","url":"…/patched"}

`config` is replaced wholesale. Two things die:

1. `secret` — gone. The next ping arrived with **no `X-Hub-Signature-256` header
   at all**. Surviving headers were only: `X-Github-Delivery`, `X-Github-Event`,
   `X-Github-Hook-Id`, `X-Github-Hook-Installation-Target-Id`,
   `X-Github-Hook-Installation-Target-Type`.
2. `content_type` — silently reverted `json` → `form`.

Either alone kills CI. And it is **unrecoverable**: GitHub never returns a
secret (`"secret": "********"`), and Drone hides `secret`/`signer` from
non-admin users, so there is nothing to read it back from.

### SAFE — `PATCH /repos/{owner}/{repo}/hooks/{id}/config`

    gh api -X PATCH repos/helixml/helix/hooks/669228346/config -f url=https://new.example/hook

    after: {"content_type":"json","insecure_ssl":"0","secret":"********","url":"…/patched2"}
    → signature present, matches known secret, Content-Type: application/json

Only the supplied fields change. This is the one to use.

Method: never test this on production CI. Create a throwaway hook with a
secret you control, point it at a bucket with no forwarding output, and let the
signature tell you the truth.

## 7. End-to-end verification

Signed ping through the full chain:

    GitHub delivery: ping 200 OK
    relay log:       status=sent code=200 69ms

Real push on a throwaway branch (`test/webhookrelay-verify`, commit `97ac01f3`):

    Drone build 3873  event=push  after=97ac01f3  status=running
    builds created for 97ac01f3: count=1     ← no duplicate delivery

Drone validated the relayed signature and created the build. A ping alone would
not have proven this — only a push exercises build creation.

## 8. Honest residual risks

- **Duplicate deliveries on tags.** If Drone processes a hook but answers
  slowly, a retry creates a second build. Harmless on branches. On a tag it is a
  second concurrent production deploy, because `deploy-prod` (`.drone.yml:2559`,
  `scripts/deploy-prod.sh "$DRONE_TAG"`) is gated on `refs/tags/*`. The 60s relay
  timeout makes this far less likely than GitHub's 10s, but the real fix is a
  lock in the deploy script. Drone does not dedupe on `X-GitHub-Delivery`.
- **Fork PRs still get nothing.** Drone repo config has `ignore_forks: true`, and
  `.drone.yml` triggers only on push/tag. https://github.com/helixml/helix/pull/3094
  (from `iamwaqasahmad/helix-fork`) is unaffected by any of this.
- **The relay is now a dependency** of all CI, and sees every payload of a
  private repo.
- **Drone is still slow sometimes.** The relay stops that costing builds; it does
  not fix the underlying handler latency.

## 9. Rollback

    gh api -X PATCH repos/helixml/helix/hooks/603552912/config \
      -f url=https://drone.lukemarsden.net/hook

## 10. Recommended belt-and-braces

A reconciliation job comparing each GitHub branch head against Drone's latest
build for that branch, triggering the missing ones:

    POST /api/repos/helixml/helix/builds?branch=<branch>

Catches losses from any cause, including an outage of the relay itself. When
the failure mode is silence, you want something that actively looks for gaps.
