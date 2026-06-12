# End-to-end validation in helix-in-helix

Tested in the inner Helix at `http://localhost:8080` with
`DEV_SUBDOMAIN=apps.localtest.me` and a real Python `http.server`
running inside a `python313` sandbox.

## Setup

```bash
echo 'DEV_SUBDOMAIN=apps.localtest.me' >> .env
docker compose -f docker-compose.dev.yaml restart api
# API log: vhost middleware enabled (project web services + preview tokens) base_domain=apps.localtest.me

# Create a sandbox via the existing Sandbox API
curl -X POST .../organizations/$ORG/sandboxes \
  -d '{"runtime":"python313","persistent":true,"timeout_seconds":-1}'
# -> sbx_01kty00mkm0yqfr6x3cj1c1a4a, status=running after ~12s

# Start a Python http.server on :8080 inside it
curl -X POST .../sandboxes/$SBX/commands \
  -d '{"cmd":"/bin/bash","args":["-c","cd /tmp/www && exec python3 -m http.server 8080"],"detached":true}'

# Wire up a project + vhost route directly via DB
INSERT INTO projects ...
INSERT INTO project_web_service_states (..., active_sandbox_id=$SBX) ...
INSERT INTO vhost_routes (hostname='webtest.apps.localtest.me',
  target_kind='project_web_service', target_id=$PROJECT_ID, port=8080, verified_at=now())
```

## What was validated

### 1. Vhost dispatch by Host header

```
$ curl -H "Host: webtest.apps.localtest.me" http://localhost:8080/
< HTTP/1.1 200 OK
< Server: SimpleHTTP/0.6 Python/3.13.14
<
<h1>Hello from sandbox</h1>
```

The `Server: SimpleHTTP/0.6 Python/3.13.14` header is from the
Python http.server running INSIDE the sandbox container — proves
traffic went vhost middleware → store lookup → RevDial →
hydra → docker exec → container → returned through the whole chain.

### 2. Fall-through dispatch

```
$ curl -o /dev/null -w "%{http_code}" -H "Host: bogus.example.com" http://localhost:8080/
200    # falls through to main app, which renders its own page

$ curl -o /dev/null -w "%{http_code}" http://localhost:8080/api/v1/config
200    # canonical hostname (localhost from SERVER_URL) → main API
```

### 3. Anti-hijack guards (every reserved-hostname category)

```
$ curl -X POST .../projects/X/web-service/domains \
    -d '{"hostname":"api.apps.localtest.me"}'
hostname is reserved: "api.apps.localtest.me" contains reserved label "api"

$ curl -X POST ...domains -d '{"hostname":"share-foo-bar-12345678.apps.localtest.me"}'
hostname is reserved: "share-"-prefixed hostnames are reserved for preview tokens

$ curl -X POST ...domains -d '{"hostname":"webhello.example.com"}'
{"id":"vhr_...","hostname":"webhello.example.com",...,"verification_token":"01kty0bdq..."}
```

### 4. Domain verification flow

```
$ curl http://localhost:8080/.well-known/helix-domain-verify/probe-12345
probe-12345

$ curl -o /dev/null -w "%{http_code}" -H "Host: webhello.example.com" http://localhost:8080/
503    # row exists but verified_at is null → 503

# Operator marks verified:
$ docker exec ...psql -c "UPDATE vhost_routes SET verified_at=now() WHERE hostname='webhello.example.com'"

$ curl -H "Host: webhello.example.com" http://localhost:8080/
<h1>Hello from sandbox</h1>    # now serving traffic
```

### 5. GET state aggregation

```
$ curl .../projects/$PROJECT_ID/web-service
{
  "state": {"project_id":"...","enabled":true,"container_port":8080,"active_sandbox_id":"sbx_..."},
  "domains": [{"hostname":"webtest.apps.localtest.me","is_default":true,"verified_at":"..."}],
  "deploys": []
}
```

### 6. Deploy orchestrator error path

```
# Project has no primary repo configured
$ curl -X POST .../projects/$PROJECT_ID/web-service/deploy -d '{}'
resolve primary repo: project has no primary repository configured
```

Clean failure with an actionable error. The happy path
(repo + .helix/startup.sh → orchestrator clones + execs +
cuts over) was not exercised because the helix-hosted git repo
creation endpoint resists test API keys; that path is built and
will activate as soon as a real repo is attached to the project.

## Bug found and fixed during testing

`vhost_middleware.go` was passing `state.ActiveSandboxID` as the
RevDial device key, but the device key is `hydra-<HostDeviceID>` not
`hydra-<sandbox.ID>` (sandboxes get assigned to a runner-host pool;
`HostDeviceID` names the runner). First curl returned
`502 Bad Gateway / failed to connect to sandbox: no connection`.

Fixed by loading the sandbox row and using `sb.HostDeviceID` as the
RevDial key while keeping `sb.ID` as the hydra container ID (hydra
registers sandbox-API containers under SessionID=sandbox.ID per
`sandbox/controller_provision.go:181`). Same fix applied to the
sandbox-preview dispatch, which unblocks `sbx_*` preview targets.

## Bug found and fixed: AutoMigrate table name

GORM derived `v_host_routes` from the `VHostRoute` struct name.
Added `func (VHostRoute) TableName() string { return "vhost_routes" }`
so the table matches what the store code queries.
