import { memo, useCallback, useState } from "react";

import ReviewCommentAnnotation from "./ReviewCommentAnnotation";

export interface WorkspaceFileCommentEntry {
  id: string;
  kind: "draft" | "comment";
  startLine: number;
  endLine: number;
  text: string;
}

interface WorkspaceFileCommentProps {
  entry: WorkspaceFileCommentEntry;
  onCancel: (entryId: string) => void;
  onSubmit: (entry: WorkspaceFileCommentEntry, text: string) => void;
}

const WorkspaceFileComment = memo(function WorkspaceFileComment({
  entry,
  onCancel,
  onSubmit,
}: WorkspaceFileCommentProps) {
  const [draftText, setDraftText] = useState(entry.text);
  const submit = useCallback(() => {
    const text = draftText.trim();
    if (text) onSubmit(entry, text);
  }, [draftText, entry, onSubmit]);
  const lineLabel = entry.startLine === entry.endLine
    ? `L${entry.startLine}`
    : `L${entry.startLine}–${entry.endLine}`;

  return (
    <ReviewCommentAnnotation
      kind={entry.kind}
      text={entry.kind === "draft" ? draftText : entry.text}
      rangeLabel={lineLabel}
      onTextChange={entry.kind === "draft" ? setDraftText : undefined}
      onCancel={() => onCancel(entry.id)}
      onSubmit={() => submit()}
      submitLabel="Add comment"
    />
  );
});

export default WorkspaceFileComment;
