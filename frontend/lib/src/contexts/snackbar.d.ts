import React, { FC } from 'react';
export type ISnackbarSeverity = 'error' | 'warning' | 'info' | 'success';
export interface ISnackbarData {
    message: string;
    severity: ISnackbarSeverity;
}
export interface ISnackbarContext {
    snackbar?: ISnackbarData;
    setSnackbar: {
        (message: string, severity?: ISnackbarSeverity): void;
    };
}
export declare const SnackbarContext: React.Context<ISnackbarContext>;
export declare const useSnackbarContext: () => ISnackbarContext;
export declare const SnackbarContextProvider: FC;
//# sourceMappingURL=snackbar.d.ts.map