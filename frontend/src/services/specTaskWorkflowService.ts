import { useMutation, useQueryClient } from "@tanstack/react-query";
import useApi from "../hooks/useApi";
import useSnackbar from "../hooks/useSnackbar";
import { TypesSpecTask, TypesSpecTaskStatus } from "../api/api";

export function useApproveImplementation(specTaskId: string) {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();
  const snackbar = useSnackbar();

  return useMutation({
    mutationFn: async () => {
      const response =
        await apiClient.v1SpecTasksApproveImplementationCreate(specTaskId);
      return response.data;
    },
    onSuccess: (response: TypesSpecTask) => {
      if (response.status === "done") {
        // Internal repo - merge succeeded
        snackbar.success("Implementation approved and merged!");
      } else if (response.status === "implementation_review") {
        // Merge failed - agent needs to rebase
        snackbar.warning(
          "Branch has diverged - agent is rebasing. Click Accept again after rebase completes.",
        );
      } else if (response.repo_pull_requests && response.repo_pull_requests.length > 0) {
        // External repo - show link to first PR
        const firstPR = response.repo_pull_requests[0];
        if (firstPR.pr_url) {
          snackbar.success(
            `Pull request opened! View PR: ${firstPR.pr_url}`,
          );
        } else {
          snackbar.success(
            `Pull request #${firstPR.pr_id} opened - awaiting merge`,
          );
        }
      } else if (response.status === "pull_request") {
        // External repo - task moved to pull_request status, waiting for agent to push
        snackbar.success("Agent will push changes to open a pull request...");
      } else {
        // Fallback
        snackbar.success("Implementation approved!");
      }
      // Invalidate queries to refetch task
      queryClient.invalidateQueries({ queryKey: ["spec-tasks", specTaskId] });
      queryClient.invalidateQueries({ queryKey: ["spec-tasks"] });
    },
    onError: (error: any) => {
      const responseData = error?.response?.data;
      if (responseData?.error === "oauth_required") {
        // Let the component handle this via mutation.error
        return;
      }
      snackbar.error(
        responseData?.message || "Failed to approve implementation",
      );
    },
  });
}

export function useStopAgent(specTaskId: string) {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();
  const snackbar = useSnackbar();

  return useMutation({
    mutationFn: async () => {
      const response = await apiClient.v1SpecTasksStopAgentCreate(specTaskId);
      return response.data;
    },
    onSuccess: () => {
      snackbar.success("Agent stop requested");
      queryClient.invalidateQueries({ queryKey: ["spec-tasks", specTaskId] });
      queryClient.invalidateQueries({ queryKey: ["spec-tasks"] });
    },
    onError: (error: any) => {
      snackbar.error(error?.response?.data?.message || "Failed to stop agent");
    },
  });
}

export function useMoveToBacklog(specTaskId: string) {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();
  const snackbar = useSnackbar();

  return useMutation({
    mutationFn: async () => {
      // First, try to stop the agent (ignore errors if no agent running)
      try {
        await apiClient.v1SpecTasksStopAgentCreate(specTaskId);
      } catch {
        // Agent may not be running, that's fine
      }
      // Then update status to backlog
      const response = await apiClient.v1SpecTasksUpdate(specTaskId, {
        status: TypesSpecTaskStatus.TaskStatusBacklog,
      });
      return response.data;
    },
    onSuccess: () => {
      snackbar.success("Task moved to backlog");
      queryClient.invalidateQueries({ queryKey: ["spec-tasks", specTaskId] });
      queryClient.invalidateQueries({ queryKey: ["spec-tasks"] });
    },
    onError: (error: any) => {
      snackbar.error(
        error?.response?.data?.message || "Failed to move task to backlog",
      );
    },
  });
}

export function useSkipSpec(specTaskId: string) {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();
  const snackbar = useSnackbar();

  return useMutation({
    mutationFn: async () => {
      // Move directly to implementation without stopping the container.
      // The running container can keep going; the user drives the agent from here.
      const response = await apiClient.v1SpecTasksUpdate(specTaskId, {
        status: TypesSpecTaskStatus.TaskStatusImplementation,
        just_do_it_mode: true,
      });
      return response.data;
    },
    onSuccess: () => {
      snackbar.success("Skipped spec - task moved to implementation");
      queryClient.invalidateQueries({ queryKey: ["spec-tasks", specTaskId] });
      queryClient.invalidateQueries({ queryKey: ["spec-tasks"] });
    },
    onError: (error: any) => {
      snackbar.error(
        error?.response?.data?.message || "Failed to skip spec",
      );
    },
  });
}

export function useReopenTask(specTaskId: string) {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();
  const snackbar = useSnackbar();

  return useMutation({
    mutationFn: async () => {
      const response = await apiClient.v1SpecTasksUpdate(specTaskId, {
        status: TypesSpecTaskStatus.TaskStatusImplementation,
      });
      return response.data;
    },
    onSuccess: () => {
      snackbar.success("Task reopened - moved back to in progress");
      queryClient.invalidateQueries({ queryKey: ["spec-tasks", specTaskId] });
      queryClient.invalidateQueries({ queryKey: ["spec-tasks"] });
    },
    onError: (error: any) => {
      snackbar.error(
        error?.response?.data?.message || "Failed to reopen task",
      );
    },
  });
}
