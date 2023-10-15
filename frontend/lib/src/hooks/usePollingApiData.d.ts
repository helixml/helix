export declare function usePollingApiData<DataType = any>({ url, defaultValue, active, reload, reloadInterval, jsonStringifyComparison, onChange, }: {
    url: string;
    defaultValue: DataType;
    active?: boolean;
    reload?: boolean;
    reloadInterval?: number;
    jsonStringifyComparison?: boolean;
    onChange?: {
        (data: DataType): void;
    };
}): [
    DataType,
    {
        (): Promise<void>;
    }
];
export default usePollingApiData;
//# sourceMappingURL=usePollingApiData.d.ts.map