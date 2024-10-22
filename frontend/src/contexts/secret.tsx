import React, { useEffect, createContext, useContext, useState, useCallback } from 'react';
import useApi from '../hooks/useApi'
import useAccount from '../hooks/useAccount'
import { ISecret, ICreateSecret } from '../types';

interface SecretContextType {
  initialized: boolean,
  secrets: ISecret[];
  listSecrets: () => Promise<void>;
  createSecret: (secret: ICreateSecret) => Promise<void>;
  deleteSecret: (id: string) => Promise<void>;
}

const SecretContext = createContext<SecretContextType | undefined>(undefined);

export const useSecret = () => {
  const context = useContext(SecretContext);
  if (!context) {
    throw new Error('useSecret must be used within a SecretProvider');
  }
  return context;
};

export const SecretProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [secrets, setSecrets] = useState<ISecret[]>([]);
  const [initialized, setInitialized] = useState(false);
  const api = useApi();
  const account = useAccount();

  const listSecrets = useCallback(async () => {
    try {
      const result = await api.get<ISecret[]>('/api/v1/secrets');
      setSecrets(result || []);
    } catch (error) {
      console.error('Failed to fetch secrets:', error);
    }
  }, [api]);

  const createSecret = useCallback(async (secret: ICreateSecret) => {
    try {
      await api.post('/api/v1/secrets', secret);
      await listSecrets(); // Refresh the list after creating a new secret
    } catch (error) {
      console.error('Failed to create secret:', error);
      throw error;
    }
  }, [api, listSecrets]);

  const deleteSecret = useCallback(async (id: string) => {
    try {
      await api.delete(`/api/v1/secrets/${id}`);
      await listSecrets(); // Refresh the list after deleting a secret
    } catch (error) {
      console.error('Failed to delete secret:', error);
      throw error;
    }
  }, [api, listSecrets]);

  useEffect(() => {
    if(!account.user) return
    listSecrets()
    if(!initialized) {
      setInitialized(true)
    }
  }, [account.user])

  const value = {
    initialized,
    secrets,
    listSecrets,
    createSecret,
    deleteSecret,
  };

  return <SecretContext.Provider value={value}>{children}</SecretContext.Provider>;
};
