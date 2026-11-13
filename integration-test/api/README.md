# Running API integration tests

1. Prepare your .env file like you have for a working local dev setup
2. Set the environment variables from it:

```bash
export $(cat .env | xargs)
```

3. Run the tests:

```bash
./stack test-integration
```

The server-asset E2E test is part of this suite and runs in Drone's
`api-integration-test` step on every push. It starts an ephemeral SSH/SFTP
server and exercises the real API, Postgres persistence, RBAC, MCP tools,
detached command lifecycle, file operations, and the Helix SSH proxy. It does
not require a public server or CI secret.

To run only that test:

```bash
cd api && CGO_ENABLED=0 go build -o /tmp/helix . && cd ..
START_HELIX_TEST_SERVER=true \
PATH=/tmp:$PATH \
go test -v ./integration-test/api -run '^TestServerAssetE2E$' -count=1
```

Set `HELIX_INTEGRATION_SERVER_PORT` and
`HELIX_INTEGRATION_ASSET_SSH_PROXY_PORT` when the default ports 8080 and 2224
are already occupied.

## Authorization matrix

`TestAuthzMatrix` (`authz_matrix_*_test.go`) checks who may do what across
tenants. It builds one fixture: organizations A and B; A's owner, a read-only
member of A, B's owner, a user in no organization, a waitlisted account and an
admin; every kind of credential A's owner can hold (personal, org, project
spec-task, session, helix-org worker and app keys); and A's resources (projects,
repositories, agents, sessions, spec tasks, secrets, knowledge, triggers,
provider endpoints, artifacts, grants, filestore files). Every table row is
then sent as every actor it lists. It needs Postgres and the browser launcher,
but no LLM or sandbox. Drone runs it in its own push step (`api-authz-matrix`)
with `HELIX_INTEGRATION_NO_PROVIDER_ENV=true`, which starts the test server
without any LLM provider credentials; the matrix refuses to run without it
when such credentials are in the environment, so no row can make a real,
paid upstream call. The other integration tests skip it.

A row in `authz_matrix_routes_test.go` is a request and one expectation per
actor:

```go
{
	Method: http.MethodDelete, Path: "/api/v1/projects/{projA}",
	Expect: strangers(expect{aOwner: allow, aMember: deny}),
	Fresh:  freshProject, Check: "/api/v1/projects/{projA}",
},
```

- `{projA}` and the other placeholders are listed on `authzFixture`.
- `allow` wants a 2xx. `deny` wants 401, 403 or 404 with no fixture marker
  (the unique names of the fixture's resources) in the body. `reached` wants
  the request past authorization for routes that cannot finish in CI (no
  desktop, no LLM); use `ReachedMarker` when that answer is itself a 404.
- `strangers(...)` adds B's owner, the outsider, the waitlisted account and an
  anonymous caller as `deny`.
- Every row needs at least one `allow` or `reached` actor, so a route that
  fails for everyone cannot pass as "refused".
- A row that changes state sets `Fresh`, so allowed actors act on a copy, and
  `Check`, which re-reads the shared resource after each refusal
  (`CheckMarker` when that path lists several resources).
- List endpoints go in `listRows()` (`Contains` / `Excludes` by placeholder).
- A known gap stays in the table with `Skip: "tracked: <one-line description>"`.

Run it locally against a throwaway Postgres and browser launcher:

```bash
docker run -d --rm --name pg-authz -e POSTGRES_PASSWORD=postgres -p 15432:5432 postgres:17
docker build -f Dockerfile.chrome -t helix-chrome:dev .
docker run -d --rm --name chrome-authz -p 17317:7317 helix-chrome:dev
(cd api && CGO_ENABLED=0 go build -o /tmp/helix-bin/helix .)
export PATH=/tmp/helix-bin:$PATH START_HELIX_TEST_SERVER=true DEV_SUBDOMAIN=dev \
  HELIX_INTEGRATION_NO_PROVIDER_ENV=true \
  POSTGRES_HOST=localhost POSTGRES_PORT=15432 POSTGRES_USER=postgres \
  POSTGRES_PASSWORD=postgres POSTGRES_DATABASE=postgres \
  RAG_CRAWLER_LAUNCHER_URL=http://localhost:17317 \
  HELIX_INTEGRATION_SERVER_PORT=18480 HELIX_INTEGRATION_ASSET_SSH_PROXY_PORT=12224
helix serve --migrate-only
go test -count=1 -run '^TestAuthzMatrix$' ./integration-test/api/
# one table:  -run '^TestAuthzMatrix$/projects'
```
