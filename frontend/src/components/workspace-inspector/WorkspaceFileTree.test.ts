import { describe, expect, it } from "vitest";
import type { TypesWorkspaceFileEntry } from "../../api/api";
import { ancestorDirectoryPaths, changedFileGitStatus, filteredTreePaths, treeSelectionNeedsSync } from "./WorkspaceFileTree";

const entries: TypesWorkspaceFileEntry[] = [
  { kind: "directory", path: "src" },
  { kind: "directory", path: "src/workspace-inspector" },
  { kind: "file", path: "src/workspace-inspector/second-turn.json" },
  { kind: "file", path: "src/workspace-inspector/other.ts" },
  { kind: "file", path: "README.md" },
];

describe("workspace file tree filtering", () => {
  it("maps changed files to Pierre git status decorations", () => {
    expect(changedFileGitStatus([
      { path: "src/changed.ts", kind: "modified" },
      { path: "src/new.ts", kind: "added" },
      { path: "src/copied.ts", kind: "copied" },
      { path: "src/mystery.ts", kind: "unknown" },
      { kind: "deleted" },
    ])).toEqual([
      { path: "src/changed.ts", status: "modified" },
      { path: "src/new.ts", status: "added" },
      { path: "src/copied.ts", status: "added" },
      { path: "src/mystery.ts", status: "modified" },
    ]);
  });

  it("uses AND-token matching and retains the matching file's ancestors", () => {
    expect(filteredTreePaths(entries, "workspace second json")).toEqual([
      "src/",
      "src/workspace-inspector/",
      "src/workspace-inspector/second-turn.json",
    ]);
  });

  it("includes a matched directory's descendants", () => {
    expect(filteredTreePaths(entries, "workspace inspector")).toEqual([
      "src/",
      "src/workspace-inspector/",
      "src/workspace-inspector/second-turn.json",
      "src/workspace-inspector/other.ts",
    ]);
  });

  it("derives every ancestor that must be expanded for search results", () => {
    expect(ancestorDirectoryPaths([
      "src/",
      "src/workspace-inspector/",
      "src/workspace-inspector/second-turn.json",
    ])).toEqual(["src/", "src/workspace-inspector/"]);
  });

  it("does not re-center a selection that originated in the tree", () => {
    expect(treeSelectionNeedsSync("src/workspace-inspector/other.ts", ["src/workspace-inspector/other.ts"])).toBe(false);
    expect(treeSelectionNeedsSync("README.md", ["src/workspace-inspector/other.ts"])).toBe(true);
  });
});
