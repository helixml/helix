# @helixml/sandbox — Vercel-style SDK for Helix Sandboxes

A small TypeScript/JavaScript SDK that mirrors the shape of Vercel's Sandbox
SDK, but talks to Helix. Create ephemeral containers, run commands, read and
write files, and stream a terminal — all scoped to an organization.

## Install

```bash
# from inside this folder
yarn install
yarn build
```

When you publish this to a registry, install with `npm i @helixml/sandbox` etc.

## Quick start

```ts
import { Sandbox } from "@helixml/sandbox";

const sandbox = await Sandbox.create({
  baseURL: process.env.HELIX_URL!,
  apiKey: process.env.HELIX_API_KEY!,
  organizationId: process.env.HELIX_ORG_ID!,
  name: "my-sandbox",
  runtime: "ubuntu-desktop",
  timeout_seconds: 600, // auto-deleted after 10 minutes
});

await sandbox.waitForRunning();

const result = await sandbox.runCommand("uname", ["-a"]);
console.log("exit", result.exitCode);
console.log(await result.stdout());

await sandbox.destroy();
```

## Required env vars for examples

- `HELIX_URL` — e.g. `http://localhost:8080`
- `HELIX_API_KEY` — `hlx_...` from /account
- `HELIX_ORG_ID` — `org_...` (visible in the Sandboxes URL `/orgs/{slug}`; use
  the id, not the slug)

## Examples

```bash
yarn build
node dist-examples/create-and-run.js   # or use --experimental-strip-types
```

The `package.json` exposes shortcuts:

```bash
yarn example:create-and-run
yarn example:files
yarn example:list
yarn example:logs
```

Each script destroys its sandbox before exiting (best-effort) so you don't
accumulate stale containers.

## Authoring

The SDK is written in plain TypeScript with no bundler — `yarn build` runs
`tsc` and emits ESM in `dist/`. The examples use Node's
`--experimental-strip-types` flag (Node 22+) to run `.ts` directly.

## Surface

```
Sandbox.create({ baseURL, apiKey, organizationId, name?, runtime?, env?, timeout_seconds?, … })
Sandbox.get({ baseURL, apiKey, organizationId, sandboxId })
Sandbox.list({ baseURL, apiKey, organizationId })

sandbox.refresh()
sandbox.waitForRunning()
sandbox.update({ name?, timeout_seconds?, tags? })
sandbox.destroy()

sandbox.runCommand("ls", ["-la"])
sandbox.runCommand({ cmd, args, cwd, env, sudo, detached, timeout_seconds })
sandbox.listCommands()
sandbox.getCommand(cmdId)
sandbox.killCommand(cmdId, signal?)
sandbox.streamLogs(cmdId, { stream, follow })   // async iterator

sandbox.readFile(path) / sandbox.readFileText(path)
sandbox.writeFile(path, body, { mode? })
sandbox.deleteFile(path, { recursive? })
sandbox.listDirectory(path)

sandbox.terminalURL({ shell? })   // open with new WebSocket(...)
```
