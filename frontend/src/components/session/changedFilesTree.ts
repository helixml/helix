import type { TypesInteractionCodeChangeFile } from "../../api/api";

export interface ChangeStat {
  additions: number;
  deletions: number;
}

export interface ChangeScopeSummary {
  label: string;
  fileCount: number;
}

export const CHANGED_FILES_AUTO_EXPAND_FILE_LIMIT = 5;
export const CHANGED_FILES_AUTO_EXPAND_LINE_LIMIT = 200;
export const CHANGED_FILES_PREVIEW_FILE_LIMIT = 3;
export const CHANGED_FILES_PREVIEW_SCOPE_LIMIT = 4;

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

function pathSegments(path: string): string[] {
  return path.replace(/\\/g, "/").split("/").filter(Boolean);
}

export function changedFileName(path: string): string {
  return pathSegments(path).at(-1) || path;
}

function changedFileScope(path: string): string {
  const segments = pathSegments(path);
  return segments.length > 1 ? segments[0] || "root" : "root";
}

export function shouldAutoExpandChangedFiles(
  files: TypesInteractionCodeChangeFile[],
  isLatest: boolean,
): boolean {
  if (!isLatest || files.length > CHANGED_FILES_AUTO_EXPAND_FILE_LIMIT) return false;
  const stat = summarizeChanges(files);
  return stat.additions + stat.deletions <= CHANGED_FILES_AUTO_EXPAND_LINE_LIMIT;
}

export function formatCompactChangeCount(value: number): string {
  if (value < 1_000) return String(value);
  if (value < 1_000_000) {
    const thousands = value / 1_000;
    return `${thousands < 10 ? thousands.toFixed(1).replace(/\.0$/, "") : Math.round(thousands)}k`;
  }
  if (value < 1_000_000_000) {
    const millions = value / 1_000_000;
    return `${millions < 10 ? millions.toFixed(1).replace(/\.0$/, "") : Math.round(millions)}m`;
  }
  const billions = value / 1_000_000_000;
  return `${billions < 10 ? billions.toFixed(1).replace(/\.0$/, "") : Math.round(billions)}b`;
}

export function summarizeChangedFileScopes(
  files: TypesInteractionCodeChangeFile[],
  limit = CHANGED_FILES_PREVIEW_SCOPE_LIMIT,
): ChangeScopeSummary[] {
  const scopes = new Map<string, { fileCount: number; firstIndex: number }>();
  codeChangeFiles(files).forEach((file, index) => {
    const label = changedFileScope(file.path);
    const current = scopes.get(label);
    scopes.set(label, {
      fileCount: (current?.fileCount || 0) + 1,
      firstIndex: current?.firstIndex ?? index,
    });
  });

  return Array.from(scopes, ([label, scope]) => ({ label, ...scope }))
    .sort(
      (left, right) =>
        right.fileCount - left.fileCount ||
        left.firstIndex - right.firstIndex ||
        left.label.localeCompare(right.label),
    )
    .slice(0, limit)
    .map(({ label, fileCount }) => ({ label, fileCount }));
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

export function representativeFiles(
  files: TypesInteractionCodeChangeFile[],
  limit = CHANGED_FILES_PREVIEW_FILE_LIMIT,
) {
  const selected: ReturnType<typeof codeChangeFiles> = [];
  const paths = new Set<string>();
  const scopes = new Set<string>();
  for (const file of codeChangeFiles(files)) {
    const scope = changedFileScope(file.path);
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
