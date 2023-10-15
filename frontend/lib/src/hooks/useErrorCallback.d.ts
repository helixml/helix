export declare const extractErrorMessage: (error: any) => string;
export declare function useErrorCallback<T = void>(handler: {
    (): Promise<T | void>;
}, snackbarActive?: boolean): () => Promise<void | T>;
export default useErrorCallback;
//# sourceMappingURL=useErrorCallback.d.ts.map