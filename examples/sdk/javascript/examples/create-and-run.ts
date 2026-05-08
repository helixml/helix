// create-and-run.ts
//
// Creates a sandbox, runs `uname -a`, prints the exit code + stdout, deletes
// the sandbox.
//
// Run with:
//   HELIX_URL=http://localhost:8080 \
//   HELIX_API_KEY=hlx_... \
//   HELIX_ORG_ID=org_... \
//   node --experimental-strip-types examples/create-and-run.ts

import { Sandbox } from "../dist/index.js";

async function main() {
  const baseURL = required("HELIX_URL");
  const apiKey = required("HELIX_API_KEY");
  const organizationId = required("HELIX_ORG_ID");

  console.log("→ Creating sandbox…");
  const sandbox = await Sandbox.create({
    baseURL,
    apiKey,
    organizationId,
    name: `sdk-example-${Date.now()}`,
    runtime: "headless-ubuntu",
    timeout_seconds: 600,
  });
  console.log("  id:", sandbox.sandboxId);

  try {
    console.log("→ Waiting for sandbox to be running…");
    await sandbox.waitForRunning({ timeoutMs: 5 * 60_000 });
    console.log("  status:", sandbox.status);

    console.log("→ Running `uname -a`…");
    const result = await sandbox.runCommand("uname", ["-a"]);
    console.log("  exit:", result.exitCode);
    console.log("  stdout:", await result.stdout());
  } finally {
    console.log("→ Destroying sandbox…");
    await sandbox.destroy().catch((e: unknown) => {
      console.warn("  destroy failed:", e);
    });
  }
}

function required(name: string): string {
  const v = process.env[name];
  if (!v) {
    console.error(`Missing required env var: ${name}`);
    process.exit(1);
  }
  return v;
}

main().catch((e) => {
  console.error("FAILED:", e);
  process.exit(1);
});
