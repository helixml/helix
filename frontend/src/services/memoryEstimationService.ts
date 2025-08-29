import { useQuery } from "@tanstack/react-query";
import useApi from "../hooks/useApi";
import { ControllerMemoryEstimationResponse } from "../api/api";

export const memoryEstimationQueryKey = (
    modelId: string,
    gpuCount?: number,
) => ["memoryEstimation", modelId, gpuCount];

export function useModelMemoryEstimation(
    modelId: string,
    gpuCount?: number,
    options?: { enabled?: boolean },
) {
    const api = useApi();
    const apiClient = api.getApiClient();

    return useQuery({
        queryKey: memoryEstimationQueryKey(modelId, gpuCount),
        queryFn: async () => {
            const query: {
                model_id: string;
                gpu_count?: number;
                context_length?: number;
                batch_size?: number;
            } = {
                model_id: modelId,
            };

            if (gpuCount !== undefined) {
                query.gpu_count = gpuCount;
            }

            const response =
                await apiClient.v1HelixModelsMemoryEstimateList(query);
            return response.data;
        },
        enabled: options?.enabled !== false && !!modelId,
        staleTime: 5 * 60 * 1000, // 5 minutes
        refetchInterval: 30 * 1000, // Refetch every 30 seconds for real-time updates
    });
}

export function useAllModelsMemoryEstimation(gpuCount?: number) {
    const api = useApi();
    const apiClient = api.getApiClient();

    return useQuery({
        queryKey: ["memoryEstimation", "all", gpuCount],
        queryFn: async () => {
            const query: { model_ids?: string; gpu_count?: number } = {};
            if (gpuCount !== undefined) {
                query.gpu_count = gpuCount;
            }

            const response =
                await apiClient.v1HelixModelsMemoryEstimatesList(query);
            return response.data;
        },
        staleTime: 5 * 60 * 1000, // 5 minutes
        refetchInterval: 60 * 1000, // Refetch every minute for background updates
    });
}

// Helper function to format memory size
export function formatMemorySize(bytes: number): string {
    if (bytes === 0) return "0 B";

    const k = 1024;
    const sizes = ["B", "KB", "MB", "GB", "TB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));

    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
}
