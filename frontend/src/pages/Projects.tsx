import React, { FC, useState } from "react";
import {
  Container,
  Box,
  Menu,
  MenuItem,
  CircularProgress,
} from "@mui/material";
import SettingsIcon from "@mui/icons-material/Settings";
import { useQueryClient } from "@tanstack/react-query";

import Page from "../components/system/Page";
import CreateProjectDialog from "../components/project/CreateProjectDialog";
import CreateRepositoryDialog from "../components/project/CreateRepositoryDialog";
import LinkExternalRepositoryDialog from "../components/project/LinkExternalRepositoryDialog";
import BrowseProvidersDialog from "../components/project/BrowseProvidersDialog";
import AgentSelectionModal from "../components/project/AgentSelectionModal";
import SampleProjectWizard from "../components/project/SampleProjectWizard";
import ProjectsListView from "../components/project/ProjectsListView";
import RepositoriesListView from "../components/project/RepositoriesListView";
import GuidelinesView from "../components/project/GuidelinesView";
import useAccount from "../hooks/useAccount";
import useRouter from "../hooks/useRouter";
import useSnackbar from "../hooks/useSnackbar";
import { useSettingsDialog } from "../contexts/settingsDialog";
import useApi from "../hooks/useApi";
import useSubscriptionGate from "../hooks/useSubscriptionGate";
import Paywall from "../components/subscription/Paywall";
import HelixOrgTopNav from "../components/helix-org/HelixOrgTopNav";
import { TypesCodeAgentExecutionConfig, TypesGitRepositoryType } from "../api/api";
import {
  useListProjects,
  useListSampleProjects,
  useInstantiateSampleProject,
  TypesProject,
  usePinnedProjectIds,
  usePinProject,
  useUnpinProject,
} from "../services";
import { useGitRepositories } from "../services/gitRepositoryService";
import { matchesAllTokens } from "../utils/searchUtils";
import { useHelixOrgSettings } from "../services/helixOrgService";
import { parseOrgDefaultRuntime } from "./newChatLogic";
import type {
  TypesExternalRepositoryType,
  TypesGitRepository,
  TypesAzureDevOps,
  TypesRepositoryInfo,
} from "../api/api";

export function parseCreateProjectConfig(value?: string): TypesCodeAgentExecutionConfig | undefined {
  if (!value) return undefined;
  try {
    const config = JSON.parse(value) as TypesCodeAgentExecutionConfig;
    return config.runtime && config.credential_type && config.model ? config : undefined;
  } catch {
    return undefined;
  }
}

const Projects: FC = () => {
  const account = useAccount();
  const router = useRouter();
  const snackbar = useSnackbar();
  const queryClient = useQueryClient();
  const api = useApi();
  const { paywallActive, navigateToBilling } = useSubscriptionGate();
  const { openDialog } = useSettingsDialog();
  const { data: orgSettings } = useHelixOrgSettings();
  const orgDefaultValue = orgSettings?.specs?.find((spec) => spec.key === "agent.default")?.value;
  const orgDefaultConfig = React.useMemo(
    () => parseOrgDefaultRuntime(orgDefaultValue),
    [orgDefaultValue],
  );

  const isLoggedIn = !!account.user;

  // Single helper to check login and show dialog if needed
  const requireLogin = React.useCallback((): boolean => {
    if (!account.user) {
      account.setShowLoginWindow(true);
      return false;
    }
    return true;
  }, [account]);

  // Show login dialog on mount if not logged in (only after account is initialized)
  React.useEffect(() => {
    if (account.initialized && !isLoggedIn) {
      account.setShowLoginWindow(true);
    }
  }, [account.initialized, isLoggedIn]);

  const isOrgResolved =
    !account.organizationTools.orgID ||
    !!account.organizationTools.organization;
  const shouldLoadProjects =
    isLoggedIn && !account.organizationTools.loading && isOrgResolved;
  const {
    data: projects = [],
    isLoading,
    error,
  } = useListProjects(account.organizationTools.organization?.id || "", {
    enabled: shouldLoadProjects,
  });
  const isProjectsLoading =
    isLoading ||
    (isLoggedIn && (account.organizationTools.loading || !isOrgResolved));
  const { data: sampleProjects = [] } = useListSampleProjects({
    enabled: isLoggedIn,
  });
  const instantiateSampleMutation = useInstantiateSampleProject();

  // Pinned projects
  const { data: pinnedProjectIds = [] } = usePinnedProjectIds(isLoggedIn);
  const pinProjectMutation = usePinProject();
  const unpinProjectMutation = useUnpinProject();

  const handlePinProject = React.useCallback((projectId: string) => {
    pinProjectMutation.mutate(projectId);
  }, [pinProjectMutation]);

  const handleUnpinProject = React.useCallback((projectId: string) => {
    unpinProjectMutation.mutate(projectId);
  }, [unpinProjectMutation]);

  // Get tab from URL query parameter
  const { tab } = router.params;
  const currentView = tab === "repositories" || tab === "guidelines" ? tab : "projects";
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const [selectedProject, setSelectedProject] = useState<TypesProject | null>(
    null,
  );
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [initialCreateConfig, setInitialCreateConfig] = useState<
    TypesCodeAgentExecutionConfig | undefined
  >();
  const [createRepositoryDialogOpen, setCreateRepositoryDialogOpen] =
    useState(false);
  const [linkRepositoryDialogOpen, setLinkRepositoryDialogOpen] =
    useState(false);

  React.useEffect(() => {
    if (!router.params.create_project_config) return;
    const config = parseCreateProjectConfig(router.params.create_project_config);
    router.removeParams(['create_project_config']);
    if (config) {
      setInitialCreateConfig(config);
      setCreateDialogOpen(true);
    }
  }, [router.params.create_project_config]);
  const [browseProvidersOpen, setBrowseProvidersOpen] = useState(false);
  const [repositoryCreating, setRepositoryCreating] = useState(false);
  const [repositoryCreateError, setRepositoryCreateError] = useState("");
  const [linkingFromBrowser, setLinkingFromBrowser] = useState(false);

  // Repository management
  const currentOrg = account.organizationTools.organization;
  // List repos by organization_id when in org context, or by owner_id for personal workspace
  const { data: repositories = [], isLoading: reposLoading } =
    useGitRepositories(
      currentOrg?.id
        ? { organizationId: currentOrg.id, enabled: isLoggedIn }
        : { ownerId: account.user?.id, enabled: isLoggedIn },
    );

  // Agent selection modal state for sample project fork
  const [agentModalOpen, setAgentModalOpen] = useState(false);
  const [pendingSampleFork, setPendingSampleFork] = useState<{
    sampleId: string;
    sampleName: string;
    sampleProject?: any;
  } | null>(null);

  // GitHub auth wizard for sample projects that require it (e.g., helix-in-helix)
  const [sampleWizardOpen, setSampleWizardOpen] = useState(false);
  const [sampleWizardProject, setSampleWizardProject] = useState<any>(null);
  const [selectedCodeAgentConfigForWizard, setSelectedCodeAgentConfigForWizard] = useState<
    TypesCodeAgentExecutionConfig | undefined
  >(undefined);

  // Pagination for projects
  const [projectsPage, setProjectsPage] = useState(0);
  const projectsPerPage = 24;

  // Search and pagination for repositories
  const [reposSearchQuery, setReposSearchQuery] = useState("");
  const [reposPage, setReposPage] = useState(0);
  const reposPerPage = 10;

  // Paginate projects (exclude pinned projects from the main list)
  const pinnedSet = new Set(pinnedProjectIds);
  const filteredProjects = projects.filter(p => !p.id || !pinnedSet.has(p.id));
  const paginatedProjects = filteredProjects.slice(
    projectsPage * projectsPerPage,
    (projectsPage + 1) * projectsPerPage,
  );
  const projectsTotalPages = Math.ceil(
    filteredProjects.length / projectsPerPage,
  );

  const filteredRepositories = repositories.filter(
    (repo: TypesGitRepository) =>
      matchesAllTokens(reposSearchQuery, repo.name, repo.description),
  );
  const paginatedRepositories = filteredRepositories.slice(
    reposPage * reposPerPage,
    (reposPage + 1) * reposPerPage,
  );
  const reposTotalPages = Math.ceil(filteredRepositories.length / reposPerPage);

  const handleMenuOpen = (
    event: React.MouseEvent<HTMLElement>,
    project: TypesProject,
  ) => {
    setAnchorEl(event.currentTarget);
    setSelectedProject(project);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
    setSelectedProject(null);
  };

  // Helper to create a new repo for the project dialog
  const handleCreateRepoForProject = async (
    name: string,
    description: string,
  ): Promise<TypesGitRepository | null> => {
    if (!name.trim() || !account.user?.id) return null;

    try {
      const apiClient = api.getApiClient();
      const response = await apiClient.v1GitRepositoriesCreate({
        name,
        description,
        owner_id: account.user.id, // Always use user ID, not org ID
        organization_id: currentOrg?.id,
        repo_type: TypesGitRepositoryType.GitRepositoryTypeCode,
        default_branch: "main",
      });

      // Invalidate repo queries (use base key to match all variants)
      await queryClient.invalidateQueries({ queryKey: ["git-repositories"] });

      return response.data;
    } catch (error) {
      console.error("Failed to create repository:", error);
      return null;
    }
  };

  // Helper to link an external repo for the project dialog
  const handleLinkRepoForProject = async (
    url: string,
    name: string,
    type: TypesExternalRepositoryType,
    username?: string,
    password?: string,
    azureDevOps?: TypesAzureDevOps,
    oauthConnectionId?: string,
    gitProviderConnectionId?: string,
  ): Promise<TypesGitRepository | null> => {
    if (!url.trim() || !account.user?.id) return null;

    try {
      const apiClient = api.getApiClient();
      const response = await apiClient.v1GitRepositoriesCreate({
        name,
        description: `External ${type} repository`,
        owner_id: account.user.id, // Always use user ID, not org ID
        organization_id: currentOrg?.id,
        repo_type: TypesGitRepositoryType.GitRepositoryTypeCode,
        default_branch: "main",
        external_url: url,
        external_type: type,
        username,
        password,
        azure_devops: azureDevOps,
        oauth_connection_id: oauthConnectionId, // OAuth connection for push access
        git_provider_connection_id: gitProviderConnectionId,
      });

      // Invalidate repo queries (use base key to match all variants)
      await queryClient.invalidateQueries({ queryKey: ["git-repositories"] });

      return response.data;
    } catch (error: any) {
      console.error("Failed to link repository:", error);
      // Re-throw with the actual error message so the dialog can display it
      const message =
        error?.response?.data?.message ||
        error?.response?.data ||
        error?.message ||
        "Failed to link repository";
      throw new Error(
        typeof message === "string" ? message : JSON.stringify(message),
      );
    }
  };

  const handleViewProject = (project: TypesProject) => {
    account.orgNavigate("project-specs", { id: project.id });
  };

  const handleViewRepository = (repo: TypesGitRepository) => {
    account.orgNavigate("git-repo-detail", { repoId: repo.id });
  };

  const handleCreateRepository = async (
    name: string,
    description: string,
    koditIndexing: boolean,
  ) => {
    if (!account.user?.id) return;

    setRepositoryCreating(true);
    setRepositoryCreateError("");
    try {
      await api.getApiClient().v1GitRepositoriesCreate({
        name,
        description,
        owner_id: account.user.id,
        organization_id: currentOrg?.id,
        repo_type: TypesGitRepositoryType.GitRepositoryTypeCode,
        default_branch: "main",
        kodit_indexing: koditIndexing,
      });
      await queryClient.invalidateQueries({ queryKey: ["git-repositories"] });
      setCreateRepositoryDialogOpen(false);
      snackbar.success("Repository created successfully");
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to create repository";
      setRepositoryCreateError(message);
      throw error;
    } finally {
      setRepositoryCreating(false);
    }
  };

  const handleLinkExternalRepository = async (
    url: string,
    name: string,
    type: "github" | "gitlab" | "ado" | "other",
    koditIndexing: boolean,
    username?: string,
    password?: string,
    organizationUrl?: string,
    token?: string,
    gitlabBaseUrl?: string,
  ) => {
    if (!account.user?.id) return;

    const externalType =
      type === "other"
        ? ("bitbucket" as TypesExternalRepositoryType)
        : (type as TypesExternalRepositoryType);
    const repositoryName =
      name.trim() ||
      url.split("/").filter(Boolean).pop()?.replace(/\.git$/, "") ||
      "external-repository";

    setRepositoryCreating(true);
    try {
      await api.getApiClient().v1GitRepositoriesCreate({
        name: repositoryName,
        description: `External ${type} repository`,
        owner_id: account.user.id,
        organization_id: currentOrg?.id,
        repo_type: TypesGitRepositoryType.GitRepositoryTypeCode,
        default_branch: "main",
        is_external: true,
        external_url: url,
        external_type: externalType,
        kodit_indexing: koditIndexing,
        username,
        password,
        github:
          type === "github" && token
            ? { personal_access_token: token }
            : undefined,
        gitlab:
          type === "gitlab" && token
            ? { personal_access_token: token, base_url: gitlabBaseUrl }
            : undefined,
        azure_devops:
          type === "ado"
            ? {
                organization_url: organizationUrl || "",
                personal_access_token: token || "",
              }
            : undefined,
        bitbucket:
          type === "other"
            ? { username: username || "", app_password: password || "" }
            : undefined,
      });
      await queryClient.invalidateQueries({ queryKey: ["git-repositories"] });
      setLinkRepositoryDialogOpen(false);
      snackbar.success("Repository linked successfully");
    } catch (error) {
      snackbar.error(
        error instanceof Error ? error.message : "Failed to link repository",
      );
      throw error;
    } finally {
      setRepositoryCreating(false);
    }
  };

  const handleBrowseSelectRepository = async (
    repo: TypesRepositoryInfo,
    providerType: string,
    oauthConnectionId?: string,
    patConnectionId?: string,
  ) => {
    if (!account.user?.id) return;

    setLinkingFromBrowser(true);
    try {
      let actualProviderType = providerType;
      let providerCredentials: {
        type?: string;
        pat?: string;
        username?: string;
        orgUrl?: string;
        gitlabBaseUrl?: string;
        githubBaseUrl?: string;
        bitbucketBaseUrl?: string;
      } | null = null;

      if (providerType.startsWith("{")) {
        providerCredentials = JSON.parse(providerType);
        actualProviderType = providerCredentials?.type || "github";
      }

      const externalTypes: Record<string, TypesExternalRepositoryType> = {
        github: "github" as TypesExternalRepositoryType,
        gitlab: "gitlab" as TypesExternalRepositoryType,
        "azure-devops": "ado" as TypesExternalRepositoryType,
        bitbucket: "bitbucket" as TypesExternalRepositoryType,
      };

      await api.getApiClient().v1GitRepositoriesCreate({
        name: repo.name || "repository",
        description: repo.description || `${actualProviderType} repository`,
        owner_id: account.user.id,
        organization_id: currentOrg?.id,
        repo_type: TypesGitRepositoryType.GitRepositoryTypeCode,
        default_branch: repo.default_branch || "main",
        is_external: true,
        external_url: repo.clone_url || repo.html_url || "",
        external_type:
          externalTypes[actualProviderType] ||
          ("github" as TypesExternalRepositoryType),
        kodit_indexing: true,
        github:
          actualProviderType === "github" && providerCredentials
            ? {
                personal_access_token: providerCredentials.pat,
                base_url: providerCredentials.githubBaseUrl,
              }
            : undefined,
        gitlab:
          actualProviderType === "gitlab" && providerCredentials
            ? {
                personal_access_token: providerCredentials.pat,
                base_url: providerCredentials.gitlabBaseUrl,
              }
            : undefined,
        azure_devops:
          actualProviderType === "azure-devops" && providerCredentials
            ? {
                organization_url: providerCredentials.orgUrl || "",
                personal_access_token: providerCredentials.pat || "",
              }
            : undefined,
        bitbucket:
          actualProviderType === "bitbucket" && providerCredentials
            ? {
                username: providerCredentials.username || "",
                app_password: providerCredentials.pat || "",
                base_url: providerCredentials.bitbucketBaseUrl,
              }
            : undefined,
        oauth_connection_id: oauthConnectionId,
        git_provider_connection_id: patConnectionId,
      });
      await queryClient.invalidateQueries({ queryKey: ["git-repositories"] });
      setBrowseProvidersOpen(false);
      snackbar.success("Repository linked successfully");
    } catch (error) {
      console.error("Failed to link repository from provider:", error);
      snackbar.error("Failed to link repository");
    } finally {
      setLinkingFromBrowser(false);
    }
  };

  const handleProjectSettings = () => {
    if (selectedProject) {
      openDialog('project-settings', { projectId: selectedProject.id });
    }
    handleMenuClose();
  };

  const handleNewProject = () => {
    if (!requireLogin()) return;
    setCreateDialogOpen(true);
  };

  // Step 1: collect the complete task execution defaults for the sample.
  const handleInstantiateSample = async (
    sampleId: string,
    sampleName: string,
  ) => {
    if (!requireLogin()) return;

    // Find the sample project
    const sampleProject = sampleProjects.find((p: any) => p.id === sampleId);

    // Store the sample project for later (GitHub auth happens after config selection).
    setPendingSampleFork({ sampleId, sampleName, sampleProject });
    setAgentModalOpen(true);
  };

  // Step 2: proceed with the selected config or show the GitHub wizard.
  const handleAgentSelected = async (codeAgentConfig: TypesCodeAgentExecutionConfig) => {
    if (!pendingSampleFork) return;

    const { sampleId, sampleName, sampleProject } = pendingSampleFork;
    setPendingSampleFork(null);

    // Check if this sample requires GitHub auth
    if (
      sampleProject?.requires_github_auth ||
      (sampleProject?.required_repositories?.length || 0) > 0
    ) {
      // Store the selected agent and open the GitHub wizard
      setSelectedCodeAgentConfigForWizard(codeAgentConfig);
      setSampleWizardProject(sampleProject);
      setSampleWizardOpen(true);
    } else {
      // Standard flow - create project directly
      try {
        snackbar.info(`Creating ${sampleName}...`);

        const result = await instantiateSampleMutation.mutateAsync({
          sampleId,
          request: {
            project_name: sampleName,
            organization_id: account.organizationTools.organization?.id,
            code_agent_config: codeAgentConfig,
          },
        });

        snackbar.success("Sample project created successfully!");

        if (result && result.project_id) {
          account.orgNavigate("project-specs", { id: result.project_id });
        }
      } catch (err) {
        snackbar.error("Failed to create sample project");
      }
    }
  };

  if (isProjectsLoading) {
    return (
      <Page breadcrumbTitle="Projects" orgBreadcrumbs={true}>
        <Container maxWidth="lg">
          <Box
            sx={{
              display: "flex",
              justifyContent: "center",
              alignItems: "center",
              minHeight: "400px",
            }}
          >
            <CircularProgress />
          </Box>
        </Container>
      </Page>
    );
  }

  // Get breadcrumb title based on current view
  const getBreadcrumbTitle = () => {
    switch (currentView) {
      case "repositories":
        return "Repositories";
      case "guidelines":
        return "Guidelines";
      default:
        return "Projects";
    }
  };

  return (
    <Page
      breadcrumbTitle={getBreadcrumbTitle()}
      breadcrumbs={[]}
      orgBreadcrumbs={true}
      globalSearch={true}
      notifications={true}
      organizationId={account.organizationTools.organization?.id}
      topbarContent={<HelixOrgTopNav />}
    >
      <Container maxWidth="lg" sx={{ mt: 4, mb: 4 }}>
        <Paywall active={paywallActive} onBillingClick={navigateToBilling}>
          {/* Projects View */}
          {currentView === "projects" && (
            <ProjectsListView
              projects={projects}
              error={isLoggedIn ? error : null}
              isLoading={isProjectsLoading}
              page={projectsPage}
              onPageChange={setProjectsPage}
              filteredProjects={filteredProjects}
              paginatedProjects={paginatedProjects}
              totalPages={projectsTotalPages}
              onViewProject={handleViewProject}
              onMenuOpen={handleMenuOpen}
              onNavigateToSettings={(id) =>
                openDialog('project-settings', { projectId: id })
              }
              onCreateEmpty={handleNewProject}
              onCreateFromSample={handleInstantiateSample}
              sampleProjects={sampleProjects}
              isCreating={instantiateSampleMutation.isPending}
              pinnedProjectIds={pinnedProjectIds}
              onPinProject={handlePinProject}
              onUnpinProject={handleUnpinProject}
            />
          )}

          {/* Repositories View */}
          {currentView === "repositories" && (
            <RepositoriesListView
              repositories={repositories}
              searchQuery={reposSearchQuery}
              onSearchChange={setReposSearchQuery}
              page={reposPage}
              onPageChange={setReposPage}
              filteredRepositories={filteredRepositories}
              paginatedRepositories={paginatedRepositories}
              totalPages={reposTotalPages}
              onViewRepository={handleViewRepository}
              onBrowseProviders={() => {
                if (requireLogin()) setBrowseProvidersOpen(true);
              }}
              onLinkExternalRepo={() => {
                if (requireLogin()) setLinkRepositoryDialogOpen(true);
              }}
              onCreateRepo={() => {
                if (requireLogin()) setCreateRepositoryDialogOpen(true);
              }}
            />
          )}

          {/* Guidelines View */}
          {currentView === "guidelines" && (
            <GuidelinesView
              organization={currentOrg}
              isPersonalWorkspace={!currentOrg}
            />
          )}

        </Paywall>
      </Container>

      {/* Project Menu */}
      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={handleProjectSettings}>
          <SettingsIcon sx={{ mr: 1 }} fontSize="small" />
          Settings
        </MenuItem>
      </Menu>

      {/* Create Project Dialog */}
      <CreateProjectDialog
        open={createDialogOpen}
        onClose={() => {
          setCreateDialogOpen(false)
          setInitialCreateConfig(undefined)
        }}
        initialCodeAgentConfig={initialCreateConfig || orgDefaultConfig}
        repositories={repositories}
        reposLoading={reposLoading}
        onCreateRepo={handleCreateRepoForProject}
        onLinkRepo={handleLinkRepoForProject}
      />

      <CreateRepositoryDialog
        open={createRepositoryDialogOpen}
        onClose={() => {
          setCreateRepositoryDialogOpen(false);
          setRepositoryCreateError("");
        }}
        onSubmit={handleCreateRepository}
        isCreating={repositoryCreating}
        error={repositoryCreateError}
      />

      <LinkExternalRepositoryDialog
        open={linkRepositoryDialogOpen}
        onClose={() => setLinkRepositoryDialogOpen(false)}
        onSubmit={handleLinkExternalRepository}
        isCreating={repositoryCreating}
      />

      <BrowseProvidersDialog
        open={browseProvidersOpen}
        onClose={() => setBrowseProvidersOpen(false)}
        onSelectRepository={handleBrowseSelectRepository}
        isLinking={linkingFromBrowser}
      />

      {/* Agent Selection Modal for Sample Project Fork */}
      <AgentSelectionModal
        open={agentModalOpen}
        onClose={() => {
          setAgentModalOpen(false);
          setPendingSampleFork(null);
        }}
        onSelect={handleAgentSelected}
        title="Select Task Defaults"
        description="Choose the coding runtime, credentials, provider, and model for tasks in this project."
      />

      {/* GitHub Auth Wizard for Sample Projects */}
      <SampleProjectWizard
        open={sampleWizardOpen}
        onClose={() => {
          setSampleWizardOpen(false);
          setSampleWizardProject(null);
        }}
        onComplete={(projectId) => {
          setSampleWizardOpen(false);
          setSampleWizardProject(null);
          snackbar.success("Project created successfully!");
          account.orgNavigate("project-specs", { id: projectId });
        }}
        sampleProject={sampleWizardProject}
        organizationId={account.organizationTools.organization?.id}
        codeAgentConfig={selectedCodeAgentConfigForWizard}
      />

    </Page>
  );
};

export default Projects;
