import { describe, expect, it } from "vitest";
import { fileDiffPath, parseRenderablePatch, resolveDiffFilePath } from "./pierreStyles";

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

// Opening a file from a diff header depends on scraping @pierre/diffs' private
// `[data-title]` node. Resolving that text against the rendered patch is what
// keeps a markup change in a future beta from opening bogus file tabs.
describe("resolveDiffFilePath", () => {
  const known = ["src/app.ts", "src/nested/app.ts", "README.md"];

  it("resolves an exact path and strips git a/ b/ prefixes", () => {
    expect(resolveDiffFilePath("src/app.ts", known)).toBe("src/app.ts");
    expect(resolveDiffFilePath("  b/README.md  ", known)).toBe("README.md");
  });

  it("resolves an abbreviated header only when the suffix is unambiguous", () => {
    expect(resolveDiffFilePath("nested/app.ts", known)).toBe("src/nested/app.ts");
    expect(resolveDiffFilePath("app.ts", known)).toBeNull();
  });

  it("refuses text that names no rendered file", () => {
    expect(resolveDiffFilePath("+12 -3", known)).toBeNull();
    expect(resolveDiffFilePath("", known)).toBeNull();
    expect(resolveDiffFilePath(undefined, known)).toBeNull();
    expect(resolveDiffFilePath("src/app.ts", [])).toBeNull();
  });
});
