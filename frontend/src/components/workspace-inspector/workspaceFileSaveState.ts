export interface ReconciledWorkspaceFileSave {
  contents: string;
  savedContents: string;
}

export function reconcileWorkspaceFileSave(
  currentContents: string,
  submittedContents: string,
  confirmedContents: string,
): ReconciledWorkspaceFileSave {
  return {
    contents:
      currentContents === submittedContents
        ? confirmedContents
        : currentContents,
    savedContents: confirmedContents,
  };
}
