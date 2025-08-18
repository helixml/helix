import { useState, useCallback } from 'react';
import useApi from './useApi';

interface MemoryEstimate {
  vram_size?: number;
  total_size?: number;
  weights?: number;
  kv_cache?: number;
  graph?: number;
  architecture?: string;
  requires_fallback?: boolean;
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
  const api = useApi();
  const apiClient = api.getApiClient();
  const [estimates, setEstimates] = useState<Record<number, MemoryEstimate>>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchEstimate = useCallback(async (modelId: string, contextLength: number, gpuCount: number = 1) => {
    if (!modelId || contextLength <= 0) return;

    setLoading(true);
    setError(null);

    try {
      const query = {
        model_id: modelId,
        context_length: contextLength,
        num_gpu: gpuCount,
      };

      const response = await apiClient.v1HelixModelsMemoryEstimateList(query);
      const data = response.data;
      
      // The API returns a single estimate object
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
  }, [apiClient]);

  const fetchMultipleEstimates = useCallback(async (modelId: string, contextLength: number) => {
    if (!modelId || contextLength <= 0) return;

    setLoading(true);
    setError(null);
    
    try {
      // Fetch estimates for all GPU scenarios in parallel
      const promises = GPU_SCENARIOS.map(async (scenario) => {
        try {
          const query = {
            model_id: modelId,
            context_length: contextLength,
            num_gpu: scenario.gpuCount,
          };

          const response = await apiClient.v1HelixModelsMemoryEstimateList(query);
          const data = response.data;
          
          if (data && data.estimate) {
            setEstimates(prev => ({ ...prev, [scenario.gpuCount]: data.estimate }));
          } else if (data && data.error) {
            console.warn(`Error for ${scenario.gpuCount} GPUs:`, data.error);
          }
        } catch (err) {
          console.warn(`Failed to fetch estimate for ${scenario.gpuCount} GPUs:`, err);
        }
      });
      
      await Promise.all(promises);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch memory estimations');
    } finally {
      setLoading(false);
    }
  }, [apiClient]);

  const formatMemorySize = useCallback((bytes: number | undefined): string => {
    if (!bytes || bytes === 0) return '0 B';
    
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    
    return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
  }, []);

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
