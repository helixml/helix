import React, { FC } from 'react';
import { IUser, IJob, IModule, IBalanceTransfer, IFileStoreItem, IFileStoreConfig } from '../types';
export interface IAccountContext {
    initialized: boolean;
    credits: number;
    user?: IUser;
    jobs: IJob[];
    modules: IModule[];
    files: IFileStoreItem[];
    filestoreConfig: IFileStoreConfig;
    filesLoading: boolean;
    transactions: IBalanceTransfer[];
    onLogin: () => void;
    onLogout: () => void;
    onSetFilestorePath: (path: string) => void;
}
export declare const AccountContext: React.Context<IAccountContext>;
export declare const useAccountContext: () => IAccountContext;
export declare const AccountContextProvider: FC;
//# sourceMappingURL=account.d.ts.map