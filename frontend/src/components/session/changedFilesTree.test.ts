import { describe, expect, it } from "vitest";
import { buildChangeTree, representativeFiles, summarizeChanges } from "./changedFilesTree";

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
});
