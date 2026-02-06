import React, { useState, useMemo } from "react";
import {
  Box,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
  Select,
  MenuItem,
  TextField,
  Chip,
  CircularProgress,
  IconButton,
} from "@mui/material";
import { Close as CloseIcon } from "@mui/icons-material";
import { useTheme } from "@mui/material/styles";
import useSnackbar from "../../hooks/useSnackbar";

import { TypesSpecTaskPriority } from "../../api/api";
import { SpecTask, useUpdateSpecTask } from "../../services/specTaskService";
import BacklogFilterBar from "./BacklogFilterBar";

// Priority order for sorting (critical at top)
const PRIORITY_ORDER: Record<string, number> = {
  critical: 0,
  high: 1,
  medium: 2,
  low: 3,
};

// Priority colors matching existing getPriorityColor function
const getPriorityColor = (priority: string) => {
  switch (priority?.toLowerCase()) {
    case "critical":
      return "error";
    case "high":
      return "warning";
    case "medium":
      return "info";
    case "low":
      return "success";
    default:
      return "default";
  }
};

interface BacklogTableViewProps {
  tasks: SpecTask[];
  onClose: () => void;
}

const BacklogTableView: React.FC<BacklogTableViewProps> = ({
  tasks,
  onClose,
}) => {
  const theme = useTheme();
  const snackbar = useSnackbar();
  const updateTask = useUpdateSpecTask();

  // Filter state
  const [search, setSearch] = useState("");
  const [priorityFilter, setPriorityFilter] = useState<TypesSpecTaskPriority[]>(
    [],
  );

  // Edit state
  const [editingTaskId, setEditingTaskId] = useState<string | null>(null);
  const [editingPrompt, setEditingPrompt] = useState("");
  const [loadingTaskId, setLoadingTaskId] = useState<string | null>(null);

  // Filter and sort tasks
  const filteredAndSortedTasks = useMemo(() => {
    let result = [...tasks];

    // Apply search filter
    if (search) {
      const searchLower = search.toLowerCase();
      result = result.filter((task) =>
        (task.original_prompt || "").toLowerCase().includes(searchLower),
      );
    }

    // Apply priority filter
    if (priorityFilter.length > 0) {
      result = result.filter((task) =>
        priorityFilter.includes(task.priority as TypesSpecTaskPriority),
      );
    }

    // Sort by priority (critical first), then by created date (newest first)
    result.sort((a, b) => {
      const priorityA = PRIORITY_ORDER[a.priority || "medium"] ?? 2;
      const priorityB = PRIORITY_ORDER[b.priority || "medium"] ?? 2;

      if (priorityA !== priorityB) {
        return priorityA - priorityB;
      }

      // Secondary sort by created date (newest first)
      const dateA = new Date(a.created || 0).getTime();
      const dateB = new Date(b.created || 0).getTime();
      return dateB - dateA;
    });

    return result;
  }, [tasks, search, priorityFilter]);

  // Handle priority change
  const handlePriorityChange = async (
    taskId: string,
    newPriority: TypesSpecTaskPriority,
  ) => {
    setLoadingTaskId(taskId);
    try {
      await updateTask.mutateAsync({
        taskId,
        updates: { priority: newPriority },
      });
    } catch (error) {
      snackbar.error("Failed to update priority");
    } finally {
      setLoadingTaskId(null);
    }
  };

  // Handle prompt edit start
  const handlePromptClick = (task: SpecTask) => {
    setEditingTaskId(task.id || null);
    setEditingPrompt(task.original_prompt || "");
  };

  // Handle prompt save
  const handlePromptSave = async (taskId: string) => {
    setLoadingTaskId(taskId);
    try {
      await updateTask.mutateAsync({
        taskId,
        updates: { description: editingPrompt },
      });
      setEditingTaskId(null);
    } catch (error) {
      snackbar.error("Failed to update prompt");
    } finally {
      setLoadingTaskId(null);
    }
  };

  // Handle prompt cancel
  const handlePromptCancel = () => {
    setEditingTaskId(null);
    setEditingPrompt("");
  };

  // Handle key press in prompt textarea
  const handlePromptKeyDown = (e: React.KeyboardEvent, taskId: string) => {
    if (e.key === "Escape") {
      handlePromptCancel();
    } else if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      handlePromptSave(taskId);
    }
  };

  return (
    <Box
      sx={{
        width: "100%",
        height: "100%",
        display: "flex",
        flexDirection: "column",
        bgcolor: "background.paper",
        borderRadius: 2,
        border: "1px solid",
        borderColor: "divider",
        overflow: "hidden",
      }}
    >
      {/* Header */}
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          px: 2.5,
          py: 2,
          borderBottom: "1px solid",
          borderColor: "divider",
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <Typography variant="body1" sx={{ fontWeight: 600 }}>
            Backlog
          </Typography>
          <Chip
            label={filteredAndSortedTasks.length}
            size="small"
            sx={{ height: 20, fontSize: "0.75rem" }}
          />
        </Box>
        <IconButton size="small" onClick={onClose}>
          <CloseIcon />
        </IconButton>
      </Box>

      {/* Filter Bar */}
      <BacklogFilterBar
        search={search}
        onSearchChange={setSearch}
        priorityFilter={priorityFilter}
        onPriorityFilterChange={setPriorityFilter}
      />

      {/* Table */}
      <TableContainer sx={{ flex: 1, overflow: "auto" }}>
        <Table stickyHeader>
          <TableHead>
            <TableRow>
              <TableCell sx={{ fontWeight: 600, width: "85%" }}>
                Prompt
              </TableCell>
              <TableCell sx={{ fontWeight: 600, width: "15%", minWidth: 120 }}>
                Priority
              </TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {filteredAndSortedTasks.length === 0 ? (
              <TableRow>
                <TableCell colSpan={2} align="center" sx={{ py: 4 }}>
                  <Typography variant="body2" color="text.secondary">
                    {search || priorityFilter.length > 0
                      ? "No tasks match your filters"
                      : "No backlog tasks"}
                  </Typography>
                </TableCell>
              </TableRow>
            ) : (
              filteredAndSortedTasks.map((task) => (
                <TableRow
                  key={task.id}
                  hover
                  sx={{
                    "&:hover": {
                      bgcolor: "action.hover",
                    },
                  }}
                >
                  {/* Prompt Cell */}
                  <TableCell
                    sx={{
                      verticalAlign: "top",
                      cursor: editingTaskId === task.id ? "default" : "pointer",
                    }}
                    onClick={() =>
                      editingTaskId !== task.id && handlePromptClick(task)
                    }
                  >
                    {editingTaskId === task.id ? (
                      <TextField
                        multiline
                        fullWidth
                        minRows={2}
                        value={editingPrompt}
                        onChange={(e) => setEditingPrompt(e.target.value)}
                        onKeyDown={(e) => handlePromptKeyDown(e, task.id || "")}
                        onBlur={() => handlePromptSave(task.id || "")}
                        autoFocus
                        disabled={loadingTaskId === task.id}
                        helperText="Ctrl+Enter to save, Escape to cancel"
                        sx={{
                          "& .MuiInputBase-root": { fontSize: "0.875rem" },
                        }}
                      />
                    ) : (
                      <Typography
                        variant="body2"
                        sx={{
                          whiteSpace: "pre-wrap",
                          wordBreak: "break-word",
                        }}
                      >
                        {task.original_prompt || "(No prompt)"}
                      </Typography>
                    )}
                  </TableCell>

                  {/* Priority Cell */}
                  <TableCell sx={{ verticalAlign: "top" }}>
                    {loadingTaskId === task.id ? (
                      <CircularProgress size={20} />
                    ) : (
                      <Select
                        value={task.priority || "medium"}
                        size="small"
                        onChange={(e) =>
                          handlePriorityChange(
                            task.id || "",
                            e.target.value as TypesSpecTaskPriority,
                          )
                        }
                        sx={{ minWidth: 100 }}
                        renderValue={(value) => (
                          <Chip
                            label={
                              value.charAt(0).toUpperCase() + value.slice(1)
                            }
                            size="small"
                            color={getPriorityColor(value) as any}
                            sx={{ height: 24 }}
                          />
                        )}
                      >
                        <MenuItem
                          value={TypesSpecTaskPriority.SpecTaskPriorityCritical}
                        >
                          <Chip
                            label="Critical"
                            size="small"
                            color="error"
                            sx={{ height: 24 }}
                          />
                        </MenuItem>
                        <MenuItem
                          value={TypesSpecTaskPriority.SpecTaskPriorityHigh}
                        >
                          <Chip
                            label="High"
                            size="small"
                            color="warning"
                            sx={{ height: 24 }}
                          />
                        </MenuItem>
                        <MenuItem
                          value={TypesSpecTaskPriority.SpecTaskPriorityMedium}
                        >
                          <Chip
                            label="Medium"
                            size="small"
                            color="info"
                            sx={{ height: 24 }}
                          />
                        </MenuItem>
                        <MenuItem
                          value={TypesSpecTaskPriority.SpecTaskPriorityLow}
                        >
                          <Chip
                            label="Low"
                            size="small"
                            color="success"
                            sx={{ height: 24 }}
                          />
                        </MenuItem>
                      </Select>
                    )}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
};

export default BacklogTableView;
