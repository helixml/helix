import React, {
  FC,
  useState,
  useEffect,
  useRef,
  useContext,
  useMemo,
} from "react";
import {
  Container,
  Box,

  Typography,
  TextField,
  Button,
  Alert,
  CircularProgress,
  Divider,
  List,
  ListItem,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  FormControlLabel,
  Checkbox,
  Chip,
  Switch,
  IconButton,
} from "@mui/material";
import CodeIcon from "@mui/icons-material/Code";
import AddIcon from "@mui/icons-material/Add";
import WarningIcon from "@mui/icons-material/Warning";
import DeleteForeverIcon from "@mui/icons-material/DeleteForever";
import DeleteIcon from "@mui/icons-material/Delete";
import LinkIcon from "@mui/icons-material/Link";
import AutoFixHighIcon from "@mui/icons-material/AutoFixHigh";
import HistoryIcon from "@mui/icons-material/History";
import DescriptionIcon from "@mui/icons-material/Description";
import VpnKeyIcon from "@mui/icons-material/VpnKey";
import VisibilityIcon from "@mui/icons-material/Visibility";
import VisibilityOffIcon from "@mui/icons-material/VisibilityOff";
import MoveUpIcon from "@mui/icons-material/MoveUp";
import ArrowForwardIcon from "@mui/icons-material/ArrowForward";
import HubIcon from "@mui/icons-material/Hub";
import EditIcon from "@mui/icons-material/Edit";

import Skills from "../components/app/Skills";
import { TypesAssistantSkills, TypesProject, TypesZFSTree, TypesZFSTreeNode } from "../api/api";
import SavingToast from "../components/widgets/SavingToast";
import StartupScriptEditor from "../components/project/StartupScriptEditor";
import CodingAgentForm from "../components/agent/CodingAgentForm";
import {
  AppsContext,
  CodeAgentRuntime,
  generateAgentName,
} from "../contexts/apps";
import { IApp, IAppFlatState, AGENT_TYPE_ZED_EXTERNAL } from "../types";
import { RECOMMENDED_CODING_MODELS } from "../constants/models";
import type { CodingAgentFormHandle } from "../components/agent/CodingAgentForm";
import ProjectRepositoriesList from "../components/project/ProjectRepositoriesList";
import AgentDropdown from "../components/agent/AgentDropdown";
import { SparkLineChart } from "@mui/x-charts";
import DesktopStreamViewer from "../components/external-agent/DesktopStreamViewer";
import { useSandboxState } from "../components/external-agent/ExternalAgentDesktopViewer";
import useAccount from "../hooks/useAccount";
import useRouter from "../hooks/useRouter";
import useSnackbar from "../hooks/useSnackbar";
import useApi from "../hooks/useApi";
import { useQueryClient, useMutation, useQuery } from "@tanstack/react-query";
import {
  useGetProject,
  useUpdateProject,
  useGetProjectRepositories,
  useSetProjectPrimaryRepository,
  useAttachRepositoryToProject,
  useDetachRepositoryFromProject,
  useDeleteProject,
  useGetProjectExploratorySession,
  useStartProjectExploratorySession,
  useStopProjectExploratorySession,
  projectExploratorySessionQueryKey,
  useGetProjectGuidelinesHistory,
} from "../services";
import { useGitRepositories } from "../services/gitRepositoryService";

interface ProjectSettingsProps {
  projectId: string;
  tab?: string;
}

const ProjectSettings: FC<ProjectSettingsProps> = ({ projectId, tab = 'general' }) => {
  const account = useAccount();
  const { navigate } = useRouter();
  const snackbar = useSnackbar();
  const api = useApi();
  const queryClient = useQueryClient();
  const { apps, loadApps } = useContext(AppsContext);

  const { data: project, isLoading, error } = useGetProject(projectId);
  const { data: repositories = [] } = useGetProjectRepositories(projectId);

  const updateProjectMutation = useUpdateProject(projectId);
  const setPrimaryRepoMutation = useSetProjectPrimaryRepository(projectId);
  const attachRepoMutation = useAttachRepositoryToProject(projectId);
  const detachRepoMutation = useDetachRepositoryFromProject(projectId);
  const deleteProjectMutation = useDeleteProject();

  // Get current org context for fetching repositories
  const currentOrg = account.organizationTools.organization;
  const { data: allUserRepositories = [] } = useGitRepositories(
    currentOrg?.id
      ? { organizationId: currentOrg.id }
      : { ownerId: account.user?.id },
  );

  // Exploratory session
  const { data: exploratorySessionData } =
    useGetProjectExploratorySession(projectId);
  const startExploratorySessionMutation =
    useStartProjectExploratorySession(projectId);
  const stopExploratorySessionMutation =
    useStopProjectExploratorySession(projectId);

  // Create SpecTask mutation for "Fix Startup Script" feature
  const createSpecTaskMutation = useMutation({
    mutationFn: async (request: {
      prompt: string;
      branch_mode?: string;
      base_branch?: string;
      working_branch?: string;
    }) => {
      const response = await api.getApiClient().v1SpecTasksFromPromptCreate({
        project_id: projectId,
        prompt: request.prompt,
        branch_mode: request.branch_mode as any,
        base_branch: request.base_branch,
        working_branch: request.working_branch,
      });
      return response.data;
    },
    onSuccess: (task) => {
      snackbar.success("Created task to fix startup script");
      account.orgNavigate("project-specs", {
        id: projectId,
        highlight: task.id,
      });
    },
    onError: () => {
      snackbar.error("Failed to create task");
    },
  });

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [startupScript, setStartupScript] = useState("");
  const [guidelines, setGuidelines] = useState("");
  const [autoStartBacklogTasks, setAutoStartBacklogTasks] = useState(false);
  const [pullRequestReviewsEnabled, setPullRequestReviewsEnabled] =
    useState(false);
  const [koditEnabled, setKoditEnabled] = useState(true);
  const [autoWarmDockerCache, setAutoWarmDockerCache] = useState(false);
  const [showGoldenBuildViewer, setShowGoldenBuildViewer] = useState(false);
  const [selectedGoldenSandboxId, setSelectedGoldenSandboxId] = useState("");
  const [showTestSession, setShowTestSession] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteConfirmName, setDeleteConfirmName] = useState("");
  const [attachRepoDialogOpen, setAttachRepoDialogOpen] = useState(false);
  const [selectedRepoToAttach, setSelectedRepoToAttach] = useState("");
  const [savingProject, setSavingProject] = useState(false);
  const [testingStartupScript, setTestingStartupScript] = useState(false);
  const [isSessionRestart, setIsSessionRestart] = useState(false);
  const [guidelinesHistoryDialogOpen, setGuidelinesHistoryDialogOpen] =
    useState(false);

  // Guidelines history
  const { data: guidelinesHistory = [] } = useGetProjectGuidelinesHistory(
    projectId,
    guidelinesHistoryDialogOpen,
  );

  // Per-sandbox golden cache state
  const sandboxCacheMap = project?.metadata?.docker_cache_status?.sandboxes ?? {};
  const sandboxEntries = Object.entries(sandboxCacheMap);
  const anyBuilding = sandboxEntries.some(([, s]) => s.status === "building");
  const anyReady = sandboxEntries.some(([, s]) => s.status === "ready");
  const anyFailed = sandboxEntries.some(([, s]) => s.status === "failed");

  // Golden build session
  const goldenBuildSessionId = selectedGoldenSandboxId
    ? sandboxCacheMap[selectedGoldenSandboxId]?.build_session_id
    : undefined;
  const { data: goldenBuildSession } = useQuery({
    queryKey: ["session", goldenBuildSessionId],
    queryFn: async () => {
      const response = await api.getApiClient().v1SessionsDetail(goldenBuildSessionId!);
      return response.data;
    },
    enabled: !!goldenBuildSessionId && showGoldenBuildViewer,
    refetchInterval: 5000,
  });
  const goldenBuildSandboxState = useSandboxState(
    goldenBuildSessionId || "",
    !!goldenBuildSessionId && showGoldenBuildViewer,
  );

  // ZFS snapshot/clone tree
  const { data: zfsTree } = useQuery({
    queryKey: ["zfs-tree", projectId],
    queryFn: async () => {
      const response = await api.getApiClient().v1ProjectsDockerCacheZfsTreeDetail(projectId);
      return response.data;
    },
    enabled: !!projectId && (autoWarmDockerCache || sandboxEntries.length > 0),
    refetchInterval: 30000,
  });

  // Poll project status while any golden build is running and viewer is open
  useEffect(() => {
    if (!showGoldenBuildViewer || !anyBuilding) return;
    const interval = setInterval(() => {
      queryClient.invalidateQueries({ queryKey: ["project", projectId] });
    }, 10000);
    return () => clearInterval(interval);
  }, [showGoldenBuildViewer, anyBuilding, projectId]);

  // Auto-close golden build viewer when selected sandbox's build finishes
  const selectedSandboxStatus = selectedGoldenSandboxId
    ? sandboxCacheMap[selectedGoldenSandboxId]?.status
    : undefined;
  useEffect(() => {
    if (showGoldenBuildViewer && selectedGoldenSandboxId && selectedSandboxStatus && selectedSandboxStatus !== "building") {
      setShowGoldenBuildViewer(false);
      setSelectedGoldenSandboxId("");
    }
  }, [showGoldenBuildViewer, selectedGoldenSandboxId, selectedSandboxStatus]);

  // Per-container blkio write rate sparkline for golden builds
  const buildingSandbox = sandboxEntries.find(([, s]) => s.status === "building");
  const buildingSandboxId = buildingSandbox?.[0];
  const buildingSessionId = buildingSandbox?.[1]?.build_session_id;
  const blkioSamplesRef = useRef<{ time: number; writeBytes: number }[]>([]);
  const lastBuildSessionRef = useRef<string | undefined>();

  if (buildingSessionId !== lastBuildSessionRef.current) {
    blkioSamplesRef.current = [];
    lastBuildSessionRef.current = buildingSessionId;
  }

  const { data: blkioStats } = useQuery({
    queryKey: ["blkio", buildingSandboxId, buildingSessionId],
    queryFn: async () => {
      const resp = await api.getApiClient().v1SandboxesContainersBlkioDetail(buildingSandboxId!, buildingSessionId!);
      return resp.data;
    },
    enabled: !!buildingSandboxId && !!buildingSessionId && anyBuilding,
    refetchInterval: 5_000,
  });

  const writeRates = useMemo(() => {
    if (!blkioStats?.write_bytes) return [];
    const now = Date.now();
    const samples = blkioSamplesRef.current;
    const last = samples[samples.length - 1];
    if (!last || blkioStats.write_bytes !== last.writeBytes) {
      samples.push({ time: now, writeBytes: blkioStats.write_bytes });
    }
    if (samples.length > 30) samples.splice(0, samples.length - 30);
    if (samples.length < 2) return [];
    return samples.slice(1).map((s, i) => {
      const prev = samples[i];
      const dtSec = (s.time - prev.time) / 1000;
      const dBytes = s.writeBytes - prev.writeBytes;
      return dtSec > 0 ? Math.max(0, dBytes / dtSec / 1e6) : 0;
    });
  }, [blkioStats]);

  // Move to organization state
  const [moveDialogOpen, setMoveDialogOpen] = useState(false);
  const [selectedOrgToMove, setSelectedOrgToMove] = useState("");
  const [movePreview, setMovePreview] = useState<{
    project: { current_name: string; new_name?: string; has_conflict: boolean };
    repositories: Array<{
      id: string;
      current_name: string;
      new_name?: string;
      has_conflict: boolean;
      affected_projects?: Array<{ id: string; name: string }>;
    }>;
    warnings?: string[];
  } | null>(null);
  const [acceptSharedRepoWarning, setAcceptSharedRepoWarning] = useState(false);
  const [loadingMovePreview, setLoadingMovePreview] = useState(false);

  // Project secrets
  const [addSecretDialogOpen, setAddSecretDialogOpen] = useState(false);
  const [newSecretName, setNewSecretName] = useState("");
  const [newSecretValue, setNewSecretValue] = useState("");
  const [showSecretValue, setShowSecretValue] = useState(false);

  // Project skills
  const [projectSkills, setProjectSkills] = useState<TypesAssistantSkills | undefined>(undefined);

  // Project secrets query
  const { data: projectSecrets = [], refetch: refetchSecrets } = useQuery({
    queryKey: ["project-secrets", projectId],
    queryFn: async () => {
      const response = await api
        .getApiClient()
        .v1ProjectsSecretsDetail(projectId);
      return response.data || [];
    },
    enabled: !!projectId,
  });

  // Create secret mutation
  const createSecretMutation = useMutation({
    mutationFn: async ({ name, value }: { name: string; value: string }) => {
      const response = await api
        .getApiClient()
        .v1ProjectsSecretsCreate(projectId, { name, value });
      return response.data;
    },
    onSuccess: () => {
      snackbar.success("Secret created successfully");
      setAddSecretDialogOpen(false);
      setNewSecretName("");
      setNewSecretValue("");
      refetchSecrets();
    },
    onError: (err: any) => {
      const message = err?.response?.data?.error || "Failed to create secret";
      snackbar.error(message);
    },
  });

  // Delete secret mutation
  const deleteSecretMutation = useMutation({
    mutationFn: async (secretId: string) => {
      await api.getApiClient().v1SecretsDelete(secretId);
    },
    onSuccess: () => {
      snackbar.success("Secret deleted");
      refetchSecrets();
    },
    onError: () => {
      snackbar.error("Failed to delete secret");
    },
  });

  const primeCacheMutation = useMutation({
    mutationFn: async () => {
      await api
        .getApiClient()
        .v1ProjectsDockerCacheBuildCreate(projectId);
    },
    onSuccess: async () => {
      snackbar.success("Golden build triggered on all sandboxes");
      await new Promise((r) => setTimeout(r, 3000));
      const freshProject = await queryClient.fetchQuery({
        queryKey: ["project", projectId],
        staleTime: 0,
      });
      const sandboxes = (freshProject as TypesProject)?.metadata?.docker_cache_status?.sandboxes;
      if (sandboxes) {
        const buildingSb = Object.entries(sandboxes).find(([, s]) => s.status === "building");
        if (buildingSb) {
          setSelectedGoldenSandboxId(buildingSb[0]);
          setShowGoldenBuildViewer(true);
        }
      }
    },
    onError: (error: any) => {
      const data = error?.response?.data;
      const msg =
        (typeof data === "string" ? data.trim() : data?.message) ||
        "Failed to trigger golden build";
      snackbar.error(msg);
    },
  });

  const clearCacheMutation = useMutation({
    mutationFn: async () => {
      await api.getApiClient().v1ProjectsDockerCacheDelete(projectId);
    },
    onSuccess: () => {
      snackbar.success("Docker cache cleared");
      queryClient.invalidateQueries({ queryKey: ["project", projectId] });
    },
    onError: (error: any) => {
      const msg =
        error?.response?.data?.message || "Failed to clear cache";
      snackbar.error(msg);
    },
  });

  const cancelBuildMutation = useMutation({
    mutationFn: async () => {
      await api.getApiClient().v1ProjectsDockerCacheCancelCreate(projectId);
    },
    onSuccess: () => {
      snackbar.success("Golden builds cancelled");
      setShowGoldenBuildViewer(false);
      setSelectedGoldenSandboxId("");
      queryClient.invalidateQueries({ queryKey: ["project", projectId] });
    },
    onError: (error: any) => {
      const msg =
        error?.response?.data?.message || "Failed to cancel builds";
      snackbar.error(msg);
    },
  });

  // Move project mutation
  const moveProjectMutation = useMutation({
    mutationFn: async (organizationId: string) => {
      const response = await api
        .getApiClient()
        .v1ProjectsMoveCreate(projectId, {
          organization_id: organizationId,
        });
      return response.data;
    },
    onSuccess: async (_data, organizationId) => {
      snackbar.success("Project moved to organization successfully");
      setMoveDialogOpen(false);
      setSelectedOrgToMove("");
      setMovePreview(null);
      setAcceptSharedRepoWarning(false);
      queryClient.invalidateQueries({ queryKey: ["project", projectId] });

      const targetOrg = account.organizationTools.organizations.find(
        (org) => org.id === organizationId
      );
      if (targetOrg?.name) {
        await account.organizationTools.loadOrganization(organizationId);
        navigate("org_project-specs", {
          org_id: targetOrg.name,
          id: projectId,
        });
      }
    },
    onError: (err: any) => {
      const message = err?.response?.data?.error || "Failed to move project";
      snackbar.error(message);
    },
  });

  const handleOrgSelectForMove = async (orgId: string) => {
    setSelectedOrgToMove(orgId);
    if (!orgId) {
      setMovePreview(null);
      return;
    }

    setLoadingMovePreview(true);
    try {
      const response = await api
        .getApiClient()
        .v1ProjectsMovePreviewCreate(projectId, {
          organization_id: orgId,
        });
      setMovePreview(response.data as any);
    } catch (err: any) {
      const message =
        err?.response?.data?.error || "Failed to check for conflicts";
      snackbar.error(message);
      setMovePreview(null);
    } finally {
      setLoadingMovePreview(false);
    }
  };

  // Board settings state
  const [wipLimits, setWipLimits] = useState({
    planning: 3,
    review: 2,
    implementation: 5,
  });

  // Default agent state
  const [selectedAgentId, setSelectedAgentId] = useState<string>("");
  const [selectedProjectManagerAgentId, setSelectedProjectManagerAgentId] =
    useState<string>("");
  const [
    selectedPullRequestReviewerAgentId,
    setSelectedPullRequestReviewerAgentId,
  ] = useState<string>("");
  const [showCreateAgentForm, setShowCreateAgentForm] = useState(false);
  const [codeAgentRuntime, setCodeAgentRuntime] =
    useState<CodeAgentRuntime>("zed_agent");
  const [claudeCodeMode, setClaudeCodeMode] =
    useState<"subscription" | "api_key">("subscription");
  const [selectedProvider, setSelectedProvider] = useState("");
  const [selectedModel, setSelectedModel] = useState("");
  const [newAgentName, setNewAgentName] = useState("-");
  const [userModifiedName, setUserModifiedName] = useState(false);
  const [creatingAgent, setCreatingAgent] = useState(false);
  const codingAgentFormRef = useRef<CodingAgentFormHandle>(null);

  const sortedApps = useMemo(() => {
    if (!apps) return [];
    const zedExternalApps: IApp[] = [];
    const otherApps: IApp[] = [];
    apps.forEach((app) => {
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
    return [...zedExternalApps, ...otherApps];
  }, [apps]);

  const primaryRepoIsExternal = useMemo(() => {
    if (!project?.default_repo_id || repositories.length === 0) return false;
    const primaryRepo = repositories.find(
      (repo) => repo.id === project.default_repo_id,
    );
    return primaryRepo?.external_url ? true : false;
  }, [project?.default_repo_id, repositories]);

  useEffect(() => {
    loadApps();
  }, [loadApps]);

  useEffect(() => {
    if (!userModifiedName && showCreateAgentForm) {
      setNewAgentName(generateAgentName(selectedModel, codeAgentRuntime));
    }
  }, [selectedModel, codeAgentRuntime, userModifiedName, showCreateAgentForm]);

  // Initialize form from server data
  useEffect(() => {
    if (project) {
      setName(project.name || "");
      setDescription(project.description || "");
      setStartupScript(project.startup_script || "");
      setGuidelines(project.guidelines || "");
      setAutoStartBacklogTasks(project.auto_start_backlog_tasks || false);
      setPullRequestReviewsEnabled(
        project.pull_request_reviews_enabled || false,
      );
      setKoditEnabled(project.kodit_enabled !== false);
      setAutoWarmDockerCache(
        project.metadata?.auto_warm_docker_cache || false,
      );
      setSelectedAgentId(project.default_helix_app_id || "");
      setSelectedProjectManagerAgentId(
        project.project_manager_helix_app_id || "",
      );
      setSelectedPullRequestReviewerAgentId(
        project.pull_request_reviewer_helix_app_id || "",
      );

      const projectWipLimits = project.metadata?.board_settings?.wip_limits;
      if (projectWipLimits) {
        setWipLimits({
          planning: projectWipLimits.planning || 3,
          review: projectWipLimits.review || 2,
          implementation: projectWipLimits.implementation || 5,
        });
      }

      setProjectSkills(project.skills);
    }
  }, [project]);

  const handleSave = async (showSuccessMessage = true) => {
    if (savingProject) return false;
    if (!project || !name) return false;

    try {
      setSavingProject(true);
      await updateProjectMutation.mutateAsync({
        name,
        description,
        startup_script: startupScript,
        guidelines,
        auto_start_backlog_tasks: autoStartBacklogTasks,
        pull_request_reviews_enabled: pullRequestReviewsEnabled,
        default_helix_app_id: selectedAgentId || undefined,
        project_manager_helix_app_id:
          selectedProjectManagerAgentId || undefined,
        metadata: {
          board_settings: {
            wip_limits: wipLimits,
          },
          auto_warm_docker_cache: autoWarmDockerCache,
        },
      });

      if (showSuccessMessage) {
        snackbar.success("Project settings saved");
      }
      return true;
    } catch (err) {
      snackbar.error("Failed to save project settings");
      throw err;
    } finally {
      setSavingProject(false);
    }
  };

  const handleFieldBlur = () => {
    handleSave(false);
  };

  const handleCreateAgent = async () => {
    const createdAgent = await codingAgentFormRef.current?.handleCreateAgent();
    if (!createdAgent?.id) return;
    setSelectedAgentId(createdAgent.id);
    setShowCreateAgentForm(false);
    await updateProjectMutation.mutateAsync({
      default_helix_app_id: createdAgent.id,
    });
    snackbar.success("Agent created and set as default");
  };

  const handleSetPrimaryRepo = async (repoId: string) => {
    try {
      await setPrimaryRepoMutation.mutateAsync(repoId);
      snackbar.success("Primary repository updated");
    } catch (err) {
      snackbar.error("Failed to update primary repository");
    }
  };

  const handleAttachRepository = async () => {
    if (!selectedRepoToAttach) {
      snackbar.error("Please select a repository");
      return;
    }
    try {
      await attachRepoMutation.mutateAsync(selectedRepoToAttach);
      snackbar.success("Repository attached successfully");
      setAttachRepoDialogOpen(false);
      setSelectedRepoToAttach("");
    } catch (err) {
      snackbar.error("Failed to attach repository");
    }
  };

  const handleDetachRepository = async (repoId: string) => {
    try {
      await detachRepoMutation.mutateAsync(repoId);
      snackbar.success("Repository detached successfully");
    } catch (err) {
      snackbar.error("Failed to detach repository");
    }
  };

  const handleTestStartupScript = async () => {
    const isRestart = !!exploratorySessionData;
    setIsSessionRestart(isRestart);
    setTestingStartupScript(true);

    try {
      const saved = await handleSave(false);
      if (!saved) {
        snackbar.error("Failed to save settings before testing");
        return;
      }

      if (exploratorySessionData) {
        try {
          await stopExploratorySessionMutation.mutateAsync();
          await new Promise((resolve) => setTimeout(resolve, 1000));
        } catch (err: any) {
          const isNotFound =
            err?.response?.status === 404 ||
            err?.response?.status === 500 ||
            err?.message?.includes("not found");
          if (!isNotFound) {
            snackbar.error("Failed to stop existing session");
            return;
          }
        }
      }

      const session = await startExploratorySessionMutation.mutateAsync();
      snackbar.success("Testing startup script");

      await queryClient.refetchQueries({
        queryKey: projectExploratorySessionQueryKey(projectId),
      });

      setShowTestSession(true);
    } catch (err: any) {
      const errorMessage =
        err?.response?.data?.error ||
        err?.message ||
        "Failed to start exploratory session";
      snackbar.error(errorMessage);
    } finally {
      const delay = isRestart ? 7000 : 2000;
      setTimeout(() => setTestingStartupScript(false), delay);
    }
  };

  const handleDeleteProject = async () => {
    if (deleteConfirmName !== project?.name) {
      snackbar.error("Project name does not match");
      return;
    }

    try {
      await deleteProjectMutation.mutateAsync(projectId);
      snackbar.success("Project deleted successfully");
      setDeleteDialogOpen(false);
      account.orgNavigate("projects");
    } catch (err) {
      snackbar.error("Failed to delete project");
    }
  };

  const skillsFlatState: IAppFlatState = useMemo(() => ({
    apiTools: projectSkills?.apis,
    mcpTools: projectSkills?.mcps,
    browserTool: projectSkills?.browser,
    webSearchTool: projectSkills?.web_search,
    calculatorTool: projectSkills?.calculator,
    emailTool: projectSkills?.email,
    projectManagerTool: projectSkills?.project_manager,
    azureDevOpsTool: projectSkills?.azure_devops,
    zapierTools: projectSkills?.zapier,
    default_agent_type: AGENT_TYPE_ZED_EXTERNAL,
  }), [projectSkills]);

  const handleSkillsUpdate = async (updates: IAppFlatState) => {
    const newSkills: TypesAssistantSkills = {
      apis: updates.apiTools,
      mcps: updates.mcpTools,
      browser: updates.browserTool,
      web_search: updates.webSearchTool,
      calculator: updates.calculatorTool,
      email: updates.emailTool,
      project_manager: updates.projectManagerTool,
      azure_devops: updates.azureDevOpsTool,
      zapier: updates.zapierTools,
    };
    setProjectSkills(newSkills);
    await updateProjectMutation.mutateAsync({
      skills: newSkills,
    });
  };

  // Loading state
  if (isLoading) {
    return (
      <Box
        sx={{
          display: "flex",
          justifyContent: "center",
          alignItems: "center",
          height: "100%",
        }}
      >
        <CircularProgress />
      </Box>
    );
  }

  if (error || !project) {
    return (
      <Box
        sx={{
          display: "flex",
          justifyContent: "center",
          alignItems: "center",
          height: "100%",
        }}
      >
        <Alert severity="error">
          {error instanceof Error ? error.message : "Project not found"}
        </Alert>
      </Box>
    );
  }

  // ─── Tab render functions ───────────────────────────────────────────

  const renderGeneralTab = () => (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
      {/* Basic Information */}
      <Box>
        <Typography variant="h6" gutterBottom>
          Basic Information
        </Typography>
        <Divider sx={{ mb: 3 }} />
        <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
          <TextField
            label="Project Name"
            fullWidth
            value={name}
            onChange={(e) => setName(e.target.value)}
            onBlur={handleFieldBlur}
            required
          />
          <TextField
            label="Description"
            fullWidth
            multiline
            rows={3}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            onBlur={handleFieldBlur}
          />
        </Box>
      </Box>

      {/* Project Guidelines */}
      <Box>
        <Box
          sx={{
            display: "flex",
            flexDirection: { xs: "column", sm: "row" },
            alignItems: { xs: "flex-start", sm: "center" },
            justifyContent: "space-between",
            gap: { xs: 1, sm: 0 },
            mb: 1,
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center" }}>
            <DescriptionIcon sx={{ mr: 1 }} />
            <Typography variant="h6">Project Guidelines</Typography>
          </Box>
          {project.guidelines_version &&
            project.guidelines_version > 0 && (
              <Button
                size="small"
                startIcon={<HistoryIcon />}
                onClick={() => setGuidelinesHistoryDialogOpen(true)}
              >
                History (v{project.guidelines_version})
              </Button>
            )}
        </Box>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Guidelines specific to this project. These are combined with
          organization guidelines and sent to AI agents during planning,
          implementation, and exploratory sessions.
        </Typography>
        <Divider sx={{ mb: 3 }} />
        <TextField
          fullWidth
          multiline
          minRows={4}
          maxRows={12}
          placeholder="Example:
- Use React Query for all API calls
- Follow the existing component patterns in src/components
- Always add unit tests for new features
- Use MUI components for UI elements"
          value={guidelines}
          onChange={(e) => setGuidelines(e.target.value)}
          onBlur={handleFieldBlur}
        />
        {project.guidelines_updated_at && (
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ mt: 1, display: "block" }}
          >
            Last updated:{" "}
            {new Date(project.guidelines_updated_at).toLocaleDateString()}
            {project.guidelines_version
              ? ` (v${project.guidelines_version})`
              : ""}
          </Typography>
        )}
      </Box>

      {/* Repositories */}
      <Box>
        <Box
          sx={{
            display: "flex",
            flexDirection: { xs: "column", sm: "row" },
            justifyContent: "space-between",
            alignItems: { xs: "stretch", sm: "flex-start" },
            gap: { xs: 2, sm: 0 },
            mb: 2,
          }}
        >
          <Box>
            <Typography variant="h6" gutterBottom>
              Repositories
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Repositories attached to this project. The primary
              repository is opened by default when agents start.
            </Typography>
          </Box>
          <Button
            variant="outlined"
            startIcon={<AddIcon />}
            onClick={() => setAttachRepoDialogOpen(true)}
            size="small"
            sx={{ flexShrink: 0, alignSelf: { xs: "flex-start", sm: "flex-start" } }}
          >
            Attach
          </Button>
        </Box>
        <Divider sx={{ mb: 2 }} />

        <Typography
          variant="caption"
          color="text.secondary"
          sx={{ fontWeight: 600, mb: 1, display: "block" }}
        >
          Code Repositories
        </Typography>

        {repositories.length === 0 ? (
          <Typography
            variant="body2"
            color="text.secondary"
            sx={{ textAlign: "center", py: 4 }}
          >
            No code repositories attached to this project yet. Click
            "Attach Repository" to add one.
          </Typography>
        ) : (
          <ProjectRepositoriesList
            repositories={repositories}
            primaryRepoId={project.default_repo_id}
            onSetPrimaryRepo={handleSetPrimaryRepo}
            onDetachRepo={handleDetachRepository}
            setPrimaryRepoPending={setPrimaryRepoMutation.isPending}
            detachRepoPending={detachRepoMutation.isPending}
          />
        )}
      </Box>
    </Box>
  );

  const renderSandboxTab = () => (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
      {/* Startup Script */}
      <Box sx={{ display: "flex", gap: 3, alignItems: "flex-start" }}>
        <Box sx={{ flex: showTestSession ? undefined : 1, width: showTestSession ? 600 : undefined, flexShrink: 0 }}>
          <Box sx={{ display: "flex", alignItems: "center", mb: 1 }}>
            <CodeIcon sx={{ mr: 1 }} />
            <Typography variant="h6">Startup Script</Typography>
          </Box>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            This script runs when an agent starts working on this project.
            Use it to install dependencies, start dev servers, etc.
          </Typography>
          <Divider sx={{ mb: 3 }} />

          <StartupScriptEditor
            value={startupScript}
            onChange={setStartupScript}
            onTest={handleTestStartupScript}
            onSave={() => handleSave(true)}
            testDisabled={startExploratorySessionMutation.isPending}
            testLoading={testingStartupScript}
            testTooltip={
              exploratorySessionData
                ? "Will restart the running exploratory session"
                : undefined
            }
            projectId={projectId}
          />
        </Box>

        {/* Test session viewer */}
        {showTestSession && exploratorySessionData && (
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Box>
              <Box sx={{ display: "flex", alignItems: "center", mb: 1 }}>
                <Typography variant="h6" sx={{ flex: 1 }}>
                  Test Session
                </Typography>
                <Button
                  size="small"
                  variant="outlined"
                  onClick={() => setShowTestSession(false)}
                >
                  Hide
                </Button>
              </Box>
              <Divider sx={{ mb: 2 }} />
              <Box
                sx={{
                  aspectRatio: "16 / 9",
                  backgroundColor: "#000",
                  overflow: "hidden",
                }}
              >
                <DesktopStreamViewer
                  sessionId={exploratorySessionData.id}
                  sandboxId={exploratorySessionData.sandbox_id || ""}
                  showLoadingOverlay={testingStartupScript}
                  isRestart={isSessionRestart}
                />
              </Box>
              <Box
                sx={{
                  mt: 2,
                  p: 2,
                  backgroundColor: "action.hover",
                  borderRadius: 1,
                }}
              >
                <Typography
                  variant="body2"
                  color="text.secondary"
                  sx={{ mb: 1 }}
                >
                  Having trouble with your startup script?
                </Typography>
                <Button
                  variant="outlined"
                  size="small"
                  startIcon={
                    createSpecTaskMutation.isPending ? (
                      <CircularProgress size={16} />
                    ) : (
                      <AutoFixHighIcon />
                    )
                  }
                  onClick={() =>
                    createSpecTaskMutation.mutate({
                      prompt: `Fix the project startup script at /home/retro/work/helix-specs/.helix/startup.sh (in the helix-specs worktree). The current script is:\n\n\`\`\`bash\n${startupScript}\n\`\`\`\n\nPlease review and fix any issues. You can run the script to test it and iterate on it until it works. It should be idempotent.\n\nIMPORTANT: The startup script lives in the helix-specs branch, NOT the main code branch. After fixing the script:\n1. Edit /home/retro/work/helix-specs/.helix/startup.sh directly\n2. Commit and push directly to helix-specs branch: cd /home/retro/work/helix-specs && git add -A && git commit -m "Fix startup script" && git push origin helix-specs\n3. The user can then test it in the project settings panel.\n\nNote: A feature branch has been created on the primary repo for any code changes (like fixing bugs in the workspace setup or build scripts), but you probably won't need to use it unless the user specifically asks you to fix something in the codebase itself.`,
                      branch_mode: "new",
                      base_branch: "main",
                    })
                  }
                  disabled={createSpecTaskMutation.isPending}
                >
                  {createSpecTaskMutation.isPending
                    ? "Creating task..."
                    : "Get AI to fix it"}
                </Button>
              </Box>
            </Box>
          </Box>
        )}
      </Box>

      {/* Docker Cache */}
      <Box sx={{ display: "flex", gap: 2, alignItems: "flex-start" }}>
        <Box sx={{ flex: 1 }}>
          <Box sx={{ display: "flex", alignItems: "center", mb: 1 }}>
            <Typography variant="h6">Docker Cache</Typography>
          </Box>
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
            }}
          >
            <Box sx={{ flex: 1, mr: 2 }}>
              <Typography variant="body2" sx={{ fontWeight: 600 }}>
                Pre-warm Docker cache
              </Typography>
              <Typography variant="caption" color="text.secondary">
                Build a golden Docker cache on merge to main. New sessions
                start with pre-built images instead of building from
                scratch.
              </Typography>
            </Box>
            <Switch
              checked={autoWarmDockerCache}
              onChange={(e) => {
                const newValue = e.target.checked;
                setAutoWarmDockerCache(newValue);
                updateProjectMutation.mutate({
                  metadata: {
                    auto_warm_docker_cache: newValue,
                  },
                });
              }}
            />
          </Box>
          {autoWarmDockerCache && (
            <Box
              sx={{
                mt: 1,
                p: 1.5,
                bgcolor: "action.hover",
                borderRadius: 1,
              }}
            >
              {sandboxEntries.length === 0 ? (
                <Typography variant="caption" color="text.secondary" component="div">
                  Waiting for first merge to main
                </Typography>
              ) : (
                sandboxEntries.map(([sbId, sbState]) => (
                  <Box key={sbId} sx={{ mb: sandboxEntries.length > 1 ? 1 : 0 }}>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <Chip
                        label={sbId}
                        size="small"
                        variant="outlined"
                        sx={{ fontFamily: "monospace", fontSize: "0.7rem" }}
                      />
                      <Typography variant="caption" color="text.secondary">
                        {sbState.status === "ready" && "Ready"}
                        {sbState.status === "building" && "Building..."}
                        {sbState.status === "failed" && "Failed"}
                        {sbState.status === "none" && "No cache"}
                        {(sbState.size_bytes ?? 0) > 0 && (
                          <> &middot; {((sbState.size_bytes ?? 0) / 1e9).toFixed(1)} GB</>
                        )}
                        {sbState.last_ready_at && (
                          <> &middot; Last built: {new Date(sbState.last_ready_at).toLocaleString()}</>
                        )}
                      </Typography>
                      {sbState.status === "building" && writeRates.length > 1 && sbId === buildingSandboxId && (
                        <Box sx={{ display: "inline-flex", alignItems: "center", ml: 0.5 }}>
                          <SparkLineChart
                            data={writeRates}
                            height={20}
                            width={60}
                            curve="natural"
                            colors={["#4caf50"]}
                          />
                          <Typography variant="caption" color="text.secondary" sx={{ ml: 0.5, fontFamily: "monospace", fontSize: "0.65rem" }}>
                            {writeRates[writeRates.length - 1]?.toFixed(0)} MB/s cache writes
                          </Typography>
                        </Box>
                      )}
                      {sbState.status === "building" && sbState.build_session_id && (
                        <Button
                          size="small"
                          variant="contained"
                          sx={{ ml: "auto", minWidth: 0, px: 1, py: 0.25, fontSize: "0.7rem" }}
                          onClick={() => {
                            setSelectedGoldenSandboxId(sbId);
                            setShowGoldenBuildViewer(true);
                          }}
                        >
                          Watch
                        </Button>
                      )}
                    </Box>
                    {sbState.error && (
                      <Typography variant="caption" color="error" component="div" sx={{ mt: 0.25, ml: 1 }}>
                        {sbState.error}
                      </Typography>
                    )}
                  </Box>
                ))
              )}
              <Box sx={{ mt: 1, display: "flex", gap: 1 }}>
                <Button
                  size="small"
                  variant="outlined"
                  disabled={primeCacheMutation.isPending || anyBuilding}
                  onClick={() => primeCacheMutation.mutate()}
                >
                  {primeCacheMutation.isPending ? "Triggering..." : "Prime Cache"}
                </Button>
                {anyBuilding && (
                  <Button
                    size="small"
                    variant="outlined"
                    color="warning"
                    disabled={cancelBuildMutation.isPending}
                    onClick={() => cancelBuildMutation.mutate()}
                  >
                    {cancelBuildMutation.isPending ? "Cancelling..." : "Cancel Build"}
                  </Button>
                )}
                {(anyReady || anyFailed) && (() => {
                  const hasActiveClones = zfsTree?.golden?.children?.some(
                    (snap: TypesZFSTreeNode) => snap.children && snap.children.length > 0
                  );
                  return (
                    <Button
                      size="small"
                      variant="outlined"
                      color="error"
                      disabled={clearCacheMutation.isPending || !!hasActiveClones}
                      onClick={() => clearCacheMutation.mutate()}
                      title={hasActiveClones ? "Stop all sessions first" : ""}
                    >
                      {clearCacheMutation.isPending ? "Clearing..." : hasActiveClones ? "Clear Cache (sessions active)" : "Clear Cache"}
                    </Button>
                  );
                })()}
              </Box>
            </Box>
          )}
        </Box>

        {/* Golden build viewer */}
        {showGoldenBuildViewer && goldenBuildSessionId && (
          <Box sx={{ flex: 2 }}>
            <Box>
              <Box sx={{ display: "flex", alignItems: "center", mb: 1 }}>
                <Typography variant="h6" sx={{ flex: 1 }}>
                  Golden Build{selectedGoldenSandboxId ? ` (${selectedGoldenSandboxId})` : ""}
                </Typography>
                <Button
                  size="small"
                  variant="outlined"
                  onClick={() => {
                    setShowGoldenBuildViewer(false);
                    setSelectedGoldenSandboxId("");
                  }}
                >
                  Hide
                </Button>
              </Box>
              <Divider sx={{ mb: 2 }} />
              {goldenBuildSession && goldenBuildSandboxState.isRunning ? (
                <Box
                  sx={{
                    width: "100%",
                    aspectRatio: "16 / 9",
                    backgroundColor: "#000",
                  }}
                >
                  <DesktopStreamViewer
                    sessionId={goldenBuildSessionId}
                    sandboxId={goldenBuildSession.sandbox_id || ""}
                  />
                </Box>
              ) : (
                <Box sx={{ p: 4, textAlign: "center" }}>
                  <CircularProgress size={24} />
                  <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
                    {goldenBuildSandboxState.statusMessage || "Starting session..."}
                  </Typography>
                </Box>
              )}
            </Box>
          </Box>
        )}
      </Box>

      {/* ZFS Snapshot/Clone Tree — below the Docker Cache flex row */}
      {zfsTree?.available && zfsTree?.golden && zfsTree.golden.children?.some(
        (snap: TypesZFSTreeNode) => snap.children && snap.children.length > 0
      ) && (
        <Box sx={{ mt: 1, mb: 4, p: 2, bgcolor: "background.paper", borderRadius: 1, border: "1px solid", borderColor: "divider" }}>
          <Typography variant="subtitle2" sx={{ mb: 1, display: "flex", alignItems: "center", gap: 0.5 }}>
            <HubIcon fontSize="small" sx={{ color: "primary.main" }} />
            ZFS Clone Tree
          </Typography>
          <Box sx={{ fontFamily: "monospace", fontSize: "0.75rem", lineHeight: 1.8 }}>
            {/* Golden zvol */}
            <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
              <Box sx={{ color: "warning.main", fontWeight: "bold" }}>⬢</Box>
              <Box sx={{ color: "text.primary", fontWeight: "bold" }}>
                {zfsTree.golden.name?.split("/").pop()}
              </Box>
              <Chip label={zfsTree.golden.refer} size="small" sx={{ height: 18, fontSize: "0.65rem", fontFamily: "monospace" }} />
            </Box>
            {/* Snapshots */}
            {zfsTree.golden.children?.map((snap: TypesZFSTreeNode, si: number) => (
              <Box key={snap.name} sx={{ ml: 2 }}>
                <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                  <Box sx={{ color: "text.secondary" }}>{si === (zfsTree.golden.children?.length ?? 0) - 1 ? "└─" : "├─"}</Box>
                  <Box sx={{ color: "info.main" }}>📸</Box>
                  <Box sx={{ color: "info.main", fontWeight: si === (zfsTree.golden.children?.length ?? 0) - 1 ? "bold" : "normal" }}>
                    @{snap.name?.split("@")[1]}
                  </Box>
                  <Chip
                    label={`Δ ${snap.used}`}
                    size="small"
                    sx={{ height: 18, fontSize: "0.65rem", fontFamily: "monospace", bgcolor: si === (zfsTree.golden.children?.length ?? 0) - 1 ? "success.main" : "action.hover", color: si === (zfsTree.golden.children?.length ?? 0) - 1 ? "success.contrastText" : "text.secondary" }}
                  />
                  {si === (zfsTree.golden.children?.length ?? 0) - 1 && (
                    <Chip label="latest" size="small" color="success" variant="outlined" sx={{ height: 18, fontSize: "0.6rem" }} />
                  )}
                </Box>
                {/* Clones */}
                {snap.children?.map((clone: TypesZFSTreeNode, ci: number) => (
                  <Box key={clone.name} sx={{ ml: 3, display: "flex", alignItems: "center", gap: 0.5 }}>
                    <Box sx={{ color: "text.secondary" }}>{ci === (snap.children?.length ?? 0) - 1 ? "└─" : "├─"}</Box>
                    <Box sx={{ color: clone.mounted ? "success.main" : "text.disabled" }}>
                      {clone.mounted ? "🟢" : "⚪"}
                    </Box>
                    <Box sx={{ color: clone.mounted ? "text.primary" : "text.disabled", fontSize: "0.7rem" }}>
                      {clone.session_id ? `ses_${clone.session_id.substring(4, 12)}…` : clone.name?.split("/").pop()}
                    </Box>
                    <Chip
                      label={clone.used}
                      size="small"
                      sx={{ height: 16, fontSize: "0.6rem", fontFamily: "monospace" }}
                    />
                    {clone.mounted && (
                      <Chip label="active" size="small" color="success" variant="outlined" sx={{ height: 16, fontSize: "0.55rem" }} />
                    )}
                  </Box>
                ))}
              </Box>
            ))}
            {/* Orphans */}
            {zfsTree.orphans && zfsTree.orphans.length > 0 && (
              <Box sx={{ mt: 1, opacity: 0.6 }}>
                <Typography variant="caption" color="text.secondary">orphaned zvols (no golden parent):</Typography>
                {zfsTree.orphans.map((o: TypesZFSTreeNode) => (
                  <Box key={o.name} sx={{ ml: 2, display: "flex", alignItems: "center", gap: 0.5 }}>
                    <Box>{o.mounted ? "🟢" : "⚪"}</Box>
                    <Box sx={{ fontSize: "0.7rem" }}>{o.session_id ? `ses_${o.session_id.substring(4, 12)}…` : o.name?.split("/").pop()}</Box>
                    <Chip label={o.used} size="small" sx={{ height: 16, fontSize: "0.6rem", fontFamily: "monospace" }} />
                  </Box>
                ))}
              </Box>
            )}
          </Box>
        </Box>
      )}
    </Box>
  );

  const renderAgentsTab = () => (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
      <Box>
        <Typography variant="h6" gutterBottom>
          Agent Configuration
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Set agents for this project. Agents are used for working on spec
          tasks, managing the project and reviewing pull requests.
        </Typography>
        <Divider sx={{ mb: 3 }} />

        {!showCreateAgentForm ? (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
            {/* Default Agent with edit button */}
            <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1 }}>
              <Box sx={{ flex: 1 }}>
                <AgentDropdown
                  value={selectedAgentId}
                  onChange={(newAgentId) => {
                    setSelectedAgentId(newAgentId);
                    updateProjectMutation.mutate({
                      default_helix_app_id: newAgentId || undefined,
                    });
                  }}
                  agents={sortedApps}
                  label="Default Agent"
                />
              </Box>
              <IconButton
                  size="small"
                  disabled={!selectedAgentId}
                  onClick={() => {
                    const orgName = currentOrg?.name;
                    if (orgName && selectedAgentId) {
                      window.open(`/orgs/${orgName}/app/${selectedAgentId}`, '_blank');
                    }
                  }}
                  sx={{ mt: 0.5 }}
                  title="Edit agent"
                >
                  <EditIcon fontSize="small" />
                </IconButton>
            </Box>

            {/* Project Manager Agent with edit button */}
            <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1 }}>
              <Box sx={{ flex: 1 }}>
                <AgentDropdown
                  value={selectedProjectManagerAgentId}
                  onChange={(newAgentId) => {
                    setSelectedProjectManagerAgentId(newAgentId);
                    updateProjectMutation.mutate({
                      project_manager_helix_app_id: newAgentId || undefined,
                    });
                  }}
                  agents={sortedApps}
                  label="Project Manager Agent"
                />
              </Box>
              <IconButton
                  size="small"
                  disabled={!selectedProjectManagerAgentId}
                  onClick={() => {
                    const orgName = currentOrg?.name;
                    if (orgName && selectedProjectManagerAgentId) {
                      window.open(`/orgs/${orgName}/app/${selectedProjectManagerAgentId}`, '_blank');
                    }
                  }}
                  sx={{ mt: 0.5 }}
                  title="Edit agent"
                >
                  <EditIcon fontSize="small" />
                </IconButton>
            </Box>

            {/* PR Reviewer Agent with edit button */}
            <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1 }}>
              <Box sx={{ flex: 1 }}>
                <AgentDropdown
                  value={selectedPullRequestReviewerAgentId}
                  onChange={(newAgentId) => {
                    setSelectedPullRequestReviewerAgentId(newAgentId);
                    updateProjectMutation.mutate({
                      pull_request_reviewer_helix_app_id:
                        newAgentId || undefined,
                    });
                  }}
                  agents={sortedApps}
                  label="Pull Request Reviewer Agent"
                  disabled={!primaryRepoIsExternal}
                  helperText={
                    !primaryRepoIsExternal
                      ? "Requires an external repository (GitHub, GitLab, etc.) as the primary repository"
                      : undefined
                  }
                />
              </Box>
              <IconButton
                  size="small"
                  disabled={!selectedPullRequestReviewerAgentId}
                  onClick={() => {
                    const orgName = currentOrg?.name;
                    if (orgName && selectedPullRequestReviewerAgentId) {
                      window.open(`/orgs/${orgName}/app/${selectedPullRequestReviewerAgentId}`, '_blank');
                    }
                  }}
                  sx={{ mt: 0.5 }}
                  title="Edit agent"
                >
                  <EditIcon fontSize="small" />
                </IconButton>
            </Box>

            <Button
              size="small"
              startIcon={<AddIcon />}
              onClick={() => setShowCreateAgentForm(true)}
              sx={{ alignSelf: "flex-start" }}
            >
              Create new agent
            </Button>
          </Box>
        ) : (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
            <Typography variant="subtitle2">Create New Agent</Typography>
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
              disabled={creatingAgent}
              recommendedModels={RECOMMENDED_CODING_MODELS}
              createAgentDescription="Code development agent for spec tasks"
              onCreateStateChange={setCreatingAgent}
              onAgentCreated={(app) => setSelectedAgentId(app.id)}
              showCreateButton={false}
              modelPickerHint="Choose a capable model for agentic coding."
              modelPickerDisplayMode="short"
            />

            <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}>
              {sortedApps.length > 0 && (
                <Button
                  size="small"
                  variant="outlined"
                  onClick={() => setShowCreateAgentForm(false)}
                  disabled={creatingAgent}
                >
                  Cancel
                </Button>
              )}
              <Button
                size="small"
                variant="outlined"
                color="secondary"
                onClick={handleCreateAgent}
                disabled={
                  creatingAgent ||
                  !newAgentName.trim() ||
                  (!(
                    codeAgentRuntime === "claude_code" &&
                    claudeCodeMode === "subscription"
                  ) &&
                    (!selectedModel || !selectedProvider))
                }
                startIcon={
                  creatingAgent ? (
                    <CircularProgress size={16} />
                  ) : undefined
                }
              >
                {creatingAgent ? "Creating..." : "Create Agent"}
              </Button>
            </Box>
          </Box>
        )}
      </Box>
    </Box>
  );

  const renderBoardTab = () => (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
      {/* Kanban Board Settings */}
      <Box>
        <Typography variant="h6" gutterBottom>
          Kanban Board Settings
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Configure work-in-progress (WIP) limits for the Kanban board
          columns.
        </Typography>
        <Divider sx={{ mb: 3 }} />
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "1fr", sm: "repeat(3, 1fr)" },
            gap: 2,
          }}
        >
          <TextField
            label="Planning Limit"
            value={wipLimits.planning}
            onChange={(e) =>
              setWipLimits({
                ...wipLimits,
                planning: parseInt(e.target.value) || 0,
              })
            }
            onBlur={handleFieldBlur}
            helperText="Max tasks in Planning"
            size="small"
          />
          <TextField
            label="Review Limit"
            value={wipLimits.review}
            onChange={(e) =>
              setWipLimits({
                ...wipLimits,
                review: parseInt(e.target.value) || 0,
              })
            }
            onBlur={handleFieldBlur}
            helperText="Max tasks in Review"
            size="small"
          />
          <TextField
            label="Implementation Limit"
            value={wipLimits.implementation}
            onChange={(e) =>
              setWipLimits({
                ...wipLimits,
                implementation: parseInt(e.target.value) || 0,
              })
            }
            onBlur={handleFieldBlur}
            helperText="Max tasks in Implementation"
            size="small"
          />
        </Box>
      </Box>

      {/* Automations */}
      <Box>
        <Typography variant="h6" gutterBottom>
          Automations
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Configure automatic task scheduling and workflow automation.
        </Typography>
        <Divider sx={{ mb: 3 }} />
        <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
            }}
          >
            <Box sx={{ flex: 1, mr: 2 }}>
              <Typography variant="body2" sx={{ fontWeight: 600 }}>
                Auto-start backlog tasks
              </Typography>
              <Typography variant="caption" color="text.secondary">
                Automatically move tasks from backlog to planning when the
                WIP limit allows.
              </Typography>
            </Box>
            <Switch
              checked={autoStartBacklogTasks}
              onChange={(e) => {
                const newValue = e.target.checked;
                setAutoStartBacklogTasks(newValue);
                updateProjectMutation.mutate({
                  auto_start_backlog_tasks: newValue,
                });
              }}
            />
          </Box>
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
            }}
          >
            <Box sx={{ flex: 1, mr: 2 }}>
              <Typography variant="body2" sx={{ fontWeight: 600 }}>
                Pull request reviews
              </Typography>
              <Typography variant="caption" color="text.secondary">
                Automatically review pull requests using the configured
                reviewer agent.
              </Typography>
            </Box>
            <Switch
              checked={pullRequestReviewsEnabled}
              onChange={(e) => {
                const newValue = e.target.checked;
                setPullRequestReviewsEnabled(newValue);
                updateProjectMutation.mutate({
                  pull_request_reviews_enabled: newValue,
                });
              }}
              disabled={!primaryRepoIsExternal}
            />
          </Box>
          {!primaryRepoIsExternal && (
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ mt: -1 }}
            >
              Pull request reviews require an external repository (GitHub,
              GitLab, etc.) as the primary repository.
            </Typography>
          )}
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
            }}
          >
            <Box sx={{ flex: 1, mr: 2 }}>
              <Typography variant="body2" sx={{ fontWeight: 600 }}>
                Code intelligence
              </Typography>
              <Typography variant="caption" color="text.secondary">
                {koditEnabled
                  ? "Code intelligence is enabled for this project"
                  : "Allow agents to use code intelligence for all of your organization's repositories"}
              </Typography>
            </Box>
            <Switch
              checked={koditEnabled}
              onChange={(e) => {
                const newValue = e.target.checked;
                setKoditEnabled(newValue);
                updateProjectMutation.mutate({
                  kodit_enabled: newValue,
                });
              }}
            />
          </Box>
        </Box>
      </Box>
    </Box>
  );

  const renderSecretsTab = () => (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
      <Box>
        <Box
          sx={{
            display: "flex",
            flexDirection: { xs: "column", sm: "row" },
            alignItems: { xs: "flex-start", sm: "center" },
            justifyContent: "space-between",
            gap: { xs: 1, sm: 0 },
            mb: 1,
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center" }}>
            <VpnKeyIcon sx={{ mr: 1 }} />
            <Typography variant="h6">Secrets</Typography>
          </Box>
          <Button
            size="small"
            variant="outlined"
            startIcon={<AddIcon />}
            onClick={() => setAddSecretDialogOpen(true)}
          >
            Add
          </Button>
        </Box>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Secrets are encrypted and injected as environment variables when
          agents work on this project.
        </Typography>
        <Divider sx={{ mb: 2 }} />
        {projectSecrets.length === 0 ? (
          <Typography
            variant="body2"
            color="text.secondary"
            sx={{ textAlign: "center", py: 4 }}
          >
            No secrets configured. Add secrets like API keys that agents
            need access to.
          </Typography>
        ) : (
          <List dense>
            {projectSecrets.map((secret) => (
              <ListItem
                key={secret.id}
                sx={{ borderBottom: "1px solid", borderColor: "divider" }}
                secondaryAction={
                  <Button
                    size="small"
                    color="error"
                    onClick={() =>
                      secret.id && deleteSecretMutation.mutate(secret.id)
                    }
                    disabled={deleteSecretMutation.isPending}
                    startIcon={<DeleteIcon />}
                  >
                    Delete
                  </Button>
                }
              >
                <Box
                  sx={{ display: "flex", alignItems: "center", gap: 1 }}
                >
                  <VpnKeyIcon fontSize="small" color="action" />
                  <Typography
                    variant="body2"
                    sx={{ fontFamily: "monospace", fontWeight: 600 }}
                  >
                    {secret.name}
                  </Typography>
                  <Chip
                    label="encrypted"
                    size="small"
                    sx={{ ml: 1, fontSize: "0.7rem" }}
                  />
                </Box>
              </ListItem>
            ))}
          </List>
        )}
      </Box>
    </Box>
  );

  const renderSkillsTab = () => (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
      <Box>
        <Box sx={{ display: "flex", alignItems: "center", mb: 2 }}>
          <HubIcon sx={{ mr: 1, color: "#10B981" }} />
          <Typography variant="h6">Skills</Typography>
        </Box>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Configure skills for this project. These overlay on top of agent-level skills.
        </Typography>
        <Divider sx={{ mb: 2 }} />
        <Skills
          app={skillsFlatState}
          onUpdate={handleSkillsUpdate}
          hideHeader
          defaultCategory="Core"
          compactGrid
        />
      </Box>
    </Box>
  );

  const renderDangerTab = () => (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
      <Box
        sx={{
          border: "2px solid",
          borderColor: "error.main",
          borderRadius: 1,
          p: 2,
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", mb: 1 }}>
          <WarningIcon sx={{ mr: 1, color: "error.main" }} />
          <Typography variant="h6" color="error">
            Danger Zone
          </Typography>
        </Box>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Irreversible and destructive actions.
        </Typography>
        <Divider sx={{ mb: 3 }} />

        {/* Move to Organization */}
        {!project?.organization_id &&
          account.organizationTools.organizations.length > 0 && (
            <Box
              sx={{
                p: 2,
                backgroundColor: "rgba(211, 47, 47, 0.05)",
                borderRadius: 1,
                border: "1px solid",
                borderColor: "error.light",
                mb: 2,
              }}
            >
              <Typography
                variant="subtitle1"
                sx={{ fontWeight: 600, mb: 1 }}
              >
                Move to Organization
              </Typography>
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{ mb: 2 }}
              >
                Transfer this project to an organization to enable team
                sharing and RBAC roles. This is a one-way operation and
                cannot be undone.
              </Typography>
              <Button
                variant="outlined"
                color="error"
                startIcon={<MoveUpIcon />}
                onClick={() => setMoveDialogOpen(true)}
              >
                Move to Organization
              </Button>
            </Box>
          )}

        <Box
          sx={{
            p: 2,
            backgroundColor: "rgba(211, 47, 47, 0.05)",
            borderRadius: 1,
            border: "1px solid",
            borderColor: "error.light",
          }}
        >
          <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 1 }}>
            Delete Project
          </Typography>
          <Typography
            variant="body2"
            color="text.secondary"
            sx={{ mb: 2 }}
          >
            Once you delete a project, there is no going back. This will
            permanently delete the project, all its tasks, and associated
            data.
          </Typography>
          <Button
            variant="outlined"
            color="error"
            startIcon={<DeleteForeverIcon />}
            onClick={() => setDeleteDialogOpen(true)}
          >
            Delete This Project
          </Button>
        </Box>
      </Box>
    </Box>
  );

  // ─── Main render ────────────────────────────────────────────────────

  return (
    <>
      <Container maxWidth="lg" sx={{ px: 2 }}>
        {tab ==="general" && renderGeneralTab()}
        {tab ==="sandbox" && renderSandboxTab()}
        {tab ==="agents" && renderAgentsTab()}
        {tab ==="board" && renderBoardTab()}
        {tab ==="secrets" && renderSecretsTab()}
        {tab ==="skills" && renderSkillsTab()}
        {tab ==="danger" && renderDangerTab()}
      </Container>

      {/* ─── Dialogs ──────────────────────────────────────────────────── */}

      {/* Attach Repository Dialog */}
      <Dialog
        open={attachRepoDialogOpen}
        onClose={() => {
          setAttachRepoDialogOpen(false);
          setSelectedRepoToAttach("");
        }}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <LinkIcon />
            Attach Repository to Project
          </Box>
        </DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
            Select a repository from your account to attach to this project.
            Attached repositories will be cloned into the agent workspace when
            working on this project.
          </Typography>
          <FormControl fullWidth>
            <InputLabel>Select Repository</InputLabel>
            <Select
              value={selectedRepoToAttach}
              onChange={(e) => setSelectedRepoToAttach(e.target.value)}
              label="Select Repository"
            >
              {allUserRepositories
                .filter((repo) => !repositories.some((pr) => pr.id === repo.id))
                .map((repo) => (
                  <MenuItem key={repo.id} value={repo.id}>
                    {repo.name}
                  </MenuItem>
                ))}
            </Select>
            {allUserRepositories.filter(
              (repo) => !repositories.some((pr) => pr.id === repo.id),
            ).length === 0 && (
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ mt: 1 }}
              >
                All your repositories are already attached to this project.
              </Typography>
            )}
          </FormControl>
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => {
              setAttachRepoDialogOpen(false);
              setSelectedRepoToAttach("");
            }}
          >
            Cancel
          </Button>
          <Button
            onClick={handleAttachRepository}
            variant="contained"
            disabled={!selectedRepoToAttach || attachRepoMutation.isPending}
            startIcon={
              attachRepoMutation.isPending ? (
                <CircularProgress size={16} />
              ) : (
                <LinkIcon />
              )
            }
          >
            {attachRepoMutation.isPending
              ? "Attaching..."
              : "Attach Repository"}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <Dialog
        open={deleteDialogOpen}
        onClose={() => {
          setDeleteDialogOpen(false);
          setDeleteConfirmName("");
        }}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <WarningIcon color="error" />
            <span>Delete Project</span>
          </Box>
        </DialogTitle>
        <DialogContent>
          <Alert severity="error" sx={{ mb: 3 }}>
            <Typography variant="body2" sx={{ fontWeight: 600, mb: 1 }}>
              This action cannot be undone!
            </Typography>
            <Typography variant="body2">
              This will permanently delete the project{" "}
              <strong>{project?.name}</strong>, all its tasks, work sessions,
              and associated data.
            </Typography>
          </Alert>

          <Typography variant="body2" sx={{ mb: 2 }}>
            Please type the project name <strong>{project?.name}</strong> to
            confirm:
          </Typography>

          <TextField
            fullWidth
            value={deleteConfirmName}
            onChange={(e) => setDeleteConfirmName(e.target.value)}
            placeholder={project?.name}
            autoFocus
          />
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => {
              setDeleteDialogOpen(false);
              setDeleteConfirmName("");
            }}
          >
            Cancel
          </Button>
          <Button
            onClick={handleDeleteProject}
            variant="contained"
            color="error"
            disabled={
              deleteConfirmName !== project?.name ||
              deleteProjectMutation.isPending
            }
            startIcon={
              deleteProjectMutation.isPending ? (
                <CircularProgress size={16} />
              ) : (
                <DeleteForeverIcon />
              )
            }
          >
            {deleteProjectMutation.isPending ? "Deleting..." : "Delete Project"}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Add Secret Dialog */}
      <Dialog
        open={addSecretDialogOpen}
        onClose={() => {
          setAddSecretDialogOpen(false);
          setNewSecretName("");
          setNewSecretValue("");
        }}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <VpnKeyIcon />
            Add Project Secret
          </Box>
        </DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
            Secrets are encrypted at rest and injected as environment variables
            when agents work on this project.
          </Typography>
          <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
            <TextField
              label="Secret Name"
              fullWidth
              value={newSecretName}
              onChange={(e) =>
                setNewSecretName(
                  e.target.value.toUpperCase().replace(/[^A-Z0-9_]/g, "_"),
                )
              }
              placeholder="ANTHROPIC_API_KEY"
              helperText="Will be available as an environment variable with this name"
            />
            <TextField
              label="Secret Value"
              fullWidth
              value={newSecretValue}
              onChange={(e) => setNewSecretValue(e.target.value)}
              type={showSecretValue ? "text" : "password"}
              placeholder="sk-ant-api03-..."
              InputProps={{
                endAdornment: (
                  <Button
                    size="small"
                    onClick={() => setShowSecretValue(!showSecretValue)}
                    sx={{ minWidth: "auto" }}
                  >
                    {showSecretValue ? (
                      <VisibilityOffIcon />
                    ) : (
                      <VisibilityIcon />
                    )}
                  </Button>
                ),
              }}
            />
          </Box>
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => {
              setAddSecretDialogOpen(false);
              setNewSecretName("");
              setNewSecretValue("");
            }}
          >
            Cancel
          </Button>
          <Button
            onClick={() =>
              createSecretMutation.mutate({
                name: newSecretName,
                value: newSecretValue,
              })
            }
            variant="contained"
            disabled={
              !newSecretName.trim() ||
              !newSecretValue.trim() ||
              createSecretMutation.isPending
            }
            startIcon={
              createSecretMutation.isPending ? (
                <CircularProgress size={16} />
              ) : (
                <AddIcon />
              )
            }
          >
            {createSecretMutation.isPending ? "Adding..." : "Add Secret"}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Guidelines History Dialog */}
      <Dialog
        open={guidelinesHistoryDialogOpen}
        onClose={() => setGuidelinesHistoryDialogOpen(false)}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <HistoryIcon />
            Guidelines Version History
          </Box>
        </DialogTitle>
        <DialogContent>
          {guidelinesHistory.length === 0 ? (
            <Typography
              variant="body2"
              color="text.secondary"
              sx={{ py: 4, textAlign: "center" }}
            >
              No previous versions found. History is created when guidelines are
              modified.
            </Typography>
          ) : (
            <List>
              {guidelinesHistory.map((entry, index) => (
                <ListItem
                  key={entry.id}
                  sx={{
                    flexDirection: "column",
                    alignItems: "flex-start",
                    borderBottom:
                      index < guidelinesHistory.length - 1
                        ? "1px solid"
                        : "none",
                    borderColor: "divider",
                    py: 2,
                  }}
                >
                  <Box
                    sx={{
                      display: "flex",
                      alignItems: "center",
                      gap: 2,
                      mb: 1,
                      width: "100%",
                    }}
                  >
                    <Chip
                      label={`v${entry.version}`}
                      size="small"
                      color="primary"
                      variant="outlined"
                    />
                    <Typography variant="body2" color="text.secondary">
                      {entry.updated_at
                        ? new Date(entry.updated_at).toLocaleString()
                        : "Unknown date"}
                    </Typography>
                    {(entry.updated_by_name || entry.updated_by_email) && (
                      <Typography variant="caption" color="text.secondary">
                        by {entry.updated_by_name || "Unknown"}
                        {entry.updated_by_email
                          ? ` (${entry.updated_by_email})`
                          : ""}
                      </Typography>
                    )}
                  </Box>
                  <Typography
                    variant="body2"
                    sx={{
                      whiteSpace: "pre-wrap",
                      fontFamily: "monospace",
                      fontSize: "0.85rem",
                      backgroundColor: "action.hover",
                      p: 1.5,
                      borderRadius: 1,
                      width: "100%",
                      maxHeight: 200,
                      overflow: "auto",
                    }}
                  >
                    {entry.guidelines || "(empty)"}
                  </Typography>
                </ListItem>
              ))}
            </List>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setGuidelinesHistoryDialogOpen(false)}>
            Close
          </Button>
        </DialogActions>
      </Dialog>

      {/* Move to Organization Dialog */}
      <Dialog
        open={moveDialogOpen}
        onClose={() => {
          setMoveDialogOpen(false);
          setSelectedOrgToMove("");
          setMovePreview(null);
        }}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <MoveUpIcon color="error" />
            <span>Move Project to Organization</span>
          </Box>
        </DialogTitle>
        <DialogContent>
          <Alert severity="warning" sx={{ mb: 3 }}>
            <Typography variant="body2" sx={{ fontWeight: 600 }}>
              This is a one-way operation!
            </Typography>
            <Typography variant="body2">
              Once moved to an organization, this project cannot be moved back
              to your personal workspace.
            </Typography>
          </Alert>

          <FormControl fullWidth sx={{ mb: 3 }}>
            <InputLabel>Select Organization</InputLabel>
            <Select
              value={selectedOrgToMove}
              label="Select Organization"
              onChange={(e) => handleOrgSelectForMove(e.target.value)}
            >
              {account.organizationTools.organizations.map((org) => (
                <MenuItem key={org.id} value={org.id}>
                  {org.display_name || org.name}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          {loadingMovePreview && (
            <Box sx={{ display: "flex", justifyContent: "center", py: 2 }}>
              <CircularProgress size={24} />
            </Box>
          )}

          {movePreview && !loadingMovePreview && (
            <Box>
              <Typography variant="subtitle2" sx={{ mb: 1 }}>
                The following will be moved:
              </Typography>

              <Box
                sx={{
                  mb: 2,
                  p: 1.5,
                  backgroundColor: "action.hover",
                  borderRadius: 1,
                }}
              >
                <Typography variant="body2" sx={{ fontWeight: 600 }}>
                  Project
                </Typography>
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 1,
                    mt: 0.5,
                  }}
                >
                  <Typography variant="body2">
                    {movePreview.project.current_name}
                  </Typography>
                  {movePreview.project.has_conflict &&
                    movePreview.project.new_name && (
                      <>
                        <ArrowForwardIcon fontSize="small" color="warning" />
                        <Typography variant="body2" color="warning.main">
                          {movePreview.project.new_name}
                        </Typography>
                        <Chip
                          label="renamed"
                          size="small"
                          color="warning"
                          sx={{ fontSize: "0.7rem" }}
                        />
                      </>
                    )}
                </Box>
              </Box>

              {movePreview.repositories &&
                movePreview.repositories.length > 0 && (
                  <Box
                    sx={{
                      p: 1.5,
                      backgroundColor: "action.hover",
                      borderRadius: 1,
                    }}
                  >
                    <Typography variant="body2" sx={{ fontWeight: 600, mb: 1 }}>
                      Repositories ({movePreview.repositories.length})
                    </Typography>
                    {movePreview.repositories.map((repo) => (
                      <Box key={repo.id} sx={{ py: 0.5 }}>
                        <Box
                          sx={{
                            display: "flex",
                            alignItems: "center",
                            gap: 1,
                          }}
                        >
                          <Typography variant="body2">
                            {repo.current_name}
                          </Typography>
                          {repo.has_conflict && repo.new_name && (
                            <>
                              <ArrowForwardIcon
                                fontSize="small"
                                color="warning"
                              />
                              <Typography variant="body2" color="warning.main">
                                {repo.new_name}
                              </Typography>
                              <Chip
                                label="renamed"
                                size="small"
                                color="warning"
                                sx={{ fontSize: "0.7rem" }}
                              />
                            </>
                          )}
                        </Box>
                        {repo.affected_projects && repo.affected_projects.length > 0 && (
                          <Box
                            sx={{
                              ml: 2,
                              mt: 0.5,
                              p: 1,
                              backgroundColor: "error.dark",
                              borderRadius: 1,
                              border: "1px solid",
                              borderColor: "error.main",
                            }}
                          >
                            <Typography variant="caption" color="error.contrastText" sx={{ fontWeight: 600 }}>
                              Warning: This repository is shared with other projects that will lose access:
                            </Typography>
                            <Box component="ul" sx={{ m: 0, pl: 2, mt: 0.5 }}>
                              {repo.affected_projects.map((proj) => (
                                <Typography
                                  key={proj.id}
                                  component="li"
                                  variant="caption"
                                  color="error.contrastText"
                                >
                                  {proj.name}
                                </Typography>
                              ))}
                            </Box>
                          </Box>
                        )}
                      </Box>
                    ))}
                  </Box>
                )}

              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ display: "block", mt: 2 }}
              >
                These repositories will become accessible to all members of the
                organization based on their roles.
              </Typography>

              {movePreview.warnings && movePreview.warnings.length > 0 && (
                <Box sx={{ mt: 2 }}>
                  {movePreview.warnings.map((warning, index) => (
                    <Alert
                      key={index}
                      severity="info"
                      icon={<WarningIcon />}
                      sx={{ mb: 1 }}
                    >
                      {warning}
                    </Alert>
                  ))}
                </Box>
              )}

              {movePreview.repositories.some(
                (r) => r.affected_projects && r.affected_projects.length > 0
              ) && (
                <FormControlLabel
                  sx={{ mt: 2 }}
                  control={
                    <Checkbox
                      checked={acceptSharedRepoWarning}
                      onChange={(e) =>
                        setAcceptSharedRepoWarning(e.target.checked)
                      }
                      color="error"
                    />
                  }
                  label={
                    <Typography variant="body2" color="error">
                      I understand that moving shared repositories will break
                      the other projects listed above
                    </Typography>
                  }
                />
              )}
            </Box>
          )}
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => {
              setMoveDialogOpen(false);
              setSelectedOrgToMove("");
              setMovePreview(null);
              setAcceptSharedRepoWarning(false);
            }}
          >
            Cancel
          </Button>
          <Button
            onClick={() => moveProjectMutation.mutate(selectedOrgToMove)}
            variant="contained"
            color="error"
            disabled={
              !selectedOrgToMove ||
              !movePreview ||
              moveProjectMutation.isPending ||
              (movePreview?.repositories.some(
                (r) => r.affected_projects && r.affected_projects.length > 0
              ) &&
                !acceptSharedRepoWarning)
            }
            startIcon={
              moveProjectMutation.isPending ? (
                <CircularProgress size={16} />
              ) : (
                <MoveUpIcon />
              )
            }
          >
            {moveProjectMutation.isPending ? "Moving..." : "Move Project"}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Saving toast */}
      <SavingToast isSaving={savingProject} />
    </>
  );
};

export default ProjectSettings;
