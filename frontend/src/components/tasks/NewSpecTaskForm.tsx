import React, {
  useState,
  useEffect,
  useRef,
  useMemo,
  useCallback,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Avatar,
  Box,
  Button,
  Typography,
  Chip,
  Stack,
  TextField,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  CircularProgress,
  Checkbox,
  FormControlLabel,
  Autocomplete,
  Tooltip,
  IconButton,
} from "@mui/material";
import { Add as AddIcon, AttachFile as AttachFileIcon, Close as CloseIcon } from "@mui/icons-material";
import { ChevronDown, UserCircle2, X } from "lucide-react";
import AssigneeSelector from "./AssigneeSelector";
import { RECOMMENDED_CODING_MODELS } from "../../constants/models";

import { CodeAgentRuntime, generateAgentName } from "../../contexts/apps";
import { AGENT_TYPE_ZED_EXTERNAL, IApp } from "../../types";
import {
  TypesSpecTaskPriority,
  TypesBranchMode,
  TypesSpecTask,
  TypesSpecTaskStatus,
  TypesUser,
} from "../../api/api";
import AgentDropdown from "../agent/AgentDropdown";
import CodingAgentForm, {
  CodingAgentFormHandle,
} from "../agent/CodingAgentForm";
import useAccount from "../../hooks/useAccount";
import useApi from "../../hooks/useApi";
import useSnackbar from "../../hooks/useSnackbar";
import useApps from "../../hooks/useApps";
import { useGetProject, useGetProjectRepositories } from "../../services";
import { useSpecTasks, useProjectLabels, useAddLabel } from "../../services/specTaskService";
import {
  SPEC_TASK_ATTACHMENT_ACCEPTED_MIME,
  SPEC_TASK_ATTACHMENT_MAX_BYTES,
  SPEC_TASK_ATTACHMENT_MAX_PER_TASK,
  useUploadSpecTaskAttachments,
} from "../../services/specTaskAttachmentsService";

const ATTACHMENT_ACCEPT_ATTR = Object.entries(SPEC_TASK_ATTACHMENT_ACCEPTED_MIME)
  .flatMap(([mime, exts]) => [mime, ...exts])
  .join(",");

function humanAttachmentSize(n: number): string {
  if (n >= 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  if (n >= 1024) return `${Math.round(n / 1024)} KB`;
  return `${n} B`;
}

const LAST_LABELS_KEY = "helix_last_task_labels";
const DRAFT_KEY_PREFIX = "helix_new_spectask_draft_";



interface NewSpecTaskFormProps {
  projectId: string;
  onTaskCreated: (task: TypesSpecTask) => void;
  onClose?: () => void;
  showHeader?: boolean; // Show header with close button (for panel mode)
  embedded?: boolean; // Embedded in tab (affects styling)
}

const NewSpecTaskForm: React.FC<NewSpecTaskFormProps> = ({
  projectId,
  onTaskCreated,
  onClose,
  showHeader = true,
  embedded = false,
}) => {
  const account = useAccount();
  const api = useApi();
  const snackbar = useSnackbar();
  const apps = useApps();
  const queryClient = useQueryClient();

  // Fetch project data
  const { data: project } = useGetProject(projectId, !!projectId);
  const { data: projectRepositories = [] } = useGetProjectRepositories(
    projectId,
    !!projectId,
  );
  const { data: projectTasks = [] } = useSpecTasks({
    projectId,
    withDependsOn: true,
    enabled: !!projectId,
  });

  const { data: projectLabels = [] } = useProjectLabels(projectId);
  const addLabelMutation = useAddLabel();

  const draftKey = `${DRAFT_KEY_PREFIX}${projectId}`;

  // Form state
  const [taskPrompt, setTaskPrompt] = useState<string>(() => {
    try {
      const raw = localStorage.getItem(`${DRAFT_KEY_PREFIX}${projectId}`);
      if (!raw) return "";
      const { content } = JSON.parse(raw);
      return content || "";
    } catch {
      return "";
    }
  });
  const draftTimer = useRef<ReturnType<typeof setTimeout>>();
  const [taskPriority, setTaskPriority] = useState("medium");
  const [taskLabels, setTaskLabels] = useState<string[]>(() => {
    try {
      return JSON.parse(localStorage.getItem(LAST_LABELS_KEY) || "[]");
    } catch {
      return [];
    }
  });
  const [selectedDependencyTaskIds, setSelectedDependencyTaskIds] = useState<
    string[]
  >([]);
  const [selectedHelixAgent, setSelectedHelixAgent] = useState("");
  const [justDoItMode, setJustDoItMode] = useState(false);
  const [autoStart, setAutoStart] = useState(false);
  const [isCreating, setIsCreating] = useState(false);
  const [pendingAttachments, setPendingAttachments] = useState<File[]>([]);
  const attachmentInput = useRef<HTMLInputElement | null>(null);
  const uploadAttachments = useUploadSpecTaskAttachments();

  // Empty string = "Unassigned". Pre-filled with the current user below.
  const [assigneeId, setAssigneeId] = useState<string>("");
  const [assigneeTouched, setAssigneeTouched] = useState(false);
  const [assigneeAnchorEl, setAssigneeAnchorEl] =
    useState<HTMLElement | null>(null);

  const orgMembers =
    account.organizationTools.organization?.memberships || [];
  const currentUserId = account.user?.id;

  // Pre-fill with current user once known. Don't overwrite a user choice
  // (including an explicit "Unassigned"), tracked via assigneeTouched.
  useEffect(() => {
    if (!assigneeTouched && currentUserId) {
      setAssigneeId(currentUserId);
    }
  }, [currentUserId, assigneeTouched]);

  const assignedUser = useMemo(() => {
    if (!assigneeId) return undefined;
    const member = orgMembers.find((m) => m.user_id === assigneeId);
    return member?.user as TypesUser | undefined;
  }, [assigneeId, orgMembers]);

  const getAssigneeDisplayName = (user: TypesUser | undefined): string => {
    if (!user) return "Unknown user";
    return user.full_name || user.username || user.email || "Unknown user";
  };
  const getAssigneeInitials = (user: TypesUser | undefined): string => {
    if (!user) return "?";
    const name = user.full_name || user.username || user.email || "";
    const parts = name.split(" ");
    if (parts.length >= 2) {
      return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
    }
    return name.slice(0, 2).toUpperCase();
  };

  // Branch configuration state
  const [branchMode, setBranchMode] = useState<TypesBranchMode>(
    TypesBranchMode.BranchModeNew,
  );
  const [baseBranch, setBaseBranch] = useState("");
  const [branchPrefix, setBranchPrefix] = useState("");
  const [workingBranch, setWorkingBranch] = useState("");
  const [showBranchCustomization, setShowBranchCustomization] = useState(false);

  // Get the default repository ID from the project
  const defaultRepoId = project?.default_repo_id;

  // Fetch branches for the default repository
  const { data: branchesData } = useQuery({
    queryKey: ["repository-branches", defaultRepoId],
    queryFn: async () => {
      if (!defaultRepoId) return [];
      const response = await api
        .getApiClient()
        .listGitRepositoryBranches(defaultRepoId);
      return response.data || [];
    },
    enabled: !!defaultRepoId,
    staleTime: 30000,
  });

  // Get the default branch name from the repository
  const defaultBranchName = useMemo(() => {
    const defaultRepo = projectRepositories.find((r) => r.id === defaultRepoId);
    return defaultRepo?.default_branch || "main";
  }, [projectRepositories, defaultRepoId]);

  const dependencyTaskOptions = useMemo(
    () =>
      projectTasks.filter(
        (task) =>
          !!task.id && task.status !== TypesSpecTaskStatus.TaskStatusDone,
      ),
    [projectTasks],
  );

  const selectedDependencyTasks = useMemo(() => {
    const taskById = new Map(
      dependencyTaskOptions.map((task) => [task.id, task]),
    );
    return selectedDependencyTaskIds
      .map((taskId) => taskById.get(taskId))
      .filter((task): task is NonNullable<typeof task> => !!task);
  }, [dependencyTaskOptions, selectedDependencyTaskIds]);

  // Set baseBranch to default when component mounts
  useEffect(() => {
    if (defaultBranchName && !baseBranch) {
      setBaseBranch(defaultBranchName);
    }
  }, [defaultBranchName, baseBranch]);

  // Inline agent creation state
  const [showCreateAgentForm, setShowCreateAgentForm] = useState(false);
  const [codeAgentRuntime, setCodeAgentRuntime] =
    useState<CodeAgentRuntime>("zed_agent");
  const [claudeCodeMode, setClaudeCodeMode] = useState<
    "subscription" | "api_key"
  >("subscription");
  const [selectedProvider, setSelectedProvider] = useState("");
  const [selectedModel, setSelectedModel] = useState("");
  const [newAgentName, setNewAgentName] = useState("-");
  const [userModifiedName, setUserModifiedName] = useState(false);
  const [creatingAgent, setCreatingAgent] = useState(false);
  const codingAgentFormRef = useRef<CodingAgentFormHandle>(null);
  // Ref for task prompt text field
  const taskPromptRef = useRef<HTMLTextAreaElement>(null);

  // Sort apps: project default first, then zed_external, then others
  const sortedApps = useMemo(() => {
    if (!apps.apps) return [];
    const zedExternalApps: IApp[] = [];
    const otherApps: IApp[] = [];
    let defaultApp: IApp | null = null;
    const projectDefaultId = project?.default_helix_app_id;

    apps.apps.forEach((app) => {
      if (projectDefaultId && app.id === projectDefaultId) {
        defaultApp = app;
        return;
      }
      const hasZedExternal =
        app.config?.helix?.assistants?.some(
          (assistant) => assistant.agent_type === AGENT_TYPE_ZED_EXTERNAL,
        ) || app.config?.helix?.default_agent_type === AGENT_TYPE_ZED_EXTERNAL;
      if (hasZedExternal) {
        zedExternalApps.push(app);
      } else {
        otherApps.push(app);
      }
    });

    // Sort zed_external agents by model quality (opus > sonnet > haiku > other)
    const modelPriority = (app: IApp): number => {
      const name = (app.config?.helix?.name || "").toLowerCase();
      if (name.includes("opus")) return 0;
      if (name.includes("sonnet")) return 1;
      if (name.includes("haiku")) return 3;
      return 2; // unknown models between sonnet and haiku
    };
    zedExternalApps.sort((a, b) => modelPriority(a) - modelPriority(b));

    const result: IApp[] = [];
    if (defaultApp) result.push(defaultApp);
    result.push(...zedExternalApps, ...otherApps);
    return result;
  }, [apps.apps, project?.default_helix_app_id]);

  // Auto-generate agent name when model or runtime changes
  useEffect(() => {
    if (!userModifiedName && showCreateAgentForm) {
      setNewAgentName(generateAgentName(selectedModel, codeAgentRuntime));
    }
  }, [selectedModel, codeAgentRuntime, userModifiedName, showCreateAgentForm]);

  // Load apps on mount
  useEffect(() => {
    if (account.user?.id) {
      apps.loadApps();
    }
  }, []);

  // Auto-select default agent
  useEffect(() => {
    if (project?.default_helix_app_id) {
      setSelectedHelixAgent(project.default_helix_app_id);
      setShowCreateAgentForm(false);
    } else if (sortedApps.length === 0) {
      setShowCreateAgentForm(true);
      setSelectedHelixAgent("");
    } else {
      setSelectedHelixAgent(sortedApps[0]?.id || "");
      setShowCreateAgentForm(false);
    }
  }, [sortedApps, project?.default_helix_app_id]);

  // Focus text field on mount
  useEffect(() => {
    setTimeout(() => {
      taskPromptRef.current?.focus();
    }, 0);
  }, []);

  // Clear debounce timer on unmount to prevent post-unmount writes
  useEffect(() => {
    return () => {
      clearTimeout(draftTimer.current);
    };
  }, []);

  // Handle inline agent creation
  // Reset form
  const resetForm = useCallback(() => {
    setTaskPrompt("");
    localStorage.removeItem(draftKey);
    setTaskPriority("medium");
    setPendingAttachments([]);
    // Labels intentionally kept — they persist to the next task via localStorage
    setSelectedDependencyTaskIds([]);
    setSelectedHelixAgent("");
    setJustDoItMode(false);
    setAutoStart(false);
    setBranchMode(TypesBranchMode.BranchModeNew);
    setBaseBranch(defaultBranchName);
    setBranchPrefix("");
    setWorkingBranch("");
    setShowBranchCustomization(false);
    setShowCreateAgentForm(false);
    setCodeAgentRuntime("zed_agent");
    setClaudeCodeMode("subscription");
    setSelectedProvider("");
    setSelectedModel("");
    setNewAgentName("-");
    setUserModifiedName(false);
    setAssigneeId(currentUserId || "");
    setAssigneeTouched(false);
  }, [defaultBranchName, currentUserId]);

  // Handle task creation
  const handleCreateTask = async () => {
    if (!account.user) {
      account.setShowLoginWindow(true);
      return;
    }

    if (!taskPrompt.trim()) {
      snackbar.error("Please describe what you want to get done");
      return;
    }

    setIsCreating(true);

    try {
      let agentId = selectedHelixAgent;

      // Create agent inline if showing create form
      if (showCreateAgentForm) {
        const createdAgent =
          await codingAgentFormRef.current?.handleCreateAgent();
        if (!createdAgent?.id) {
          setIsCreating(false);
          return;
        }
        agentId = createdAgent.id;
      }

      const createTaskRequest = {
        prompt: taskPrompt,
        priority: taskPriority as TypesSpecTaskPriority,
        project_id: projectId,
        app_id: agentId || undefined,
        assignee_id: assigneeId || undefined,
        just_do_it_mode: justDoItMode,
        auto_start: autoStart,
        depends_on: selectedDependencyTasks
          .map((task) => task.id || "")
          .filter((taskId) => !!taskId),
        branch_mode: branchMode,
        base_branch:
          branchMode === TypesBranchMode.BranchModeNew ? baseBranch : undefined,
        branch_prefix:
          branchMode === TypesBranchMode.BranchModeNew && branchPrefix
            ? branchPrefix
            : undefined,
        working_branch:
          branchMode === TypesBranchMode.BranchModeExisting
            ? workingBranch
            : undefined,
      };

      const response = await api
        .getApiClient()
        .v1SpecTasksFromPromptCreate(createTaskRequest);

      if (response.data) {
        // Invalidate immediately so the task appears in the list without waiting for polling
        queryClient.invalidateQueries({ queryKey: ["spec-tasks"] });

        // Persist labels to localStorage for next task
        localStorage.setItem(LAST_LABELS_KEY, JSON.stringify(taskLabels));

        // Add labels to the newly created task
        const taskId = response.data.id;
        if (taskId && taskLabels.length > 0) {
          for (const label of taskLabels) {
            await addLabelMutation.mutateAsync({ taskId, label });
          }
        }

        // Upload any pending attachments. We do this AFTER task creation so the
        // task always exists even if attachment upload fails — the user can retry
        // from the detail page.
        if (taskId && pendingAttachments.length > 0) {
          try {
            await uploadAttachments.mutateAsync({ taskId, files: pendingAttachments });
          } catch (e: any) {
            snackbar.error(
              `Task created but attachment upload failed: ${e?.response?.data || e?.message || 'unknown error'}. You can retry from the task detail page.`,
            );
          }
        }

        snackbar.success(
          "SpecTask created! Planning agent will generate specifications.",
        );
        // Invalidate task list to update kanban board immediately
        queryClient.invalidateQueries({ queryKey: ["spec-tasks"] });
        resetForm();
        onTaskCreated(response.data);
      }
    } catch (error: any) {
      console.error("Failed to create SpecTask:", error);
      const errorMessage =
        error?.response?.data?.message ||
        error?.message ||
        "Failed to create SpecTask. Please try again.";
      snackbar.error(errorMessage);
    } finally {
      setIsCreating(false);
    }
  };

  // Keyboard shortcut: Ctrl/Cmd+Enter to submit
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
        if (taskPrompt.trim()) {
          e.preventDefault();
          handleCreateTask();
        }
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [taskPrompt, justDoItMode, selectedHelixAgent, selectedDependencyTaskIds]);

  // Keyboard shortcut: Ctrl/Cmd+J to toggle Just Do It mode
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "j") {
        e.preventDefault();
        setJustDoItMode((prev) => !prev);
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);

  return (
    <Box sx={{ height: "100%", display: "flex", flexDirection: "column" }}>
      {/* Header */}
      {showHeader && (
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            p: 2,
            borderBottom: 1,
            borderColor: "divider",
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <AddIcon />
            <Typography variant="h6">New SpecTask</Typography>
          </Box>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            {onClose && (
              <IconButton onClick={onClose}>
                <X size={20} />
              </IconButton>
            )}
          </Box>
        </Box>
      )}

      {/* Content */}
      <Box sx={{ flex: 1, overflow: "auto", p: 2 }}>
        <Stack spacing={2}>
          {/* Priority Selector */}
          <FormControl fullWidth size="small">
            <InputLabel>Priority</InputLabel>
            <Select
              value={taskPriority}
              onChange={(e) => setTaskPriority(e.target.value)}
              label="Priority"
            >
              <MenuItem value="low">Low</MenuItem>
              <MenuItem value="medium">Medium</MenuItem>
              <MenuItem value="high">High</MenuItem>
              <MenuItem value="critical">Critical</MenuItem>
            </Select>
          </FormControl>

          {/* Assignee Selector */}
          <Button
            variant="outlined"
            fullWidth
            onClick={(e) => setAssigneeAnchorEl(e.currentTarget)}
            endIcon={<ChevronDown size={16} />}
            sx={{
              justifyContent: "space-between",
              textTransform: "none",
              color: "text.primary",
              borderColor: "rgba(255,255,255,0.23)",
              px: 1.5,
              py: 1,
              "&:hover": { borderColor: "text.primary" },
            }}
          >
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, minWidth: 0 }}>
              {assignedUser ? (
                <Avatar sx={{ width: 24, height: 24, fontSize: "0.7rem" }}>
                  {getAssigneeInitials(assignedUser)}
                </Avatar>
              ) : (
                <UserCircle2 size={20} style={{ opacity: 0.5 }} />
              )}
              <Typography variant="body2" sx={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                {assignedUser
                  ? `Assignee: ${getAssigneeDisplayName(assignedUser)}`
                  : "Assignee: Unassigned"}
              </Typography>
            </Box>
          </Button>
          <AssigneeSelector
            assigneeId={assigneeId || undefined}
            members={orgMembers}
            currentUserId={currentUserId}
            onAssigneeChange={(userId) => {
              setAssigneeId(userId || "");
              setAssigneeTouched(true);
            }}
            anchorEl={assigneeAnchorEl}
            onClose={() => setAssigneeAnchorEl(null)}
          />

          {/* Labels */}
          <Autocomplete
            multiple
            freeSolo
            options={projectLabels.filter((l) => !taskLabels.includes(l))}
            value={taskLabels}
            filterOptions={(options, params) => {
              const filtered = options.filter((o) =>
                o.toLowerCase().includes(params.inputValue.toLowerCase()),
              );
              const trimmed = params.inputValue.trim();
              if (
                trimmed &&
                !taskLabels.some(
                  (l) => l.toLowerCase() === trimmed.toLowerCase(),
                ) &&
                !options.some(
                  (o) => o.toLowerCase() === trimmed.toLowerCase(),
                )
              ) {
                filtered.push(`__create__:${trimmed}`);
              }
              return filtered;
            }}
            onChange={(_, newValue) => {
              const resolved = (newValue as string[]).map((v) =>
                v.startsWith("__create__:") ? v.slice("__create__:".length) : v,
              );
              setTaskLabels(resolved);
            }}
            getOptionLabel={(option) =>
              option.startsWith("__create__:")
                ? option.slice("__create__:".length)
                : option
            }
            renderOption={(props, option) => {
              if (option.startsWith("__create__:")) {
                const label = option.slice("__create__:".length);
                return (
                  <li {...props} key="__create__">
                    <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                      <Typography variant="body2" color="primary">
                        + Create &ldquo;{label}&rdquo;
                      </Typography>
                    </Box>
                  </li>
                );
              }
              return <li {...props} key={option}>{option}</li>;
            }}
            renderTags={(value, getTagProps) =>
              value.map((label, index) => (
                <Chip
                  key={label}
                  label={label}
                  size="small"
                  {...getTagProps({ index })}
                />
              ))
            }
            renderInput={(params) => (
              <TextField
                {...params}
                label="Labels"
                placeholder={taskLabels.length === 0 ? "Add label..." : ""}
                size="small"
                helperText="Optional: labels are remembered for your next task"
              />
            )}
          />

          {/* Single text box for everything */}
          <TextField
            label="Describe what you want to get done"
            fullWidth
            required
            multiline
            rows={embedded ? 6 : 9}
            value={taskPrompt}
            onChange={(e) => {
              const value = e.target.value;
              setTaskPrompt(value);
              clearTimeout(draftTimer.current);
              draftTimer.current = setTimeout(() => {
                if (value) {
                  localStorage.setItem(draftKey, JSON.stringify({ content: value }));
                } else {
                  localStorage.removeItem(draftKey);
                }
              }, 300);
            }}
            onKeyDown={(e) => {
              // If user presses Enter in empty text box, close panel
              if (
                e.key === "Enter" &&
                !taskPrompt.trim() &&
                !e.shiftKey &&
                !e.ctrlKey &&
                !e.metaKey
              ) {
                e.preventDefault();
                onClose?.();
              }
            }}
            placeholder={
              justDoItMode
                ? "Describe what you want the agent to do. It will start immediately without planning."
                : "Describe the task - the AI will generate specs from this."
            }
            helperText={
              justDoItMode
                ? "Planning will be skipped — agent starts implementation immediately"
                : "Planning agent extracts task name, description, and generates specifications"
            }
            inputRef={taskPromptRef}
            size="small"
          />

          {/* Attachment picker.
              IMPORTANT: do NOT wrap MUI Button around the input. Multiple iterations
              of `<Button component="label">` failed to open the native file dialog
              because MUI's button wrapping intercepts the click before the label
              fires. Use a plain styled <label htmlFor> pointing at a sibling <input>
              elsewhere on the page — the browser's label-for-input mechanism opens
              the dialog reliably with no JS involvement.

              Uploads happen after task creation in handleCreateTask. */}
          <Box>
            <input
              ref={attachmentInput}
              id="new-spectask-attach-input"
              type="file"
              multiple
              accept={ATTACHMENT_ACCEPT_ATTR}
              style={{ display: "none" }}
              onChange={(e) => {
                const files = Array.from(e.target.files || []);
                e.target.value = "";
                if (!files.length) return;

                const remaining = SPEC_TASK_ATTACHMENT_MAX_PER_TASK - pendingAttachments.length;
                if (files.length > remaining) {
                  snackbar.error(
                    `Can only attach ${remaining} more file(s) — limit is ${SPEC_TASK_ATTACHMENT_MAX_PER_TASK}.`,
                  );
                  return;
                }
                const accepted: File[] = [];
                for (const f of files) {
                  if (f.size > SPEC_TASK_ATTACHMENT_MAX_BYTES) {
                    snackbar.error(`${f.name} is too large (max ${humanAttachmentSize(SPEC_TASK_ATTACHMENT_MAX_BYTES)}).`);
                    continue;
                  }
                  accepted.push(f);
                }
                if (accepted.length) {
                  setPendingAttachments((prev) => [...prev, ...accepted]);
                }
              }}
            />
            <Stack direction="row" spacing={1} alignItems="center" sx={{ flexWrap: "wrap", gap: 1 }}>
              <Box
                component="label"
                htmlFor="new-spectask-attach-input"
                sx={{
                  display: "inline-flex",
                  alignItems: "center",
                  gap: 0.75,
                  px: 1.5,
                  py: 0.5,
                  border: "1px solid",
                  borderColor: pendingAttachments.length >= SPEC_TASK_ATTACHMENT_MAX_PER_TASK ? "action.disabled" : "primary.main",
                  color: pendingAttachments.length >= SPEC_TASK_ATTACHMENT_MAX_PER_TASK ? "action.disabled" : "primary.main",
                  borderRadius: 1,
                  fontSize: "0.8125rem",
                  fontWeight: 500,
                  textTransform: "uppercase",
                  letterSpacing: "0.02857em",
                  cursor: pendingAttachments.length >= SPEC_TASK_ATTACHMENT_MAX_PER_TASK ? "not-allowed" : "pointer",
                  userSelect: "none",
                  "&:hover": {
                    backgroundColor: pendingAttachments.length >= SPEC_TASK_ATTACHMENT_MAX_PER_TASK ? "transparent" : "action.hover",
                  },
                }}
              >
                <AttachFileIcon sx={{ fontSize: 18 }} />
                Attach files
              </Box>
              {pendingAttachments.map((f, idx) => (
                <Chip
                  key={`${f.name}-${idx}`}
                  size="small"
                  label={`${f.name} (${humanAttachmentSize(f.size)})`}
                  onDelete={() =>
                    setPendingAttachments((prev) => prev.filter((_, i) => i !== idx))
                  }
                />
              ))}
              {pendingAttachments.length === 0 && (
                <Typography variant="caption" color="text.secondary">
                  Optional — screenshots / PDFs / text the agent should look at.
                  Max {SPEC_TASK_ATTACHMENT_MAX_PER_TASK} files,{" "}
                  {humanAttachmentSize(SPEC_TASK_ATTACHMENT_MAX_BYTES)} each.
                </Typography>
              )}
            </Stack>
          </Box>

          <Autocomplete
            multiple
            options={dependencyTaskOptions}
            value={selectedDependencyTasks}
            onChange={(_, selectedTasks) => {
              setSelectedDependencyTaskIds(
                selectedTasks
                  .map((task) => task.id || "")
                  .filter((taskId) => !!taskId),
              );
            }}
            isOptionEqualToValue={(option, value) => option.id === value.id}
            getOptionLabel={(task) =>
              task.name ||
              task.short_title ||
              task.description ||
              task.original_prompt ||
              task.id ||
              "Untitled task"
            }
            filterSelectedOptions
            clearOnBlur={false}
            renderInput={(params) => (
              <TextField
                {...params}
                label="Depends on"
                placeholder="Search and select tasks"
                size="small"
                helperText="Optional: this task will be blocked until selected dependencies are done"
              />
            )}
            renderOption={(props, option) => (
              <li {...props} key={option.id}>
                <Box
                  sx={{ display: "flex", flexDirection: "column", py: 0.25 }}
                >
                  <Typography variant="body2">
                    {option.name ||
                      option.short_title ||
                      `Task #${option.task_number || "?"}`}
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    {`#${option.task_number || "?"} • ${option.status || "unknown"}`}
                  </Typography>
                </Box>
              </li>
            )}
          />

          {/* Branch Configuration */}
          {defaultRepoId && (
            <Box
              sx={{ border: 1, borderColor: "divider", borderRadius: 1, p: 2 }}
            >
              <Typography variant="subtitle2" gutterBottom>
                Where do you want to work?
              </Typography>

              {/* Mode Selection - Two Cards */}
              <Box sx={{ display: "flex", gap: 1, mb: 2 }}>
                <Box
                  onClick={() => setBranchMode(TypesBranchMode.BranchModeNew)}
                  sx={{
                    flex: 1,
                    p: 1.5,
                    border: 2,
                    borderColor:
                      branchMode === TypesBranchMode.BranchModeNew
                        ? "primary.main"
                        : "divider",
                    borderRadius: 1,
                    cursor: "pointer",
                    bgcolor:
                      branchMode === TypesBranchMode.BranchModeNew
                        ? "action.selected"
                        : "transparent",
                    "&:hover": { bgcolor: "action.hover" },
                  }}
                >
                  <Typography variant="body2" fontWeight={600}>
                    Start fresh
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Create a new branch
                  </Typography>
                </Box>
                <Box
                  onClick={() =>
                    setBranchMode(TypesBranchMode.BranchModeExisting)
                  }
                  sx={{
                    flex: 1,
                    p: 1.5,
                    border: 2,
                    borderColor:
                      branchMode === TypesBranchMode.BranchModeExisting
                        ? "primary.main"
                        : "divider",
                    borderRadius: 1,
                    cursor: "pointer",
                    bgcolor:
                      branchMode === TypesBranchMode.BranchModeExisting
                        ? "action.selected"
                        : "transparent",
                    "&:hover": { bgcolor: "action.hover" },
                  }}
                >
                  <Typography variant="body2" fontWeight={600}>
                    Continue existing
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Resume work on a branch
                  </Typography>
                </Box>
              </Box>

              {/* Mode-specific options */}
              {branchMode === TypesBranchMode.BranchModeNew ? (
                <Stack spacing={1.5}>
                  {!showBranchCustomization ? (
                    <Box>
                      <Typography variant="caption" color="text.secondary">
                        New branch from{" "}
                        <strong>{baseBranch || defaultBranchName}</strong>
                      </Typography>
                      <Button
                        size="small"
                        onClick={() => setShowBranchCustomization(true)}
                        sx={{
                          display: "block",
                          textTransform: "none",
                          fontSize: "0.75rem",
                          p: 0,
                          mt: 0.5,
                        }}
                      >
                        Customize branches
                      </Button>
                    </Box>
                  ) : (
                    <Box
                      sx={{
                        display: "flex",
                        flexDirection: "column",
                        gap: 1.5,
                      }}
                    >
                      <Box>
                        <FormControl fullWidth size="small">
                          <InputLabel>Base branch</InputLabel>
                          <Select
                            value={baseBranch}
                            onChange={(e) => setBaseBranch(e.target.value)}
                            label="Base branch"
                          >
                            {branchesData
                              ?.slice()
                              .sort((a: string, b: string) => a.localeCompare(b, undefined, { sensitivity: 'base' }))
                              .map((branch: string) => (
                              <MenuItem key={branch} value={branch}>
                                {branch}
                                {branch === defaultBranchName && (
                                  <Chip
                                    label="default"
                                    size="small"
                                    sx={{ ml: 1, height: 18 }}
                                  />
                                )}
                              </MenuItem>
                            ))}
                          </Select>
                        </FormControl>
                        <Typography
                          variant="caption"
                          color="text.secondary"
                          sx={{ mt: 0.5, ml: 1.75, display: "block" }}
                        >
                          New branch will be created from this base. Use to
                          build on existing work.
                        </Typography>
                      </Box>
                      <TextField
                        label="Working branch name"
                        size="small"
                        fullWidth
                        value={branchPrefix}
                        onChange={(e) => setBranchPrefix(e.target.value)}
                        placeholder="feature/user-auth"
                        helperText={
                          branchPrefix
                            ? `Work will be done on: ${branchPrefix}-{task#}`
                            : "Leave empty to auto-generate. This is where the agent commits changes."
                        }
                      />
                      <Button
                        size="small"
                        onClick={() => {
                          setShowBranchCustomization(false);
                          setBaseBranch(defaultBranchName);
                          setBranchPrefix("");
                        }}
                        sx={{
                          alignSelf: "flex-start",
                          textTransform: "none",
                          fontSize: "0.75rem",
                        }}
                      >
                        Use defaults
                      </Button>
                    </Box>
                  )}
                </Stack>
              ) : (
                <FormControl fullWidth size="small">
                  <InputLabel>Select branch</InputLabel>
                  <Select
                    value={workingBranch}
                    onChange={(e) => setWorkingBranch(e.target.value)}
                    label="Select branch"
                  >
                    {branchesData
                      ?.filter((branch: string) => branch !== defaultBranchName)
                      .sort((a: string, b: string) => a.localeCompare(b, undefined, { sensitivity: 'base' }))
                      .map((branch: string) => (
                        <MenuItem key={branch} value={branch}>
                          {branch}
                        </MenuItem>
                      ))}
                    {branchesData?.filter(
                      (branch: string) => branch !== defaultBranchName,
                    ).length === 0 && (
                      <MenuItem disabled value="">
                        No feature branches available
                      </MenuItem>
                    )}
                  </Select>
                </FormControl>
              )}
            </Box>
          )}

          {/* Agent Selection (dropdown) */}
          <Box>
            {!showCreateAgentForm ? (
              <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                <AgentDropdown
                  value={selectedHelixAgent}
                  onChange={setSelectedHelixAgent}
                  agents={sortedApps}
                />
                <Button
                  size="small"
                  onClick={() => setShowCreateAgentForm(true)}
                  sx={{
                    alignSelf: "flex-start",
                    textTransform: "none",
                    fontSize: "0.75rem",
                  }}
                >
                  + Create new agent
                </Button>
              </Box>
            ) : (
              <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
                <CodingAgentForm
                  ref={codingAgentFormRef}
                  value={{
                    codeAgentRuntime,
                    claudeCodeMode,
                    selectedProvider,
                    selectedModel,
                    agentName: newAgentName,
                  }}
                  onChange={(nextValue) => {
                    setCodeAgentRuntime(nextValue.codeAgentRuntime);
                    setClaudeCodeMode(nextValue.claudeCodeMode);
                    setSelectedProvider(nextValue.selectedProvider);
                    setSelectedModel(nextValue.selectedModel);
                    if (nextValue.agentName !== newAgentName) {
                      setUserModifiedName(true);
                    }
                    setNewAgentName(nextValue.agentName);
                  }}
                  disabled={creatingAgent || isCreating}
                  recommendedModels={RECOMMENDED_CODING_MODELS}
                  createAgentDescription="Code development agent for spec tasks"
                  onCreateStateChange={setCreatingAgent}
                  onAgentCreated={(app) => {
                    setSelectedHelixAgent(app.id);
                    setShowCreateAgentForm(false);
                  }}
                  modelPickerHint="Choose a capable model for agentic coding."
                  modelPickerDisplayMode="short"
                  sx={{ display: "flex", flexDirection: "column", gap: 0 }}
                />

                {sortedApps.length > 0 && (
                  <Button
                    size="small"
                    onClick={() => setShowCreateAgentForm(false)}
                    sx={{ alignSelf: "flex-start" }}
                    disabled={creatingAgent}
                  >
                    Back to agent list
                  </Button>
                )}
              </Box>
            )}
          </Box>

          {/* Skip Spec Checkbox */}
          <FormControl fullWidth>
            <Tooltip
              title={`Skip planning and go straight to implementation (${navigator.platform.includes("Mac") ? "⌘J" : "Ctrl+J"})`}
              placement="top"
            >
              <FormControlLabel
                control={
                  <Checkbox
                    checked={justDoItMode}
                    onChange={(e) => setJustDoItMode(e.target.checked)}
                    color="warning"
                  />
                }
                label={
                  <Box>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <Typography variant="body2" sx={{ fontWeight: 600 }}>
                        Skip planning
                      </Typography>
                      <Box
                        component="span"
                        sx={{
                          fontSize: "0.65rem",
                          opacity: 0.6,
                          fontFamily: "monospace",
                          border: "1px solid",
                          borderColor: "divider",
                          borderRadius: "3px",
                          px: 0.5,
                        }}
                      >
                        {navigator.platform.includes("Mac") ? "⌘J" : "Ctrl+J"}
                      </Box>
                    </Box>
                    <Typography variant="caption" color="text.secondary">
                      Skip planning — go straight to implementation
                    </Typography>
                  </Box>
                }
              />
            </Tooltip>
          </FormControl>

          {/* Start Immediately Checkbox */}
          <FormControl fullWidth>
            <Tooltip
              title="Start the task immediately on creation, regardless of the project's auto-start setting"
              placement="top"
            >
              <FormControlLabel
                control={
                  <Checkbox
                    checked={autoStart}
                    onChange={(e) => setAutoStart(e.target.checked)}
                    color="primary"
                  />
                }
                label={
                  <Box>
                    <Typography variant="body2" sx={{ fontWeight: 600 }}>
                      Start immediately
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      Skip the backlog — start as soon as the task is created
                    </Typography>
                  </Box>
                }
              />
            </Tooltip>
          </FormControl>
        </Stack>
      </Box>

      {/* Footer Actions */}
      <Box
        sx={{
          p: 2,
          borderTop: 1,
          borderColor: "divider",
          display: "flex",
          gap: 2,
          justifyContent: "flex-end",
        }}
      >
        {onClose && (
          <Button
            onClick={() => {
              resetForm();
              onClose();
            }}
          >
            Cancel
          </Button>
        )}
        <Button
          onClick={handleCreateTask}
          variant="contained"
          color="secondary"
          disabled={
            !taskPrompt.trim() ||
            isCreating ||
            creatingAgent ||
            (showCreateAgentForm &&
              !(
                codeAgentRuntime === "claude_code" &&
                claudeCodeMode === "subscription"
              ) &&
              (!selectedModel || !selectedProvider))
          }
          startIcon={
            isCreating || creatingAgent ? (
              <CircularProgress size={16} />
            ) : (
              <AddIcon />
            )
          }
          sx={{
            "& .MuiButton-endIcon": {
              ml: 1,
              opacity: 0.6,
              fontSize: "0.75rem",
            },
          }}
          endIcon={
            <Box
              component="span"
              sx={{
                fontSize: "0.75rem",
                opacity: 0.6,
                fontFamily: "monospace",
                ml: 1,
              }}
            >
              {navigator.platform.includes("Mac") ? "⌘↵" : "Ctrl+↵"}
            </Box>
          }
        >
          Create Task
        </Button>
      </Box>
    </Box>
  );
};

export default NewSpecTaskForm;
