// file-roundtrip.ts
//
// Demonstrates writing a file into the sandbox, reading it back, listing the
// directory, and cleaning up.

import { Sandbox } from "../dist/index.js";

async function main() {
  const baseURL = required("HELIX_URL");
  const apiKey = required("HELIX_API_KEY");
  const organizationId = required("HELIX_ORG_ID");

  const sandbox = await Sandbox.create({
    baseURL,
    apiKey,
    organizationId,
    name: `sdk-files-${Date.now()}`,
    runtime: "headless-ubuntu",
    timeout_seconds: 600,
  });
  console.log("created sandbox", sandbox.sandboxId);

  try {
    await sandbox.waitForRunning();

    // Write a couple of files.
    await sandbox.writeFile("/tmp/hello.txt", "hello from helix\n");
    await sandbox.writeFile(
      "/tmp/script.sh",
      "#!/bin/bash\necho 'I am executable'\n",
      { mode: 0o755 },
    );

    // Read text back.
    const text = await sandbox.readFileText("/tmp/hello.txt");
    console.log("/tmp/hello.txt =>", JSON.stringify(text));

    // Run the script we just wrote.
    const r = await sandbox.runCommand("/tmp/script.sh");
    console.log("script exit:", r.exitCode);
    console.log("script stdout:", await r.stdout());

    // List /tmp.
    const entries = await sandbox.listDirectory("/tmp");
    console.log(`/tmp has ${entries.length} entries:`);
    for (const e of entries) {
      console.log(`  ${e.is_dir ? "d" : "-"} ${e.mode} ${e.size.toString().padStart(8)} ${e.name}`);
    }

    // Tidy up.
    await sandbox.deleteFile("/tmp/hello.txt");
    await sandbox.deleteFile("/tmp/script.sh");
    console.log("files removed");
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
