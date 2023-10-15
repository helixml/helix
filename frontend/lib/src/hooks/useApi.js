"use strict";
var __awaiter = (this && this.__awaiter) || function (thisArg, _arguments, P, generator) {
    function adopt(value) { return value instanceof P ? value : new P(function (resolve) { resolve(value); }); }
    return new (P || (P = Promise))(function (resolve, reject) {
        function fulfilled(value) { try { step(generator.next(value)); } catch (e) { reject(e); } }
        function rejected(value) { try { step(generator["throw"](value)); } catch (e) { reject(e); } }
        function step(result) { result.done ? resolve(result.value) : adopt(result.value).then(fulfilled, rejected); }
        step((generator = generator.apply(thisArg, _arguments || [])).next());
    });
};
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.useApi = exports.getTokenHeaders = void 0;
const axios_1 = __importDefault(require("axios"));
const react_1 = require("react");
const snackbar_1 = require("../contexts/snackbar");
const useErrorCallback_1 = require("./useErrorCallback");
const API_MOUNT = "";
const getTokenHeaders = (token) => {
    return {
        Authorization: `Bearer ${token}`,
    };
};
exports.getTokenHeaders = getTokenHeaders;
const useApi = () => {
    const snackbar = (0, react_1.useContext)(snackbar_1.SnackbarContext);
    const get = (0, react_1.useCallback)(function (url, axiosConfig, options) {
        return __awaiter(this, void 0, void 0, function* () {
            try {
                const res = yield axios_1.default.get(`${API_MOUNT}${url}`, axiosConfig);
                return res.data;
            }
            catch (e) {
                const errorMessage = (0, useErrorCallback_1.extractErrorMessage)(e);
                if ((options === null || options === void 0 ? void 0 : options.snackbar) !== false)
                    snackbar.setSnackbar(errorMessage, 'error');
                return null;
            }
        });
    }, []);
    const post = (0, react_1.useCallback)(function (url, data, axiosConfig, options) {
        return __awaiter(this, void 0, void 0, function* () {
            try {
                const res = yield axios_1.default.post(`${API_MOUNT}${url}`, data, axiosConfig);
                return res.data;
            }
            catch (e) {
                const errorMessage = (0, useErrorCallback_1.extractErrorMessage)(e);
                if ((options === null || options === void 0 ? void 0 : options.snackbar) !== false)
                    snackbar.setSnackbar(errorMessage, 'error');
                return null;
            }
        });
    }, []);
    const put = (0, react_1.useCallback)(function (url, data, axiosConfig, options) {
        return __awaiter(this, void 0, void 0, function* () {
            try {
                const res = yield axios_1.default.put(`${API_MOUNT}${url}`, data, axiosConfig);
                return res.data;
            }
            catch (e) {
                const errorMessage = (0, useErrorCallback_1.extractErrorMessage)(e);
                if ((options === null || options === void 0 ? void 0 : options.snackbar) !== false)
                    snackbar.setSnackbar(errorMessage, 'error');
                return null;
            }
        });
    }, []);
    const del = (0, react_1.useCallback)(function (url, axiosConfig, options) {
        return __awaiter(this, void 0, void 0, function* () {
            try {
                const res = yield axios_1.default.delete(`${API_MOUNT}${url}`, axiosConfig);
                return res.data;
            }
            catch (e) {
                const errorMessage = (0, useErrorCallback_1.extractErrorMessage)(e);
                if ((options === null || options === void 0 ? void 0 : options.snackbar) !== false)
                    snackbar.setSnackbar(errorMessage, 'error');
                return null;
            }
        });
    }, []);
    // this will work globally because we are applying this to the root import of axios
    // therefore we don't need to worry about passing the token around to other contexts
    // we can just call useApi() from anywhere and we will get the token injected into the request
    // because the top level account context has called this
    const setToken = (0, react_1.useCallback)(function (token) {
        axios_1.default.defaults.headers.common = token ? (0, exports.getTokenHeaders)(token) : {};
    }, []);
    return {
        get,
        post,
        put,
        delete: del,
        setToken,
    };
};
exports.useApi = useApi;
exports.default = exports.useApi;
//# sourceMappingURL=useApi.js.map