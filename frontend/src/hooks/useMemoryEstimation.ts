import { useState, useCallback } from 'react';

interface MemoryEstimate {
  vram_size: number;
  total_size: number;
  weights: number;
  kv_cache: number;
  graph: number;
  architecture: string;
  requires_fallback: boolean;
  error?: string;
}

interface MemoryEstimationResponse {
  estimate?: MemoryEstimate;
  error?: string;
  cached?: boolean;
  model_id?: string;
}

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
  const [estimates, setEstimates] = useState<Record<number, MemoryEstimate>>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchEstimate = useCallback(async (modelId: string, contextLength: number, gpuCount: number = 1) => {
    if (!modelId || contextLength <= 0) return;

    setLoading(true);
    setError(null);

    try {
      // Create a temporary URL with query parameters to get estimates for this specific context length
      const url = new URL('/api/v1/helix-models/memory-estimate', window.location.origin);
      url.searchParams.set('model_id', modelId);
      url.searchParams.set('context_length', contextLength.toString());
      url.searchParams.set('num_gpu', gpuCount.toString());

      const response = await fetch(url.toString());
      
      if (!response.ok) {
        throw new Error(`Failed to fetch memory estimation: ${response.statusText}`);
      }

      const data = await response.json();
      
      // The new API returns a single estimate object, not a map
      if (data && data.estimate) {
        setEstimates(prev => ({ ...prev, [gpuCount]: data.estimate }));
      } else if (data && data.error) {
        throw new Error(data.error);
      } else {
        throw new Error('No estimate returned for model');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch memory estimation');
      console.error('Memory estimation error:', err);
    } finally {
      setLoading(false);
    }
  }, []);

  const fetchMultipleEstimates = useCallback(async (modelId: string, contextLength: number) => {
    if (!modelId || contextLength <= 0) return;

    setLoading(true);
    setError(null);
    
    try {
      // Fetch estimates for all GPU scenarios in parallel
      const promises = GPU_SCENARIOS.map(scenario => 
        fetchEstimate(modelId, contextLength, scenario.gpuCount)
      );
      
      await Promise.all(promises);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch memory estimations');
    } finally {
      setLoading(false);
    }
  }, [fetchEstimate]);

  const formatMemorySize = useCallback((bytes: number): string => {
    if (bytes === 0) return '0 B';
    
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    
    return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
  }, []);

  return {
    estimates,
    loading,
    error,
    fetchEstimate,
    fetchMultipleEstimates,
    formatMemorySize,
    GPU_SCENARIOS,
    clearEstimates: () => setEstimates({}),
  };
};
