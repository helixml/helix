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
exports.FilestoreContextProvider = exports.useFilestoreContext = exports.FilestoreContext = void 0;
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const useApi_1 = __importDefault(require("../hooks/useApi"));
const useAccount_1 = __importDefault(require("../hooks/useAccount"));
const useRouter_1 = __importDefault(require("../hooks/useRouter"));
exports.FilestoreContext = (0, react_1.createContext)({
    files: [],
    loading: false,
    config: {
        user_prefix: '',
        folders: [],
    },
    onSetPath: () => { },
});
const useFilestoreContext = () => {
    const api = (0, useApi_1.default)();
    const account = (0, useAccount_1.default)();
    const { params, navigate, } = (0, useRouter_1.default)();
    const [files, setFiles] = (0, react_1.useState)([]);
    const [loading, setLoading] = (0, react_1.useState)(false);
    const [config, setConfig] = (0, react_1.useState)({
        user_prefix: '',
        folders: [],
    });
    const onSetPath = (0, react_1.useCallback)((path) => {
        const update = {};
        if (path) {
            update.path = path;
        }
        navigate('files', update);
    }, [
        navigate,
    ]);
    const loadConfig = (0, react_1.useCallback)(() => __awaiter(void 0, void 0, void 0, function* () {
        const configResult = yield api.get('/api/v1/filestore/config');
        if (!configResult)
            return;
        setConfig(configResult);
    }), []);
    const loadFiles = (0, react_1.useCallback)((path) => __awaiter(void 0, void 0, void 0, function* () {
        setLoading(true);
        try {
            const filesResult = yield api.get('/api/v1/filestore/list', {
                params: {
                    path,
                }
            });
            if (!filesResult)
                return;
            setFiles(filesResult || []);
        }
        catch (e) { }
        setLoading(false);
    }), []);
    (0, react_1.useEffect)(() => {
        if (!params.path)
            return;
        if (!account.user)
            return;
        loadFiles(params.path);
    }, [
        account.user,
        params.path,
    ]);
    (0, react_1.useEffect)(() => {
        if (!account.user)
            return;
        loadConfig();
    }, [
        account.user,
    ]);
    const contextValue = (0, react_1.useMemo)(() => ({
        files,
        loading,
        config,
        onSetPath,
    }), [
        files,
        loading,
        config,
        onSetPath,
    ]);
    return contextValue;
};
exports.useFilestoreContext = useFilestoreContext;
const FilestoreContextProvider = ({ children }) => {
    const value = (0, exports.useFilestoreContext)();
    return ((0, jsx_runtime_1.jsx)(exports.FilestoreContext.Provider, Object.assign({ value: value }, { children: children })));
};
exports.FilestoreContextProvider = FilestoreContextProvider;
//# sourceMappingURL=filestore.js.map