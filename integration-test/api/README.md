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
