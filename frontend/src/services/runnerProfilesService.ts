// Runner profile (compose-based) service.
//
// TODO: replace direct axios calls with the generated API client once
// `./stack update_openapi` is run against the new swagger annotations on
// api/pkg/server/runner_profile_handlers.go and runner_assignment_handlers.go.
// Until then we use axios directly with explicit endpoint paths.
//
// Per-call shape mirrors helixModelsService.ts so the swap-over is mechanical.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import axios from "axios";

// Mirror of api/pkg/types/runner_profile.go RunnerProfile.
export interface RunnerProfile {
  id: string;
  name: string;
  description?: string;
  compose_yaml: string;
  models?: ProfileModel[];
  gpu_requirement: ProfileGPURequirement;
  created_at?: string;
  updated_at?: string;
}

export interface ProfileModel {
  name: string;
  container_name: string;
  internal_port: number;
}

export interface ProfileGPURequirement {
  count: number;
  vendor?: "nvidia" | "amd" | "";
  architectures?: string[];
  model_match?: string;
  min_vram_bytes?: number;
}

// What the create / update endpoints accept.
export interface RunnerProfileSaveRequest {
  name: string;
  description?: string;
  compose_yaml: string;
  vendor?: "nvidia" | "amd" | "";
  architectures?: string[];
  model_match?: string;
  min_vram_bytes?: number;
}

const BASE = "/api/v1/runner-profiles";

export const runnerProfilesQueryKey = () => ["runnerProfiles"];

// Options that callers may use to tune the cache/refresh behaviour. Most
// admin surfaces only need the default (staleTime 3s, no polling), but
// the Agent Sandboxes panel polls debug data on a 5s cadence and wants
// the profiles list to stay aligned with that so the per-sandbox card
// toggles in/out of view without manual refresh.
export interface UseListRunnerProfilesOptions {
  refetchInterval?: number;
  staleTime?: number;
}

export function useListRunnerProfiles(opts?: UseListRunnerProfilesOptions) {
  return useQuery<RunnerProfile[]>({
    queryKey: runnerProfilesQueryKey(),
    queryFn: async () => (await axios.get<RunnerProfile[]>(BASE)).data || [],
    staleTime: opts?.staleTime ?? 3000,
    refetchInterval: opts?.refetchInterval,
  });
}

export function useCreateRunnerProfile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: RunnerProfileSaveRequest) =>
      (await axios.post<RunnerProfile>(BASE, body)).data,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: runnerProfilesQueryKey() });
    },
  });
}

export function useUpdateRunnerProfile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      id,
      body,
    }: {
      id: string;
      body: RunnerProfileSaveRequest;
    }) => (await axios.put<RunnerProfile>(`${BASE}/${id}`, body)).data,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: runnerProfilesQueryKey() });
    },
  });
}

export function useDeleteRunnerProfile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      await axios.delete(`${BASE}/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: runnerProfilesQueryKey() });
    },
  });
}

// Runner-side: get assignment / list compatible profiles / assign / clear.

export function useGetRunnerAssignment(runnerID: string, enabled = true) {
  return useQuery({
    queryKey: ["runnerAssignment", runnerID],
    enabled: enabled && Boolean(runnerID),
    queryFn: async () => {
      try {
        return (
          await axios.get(`/api/v1/runners/${runnerID}/assignment`)
        ).data;
      } catch (err: any) {
        if (err?.response?.status === 404) return null;
        throw err;
      }
    },
    staleTime: 3000,
  });
}

export function useListCompatibleRunnerProfiles(runnerID: string, enabled = true) {
  return useQuery<RunnerProfile[]>({
    queryKey: ["compatibleRunnerProfiles", runnerID],
    enabled: enabled && Boolean(runnerID),
    queryFn: async () =>
      (
        await axios.get<RunnerProfile[]>(
          `/api/v1/runners/${runnerID}/compatible-profiles`,
        )
      ).data || [],
    staleTime: 3000,
  });
}

export function useAssignRunnerProfile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ runnerID, profileID }: { runnerID: string; profileID: string }) =>
      (
        await axios.post(`/api/v1/runners/${runnerID}/assign-profile`, {
          profile_id: profileID,
        })
      ).data,
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: ["runnerAssignment", vars.runnerID] });
    },
  });
}

// Profile-derived list of currently-available models. Source of truth =
// the union of models exposed by every connected sandbox whose active
// profile is "running". Used to overlay an "available now" badge on
// HelixModelsTable so the registry tab shows what's actually being served.
export interface OpenAIModelEntry {
  id: string;
  object: string;
  created: number;
  owned_by: string;
}
export interface OpenAIModelsResponse {
  object: string;
  data: OpenAIModelEntry[];
}

export function useListInferenceModels() {
  return useQuery<OpenAIModelsResponse>({
    queryKey: ["inferenceModels"],
    queryFn: async () => (await axios.get<OpenAIModelsResponse>("/api/v1/v1/models")).data,
    staleTime: 5000,
    refetchInterval: 10000,
  });
}

export function useClearRunnerProfile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (runnerID: string) => {
      await axios.post(`/api/v1/runners/${runnerID}/clear-profile`);
    },
    onSuccess: (_data, runnerID) => {
      queryClient.invalidateQueries({ queryKey: ["runnerAssignment", runnerID] });
    },
  });
}
