import React, { FC } from 'react';
import { IFileStoreItem, IFileStoreConfig } from '../types';
export interface IFilestoreContext {
    files: IFileStoreItem[];
    config: IFileStoreConfig;
    loading: boolean;
    onSetPath: (path: string) => void;
}
export declare const FilestoreContext: React.Context<IFilestoreContext>;
export declare const useFilestoreContext: () => IFilestoreContext;
export declare const FilestoreContextProvider: FC;
//# sourceMappingURL=filestore.d.ts.map