import { DEFAULT_THEMES, parsePatchFiles, type FileDiffMetadata } from "@pierre/diffs";

export const PIERRE_THEMES = DEFAULT_THEMES;

export const DIFF_UNSAFE_CSS = `
[data-diffs-header], [data-diff], [data-file], [data-error-wrapper], [data-virtualizer-buffer] {
  --diffs-header-font-family: ui-sans-serif, system-ui, sans-serif !important;
  --diffs-font-family: ui-monospace, SFMono-Regular, Menlo, monospace !important;
  --diffs-bg: var(--workspace-bg) !important;
  --diffs-light-bg: var(--workspace-bg) !important;
  --diffs-dark-bg: var(--workspace-bg) !important;
  --diffs-token-light-bg: transparent;
  --diffs-token-dark-bg: transparent;
  --diffs-bg-context-override: color-mix(in srgb, var(--workspace-bg) 97%, var(--workspace-fg));
  --diffs-bg-hover-override: color-mix(in srgb, var(--workspace-bg) 94%, var(--workspace-fg));
  --diffs-bg-separator-override: color-mix(in srgb, var(--workspace-bg) 96%, var(--workspace-fg));
  --diffs-bg-buffer-override: color-mix(in srgb, var(--workspace-bg) 92%, var(--workspace-fg));
  --diffs-bg-addition-override: color-mix(in srgb, var(--workspace-bg) 84%, #2ea043);
  --diffs-bg-addition-number-override: color-mix(in srgb, var(--workspace-bg) 76%, #2ea043);
  --diffs-bg-addition-hover-override: color-mix(in srgb, var(--workspace-bg) 78%, #2ea043);
  --diffs-bg-deletion-override: color-mix(in srgb, var(--workspace-bg) 84%, #f85149);
  --diffs-bg-deletion-number-override: color-mix(in srgb, var(--workspace-bg) 76%, #f85149);
  --diffs-bg-deletion-hover-override: color-mix(in srgb, var(--workspace-bg) 78%, #f85149);
  background: var(--workspace-bg) !important;
}
[data-diffs-header] {
  position: sticky !important;
  top: 0;
  z-index: 4;
  min-height: 32px !important;
  padding: 6px 10px !important;
  border-bottom-color: var(--workspace-border) !important;
  background: var(--workspace-bg) !important;
  color: var(--workspace-fg) !important;
  font-size: 12px !important;
}
[data-file-info] { background: var(--workspace-bg) !important; }
:is([data-separator="line-info"], [data-separator="line-info-basic"]) {
  height: 24px !important;
  margin: 0 !important;
  background: var(--workspace-bg) !important;
  color: var(--workspace-muted) !important;
}
[data-column-number] { color: var(--workspace-muted) !important; }
`;

export const TREE_UNSAFE_CSS = `
:host {
  --trees-bg-override: transparent;
  --trees-selected-bg-override: color-mix(in srgb, currentColor 12%, transparent);
  --trees-hover-bg-override: color-mix(in srgb, currentColor 7%, transparent);
  --trees-border-color-override: color-mix(in srgb, currentColor 14%, transparent);
  --trees-font-family-override: ui-sans-serif, system-ui, sans-serif;
  --trees-font-size-override: 12px;
}
button[data-type='item'] { border-radius: 5px; }
`;

export type RenderablePatch =
  | { kind: "files"; files: FileDiffMetadata[] }
  | { kind: "raw"; text: string };

export function parseRenderablePatch(patch: string | undefined): RenderablePatch | null {
  const normalized = patch?.trim();
  if (!normalized) return null;
  try {
    const files = parsePatchFiles(normalized).flatMap((parsed) => parsed.files);
    return files.length > 0 ? { kind: "files", files } : { kind: "raw", text: normalized };
  } catch {
    return { kind: "raw", text: normalized };
  }
}

export function fileDiffPath(file: FileDiffMetadata): string {
  const path = file.name || file.prevName || "";
  return path.startsWith("a/") || path.startsWith("b/") ? path.slice(2) : path;
}
