type asyncFunction = {
    (): Promise<void>;
};
type asyncFunctionBoolan = {
    (): Promise<boolean>;
};
export declare const useLoadingErrorHandler: ({ withSnackbar, withLoading, }?: {
    withSnackbar?: boolean | undefined;
    withLoading?: boolean | undefined;
}) => (handler: asyncFunction) => asyncFunctionBoolan;
export default useLoadingErrorHandler;
//# sourceMappingURL=useLoadingErrorHandler.d.ts.map