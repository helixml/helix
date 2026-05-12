import { useQuery } from "@tanstack/react-query";
import { MutableRefObject } from "react";

export interface LogEntry {
    timestamp: string;
    level: string;
    message: string;
    source: string;
}

export interface LogMetadata {
    slot_id: string;
    model_id: string;
    created_at: string;
    status: string;
    last_error?: string;
}

export interface LogResponse {
    slot_id: string;
    metadata: LogMetadata;
    logs: LogEntry[];
    count: number;
}

export interface SlotLogsQuery {
    lines?: number;
    level?: string;
}

// Stable query key excludes 'since' to prevent cache pollution
export const slotLogsQueryKey = (slotId: string, query?: SlotLogsQuery) => [
    "slot-logs",
    slotId,
    query,
];

export function useSlotLogs(
    slotId: string,
    query?: SlotLogsQuery,
    options?: {
        enabled?: boolean;
        refetchInterval?: number | false;
        sinceRef?: MutableRefObject<string | null>; // Read 'since' from ref, not query key
    }
) {
    return useQuery({
        queryKey: slotLogsQueryKey(slotId, query),
        queryFn: async (): Promise<LogResponse> => {
            throw new Error("Runner slot logs are no longer available");
        },
        enabled: options?.enabled ?? false,
        refetchInterval: options?.refetchInterval,
        staleTime: 0, // Always consider stale for fresh logs
    });
}
