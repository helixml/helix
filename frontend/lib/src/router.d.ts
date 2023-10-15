import { Route } from 'router5';
export interface IApplicationRoute extends Route {
    render: () => JSX.Element;
    meta: Record<string, any>;
}
export declare const NOT_FOUND_ROUTE: IApplicationRoute;
export declare const router: import("router5").Router<import("router5/dist/types/router").DefaultDependencies>;
export declare function useApplicationRoute(): IApplicationRoute;
export declare function RenderPage(): JSX.Element;
export default router;
//# sourceMappingURL=router.d.ts.map