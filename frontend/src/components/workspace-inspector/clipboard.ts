export { copyTextToClipboard } from '../../utils/clipboard'

export function workspaceFilePath(workspaceRoot: string, relativePath: string): string {
  return `${workspaceRoot.replace(/\/+$/, "")}/${relativePath.replace(/^\/+|\/+$/g, "")}`;
}
