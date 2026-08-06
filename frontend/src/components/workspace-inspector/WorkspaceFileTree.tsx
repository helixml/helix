import React, { FC, useEffect, useMemo, useRef, useState } from "react";
import { FileTree, useFileTree } from "@pierre/trees/react";
import {
  Box,
  CircularProgress,
  IconButton,
  InputAdornment,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import { RefreshCw, Search, X } from "lucide-react";
import type { TypesWorkspaceFileEntry } from "../../api/api";
import { matchesAllTokens } from "../../utils/searchUtils";
import { TREE_UNSAFE_CSS } from "./pierreStyles";
import { useWorkspaceFiles } from "./workspaceReviewService";

interface WorkspaceFileTreeProps {
  sessionId: string;
  workspace?: string;
  selectedPath: string | null;
  onOpenFile: (path: string) => void;
}

function treePath(entry: TypesWorkspaceFileEntry): string | null {
  if (!entry.path) return null;
  return entry.kind === "directory" ? `${entry.path.replace(/\/$/, "")}/` : entry.path;
}

function filteredTreePaths(entries: TypesWorkspaceFileEntry[], query: string): string[] {
  const allPaths = entries.flatMap((entry) => {
    const path = treePath(entry);
    return path ? [path] : [];
  });
  if (!query.trim()) return allPaths;

  const included = new Set<string>();
  for (const path of allPaths) {
    const normalized = path.replace(/\/$/, "");
    if (!matchesAllTokens(query, normalized)) continue;
    included.add(path);
    const segments = normalized.split("/");
    for (let index = 1; index < segments.length; index += 1) {
      included.add(`${segments.slice(0, index).join("/")}/`);
    }
    if (path.endsWith("/")) {
      for (const descendant of allPaths) {
        if (descendant.startsWith(path)) included.add(descendant);
      }
    }
  }
  return allPaths.filter((path) => included.has(path));
}

const WorkspaceFileTree: FC<WorkspaceFileTreeProps> = ({
  sessionId,
  workspace,
  selectedPath,
  onOpenFile,
}) => {
  const filesQuery = useWorkspaceFiles(sessionId, workspace);
  const [query, setQuery] = useState("");
  const syncingSelection = useRef(false);
  const entries = filesQuery.data?.entries || [];
  const entryKinds = useMemo(
    () => new Map(entries.flatMap((entry) => entry.path && entry.kind ? [[entry.path, entry.kind]] : [])),
    [entries],
  );
  const paths = useMemo(() => filteredTreePaths(entries, query), [entries, query]);
  const { model } = useFileTree({
    paths: [],
    density: "compact",
    flattenEmptyDirectories: true,
    initialExpansion: 1,
    search: false,
    renaming: false,
    dragAndDrop: false,
    unsafeCSS: TREE_UNSAFE_CSS,
    onSelectionChange: (selectedPaths) => {
      if (syncingSelection.current) return;
      const path = selectedPaths.at(-1)?.replace(/\/$/, "");
      if (path && entryKinds.get(path) === "file") onOpenFile(path);
    },
  });

  useEffect(() => {
    model.resetPaths(paths);
  }, [model, paths]);

  useEffect(() => {
    if (!selectedPath || !entryKinds.has(selectedPath)) return;
    const item = model.getItem(selectedPath);
    if (!item) return;
    syncingSelection.current = true;
    for (const selected of model.getSelectedPaths()) model.getItem(selected)?.deselect();
    const segments = selectedPath.split("/");
    for (let index = 1; index < segments.length; index += 1) {
      const ancestor = model.getItem(`${segments.slice(0, index).join("/")}/`);
      if (ancestor && "expand" in ancestor) ancestor.expand();
    }
    item.select();
    model.scrollToPath(selectedPath, { offset: "center" });
    queueMicrotask(() => {
      syncingSelection.current = false;
    });
  }, [entryKinds, model, selectedPath, paths]);

  return (
    <Box sx={{ display: "flex", flexDirection: "column", height: "100%", minHeight: 0 }}>
      <Box sx={{ display: "flex", gap: 0.5, alignItems: "center", p: 0.75, borderBottom: "1px solid", borderColor: "divider" }}>
        <TextField
          fullWidth
          size="small"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="Search files"
          inputProps={{ "aria-label": "Search workspace files" }}
          InputProps={{
            startAdornment: <InputAdornment position="start"><Search size={14} /></InputAdornment>,
            endAdornment: query ? (
              <InputAdornment position="end">
                <IconButton size="small" onClick={() => setQuery("")} aria-label="Clear search"><X size={13} /></IconButton>
              </InputAdornment>
            ) : undefined,
          }}
          sx={{ "& .MuiInputBase-root": { height: 30, fontSize: 12 } }}
        />
        <Tooltip title="Refresh files">
          <span>
            <IconButton size="small" onClick={() => filesQuery.refetch()} disabled={filesQuery.isFetching}>
              <RefreshCw size={15} />
            </IconButton>
          </span>
        </Tooltip>
      </Box>
      {filesQuery.isLoading ? (
        <Box sx={{ flex: 1, display: "grid", placeItems: "center" }}><CircularProgress size={20} /></Box>
      ) : filesQuery.isError ? (
        <Typography variant="caption" color="error" sx={{ p: 1.5 }}>Could not load workspace files.</Typography>
      ) : paths.length === 0 ? (
        <Typography variant="caption" color="text.secondary" sx={{ p: 1.5 }}>
          {query ? "No files match this search." : "No files found."}
        </Typography>
      ) : (
        <FileTree model={model} style={{ minHeight: 0, height: "100%", overflow: "hidden" }} />
      )}
      {filesQuery.data?.truncated && (
        <Typography variant="caption" color="warning.main" sx={{ px: 1, py: 0.5 }}>File list truncated.</Typography>
      )}
    </Box>
  );
};

export default WorkspaceFileTree;
