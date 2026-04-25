# Helix Local Development

This guide walks you through setting up a local Helix development environment from a fresh clone. If you only want to run Helix (not modify it), use the quickstart installer in the [README](./README.md) instead.

If you get stuck, please ask in [Discord](https://discord.gg/VJftd844GE).

## Prerequisites

| Tool | Version | Notes |
|------|---------|-------|
| Docker | latest | Docker Desktop on macOS/Windows, or Docker Engine + Compose v2 on Linux |
| Go | 1.24+ | Required to build / test the API and CLI |
| Node.js | 18+ | Required to build the frontend (also pulls in `yarn`) |
| Make | any | Used by a few helper targets |
| Git | 2.x | |

A GPU is **not** required for local development - you can point Helix at any OpenAI-compatible API instead of running a local runner. See [Inference: GPU runner or external provider](#3-inference-gpu-runner-or-external-provider) below.

## 1. Clone and configure

```bash
git clone git@github.com:helixml/helix.git
cd helix
cp .env.example-prod .env
```

External contributors should fork the repo first and clone their fork.

The defaults in `.env.example-prod` are tuned for production. For local dev they work as-is, but you will typically want to set at least:

```dotenv
# Point Helix at an external LLM provider (skip if you're attaching a GPU runner)
INFERENCE_PROVIDER=openai
OPENAI_API_KEY=sk-...
OPENAI_BASE_URL=https://api.openai.com/v1
```

The full list of configuration options lives in [`api/pkg/config/config.go`](./api/pkg/config/config.go).

## 2. Start the stack

```bash
./stack start
```

This brings up the dev compose stack (`docker-compose.dev.yaml`) - the API, frontend, Postgres, vectorchord-kodit (RAG), and supporting services. The control plane is served on <http://localhost:8080>.

Verify the stack is healthy:

```bash
docker compose -f docker-compose.dev.yaml ps
```

You should see something like:

```
NAME                          STATUS    PORTS
helix-api-1                   Up        0.0.0.0:8080->80/tcp
helix-frontend-1              Up        0.0.0.0:8081->8081/tcp
helix-postgres-1              Up        0.0.0.0:5432->5432/tcp
helix-vectorchord-kodit-1     Up
helix-chrome-1                Up
helix-searxng-1               Up
helix-registry-1              Up
```

Then open <http://localhost:8080> and create an account. In dev mode the first registered user is automatically promoted to admin.

`./stack help` lists every supported subcommand.

## 3. Inference: GPU runner or external provider

Helix needs an inference backend. Pick **one** of:

### Option A: External LLM provider (no GPU needed)

Set `INFERENCE_PROVIDER`, `OPENAI_API_KEY`, and `OPENAI_BASE_URL` in `.env` (any OpenAI-compatible endpoint works - OpenAI, Together, vLLM, etc.). Restart the API:

```bash
docker compose -f docker-compose.dev.yaml restart api
```

### Option B: Local GPU runner

Follow the [runner attachment docs](https://helix.ml/docs). If your dev machine has no GPU, you can:

- Run the runner on a remote GPU box and tunnel it back over SSH.
- Spin up a short-lived runner on RunPod (or similar) and reach your laptop via [webhookrelay](https://webhookrelay.com/).

#### SSH tunnel example

On the remote GPU machine:

```bash
ssh -p $SSH_PORT -R 8080:localhost:8080 user@remote.example.com
git clone https://github.com/helixml/helix
cd helix
cat > .env <<'EOF'
SERVER_PORT=9080                          # runner uses 9080; API stays on 8080
API_HOST=http://localhost:8080            # forwarded back to your laptop
RUNNER_TOKEN=oh-hallo-insecure-token      # MUST match the control plane token
EOF
docker compose -f docker-compose.runner.yaml up -d
```

Then visit `/dashboard` on your local Helix and confirm the runner appears. `RUNNER_TOKEN` must match between control plane and runner - the dev stack uses `oh-hallo-insecure-token` by default. **Never use that value in production.**

## 4. Day-to-day workflow

### Hot reload

- **API**: the dev container watches `api/` with [air](https://github.com/air-verse/air) and rebuilds on save. No manual restart needed.
- **Frontend**: Vite serves the React app on port 8081 with HMR; the API at 8080 proxies to it. Edits in `frontend/src/` are visible immediately.

### Rebuilding individual services

```bash
./stack rebuild <service>          # rebuild + restart one service
./stack up <service>               # start one service if it's down
```

You typically only need a rebuild after changing dependencies or Dockerfiles.

### Stopping the stack

```bash
./stack stop
```

Add `STOP_POSTGRES=1` / `STOP_PGVECTOR=1` if you want to stop the data services too.

## 5. Linting and tests

### Go

```bash
./stack lint                    # runs golangci-lint
./stack test [./path/...]       # runs go test (boots postgres/chrome via compose)
```

You'll need [`golangci-lint`](https://golangci-lint.run/welcome/install/) installed locally for `lint`.

For a quick build check without running the suite:

```bash
cd api && go build ./...
```

### Frontend

```bash
cd frontend
yarn install
yarn build                      # type-check + production build
yarn lint                       # eslint
```

### CI

CI runs the full suite (Drone). Push your branch and open a PR - see [CONTRIBUTING.md](./CONTRIBUTING.md) for the workflow.

## 6. API client / OpenAPI

The frontend uses a generated TypeScript client. After changing API handlers or types, regenerate:

```bash
./stack update_openapi
```

This refreshes the swagger spec and the generated client in `frontend/src/api/`.

## Project structure

```
helix/
├── api/                   # Go backend (control plane, agents, runner)
│   ├── cmd/               # CLI entrypoints (helix serve, helix runner, ...)
│   ├── pkg/               # Library packages (agent, server, store, scheduler, ...)
│   └── main.go
├── frontend/              # React + TypeScript + Vite + MUI
│   └── src/
├── runner/                # GPU runner configuration
├── scripts/               # Helper scripts
├── stack                  # Dev/build orchestrator (entrypoint for most tasks)
├── docker-compose.dev.yaml    # Dev stack
├── docker-compose.runner.yaml # Standalone runner
└── .env                   # Local config (gitignored)
```

## Troubleshooting

- **Port 8080 already in use**: another service is bound to 8080. Stop it or change the host port mapping in `docker-compose.dev.yaml`.
- **Frontend shows "API not reachable"**: check `docker compose -f docker-compose.dev.yaml logs api` - most often a missing required env var.
- **Runner not appearing in `/dashboard`**: check `RUNNER_TOKEN` matches and that the runner container can reach `API_HOST`.
- **`./stack lint` fails to find `golangci-lint`**: install it from <https://golangci-lint.run/welcome/install/>.

## Further reading

- [Helix documentation](https://helix.ml/docs)
- [`CONTRIBUTING.md`](./CONTRIBUTING.md) - branch / PR workflow
- [`README.md`](./README.md) - product overview
