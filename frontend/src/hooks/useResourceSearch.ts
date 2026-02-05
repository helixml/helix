import { useQuery } from '@tanstack/react-query';
import useApi from './useApi';
import { TypesResource, TypesResourceSearchRequest, TypesResourceSearchResponse } from '../api/api';

export const RESOURCE_SEARCH_QUERY_KEY = 'resource-search';

export interface UseResourceSearchOptions {
  query: string;
  types?: TypesResource[];
  limit?: number;
  orgId?: string;
  enabled?: boolean;
}

export const useResourceSearch = ({
  query,
  types,
  limit = 10,
  orgId,
  enabled = true,
}: UseResourceSearchOptions) => {
  const api = useApi();
  const client = api.getApiClient();

  return useQuery({
    queryKey: [RESOURCE_SEARCH_QUERY_KEY, { query, types, limit, orgId }],
    queryFn: async (): Promise<TypesResourceSearchResponse> => {
      const request: TypesResourceSearchRequest = {
        query,
        types,
        limit,
        org_id: orgId,
      };
      const response = await client.v1ResourceSearchCreate(request);
      return response.data;
    },
    enabled: enabled && query.length > 0,
    staleTime: 30 * 1000, // 30 seconds
    gcTime: 60 * 1000, // 1 minute
  });
};

export { TypesResource };
export default useResourceSearch;
