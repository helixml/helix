import { useState, useCallback } from "react";
import { MemoryEstimate } from "../types/dashboard";

// Using API types instead of local interfaces

interface GPUScenario {
    name: string;
    gpuCount: number;
}

const GPU_SCENARIOS: GPUScenario[] = [
    { name: "Single GPU", gpuCount: 1 },
    { name: "2 GPUs", gpuCount: 2 },
    { name: "4 GPUs", gpuCount: 4 },
    { name: "8 GPUs", gpuCount: 8 },
];

export const useMemoryEstimation = () => {
    const [estimates, setEstimates] = useState<
        Record<number, MemoryEstimate>
    >({});
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);

    const fetchEstimate = useCallback(
        async (
            modelId: string,
            contextLength: number,
            gpuCount: number = 1,
            numParallel?: number,
        ) => {
            if (!modelId || contextLength <= 0) return;

            setLoading(true);
            setError(null);

            setEstimates((prev) => {
                const next = { ...prev };
                delete next[gpuCount];
                return next;
            });
            setError("Memory estimation is no longer available");
            setLoading(false);
        },
        [],
    );

    const fetchMultipleEstimates = useCallback(
        async (
            modelId: string,
            contextLength: number,
            numParallel?: number,
        ) => {
            if (!modelId || contextLength <= 0) return;

            setLoading(true);
            setError(null);

            setEstimates({});
            setError("Memory estimation is no longer available");
            setLoading(false);
        },
        [],
    );

    const formatMemorySize = useCallback(
        (bytes: number | undefined): string => {
            if (!bytes || bytes === 0) return "0 B";

            const k = 1024;
            const sizes = ["B", "KB", "MB", "GB", "TB"];
            const i = Math.floor(Math.log(bytes) / Math.log(k));

            return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
        },
        [],
    );

    const clearEstimates = useCallback(() => {
        setEstimates({});
    }, []);

    return {
        estimates,
        loading,
        error,
        fetchEstimate,
        fetchMultipleEstimates,
        formatMemorySize,
        GPU_SCENARIOS,
        clearEstimates,
    };
};
