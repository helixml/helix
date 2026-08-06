import type { TypesInteractionCodeChangeFile } from "../../api/api";

export interface ChangeStat {
  additions: number;
  deletions: number;
}

export type ChangeTreeNode =
  | { kind: "file"; name: string; path: string; stat: ChangeStat }
  | { kind: "directory"; name: string; path: string; stat: ChangeStat; children: ChangeTreeNode[] };

interface MutableDirectory {
  name: string;
  path: string;
  stat: ChangeStat;
  directories: Map<string, MutableDirectory>;
  files: ChangeTreeNode[];
}

export function codeChangeFiles(files: TypesInteractionCodeChangeFile[] | undefined) {
  return (files || []).filter(
    (file): file is TypesInteractionCodeChangeFile & { path: string } => !!file.path,
  );
}

export function summarizeChanges(files: TypesInteractionCodeChangeFile[]): ChangeStat {
  return files.reduce<ChangeStat>(
    (total, file) => ({
      additions: total.additions + (file.additions || 0),
      deletions: total.deletions + (file.deletions || 0),
    }),
    { additions: 0, deletions: 0 },
  );
}

function compact(node: ChangeTreeNode): ChangeTreeNode {
  if (node.kind === "file") return node;
  let result = { ...node, children: node.children.map(compact) };
  while (result.children.length === 1 && result.children[0].kind === "directory") {
    const child = result.children[0];
    result = { ...child, name: `${result.name}/${child.name}` };
  }
  return result;
}

function toNodes(directory: MutableDirectory): ChangeTreeNode[] {
  const byName = (a: { name: string }, b: { name: string }) =>
    a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" });
  const directories = Array.from(directory.directories.values())
    .sort(byName)
    .map<ChangeTreeNode>((child) => compact({
      kind: "directory",
      name: child.name,
      path: child.path,
      stat: child.stat,
      children: toNodes(child),
    }));
  return [...directories, ...directory.files.sort(byName)];
}

export function buildChangeTree(files: TypesInteractionCodeChangeFile[]): ChangeTreeNode[] {
  const root: MutableDirectory = {
    name: "",
    path: "",
    stat: { additions: 0, deletions: 0 },
    directories: new Map(),
    files: [],
  };
  for (const file of codeChangeFiles(files)) {
    const segments = file.path.replace(/\\/g, "/").split("/").filter(Boolean);
    const name = segments.at(-1);
    if (!name) continue;
    const stat = { additions: file.additions || 0, deletions: file.deletions || 0 };
    let current = root;
    const ancestors = [root];
    for (const segment of segments.slice(0, -1)) {
      const path = current.path ? `${current.path}/${segment}` : segment;
      let next = current.directories.get(segment);
      if (!next) {
        next = { name: segment, path, stat: { additions: 0, deletions: 0 }, directories: new Map(), files: [] };
        current.directories.set(segment, next);
      }
      current = next;
      ancestors.push(current);
    }
    current.files.push({ kind: "file", name, path: file.path, stat });
    for (const ancestor of ancestors) {
      ancestor.stat.additions += stat.additions;
      ancestor.stat.deletions += stat.deletions;
    }
  }
  return toNodes(root);
}

export function representativeFiles(files: TypesInteractionCodeChangeFile[], limit = 3) {
  const selected: ReturnType<typeof codeChangeFiles> = [];
  const paths = new Set<string>();
  const scopes = new Set<string>();
  for (const file of codeChangeFiles(files)) {
    const scope = file.path.split("/")[0] || "root";
    if (scopes.has(scope)) continue;
    selected.push(file);
    paths.add(file.path);
    scopes.add(scope);
    if (selected.length === limit) return selected;
  }
  for (const file of codeChangeFiles(files)) {
    if (!paths.has(file.path)) selected.push(file);
    if (selected.length === limit) break;
  }
  return selected;
}
