import React, { FC, useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { LineAnnotation, SelectedLineRange } from "@pierre/diffs";
import { Editor } from "@pierre/diffs/editor";
import { EditProvider, File, Virtualizer } from "@pierre/diffs/react";
import {
  Box,
  Button,
  CircularProgress,
  IconButton,
  Tooltip,
  Typography,
} from "@mui/material";
import { RefreshCw, Save } from "lucide-react";
import useLightTheme from "../../hooks/useLightTheme";
import useSnackbar from "../../hooks/useSnackbar";
import { DIFF_UNSAFE_CSS, PIERRE_THEMES } from "./pierreStyles";
import {
  getWorkspaceFileSaveError,
  useUpdateWorkspaceFile,
} from "./workspaceReviewService";
import {
  buildWorkspaceReviewComment,
  type WorkspaceReviewComment,
} from "./workspaceReviewComments";
import WorkspaceFileComment, {
  type WorkspaceFileCommentEntry,
} from "./WorkspaceFileComment";

interface CommentGroup {
  entries: WorkspaceFileCommentEntry[];
}

type CommentAnnotation = LineAnnotation<CommentGroup>;

interface WorkspaceEditableFileProps {
  sessionId: string;
  workspace?: string;
  path: string;
  initialContents: string;
  initialContentHash: string;
  comments: readonly WorkspaceReviewComment[];
  onUpsertComment: (comment: WorkspaceReviewComment) => void;
  onRemoveComment: (commentId: string) => void;
  onReload: () => Promise<{ contents?: string; content_hash?: string } | undefined>;
}

let commentSequence = 0;

function nextCommentId(): string {
  commentSequence += 1;
  return `workspace-comment-${Date.now()}-${commentSequence}`;
}

function normalizeRange(range: SelectedLineRange): { startLine: number; endLine: number } {
  return {
    startLine: Math.min(range.start, range.end),
    endLine: Math.max(range.start, range.end),
  };
}

function annotationsFromComments(
  comments: readonly WorkspaceReviewComment[],
  path: string,
): CommentAnnotation[] {
  return comments
    .filter((comment) => comment.filePath === path)
    .map((comment) => ({
      lineNumber: comment.endIndex + 1,
      metadata: {
        entries: [{
          id: comment.id,
          kind: "comment" as const,
          startLine: comment.startIndex + 1,
          endLine: comment.endIndex + 1,
          text: comment.text,
        }],
      },
    }));
}

function remapAnnotations(annotations: readonly CommentAnnotation[]): CommentAnnotation[] {
  return annotations.map((annotation) => ({
    ...annotation,
    metadata: {
      entries: annotation.metadata.entries.map((entry) => {
        const lineSpan = entry.endLine - entry.startLine;
        return {
          ...entry,
          endLine: annotation.lineNumber,
          startLine: Math.max(1, annotation.lineNumber - lineSpan),
        };
      }),
    },
  }));
}

const WorkspaceEditableFile: FC<WorkspaceEditableFileProps> = ({
  sessionId,
  workspace,
  path,
  initialContents,
  initialContentHash,
  comments,
  onUpsertComment,
  onRemoveComment,
  onReload,
}) => {
  const lightTheme = useLightTheme();
  const snackbar = useSnackbar();
  const updateFile = useUpdateWorkspaceFile(sessionId, workspace, path);
  const updateFileRef = useRef(updateFile);
  const upsertCommentRef = useRef(onUpsertComment);
  const removeCommentRef = useRef(onRemoveComment);
  const reloadFileRef = useRef(onReload);
  const snackbarRef = useRef(snackbar);
  const [contents, setContents] = useState(initialContents);
  const [contentHash, setContentHash] = useState(initialContentHash);
  const [savedContents, setSavedContents] = useState(initialContents);
  const [selectedLines, setSelectedLines] = useState<SelectedLineRange | null>(null);
  const [annotations, setAnnotations] = useState<CommentAnnotation[]>(() =>
    annotationsFromComments(comments, path),
  );
  const [conflicted, setConflicted] = useState(false);
  const contentsRef = useRef(contents);
  const savedContentsRef = useRef(savedContents);
  const sourceContentHashRef = useRef(initialContentHash);
  updateFileRef.current = updateFile;
  upsertCommentRef.current = onUpsertComment;
  removeCommentRef.current = onRemoveComment;
  reloadFileRef.current = onReload;
  snackbarRef.current = snackbar;
  contentsRef.current = contents;
  savedContentsRef.current = savedContents;
  const dirty = contents !== savedContents;
  const commentsFingerprint = comments
    .filter((comment) => comment.filePath === path)
    .map((comment) => `${comment.id}:${comment.startIndex}:${comment.endIndex}:${comment.text}`)
    .join("\0");

  useEffect(() => {
    setAnnotations((current) => {
      const draft = current.flatMap((annotation) =>
        annotation.metadata.entries.some((entry) => entry.kind === "draft") ? [annotation] : [],
      );
      return [...annotationsFromComments(comments, path), ...draft];
    });
    // commentsFingerprint is the primitive snapshot of the relevant comments.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [commentsFingerprint, path]);

  const editor = useMemo(() => new Editor<CommentGroup>({
    persistState: true,
    persistStateStorage: "inMemory",
    onChange: (file, nextAnnotations) => {
      contentsRef.current = file.contents;
      setContents(file.contents);
      if (!nextAnnotations) return;
      const remapped = remapAnnotations(nextAnnotations as CommentAnnotation[]);
      setAnnotations(remapped);
      for (const annotation of remapped) {
        for (const entry of annotation.metadata.entries) {
          if (entry.kind !== "comment") continue;
          upsertCommentRef.current(buildWorkspaceReviewComment({
            id: entry.id,
            filePath: path,
            startLine: entry.startLine,
            endLine: entry.endLine,
            text: entry.text,
            fileContents: file.contents,
          }));
        }
      }
    },
  }), [path]);

  useEffect(() => () => editor.cleanUp(), [path]);

  useEffect(() => {
    if (initialContentHash === sourceContentHashRef.current) return;
    if (contentsRef.current !== savedContentsRef.current) {
      setConflicted(true);
      return;
    }
    sourceContentHashRef.current = initialContentHash;
    setContents(initialContents);
    setSavedContents(initialContents);
    setContentHash(initialContentHash);
    contentsRef.current = initialContents;
    savedContentsRef.current = initialContents;
  }, [initialContentHash, initialContents]);

  const beginComment = useCallback((range: SelectedLineRange | null) => {
    setSelectedLines(range);
    if (!range) return;
    const { startLine, endLine } = normalizeRange(range);
    const entry: WorkspaceFileCommentEntry = {
      id: nextCommentId(),
      kind: "draft",
      startLine,
      endLine,
      text: "",
    };
    setAnnotations((current) => [
      ...current.flatMap((annotation) => {
        const entries = annotation.metadata.entries.filter((candidate) => candidate.kind !== "draft");
        return entries.length > 0 ? [{ ...annotation, metadata: { entries } }] : [];
      }),
      { lineNumber: endLine, metadata: { entries: [entry] } },
    ]);
  }, []);

  const removeEntry = useCallback((entryId: string) => {
    setSelectedLines(null);
    removeCommentRef.current(entryId);
    setAnnotations((current) => current.flatMap((annotation) => {
      const entries = annotation.metadata.entries.filter((entry) => entry.id !== entryId);
      return entries.length > 0 ? [{ ...annotation, metadata: { entries } }] : [];
    }));
  }, []);

  const submitEntry = useCallback((entry: WorkspaceFileCommentEntry, text: string) => {
    upsertCommentRef.current(buildWorkspaceReviewComment({
      id: entry.id,
      filePath: path,
      startLine: entry.startLine,
      endLine: entry.endLine,
      text,
      fileContents: contentsRef.current,
    }));
    setSelectedLines(null);
    setAnnotations((current) => current.map((annotation) => ({
      ...annotation,
      metadata: {
        entries: annotation.metadata.entries.map((candidate) =>
          candidate.id === entry.id ? { ...candidate, kind: "comment", text } : candidate,
        ),
      },
    })));
  }, [path]);

  const renderAnnotation = useCallback((annotation: CommentAnnotation) => (
    <Box sx={{ py: 0.5 }}>
      {annotation.metadata.entries.map((entry) => (
        <WorkspaceFileComment
          key={entry.id}
          entry={entry}
          onCancel={removeEntry}
          onSubmit={submitEntry}
        />
      ))}
    </Box>
  ), [removeEntry, submitEntry]);

  const save = useCallback(async () => {
    if (contentsRef.current === savedContentsRef.current || updateFileRef.current.isPending) return;
    try {
      const saved = await updateFileRef.current.mutateAsync({
        contents: contentsRef.current,
        expectedContentHash: contentHash,
      });
      const confirmedContents = saved.contents ?? contentsRef.current;
      setContents(confirmedContents);
      setSavedContents(confirmedContents);
      setContentHash(saved.content_hash || contentHash);
      contentsRef.current = confirmedContents;
      savedContentsRef.current = confirmedContents;
      setConflicted(false);
      snackbarRef.current.success(`Saved ${path}`);
    } catch (error) {
      const status = (error as { response?: { status?: number } })?.response?.status;
      setConflicted(status === 409);
      snackbarRef.current.error(getWorkspaceFileSaveError(error, path));
    }
  }, [contentHash, path]);

  const reload = useCallback(async () => {
    const fresh = await reloadFileRef.current();
    if (fresh?.contents === undefined || !fresh.content_hash) return;
    sourceContentHashRef.current = fresh.content_hash;
    setContents(fresh.contents);
    setSavedContents(fresh.contents);
    setContentHash(fresh.content_hash);
    contentsRef.current = fresh.contents;
    savedContentsRef.current = fresh.contents;
    setConflicted(false);
  }, []);

  return (
    <Box
      data-workspace-editable-file
      onKeyDownCapture={(event) => {
        if (event.key.toLowerCase() !== "s" || (!event.ctrlKey && !event.metaKey)) return;
        event.preventDefault();
        void save();
      }}
      sx={{ height: "100%", minHeight: 0, display: "flex", flexDirection: "column" }}
    >
      <Box sx={{ minHeight: 34, px: 1, display: "flex", alignItems: "center", gap: 1, borderBottom: "1px solid", borderColor: "divider", flexShrink: 0 }}>
        <Typography variant="caption" noWrap title={path} sx={{ flex: 1, color: "text.secondary" }}>
          {path}
        </Typography>
        {conflicted ? (
          <Button size="small" startIcon={<RefreshCw size={14} />} onClick={reload} sx={{ minHeight: 28, textTransform: "none" }}>
            Reload changed file
          </Button>
        ) : (
          <Typography variant="caption" color={dirty ? "warning.main" : "text.secondary"}>
            {dirty ? "Unsaved" : "Saved"}
          </Typography>
        )}
        <Tooltip title="Save file">
          <span>
            <IconButton
              aria-label={`Save ${path}`}
              size="small"
              onClick={save}
              disabled={!dirty || updateFile.isPending || conflicted}
              sx={{ width: 30, height: 30 }}
            >
              {updateFile.isPending ? <CircularProgress size={18} /> : <Save size={18} />}
            </IconButton>
          </span>
        </Tooltip>
      </Box>
      <EditProvider editor={editor as unknown as React.ComponentProps<typeof EditProvider>["editor"]}>
        <Virtualizer style={{ flex: 1, minHeight: 0, overflow: "auto" }}>
          <File<CommentGroup>
            file={{ name: path, contents, cacheKey: `${path}:${contentHash}` }}
            contentEditable
            selectedLines={selectedLines}
            lineAnnotations={annotations}
            className="workspace-editable-file"
            options={{
              disableFileHeader: true,
              enableGutterUtility: !annotations.some((annotation) =>
                annotation.metadata.entries.some((entry) => entry.kind === "draft"),
              ),
              enableLineSelection: true,
              onGutterUtilityClick: setSelectedLines,
              onLineSelectionChange: setSelectedLines,
              onLineSelectionEnd: beginComment,
              overflow: "scroll",
              theme: PIERRE_THEMES,
              themeType: lightTheme.isLight ? "light" : "dark",
              tokenizeMaxLineLength: 1_000,
              unsafeCSS: DIFF_UNSAFE_CSS,
            }}
            renderAnnotation={renderAnnotation}
          />
        </Virtualizer>
      </EditProvider>
    </Box>
  );
};

export default WorkspaceEditableFile;
