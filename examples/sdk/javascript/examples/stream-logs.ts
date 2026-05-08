// stream-logs.ts
//
// Starts a long-running command in detached mode and streams the live SSE log
// feed to the terminal until the command finishes.

import { Sandbox } from "../dist/index.js";

async function main() {
  const baseURL = required("HELIX_URL");
  const apiKey = required("HELIX_API_KEY");
  const organizationId = required("HELIX_ORG_ID");

  const sandbox = await Sandbox.create({
    baseURL,
    apiKey,
    organizationId,
    name: `sdk-logs-${Date.now()}`,
    runtime: "headless-ubuntu",
    timeout_seconds: 600,
  });

  try {
    await sandbox.waitForRunning();

    // A short loop so we don't tie up CI forever — adjust as needed.
    const cmd = await sandbox.runCommand({
      cmd: "/bin/bash",
      args: ["-c", "for i in 1 2 3 4 5; do echo \"tick $i\"; sleep 1; done"],
      detached: true,
    });
    console.log("started cmd:", cmd.cmdId);

    for await (const chunk of cmd.logs({ follow: true })) {
      process.stdout.write(`[${chunk.stream}] ${chunk.data}`);
      if (!chunk.data.endsWith("\n")) process.stdout.write("\n");
    }

    const final = await cmd.wait();
    console.log("\nfinal exit:", final.exitCode);
  } finally {
    await sandbox.destroy().catch((e: unknown) => console.warn("destroy:", e));
  }
}

function required(name: string): string {
  const v = process.env[name];
  if (!v) {
    console.error(`Missing env var: ${name}`);
    process.exit(1);
  }
  return v;
}

main().catch((e) => {
  console.error("FAILED:", e);
  process.exit(1);
});
