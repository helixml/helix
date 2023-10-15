import React, { FC } from 'react';
export interface IRouterContext {
    name: string;
    params: Record<string, string>;
    render: () => JSX.Element;
    meta: Record<string, any>;
    navigate: {
        (name: string, params?: Record<string, any>): void;
    };
    setParams: {
        (params: Record<string, string>, replace?: boolean): void;
    };
    removeParams: {
        (params: string[]): void;
    };
}
export declare const RouterContext: React.Context<IRouterContext>;
export declare const useRouterContext: () => IRouterContext;
export declare const RouterContextProvider: FC;
//# sourceMappingURL=router.d.ts.map