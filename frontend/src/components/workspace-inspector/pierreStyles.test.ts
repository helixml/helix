import { describe, expect, it } from "vitest";
import { fileDiffPath, parseRenderablePatch } from "./pierreStyles";

describe("parseRenderablePatch", () => {
  it("parses multiple git files for the virtualized diff surface", () => {
    const result = parseRenderablePatch([
      "diff --git a/a.ts b/a.ts",
      "index 1111111..2222222 100644",
      "--- a/a.ts",
      "+++ b/a.ts",
      "@@ -1 +1 @@",
      "-const a = 1",
      "+const a = 2",
      "diff --git a/b.ts b/b.ts",
      "new file mode 100644",
      "--- /dev/null",
      "+++ b/b.ts",
      "@@ -0,0 +1 @@",
      "+export {}",
    ].join("\n"));

    expect(result?.kind).toBe("files");
    if (result?.kind !== "files") return;
    expect(result.files.map(fileDiffPath)).toEqual(["a.ts", "b.ts"]);
  });

  it("returns null for an empty patch", () => {
    expect(parseRenderablePatch("  ")).toBeNull();
  });
});
