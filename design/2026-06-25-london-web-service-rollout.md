# London (prod) web-service hosting rollout — runbook

Date: 2026-06-25. Goal: switch prod (helix.ml) on to **web-service hosting** — host web services
(find-ai first) on `*.apps.helix.ml`, mirroring what now works on meta.helix.ml, with nginx
terminating TLS via a certbot DNS-01 wildcard cert.

This is a **one-time feature switch**. The version-bump portion (Phase 3) is delegated to the
generic **`design/2026-06-25-prod-version-bump-runbook.md`** — read that for the controlplane +
runner upgrade mechanics, topology, and rollback. This doc covers only the web-service-specific
work.

## STATUS (2026-06-25)
- **Phase 1 ✅** — `*.apps.helix.ml` DNS (grey-cloud, pre-existing) + certbot DNS-01 wildcard issued.
- **Phase 2 ✅** — nginx `*.apps.helix.ml` block live, TLS verified, no regression on existing domains.
- **Phase 3 ✅** — controlplane + runner both on **2.11.28** (clean, 0 errors). Rollback snapshot:
  `data/helix-postgres@pre-2.11.28-20260625-071423`.
- **Phase 4 ✅** — `DEV_SUBDOMAIN=apps.helix.ml` set; log confirms `vhost middleware enabled
  base_domain=apps.helix.ml`. certmagic off (nginx terminates).
- **Phase 5 ⏳ BLOCKED** — no FindAI project exists on prod (it's only on meta); zero web-service
  states on prod. Needs the project created on prod (owner org + repo) before deploy. **Decision
  pending.**
- **Phase 6** — vestigial flag cleanup, not started.

## Why this shape
- Prod's TLS edge is HOST nginx (`/etc/nginx/nginx.conf`, monolithic) which owns `:443`/`:80`
  for ~13 live domains incl. registry.helix.ml + app.helix.ml. You can't cleanly mix
  SNI-passthrough on that shared `:443`, so **nginx terminates the wildcard** (certbot DNS-01)
  and Helix serves the vhost over **HTTP behind nginx** — certmagic stays OFF in prod (unlike
  meta, which has no nginx and lets certmagic terminate per-host on-demand).
- The web-service feature lives in the 2.11.28 images (PRs #2718 certmagic-persistence,
  #2720 self-healing, #2721 dns-proxy + lean boot, #2722 supervisor fix).

## Prerequisites
- Release **2.11.28** built & pushed (Step 1 of the generic runbook) — includes the four PRs above.

## Phase 1 — Cloudflare DNS + wildcard cert (no prod impact until nginx reload)
1. Cloudflare: add `*.apps.helix.ml` A record → london public IP (same as app.helix.ml),
   **grey-cloud (DNS-only)** so nginx terminates TLS, not CF. (TXT for the DNS-01 challenge is
   created/removed automatically by the plugin; cloud colour is irrelevant for TXT.)
2. On london, add the cloudflare DNS plugin to the snap certbot (5.6.0 already installed):
   ```bash
   snap install certbot-dns-cloudflare
   snap set certbot trust-plugin-with-root=ok
   snap connect certbot:plugin certbot-dns-cloudflare
   mkdir -p /root/.secrets
   printf 'dns_cloudflare_api_token = <ZONE_TOKEN>\n' > /root/.secrets/cloudflare.ini
   chmod 600 /root/.secrets/cloudflare.ini
   ```
   (Same Cloudflare zone token used for meta.)
3. Issue the wildcard via DNS-01 + register an nginx-reload deploy hook (renews automatically on
   the existing `snap.certbot.renew.timer`):
   ```bash
   certbot certonly --dns-cloudflare \
     --dns-cloudflare-credentials /root/.secrets/cloudflare.ini \
     -d '*.apps.helix.ml' \
     --deploy-hook 'systemctl reload nginx'
   ```
   → `/etc/letsencrypt/live/apps.helix.ml/{fullchain,privkey}.pem`.

## Phase 2 — nginx wildcard server block (shared blast radius — gate on `nginx -t`) — DONE 2026-06-25
nginx.conf is **monolithic** (no conf.d/sites-enabled include); the `$connection_upgrade` map
already exists. The controlplane is `http://localhost:8001` (helix-api-1 → :8080); `localhost:8180`
is keycloak. The vhost router lives in the controlplane, so proxy to **8001**.
4. Back up `/etc/nginx/nginx.conf` (timestamped). Insert the block below at the **end of the
   `http{}` block** (before its final closing brace), so it sees the maps/upstreams/cert includes:
   ```nginx
   server {                                   # http→https redirect
       listen 80; listen [::]:80;
       server_name *.apps.helix.ml;
       return 301 https://$host$request_uri;
   }
   server {
       listen 443 ssl; listen [::]:443 ssl;
       server_name *.apps.helix.ml;
       ssl_certificate     /etc/letsencrypt/live/apps.helix.ml/fullchain.pem;
       ssl_certificate_key /etc/letsencrypt/live/apps.helix.ml/privkey.pem;
       include /etc/letsencrypt/options-ssl-nginx.conf;
       ssl_dhparam /etc/letsencrypt/ssl-dhparams.pem;
       location / {
           proxy_pass         http://localhost:8001;   # controlplane (vhost router by Host)
           proxy_redirect off; proxy_http_version 1.1;
           client_max_body_size 0; proxy_buffering off; proxy_cache off;
           proxy_set_header   Host $host;               # vhost router keys on Host — MUST be $host (not $server_name)
           proxy_set_header   X-Real-IP $remote_addr;
           proxy_set_header   X-Forwarded-For $proxy_add_x_forwarded_for;
           proxy_set_header   X-Forwarded-Proto https;
           proxy_set_header   X-Forwarded-Host $host;
           proxy_set_header   Upgrade $http_upgrade;    # ws (terminal/logs streams)
           proxy_set_header   Connection $connection_upgrade;
       }
   }
   ```
5. `nginx -t` → `systemctl reload nginx` (graceful; registry.helix.ml/app.helix.ml stay up).
   **Risk gate**: a bad config fails `-t`; never reload without it passing — this nginx also
   fronts the registry and the cloud app.
6. Install a renewal-reload hook so future cert renewals reload nginx:
   `printf '#!/bin/sh\nsystemctl reload nginx\n' > /etc/letsencrypt/renewal-hooks/deploy/reload-nginx.sh && chmod +x`.
7. **Copy the working config to `/data/helix-app/nginx.conf`** (it's on ZFS, snapshotted +
   backed up to rsync.net): `cp -a /etc/nginx/nginx.conf /data/helix-app/nginx.conf`.
8. Verify (from london, via `--resolve …:127.0.0.1` to avoid hairpin-NAT): SNI
   `find-ai.apps.helix.ml` serves `CN=*.apps.helix.ml`; `curl` → 200; app.helix.ml /
   registry.helix.ml still serve their own cert + 200 (no regression). **Verified.**

## Phase 3 — upgrade controlplane + runner to 2.11.28
6. Follow **`design/2026-06-25-prod-version-bump-runbook.md`** with `NEW_VERSION=2.11.28`,
   `OLD_VERSION=2.11.14` (DB backup → controlplane → runner → verify both online on 2.11.28).
   This is the highest-risk step (it upgrades the live cloud app), covered there.

## Phase 4 — enable web-service hosting config on prod
7. In `/data/helix-app/.env`, mirror meta's web-service env, swapping base domain + disabling
   certmagic (nginx terminates TLS here): set the web-service base domain to `apps.helix.ml`
   and `HELIX_VHOST_TLS_MODE` to OFF. `docker compose up -d api`.
   (Verify exact var names against meta's `.env` at execution — mirror, don't guess.)

## Phase 5 — deploy find-ai + verify
8. Enable web-service on the FindAI project, deploy, verify
   `curl https://find-ai.apps.helix.ml` → 200, DB persisted to per-project `/data`.
9. Confirm health-monitor running: `docker logs helix-api-1 | grep -i "health-monitor"`.

## Phase 6 — cleanup (separate, non-blocking)
10. Remove vestigial `privileged_mode_enabled` flag (write-only; nothing branches on it) in its
    own small PR.

## Open items to confirm at execution
- Exact meta web-service env var names (base domain + TLS mode) to mirror.
- Whether app.helix.ml's cert is a SAN on helix.ml or standalone (informational only).
