export type WorkspaceTabCloseAction = "close" | "close_others" | "close_right" | "close_all";

interface WorkspaceTabCloseResult {
  activeFile: string | null;
  openFiles: string[];
}

export function closeWorkspaceTabs(
  openFiles: readonly string[],
  activeFile: string | null,
  targetFile: string,
  action: WorkspaceTabCloseAction,
): WorkspaceTabCloseResult {
  const targetIndex = openFiles.indexOf(targetFile);
  if (targetIndex === -1) return { activeFile, openFiles: [...openFiles] };

  let remainingFiles: string[];
  switch (action) {
    case "close":
      remainingFiles = openFiles.filter((file) => file !== targetFile);
      break;
    case "close_others":
      remainingFiles = [targetFile];
      break;
    case "close_right":
      remainingFiles = openFiles.slice(0, targetIndex + 1);
      break;
    case "close_all":
      remainingFiles = [];
      break;
  }

  if (!activeFile || remainingFiles.includes(activeFile)) {
    return { activeFile, openFiles: remainingFiles };
  }

  return {
    activeFile: remainingFiles[Math.min(targetIndex, remainingFiles.length - 1)] || null,
    openFiles: remainingFiles,
  };
}
