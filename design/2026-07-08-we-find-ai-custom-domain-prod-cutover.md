# we-find.ai custom-domain cutover onto prod SaaS (app.helix.ml)

Date: 2026-07-08. Goal: serve the **Find AI** web service at the customer apex **we-find.ai**
(+ www) from the **prod** SaaS, cutting it over from its current Replit host.

## Topology facts (verified live)
- Project (prod): **`prj_01kvz0e7b401545376fyyfxtta`** "Find AI", org `org_01kvzf9s7tarpm9pmg7vmwhfn6`,
  web service **enabled**, backend sandbox `sbx_01kwf3fghqychvyahmxs7d3zh5` port 8080, host `code-for-app`.
  (The `prj_01kv5j…` the user first linked is the **meta** copy — NOT this one.)
- Prod is `helix-cloud-london` (GCE europe-west2-a). Public ingress IP **34.39.116.64** = `ingress.helix.ml`.
- Prod version **2.11.45** (has TLS-ALPN-01 fallback, commit `eac0aab1f`). #2813 self-serve-acme is
  NOT needed for the direct-A approach; it only adds the `_acme-challenge` UI helper. (2.11.46 cut,
  CD pending, but not required here.)
- Prod `.env`: `HELIX_VHOST_TLS_MODE=auto`, `HELIX_VHOST_ACME_DNS_PROVIDER=cloudflare`,
  `HELIX_VHOST_CNAME_TARGET=ingress.helix.ml`. certmagic uses **production** LE
  (find-ai.apps.helix.ml cert issuer = CN=YE1). Staging lines in logs are just certmagic's
  post-failure fallback — red herring.

## nginx edge (prod, /etc/nginx/nginx.conf, monolithic)
- `:443` = **stream ssl_preread** map by SNI: legacy names → 127.0.0.1:8443 (nginx http),
  **`default` → 127.0.0.1:8444** = helix-api-1 :443 (certmagic). So unknown SNI (custom domains)
  DOES reach certmagic. `8444` confirmed = `127.0.0.1:8444->443/tcp` on helix-api-1.
- `:80` = per-host server blocks (return 301 → https). **No default_server / catch-all** →
  unknown host (we-find.ai) gets nginx **404**.

## DNS-01 vs ALPN (why no _acme-challenge record needed)
`vhost_tls.go` configures two issuers, tried in order: (1) DNS-01 via Cloudflare — only works for
names in the CF zone or with `_acme-challenge.<host>` CNAME delegation; (2) **TLS-ALPN-01 fallback**
over :443 — works for domains pointed DIRECTLY at the origin. we-find.ai will be a plain A → prod
(no CF proxy), so ALPN issues the cert. **No `_acme-challenge` record required.**
Confirmed by logs: DNS-01 failed (`expected 1 zone, got 0` — we-find.ai not in CF), ALPN-01 failed
ONLY because public DNS still points at Replit (34.111.179.208) — self-heals on DNS flip.

## The one real gap: domain verification over :80
`webservice/verifier.go` polls every 60s → GET `http://<host>/.well-known/helix-domain-verify/<token>`
(port 80, no redirect-follow). Dispatch (`vhost_middleware.go:149`) returns **503 "domain not yet
verified"** until `verified_at` is set. Prod nginx :80 → 404 for we-find.ai → auto-verify can never
succeed. Cert issuance does NOT need verification (gate allows any vhost_routes row), but **serving
the app does**.

### Resolution taken
- Added routes via prod API (owner key from DB): `we-find.ai` = `vhr_01kwy2e4bdsved5qqy7gmmzthm`,
  `www.we-find.ai` = `vhr_01kwy2e4bvhbvq84s22ynk4rm3`.
- **Manually set `verified_at=now()`** on both (we own the domain; HTTP ownership proof is redundant
  for an operator cutover). Reversible: `UPDATE vhost_routes SET verified_at=NULL WHERE id IN (...)`.
- **Pre-flight PASSED**: `curl -H 'Host: we-find.ai' http://localhost:8001/` → 200, 142KB,
  `<title>Find AI — AI Talent, Matched with Purpose</title>`, identical to find-ai baseline.
- Proper fix (still TODO, optional): nginx :80 `default_server` proxying
  `/.well-known/helix-domain-verify/` → helix (localhost:8001) + `return 301 https://$host` — makes
  auto-verify + http→https work for ALL future custom domains. Touches shared prod edge (gate on `nginx -t`).

## Remaining step (NOT done — needs explicit go: live customer-domain cutover)
123-reg DNS for we-find.ai (registrar; nameservers ns45/ns46.domaincontrol.com):
- apex **A `@`**: `34.111.179.208` → **`34.39.116.64`** (staged in the 123-reg edit form, UNSAVED).
- `www` CNAME → `we-find.ai` already (resolves to prod after apex change) — leave as-is.
- Leave NS, MX (smtp.google.com email), google-site-verification TXT, replit-verify TXT.
- TTL 600s. **Rollback** = set apex A back to `34.111.179.208`.

## INCIDENT 2026-07-09: find-ai.apps.helix.ml down (blocks switchover)
Discovered while pre-flighting: `find-ai.apps.helix.ml` returning 502 (backend down),
NOT caused by the nginx change (the `:8001` backend path 502s too).

**Root cause (app):** `.helix/startup.sh` (helix-specs branch, line 56) ran
`exec docker compose -f docker-compose.prod.yml up`, but **`docker-compose.prod.yml` never
existed** in helixml/find-ai (`git log --all` empty). The repo only tracks `docker-compose.yml`
(dev stack: Go `api` on 8080 proxying Next.js `npm run dev` frontend + Postgres). A redeploy
~06:30 UTC killed the old `docker compose up` (→ stack stopped), then the new startup.sh failed
`open docker-compose.prod.yml: no such file` → app never bound :8080 → health-monitor looped
a failed redeploy every ~11 min; rollback also failed (the broken ref is in startup.sh, not the
app commit, so every SHA fails identically).

**Fix (restore):** committed one-liner to helixml/find-ai@helix-specs (`be2c45d`):
`docker-compose.prod.yml` → `docker-compose.yml`. Triggered
`POST /projects/prj_01kvz0e7b401545376fyyfxtta/web-service/deploy` → deploy `live`, all 3
containers healthy, `https://find-ai.apps.helix.ml` → 200/0.27s. Recovery loop stopped.
(Follow-up for find-ai team: if a static prod build was intended, add a real
`docker-compose.prod.yml` and re-point startup.sh.)

**Platform bug (Helix — "make it not happen again", NOT yet done):** `webservice/controller.go`
`runDeploy`/`deployInPlace` (~L173-229, L516-549) kills the running `docker compose up` before the
new startup proves healthy, and on readiness failure `rollback()` (L604-618) re-runs the SAME
startup.sh → also fails → site stays down indefinitely + retries destructively. The header comment
(L9-10) intentionally accepts a *brief* restart window (single /data DB writer ⇒ not trivially
blue-green). Proposed hardening options (needs Luke's call, then code + release + prod deploy):
  1. **Pre-teardown validation** — before killing the running stack, run `docker compose config`
     (or check the referenced compose file exists) in the sandbox; abort the deploy and KEEP the
     old stack if invalid. Cheap, directly prevents this class (missing/invalid compose).
  2. **Rollback-to-last-KNOWN-GOOD + stop-loop** — if rollback also fails readiness, stop the
     auto-retry, mark degraded, and alert (Slack/janitor) instead of looping every 11 min.
  3. (Bigger) true keep-old-until-new-healthy, constrained by the single-DB-writer design.
Recommend 1 + 2 together (small, targeted). we-find.ai switchover HELD until this ships.

## FOLLOW-UP 2026-07-09: proper prod-build fix (supersedes the dev fallback)
The dev fallback (be2c45d) was a stopgap. Real intent: spec task **#2242 "Serve Production
Build in Helix Web-Service Mode"** (`spt_01kwyaer8v1t9122dr0cfpmyn9`, meta, still status
`implementation`) built the prod compose but was never merged. Its startup.sh half landed on
helix-specs; its app half (docker-compose.prod.yml + api/Dockerfile.prod + /api/version) sat
unmerged on `feature/002242-serve-production-build`.
- Merged that branch → main via **find-ai PR #18** (main now `42e8407`).
- Smoke-built the prod image in the sandbox (isolated `-p ftprodtest build`) — clean, warmed cache.
- Re-pointed startup.sh → docker-compose.prod.yml (helix-specs `bc483dd`), redeployed.
- **Verified live:** `mode=static`, `Serving static frontend from /www`, listening :8080,
  `https://find-ai.apps.helix.ml` → 200 (77KB static vs 142KB dev). Prod app+db containers only
  (no dev frontend server). Rollback = startup.sh → docker-compose.yml.
- Nit: `/api/version` shows version/gitSha/buildTime = "unknown" (ldflags not injected in the
  web-service deploy path) — cosmetic, follow-up.
- Renamed meta's find-ai project (`prj_01kv5j…`) → "Find AI (DEPRECATED – use SaaS)"; its web
  service is still enabled at find-ai.meta.helix.ml (disable pending).

## STILL TODO (user's sequence: harden Helix → THEN we-find.ai switchover)
Helix observability feature (branch `feature/web-service-deploy-logs`, not finished): surface the
sandbox deploy log (`/data/.helix-webservice.log`, read via hydra exec) in the Web Service tab;
friendly "stack didn't bind to port N — view logs" errors (deploy.Error already stored); stop the
public leak (hydra server.go:584 passes the app-down 502 with internal IP straight through —
api proxyToContainer only catches transport errors, not hydra's 5xx passthrough). Do NOT assume
startup.sh uses compose. Then release + deploy to prod, THEN flip we-find.ai DNS.

## Browser note
chrome-devtools MCP connects to `--browserUrl 127.0.0.1:9222`. Original Chrome there was headless
(invisible on RDP). Fixed by killing it and relaunching **headful on the Wayland session**:
`XDG_RUNTIME_DIR=/run/user/1000 WAYLAND_DISPLAY=wayland-0 DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus
google-chrome-stable --remote-debugging-port=9222 --user-data-dir=/tmp/chrome-visible --ozone-platform=wayland
--no-sandbox ...`. (pkill footgun: `pkill -f 'chrome-mcp'` matched its own shell — use PIDs.)
