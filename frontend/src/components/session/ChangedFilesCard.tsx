import React, { FC, useEffect, useMemo, useState } from "react";
import { Box, Button, IconButton, Tooltip, Typography } from "@mui/material";
import { useTheme } from "@mui/material/styles";
import {
  ChevronRight,
  ChevronsDownUp,
  ChevronsUpDown,
  FileDiff,
  Folder,
  FolderClosed,
} from "lucide-react";
import type { TypesInteraction } from "../../api/api";
import useRouter from "../../hooks/useRouter";
import ChangedFileIcon from "./ChangedFileIcon";
import {
  buildChangeTree,
  changedFileName,
  codeChangeFiles,
  formatCompactChangeCount,
  representativeFiles,
  shouldAutoExpandChangedFiles,
  summarizeChangedFileScopes,
  summarizeChanges,
  type ChangeStat,
  type ChangeTreeNode,
} from "./changedFilesTree";

const Stat: FC<ChangeStat> = ({ additions, deletions }) => {
  if (additions === 0 && deletions === 0) return null;
  return (
    <Box
      component="span"
      role="group"
      aria-label={`${additions} additions, ${deletions} deletions`}
      sx={{ display: "inline-flex", alignItems: "center", gap: 0.5, fontFamily: "monospace", fontSize: 10, lineHeight: 1, whiteSpace: "nowrap", fontVariantNumeric: "tabular-nums" }}
    >
      <Box component="span" aria-hidden="true" sx={{ color: "success.main" }}>+{formatCompactChangeCount(additions)}</Box>
      <Box component="span" aria-hidden="true" sx={{ color: "error.main" }}>-{formatCompactChangeCount(deletions)}</Box>
    </Box>
  );
};

interface ChangedFilesCardProps {
  interaction: TypesInteraction;
  isLatest: boolean;
}

const ChangedFilesCard: FC<ChangedFilesCardProps> = ({ interaction, isLatest }) => {
  const router = useRouter();
  const theme = useTheme();
  const darkMode = theme.palette.mode === "dark";
  const files = useMemo(() => codeChangeFiles(interaction.code_changes?.files), [interaction.code_changes?.files]);
  const total = useMemo(() => summarizeChanges(files), [files]);
  const autoExpanded = shouldAutoExpandChangedFiles(files, isLatest);
  const storageKey = `helix:changed-files:${interaction.id}`;
  const [expanded, setExpandedState] = useState(() => {
    const stored = localStorage.getItem(storageKey);
    return stored === null ? autoExpanded : stored === "expanded";
  });
  const [expandDirectories, setExpandDirectories] = useState(autoExpanded);
  const tree = useMemo(() => buildChangeTree(files), [files]);
  const scopeSummary = useMemo(() => summarizeChangedFileScopes(files), [files]);
  const preview = useMemo(() => representativeFiles(files), [files]);
  const compactPreviewVisible = isLatest && !expanded;

  const status = interaction.code_changes?.status;
  // A turn whose checkpoint capture failed used to render nothing at all, so a
  // workspace that had silently stopped producing receipts was indistinguishable
  // from a turn that changed no files. Say so instead, quietly.
  if (interaction.id && (status === "missing" || status === "error")) {
    return (
      <Box
        data-changed-files-state="unavailable"
        sx={{ mt: 2, display: "flex", alignItems: "center", gap: 0.75, color: "text.secondary" }}
      >
        <FileDiff size={13} />
        <Tooltip title={interaction.code_changes?.error || ""}>
          <Typography variant="caption">Changed files unavailable for this turn</Typography>
        </Tooltip>
      </Box>
    );
  }
  if (status !== "ready" || files.length === 0 || !interaction.id) return null;

  const setExpanded = (value: boolean) => {
    setExpandedState(value);
    localStorage.setItem(storageKey, value ? "expanded" : "collapsed");
  };
  const openDiff = (path?: string) => {
    const params: Record<string, string> = {
      ...router.params,
      view: "changes",
      interaction: interaction.id!,
      ...(path ? { file: path } : {}),
    };
    delete params.preview;
    router.setParams(params, true);
  };

  return (
    <Box
      data-changed-files-state={expanded ? "expanded" : compactPreviewVisible ? "preview" : "collapsed"}
      sx={(theme) => ({
        mt: 2,
        p: 1,
        boxSizing: "border-box",
        width: "min(100%, 760px)",
        borderRadius: 4,
        border: "1px solid",
        borderColor: theme.palette.mode === "light" ? "divider" : "transparent",
        bgcolor: theme.palette.mode === "light" ? "action.hover" : "rgba(255,255,255,0.035)",
      })}
    >
      <Box
        sx={(theme) => ({
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 1,
          px: 0.5,
          borderRadius: 2,
          ...(expanded && {
            position: "sticky",
            top: 8,
            zIndex: 1,
            mb: 1,
            bgcolor: `color-mix(in srgb, ${theme.palette.text.primary} 3%, ${theme.palette.background.default})`,
          }),
        })}
      >
        <Button
          onClick={() => setExpanded(!expanded)}
          aria-expanded={expanded}
          color="inherit"
          sx={{
            minWidth: 0,
            minHeight: 0,
            flex: 1,
            justifyContent: "flex-start",
            gap: 0.75,
            px: 0.5,
            py: 0.75,
            borderRadius: 1.5,
            textTransform: "none",
            overflow: "hidden",
            "&:hover": { bgcolor: "action.hover" },
          }}
        >
          <ChevronRight
            size={14}
            style={{ flexShrink: 0, opacity: 0.72, transition: "transform 150ms", transform: expanded ? "rotate(90deg)" : "none" }}
          />
          <Box sx={{ display: "inline-flex", alignItems: "center", gap: 0.75, minWidth: 0, whiteSpace: "nowrap" }}>
            <Typography component="span" sx={{ fontSize: 12, lineHeight: "16px", fontWeight: 600 }}>
              {files.length} changed file{files.length === 1 ? "" : "s"}
            </Typography>
            <Stat {...total} />
          </Box>
          <Typography
            component="span"
            color="text.secondary"
            sx={{ ml: 0.5, display: { xs: "none", sm: "block" }, minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", fontSize: 11, lineHeight: "16px" }}
          >
            {expanded ? "Hide files" : "Show files"}
          </Typography>
        </Button>
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, flexShrink: 0 }}>
          {expanded && (
            <Tooltip title={expandDirectories ? "Collapse all folders" : "Expand all folders"}>
              <IconButton
                size="small"
                aria-label={expandDirectories ? "Collapse all folders" : "Expand all folders"}
                onClick={() => setExpandDirectories((value) => !value)}
                sx={{ width: 24, height: 24, border: "1px solid", borderColor: "divider", borderRadius: 1.5 }}
              >
                {expandDirectories ? <ChevronsDownUp size={12} /> : <ChevronsUpDown size={12} />}
              </IconButton>
            </Tooltip>
          )}
          <Tooltip title="Open the full diff">
            <Button
              size="small"
              variant="outlined"
              color="inherit"
              aria-label="Open diff"
              onClick={() => openDiff(files[0]?.path)}
              sx={{ minWidth: 0, minHeight: 24, height: 24, px: 0.75, py: 0, gap: 0.5, borderRadius: 1.5, borderColor: "divider", fontSize: 11, fontWeight: 600, lineHeight: 1, textTransform: "none" }}
            >
              <FileDiff size={12} />
              <Box component="span" sx={{ display: { xs: "none", sm: "inline" } }}>Open diff</Box>
            </Button>
          </Tooltip>
        </Box>
      </Box>
      {expanded ? (
        <Box sx={{ display: "flex", flexDirection: "column", gap: 0.25 }}>
          {tree.map((node) => (
            <TreeNode key={`${node.kind}:${node.path}`} node={node} depth={0} defaultExpanded={expandDirectories} hasDirectories={tree.some((entry) => entry.kind === "directory")} darkMode={darkMode} onOpen={openDiff} />
          ))}
        </Box>
      ) : compactPreviewVisible ? (
        <Box sx={{ px: 1, pt: 0.5, pb: 0.75 }}>
          <Box sx={{ display: "flex", flexWrap: "wrap", alignItems: "center", columnGap: 0.75, rowGap: 0.25 }}>
            {scopeSummary.map((scope, index) => (
              <Box key={scope.label} component="span" sx={{ display: "inline-flex", alignItems: "center", gap: 0.5, fontSize: 11, color: "text.secondary" }}>
                {index > 0 && <Box component="span" aria-hidden="true">·</Box>}
                <Box component="span" sx={{ fontFamily: "monospace", color: "text.primary", opacity: 0.75 }}>{scope.label}</Box>
                <Box component="span">{scope.fileCount} file{scope.fileCount === 1 ? "" : "s"}</Box>
              </Box>
            ))}
          </Box>
          <Box sx={{ mt: 1, display: "flex", flexWrap: "wrap", alignItems: "center", gap: 0.75 }}>
            {preview.map((file) => (
              <Button
                key={file.path}
                title={file.path}
                size="small"
                color="inherit"
                onClick={() => openDiff(file.path)}
                sx={(theme) => ({
                  minWidth: 0,
                  maxWidth: 192,
                  minHeight: 24,
                  px: 0.75,
                  py: 0.5,
                  gap: 0.5,
                  border: "1px solid",
                  borderColor: "divider",
                  borderRadius: 1.5,
                  bgcolor: theme.palette.mode === "light" ? "rgba(255,255,255,0.5)" : "rgba(0,0,0,0.16)",
                  fontFamily: "monospace",
                  fontSize: 10,
                  fontWeight: 400,
                  lineHeight: 1,
                  color: "text.secondary",
                  textTransform: "none",
                  "&:hover": { bgcolor: "action.hover", color: "text.primary" },
                })}
              >
                <ChangedFileIcon path={file.path} darkMode={darkMode} size={12} />
                <Box component="span" sx={{ minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{changedFileName(file.path)}</Box>
              </Button>
            ))}
            <Button
              size="small"
              color="inherit"
              onClick={() => setExpanded(true)}
              sx={{ minWidth: 0, minHeight: 24, px: 0.75, py: 0.5, borderRadius: 1.5, color: "text.secondary", fontSize: 11, fontWeight: 600, lineHeight: 1, textTransform: "none" }}
            >
              Show all {files.length} files
            </Button>
          </Box>
        </Box>
      ) : null}
    </Box>
  );
};

const TreeNode: FC<{
  node: ChangeTreeNode;
  depth: number;
  defaultExpanded: boolean;
  hasDirectories: boolean;
  darkMode: boolean;
  onOpen: (path: string) => void;
}> = ({ node, depth, defaultExpanded, hasDirectories, darkMode, onOpen }) => {
  const [override, setOverride] = useState<boolean | null>(null);
  useEffect(() => setOverride(null), [defaultExpanded]);
  const expanded = override ?? defaultExpanded;
  const leftPadding = 8 + depth * 14;

  if (node.kind === "directory") {
    return (
      <Box>
        <Button
          color="inherit"
          onClick={() => setOverride(!expanded)}
          sx={{ width: "100%", minHeight: 24, justifyContent: "flex-start", gap: 0.75, pl: `${leftPadding}px`, pr: 1.5, py: 0.5, borderRadius: 2, textTransform: "none" }}
        >
          <ChevronRight size={14} style={{ flexShrink: 0, opacity: 0.65, transition: "transform 150ms", transform: expanded ? "rotate(90deg)" : "none" }} />
          {expanded ? <Folder size={14} style={{ flexShrink: 0, opacity: 0.72 }} /> : <FolderClosed size={14} style={{ flexShrink: 0, opacity: 0.72 }} />}
          <Typography component="span" sx={{ minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", fontFamily: "monospace", fontSize: 11, color: "text.secondary" }}>{node.name}</Typography>
          <Box sx={{ ml: "auto", lineHeight: 0 }}><Stat {...node.stat} /></Box>
        </Button>
        {expanded && (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 0.25 }}>
            {node.children.map((child) => <TreeNode key={`${child.kind}:${child.path}`} node={child} depth={depth + 1} defaultExpanded={defaultExpanded} hasDirectories darkMode={darkMode} onOpen={onOpen} />)}
          </Box>
        )}
      </Box>
    );
  }

  return (
    <Button
      color="inherit"
      onClick={() => onOpen(node.path)}
      sx={{ width: "100%", minHeight: 24, justifyContent: "flex-start", gap: 0.75, pl: `${leftPadding}px`, pr: 1.5, py: 0.5, borderRadius: 2, textTransform: "none" }}
    >
      {hasDirectories || depth > 0 ? <Box component="span" aria-hidden="true" sx={{ width: 14, flexShrink: 0 }} /> : null}
      <ChangedFileIcon path={node.path} darkMode={darkMode} />
      <Typography component="span" sx={{ minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", fontFamily: "monospace", fontSize: 11, color: "text.secondary" }}>{node.name}</Typography>
      <Box sx={{ ml: "auto", lineHeight: 0 }}><Stat {...node.stat} /></Box>
    </Button>
  );
};

export default ChangedFilesCard;
