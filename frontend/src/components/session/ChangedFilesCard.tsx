import React, { FC, useEffect, useMemo, useState } from "react";
import { Box, Button, IconButton, Tooltip, Typography } from "@mui/material";
import {
  ChevronDown,
  ChevronRight,
  ChevronsDownUp,
  ChevronsUpDown,
  FileCode2,
  FileDiff,
  Folder,
  FolderClosed,
} from "lucide-react";
import type { TypesInteraction } from "../../api/api";
import useRouter from "../../hooks/useRouter";
import {
  buildChangeTree,
  codeChangeFiles,
  representativeFiles,
  summarizeChanges,
  type ChangeStat,
  type ChangeTreeNode,
} from "./changedFilesTree";

const Stat: FC<ChangeStat> = ({ additions, deletions }) => (
  <Box component="span" sx={{ ml: "auto", fontFamily: "monospace", fontSize: 10, whiteSpace: "nowrap" }}>
    <Box component="span" sx={{ color: "success.main" }}>+{additions}</Box>
    <Box component="span" sx={{ color: "error.main", ml: 0.75 }}>-{deletions}</Box>
  </Box>
);

interface ChangedFilesCardProps {
  interaction: TypesInteraction;
  isLatest: boolean;
}

const ChangedFilesCard: FC<ChangedFilesCardProps> = ({ interaction, isLatest }) => {
  const router = useRouter();
  const files = useMemo(() => codeChangeFiles(interaction.code_changes?.files), [interaction.code_changes?.files]);
  const total = useMemo(() => summarizeChanges(files), [files]);
  const autoExpanded = isLatest && files.length <= 5 && total.additions + total.deletions <= 200;
  const storageKey = `helix:changed-files:${interaction.id}`;
  const [expanded, setExpandedState] = useState(() => {
    const stored = localStorage.getItem(storageKey);
    return stored === null ? autoExpanded : stored === "expanded";
  });
  const [expandDirectories, setExpandDirectories] = useState(true);
  const tree = useMemo(() => buildChangeTree(files), [files]);
  const preview = useMemo(() => representativeFiles(files), [files]);

  if (interaction.code_changes?.status !== "ready" || files.length === 0 || !interaction.id) return null;

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
      data-changed-files-state={expanded ? "expanded" : "collapsed"}
      sx={(theme) => ({
        mt: 2,
        p: 1,
        width: "min(100%, 760px)",
        borderRadius: 2.5,
        border: "1px solid",
        borderColor: theme.palette.mode === "light" ? "divider" : "transparent",
        bgcolor: theme.palette.mode === "light" ? "action.hover" : "rgba(255,255,255,0.035)",
      })}
    >
      <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
        <Button
          onClick={() => setExpanded(!expanded)}
          aria-expanded={expanded}
          color="inherit"
          sx={{ minWidth: 0, flex: 1, justifyContent: "flex-start", px: 0.5, py: 0.6, borderRadius: 1.5, textTransform: "none" }}
        >
          {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          <Typography variant="caption" sx={{ ml: 0.5, fontWeight: 600 }}>
            {files.length} changed file{files.length === 1 ? "" : "s"}
          </Typography>
          <Stat {...total} />
          <Typography variant="caption" color="text.secondary" sx={{ ml: 1 }}>
            {expanded ? "Hide files" : "Show files"}
          </Typography>
        </Button>
        {expanded && (
          <Tooltip title={expandDirectories ? "Collapse all folders" : "Expand all folders"}>
            <IconButton size="small" onClick={() => setExpandDirectories((value) => !value)}>
              {expandDirectories ? <ChevronsDownUp size={14} /> : <ChevronsUpDown size={14} />}
            </IconButton>
          </Tooltip>
        )}
        <Button size="small" variant="outlined" color="inherit" onClick={() => openDiff(files[0]?.path)} sx={{ px: 1, minWidth: 0, fontSize: 11, textTransform: "none" }}>
          <FileDiff size={13} style={{ marginRight: 5 }} /> Open diff
        </Button>
      </Box>
      {expanded ? (
        <Box sx={{ mt: 0.5 }}>
          {tree.map((node) => (
            <TreeNode key={`${node.kind}:${node.path}`} node={node} depth={0} defaultExpanded={expandDirectories} onOpen={openDiff} />
          ))}
        </Box>
      ) : isLatest && files.length > 5 ? (
        <Box sx={{ px: 1, pb: 0.5, display: "flex", flexWrap: "wrap", gap: 0.75 }}>
          {preview.map((file) => (
            <Button key={file.path} size="small" color="inherit" onClick={() => openDiff(file.path)} sx={{ maxWidth: 190, px: 0.75, py: 0.35, border: "1px solid", borderColor: "divider", fontFamily: "monospace", fontSize: 10, textTransform: "none" }}>
              <FileCode2 size={12} style={{ marginRight: 4 }} />
              <Box component="span" sx={{ overflow: "hidden", textOverflow: "ellipsis" }}>{file.path.split("/").at(-1)}</Box>
            </Button>
          ))}
          <Button size="small" color="inherit" onClick={() => setExpanded(true)} sx={{ fontSize: 10, textTransform: "none" }}>Show all {files.length}</Button>
        </Box>
      ) : null}
    </Box>
  );
};

const TreeNode: FC<{
  node: ChangeTreeNode;
  depth: number;
  defaultExpanded: boolean;
  onOpen: (path: string) => void;
}> = ({ node, depth, defaultExpanded, onOpen }) => {
  const [override, setOverride] = useState<boolean | null>(null);
  useEffect(() => setOverride(null), [defaultExpanded]);
  const expanded = override ?? defaultExpanded;
  if (node.kind === "directory") {
    return (
      <Box>
        <Button color="inherit" onClick={() => setOverride(!expanded)} sx={{ width: "100%", justifyContent: "flex-start", pl: 1 + depth * 1.5, pr: 1, py: 0.35, borderRadius: 1.5, textTransform: "none" }}>
          {expanded ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
          {expanded ? <Folder size={14} /> : <FolderClosed size={14} />}
          <Typography variant="caption" sx={{ ml: 0.75, fontFamily: "monospace" }}>{node.name}</Typography>
          <Stat {...node.stat} />
        </Button>
        {expanded && node.children.map((child) => <TreeNode key={`${child.kind}:${child.path}`} node={child} depth={depth + 1} defaultExpanded={defaultExpanded} onOpen={onOpen} />)}
      </Box>
    );
  }
  return (
    <Button color="inherit" onClick={() => onOpen(node.path)} sx={{ width: "100%", justifyContent: "flex-start", pl: 3.5 + depth * 1.5, pr: 1, py: 0.35, borderRadius: 1.5, textTransform: "none" }}>
      <FileCode2 size={14} />
      <Typography variant="caption" sx={{ ml: 0.75, fontFamily: "monospace", overflow: "hidden", textOverflow: "ellipsis" }}>{node.name}</Typography>
      <Stat {...node.stat} />
    </Button>
  );
};

export default ChangedFilesCard;
