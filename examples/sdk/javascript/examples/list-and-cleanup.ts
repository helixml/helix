// list-and-cleanup.ts
//
// Lists every sandbox in the organization, prints them as a table, and
// optionally deletes those tagged `cleanup=true` (set HELIX_DESTROY=1 to
// actually delete; otherwise it's a dry-run).

import { Sandbox } from "../dist/index.js";

async function main() {
  const baseURL = required("HELIX_URL");
  const apiKey = required("HELIX_API_KEY");
  const organizationId = required("HELIX_ORG_ID");
  const destroy = process.env.HELIX_DESTROY === "1";

  const sandboxes = await Sandbox.list({ baseURL, apiKey, organizationId });

  if (sandboxes.length === 0) {
    console.log("no sandboxes in this organization");
    return;
  }

  console.log(`Found ${sandboxes.length} sandbox(es):`);
  console.log("ID                                   STATUS    RUNTIME          NAME");
  for (const s of sandboxes) {
    const id = s.sandboxId.padEnd(36);
    const status = (s.status ?? "").padEnd(9);
    const runtime = (s.runtime ?? "").padEnd(16);
    console.log(`${id} ${status} ${runtime} ${s.name ?? ""}`);
  }

  // Delete sandboxes flagged cleanup=true.
  for (const s of sandboxes) {
    if (s.details.tags?.cleanup === "true") {
      if (destroy) {
        console.log(`destroying ${s.sandboxId}…`);
        await s.destroy().catch((e: unknown) => console.warn("  failed:", e));
      } else {
        console.log(`(dry-run) would destroy ${s.sandboxId} (tag cleanup=true)`);
      }
    }
  }

  if (!destroy) {
    console.log('\nset HELIX_DESTROY=1 to actually delete tagged sandboxes');
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
