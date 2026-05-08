import { useQuery } from "@tanstack/react-query";
import { MemoryEstimationResponse } from "../types/dashboard";

export const memoryEstimationQueryKey = (
    modelId: string,
    gpuCount?: number,
) => ["memoryEstimation", modelId, gpuCount];

export function useModelMemoryEstimation(
    modelId: string,
    gpuCount?: number,
    options?: { enabled?: boolean },
) {
    return useQuery({
        queryKey: memoryEstimationQueryKey(modelId, gpuCount),
        queryFn: async () => {
            return {
                error: "Memory estimation is no longer available",
            } satisfies MemoryEstimationResponse;
        },
        enabled: options?.enabled !== false && !!modelId,
        staleTime: Infinity,
    });
}

export function useAllModelsMemoryEstimation(gpuCount?: number) {
    return useQuery({
        queryKey: ["memoryEstimation", "all", gpuCount],
        queryFn: async () => {
            return {
                error: "Memory estimation is no longer available",
            } satisfies MemoryEstimationResponse;
        },
        staleTime: Infinity,
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
