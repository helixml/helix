import React, { FC, useEffect, useMemo, useRef, useState } from "react";
import { CodeView, type CodeViewHandle } from "@pierre/diffs/react";
import type { CodeViewDiffItem } from "@pierre/diffs";
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  IconButton,
  MenuItem,
  Select,
  Tooltip,
  Typography,
} from "@mui/material";
import { Columns2, Pilcrow, RefreshCw, Rows3, WrapText } from "lucide-react";
import type { TypesWorkspaceReviewSource } from "../../api/api";
import useLightTheme from "../../hooks/useLightTheme";
import {
  useTurnWorkspaceReview,
  useWorkspaceReview,
} from "./workspaceReviewService";
import {
  DIFF_UNSAFE_CSS,
  PIERRE_THEMES,
  fileDiffPath,
  parseRenderablePatch,
} from "./pierreStyles";

type ReviewScope = "all" | "branch" | "working_tree";

interface WorkspaceDiffSurfaceProps {
  sessionId: string;
  workspace?: string;
  baseBranch: string;
  pollInterval: number;
  interactionId?: string;
  selectedFile?: string;
  onOpenFile: (path: string) => void;
  onExitTurn: () => void;
  onWorkspaceResolved: (workspace: string | undefined) => void;
}

const sourceLabel = (source: TypesWorkspaceReviewSource | undefined) =>
  source?.title || "Changes";

const WorkspaceDiffSurface: FC<WorkspaceDiffSurfaceProps> = ({
  sessionId,
  workspace,
  baseBranch,
  pollInterval,
  interactionId,
  selectedFile,
  onOpenFile,
  onExitTurn,
  onWorkspaceResolved,
}) => {
  const lightTheme = useLightTheme();
  const [scope, setScope] = useState<ReviewScope>("all");
  const [ignoreWhitespace, setIgnoreWhitespace] = useState(false);
  const [layout, setLayout] = useState<"unified" | "split">("unified");
  const [wordWrap, setWordWrap] = useState(false);
  const viewerRef = useRef<CodeViewHandle<undefined>>(null);
  const liveReview = useWorkspaceReview(
    sessionId,
    workspace,
    baseBranch,
    ignoreWhitespace,
    pollInterval,
  );
  const turnReview = useTurnWorkspaceReview(
    sessionId,
    interactionId,
    ignoreWhitespace,
  );

  useEffect(() => {
    onWorkspaceResolved(liveReview.data?.workspace);
  }, [liveReview.data?.workspace, onWorkspaceResolved]);

  const source = interactionId
    ? turnReview.data
    : liveReview.data?.sources?.find((candidate) => candidate.id === scope);
  const liveSources = liveReview.data?.sources || [];
  const selectedScope = liveSources.some((candidate) => candidate.id === scope)
    ? scope
    : "";
  const query = interactionId ? turnReview : liveReview;
  const fileCount = source?.files?.length || 0;
  const renderable = useMemo(
    () => parseRenderablePatch(source?.patch),
    [source?.patch],
  );
  const items = useMemo<CodeViewDiffItem[]>(() => {
    if (renderable?.kind !== "files") return [];
    return [...renderable.files]
      .sort((a, b) => fileDiffPath(a).localeCompare(fileDiffPath(b)))
      .map((fileDiff, index) => ({
        id: fileDiff.cacheKey || `${fileDiffPath(fileDiff)}:${index}`,
        type: "diff",
        fileDiff,
      }));
  }, [renderable]);

  useEffect(() => {
    if (!selectedFile) return;
    const item = items.find((candidate) => fileDiffPath(candidate.fileDiff) === selectedFile);
    if (item) viewerRef.current?.scrollTo({ type: "item", id: item.id, align: "start" });
  }, [items, selectedFile]);

  return (
    <Box sx={{ display: "flex", flexDirection: "column", minHeight: 0, height: "100%" }}>
      <Box
        sx={{
          minHeight: 40,
          px: 1,
          display: "flex",
          alignItems: "center",
          gap: 1,
          borderBottom: "1px solid",
          borderColor: "divider",
        }}
      >
        {interactionId ? (
          <Button size="small" color="inherit" onClick={onExitTurn} sx={{ fontSize: 11, textTransform: "none" }}>
            Turn changes · Back to current
          </Button>
        ) : (
          <Select
            size="small"
            value={selectedScope}
            onChange={(event) => setScope(event.target.value as ReviewScope)}
            aria-label="Diff scope"
            sx={{ height: 28, fontSize: 12, minWidth: 150 }}
          >
            {liveSources.length === 0 && (
              <MenuItem value="" disabled>
                Changes unavailable
              </MenuItem>
            )}
            {liveSources.map((candidate) => (
              <MenuItem key={candidate.id} value={candidate.id} sx={{ fontSize: 12 }}>
                {sourceLabel(candidate)}
              </MenuItem>
            ))}
          </Select>
        )}
        <Typography variant="caption" sx={{ color: "text.secondary", whiteSpace: "nowrap" }}>
          {fileCount} {fileCount === 1 ? "file" : "files"}
          <Box component="span" sx={{ color: "success.main", ml: 1 }}>+{source?.total_additions || 0}</Box>
          <Box component="span" sx={{ color: "error.main", ml: 0.5 }}>-{source?.total_deletions || 0}</Box>
        </Typography>
        <Box sx={{ flex: 1 }} />
        <Box
          role="group"
          aria-label="Diff layout"
          sx={{ display: "flex", height: 28, border: "1px solid", borderColor: "divider", borderRadius: 1, overflow: "hidden" }}
        >
          <Tooltip title="Unified diff">
            <IconButton
              size="small"
              onClick={() => setLayout("unified")}
              aria-label="Unified diff"
              aria-pressed={layout === "unified"}
              sx={{ width: 30, height: 28, borderRadius: 0, bgcolor: layout === "unified" ? "action.selected" : "transparent" }}
            >
              <Rows3 size={15} />
            </IconButton>
          </Tooltip>
          <Tooltip title="Split diff">
            <IconButton
              size="small"
              onClick={() => setLayout("split")}
              aria-label="Split diff"
              aria-pressed={layout === "split"}
              sx={{ width: 30, height: 28, borderLeft: "1px solid", borderColor: "divider", borderRadius: 0, bgcolor: layout === "split" ? "action.selected" : "transparent" }}
            >
              <Columns2 size={15} />
            </IconButton>
          </Tooltip>
        </Box>
        <Tooltip title={wordWrap ? "Disable line wrapping" : "Wrap long lines"}>
          <IconButton
            size="small"
            onClick={() => setWordWrap((value) => !value)}
            color={wordWrap ? "primary" : "default"}
            aria-label={wordWrap ? "Disable line wrapping" : "Wrap long lines"}
            aria-pressed={wordWrap}
          >
            <WrapText size={16} />
          </IconButton>
        </Tooltip>
        <Tooltip title={ignoreWhitespace ? "Include whitespace changes" : "Ignore whitespace changes"}>
          <IconButton
            size="small"
            onClick={() => setIgnoreWhitespace((value) => !value)}
            color={ignoreWhitespace ? "primary" : "default"}
            aria-label={ignoreWhitespace ? "Include whitespace changes" : "Ignore whitespace changes"}
            aria-pressed={ignoreWhitespace}
          >
            <Pilcrow size={16} />
          </IconButton>
        </Tooltip>
        <Tooltip title="Refresh changes">
          <span>
            <IconButton
              size="small"
              onClick={() => query.refetch()}
              disabled={query.isFetching}
              aria-label="Refresh changes"
            >
              <RefreshCw size={15} />
            </IconButton>
          </span>
        </Tooltip>
      </Box>

      {source?.truncated && (
        <Alert severity="warning" square sx={{ py: 0, fontSize: 12 }}>
          Preview truncated at the server limit. File counts remain complete.
        </Alert>
      )}
      {query.isLoading ? (
        <Box sx={{ flex: 1, display: "grid", placeItems: "center" }}><CircularProgress size={22} /></Box>
      ) : query.isError ? (
        <Box sx={{ p: 2 }}><Alert severity="error">Could not load workspace changes.</Alert></Box>
      ) : !renderable ? (
        <Box sx={{ flex: 1, display: "grid", placeItems: "center", color: "text.secondary" }}>
          <Typography variant="body2">No net changes in this selection.</Typography>
        </Box>
      ) : renderable.kind === "raw" ? (
        <Box component="pre" sx={{ m: 0, p: 2, flex: 1, overflow: "auto", fontSize: 12, whiteSpace: wordWrap ? "pre-wrap" : "pre" }}>
          {renderable.text}
        </Box>
      ) : (
        <Box
          sx={{ flex: 1, minHeight: 0 }}
          onClickCapture={(event) => {
            const path = event.nativeEvent.composedPath?.() || [];
            const title = path.find(
              (node): node is HTMLElement => node instanceof HTMLElement && node.hasAttribute("data-title"),
            );
            const filePath = title?.textContent?.trim();
            if (filePath) onOpenFile(filePath.replace(/^[ab]\//, ""));
          }}
        >
          <CodeView
            ref={viewerRef}
            items={items}
            className="workspace-code-view"
            style={{ height: "100%", minHeight: 0 }}
            options={{
              theme: PIERRE_THEMES,
              themeType: lightTheme.isLight ? "light" : "dark",
              diffStyle: layout,
              diffIndicators: "bars",
              lineDiffType: "word-alt",
              overflow: wordWrap ? "wrap" : "scroll",
              stickyHeaders: true,
              tokenizeMaxLineLength: 1_000,
              unsafeCSS: DIFF_UNSAFE_CSS,
              itemMetrics: { diffHeaderHeight: 32, hunkSeparatorHeight: 24 },
              layout: { gap: 0, paddingTop: 0, paddingBottom: 0 },
            }}
          />
        </Box>
      )}
    </Box>
  );
};

export default WorkspaceDiffSurface;
