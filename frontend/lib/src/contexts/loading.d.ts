import React, { FC } from 'react';
export interface ILoadingContext {
    loading: boolean;
    setLoading: {
        (val: boolean): void;
    };
}
export declare const LoadingContext: React.Context<ILoadingContext>;
export declare const useLoadingContext: () => ILoadingContext;
export declare const LoadingContextProvider: FC;
//# sourceMappingURL=loading.d.ts.map