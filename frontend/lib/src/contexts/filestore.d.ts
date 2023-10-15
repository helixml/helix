import React, { FC } from 'react';
import { IFileStoreItem, IFileStoreConfig } from '../types';
export interface IFilestoreContext {
    initialized: boolean;
    files: IFileStoreItem[];
    filestoreConfig: IFileStoreConfig;
    filesLoading: boolean;
    onSetFilestorePath: (path: string) => void;
}
export declare const FilestoreContext: React.Context<IFilestoreContext>;
export declare const useFilestoreContext: () => IFilestoreContext;
export declare const AccountContextProvider: FC;
//# sourceMappingURL=filestore.d.ts.map