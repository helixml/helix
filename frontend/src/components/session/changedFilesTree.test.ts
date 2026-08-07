import { describe, expect, it } from "vitest";
import {
  buildChangeTree,
  changedFileName,
  formatCompactChangeCount,
  representativeFiles,
  shouldAutoExpandChangedFiles,
  summarizeChangedFileScopes,
  summarizeChanges,
} from "./changedFilesTree";

const files = [
  { path: "frontend/src/App.tsx", additions: 8, deletions: 2 },
  { path: "frontend/src/main.tsx", additions: 3, deletions: 0 },
  { path: "api/pkg/server.go", additions: 4, deletions: 1 },
  { path: "design/review.md", additions: 12, deletions: 0 },
];

describe("changedFilesTree", () => {
  it("aggregates directory statistics and compacts single-child directories", () => {
    const tree = buildChangeTree(files);

    expect(tree.map((node) => node.name)).toEqual(["api/pkg", "design", "frontend/src"]);
    expect(tree.find((node) => node.path === "frontend/src")?.stat).toEqual({
      additions: 11,
      deletions: 2,
    });
    expect(summarizeChanges(files)).toEqual({ additions: 27, deletions: 3 });
  });

  it("selects a representative file from each top-level scope first", () => {
    expect(representativeFiles(files).map((file) => file.path)).toEqual([
      "frontend/src/App.tsx",
      "api/pkg/server.go",
      "design/review.md",
    ]);
  });

  it("summarizes prominent scopes and normalizes Windows paths", () => {
    expect(summarizeChangedFileScopes([
      ...files,
      { path: "frontend\\src\\Other.tsx", additions: 1, deletions: 0 },
      { path: "README.md", additions: 1, deletions: 0 },
    ])).toEqual([
      { label: "frontend", fileCount: 3 },
      { label: "api", fileCount: 1 },
      { label: "design", fileCount: 1 },
      { label: "root", fileCount: 1 },
    ]);
    expect(changedFileName("frontend\\src\\Other.tsx")).toBe("Other.tsx");
  });

  it("uses the T3 compact thresholds and stat formatting", () => {
    expect(shouldAutoExpandChangedFiles(files, true)).toBe(true);
    expect(shouldAutoExpandChangedFiles(files, false)).toBe(false);
    expect(shouldAutoExpandChangedFiles([{ path: "big.go", additions: 201 }], true)).toBe(false);
    expect(formatCompactChangeCount(999)).toBe("999");
    expect(formatCompactChangeCount(6_500)).toBe("6.5k");
    expect(formatCompactChangeCount(1_300_000)).toBe("1.3m");
  });
});
