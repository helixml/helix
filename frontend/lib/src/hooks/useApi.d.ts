import { AxiosRequestConfig } from 'axios';
export interface IApiOptions {
    snackbar?: boolean;
}
export declare const getTokenHeaders: (token: string) => {
    Authorization: string;
};
export declare const useApi: () => {
    get: <ResT = any>(url: string, axiosConfig?: AxiosRequestConfig, options?: IApiOptions) => Promise<ResT | null>;
    post: <ReqT = any, ResT_1 = any>(url: string, data: ReqT, axiosConfig?: AxiosRequestConfig, options?: IApiOptions) => Promise<ResT_1 | null>;
    put: <ReqT_1 = any, ResT_2 = any>(url: string, data: ReqT_1, axiosConfig?: AxiosRequestConfig, options?: IApiOptions) => Promise<ResT_2 | null>;
    delete: <ResT_3 = any>(url: string, axiosConfig?: AxiosRequestConfig, options?: IApiOptions) => Promise<ResT_3 | null>;
    setToken: (token: string) => void;
};
export default useApi;
//# sourceMappingURL=useApi.d.ts.map