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
exports.AccountContextProvider = exports.useFilestoreContext = exports.FilestoreContext = void 0;
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const bluebird_1 = __importDefault(require("bluebird"));
const keycloak_js_1 = __importDefault(require("keycloak-js"));
const reconnecting_websocket_1 = __importDefault(require("reconnecting-websocket"));
const useApi_1 = __importDefault(require("../hooks/useApi"));
const useSnackbar_1 = __importDefault(require("../hooks/useSnackbar"));
const useLoading_1 = __importDefault(require("../hooks/useLoading"));
const useErrorCallback_1 = require("../hooks/useErrorCallback");
const router_1 = __importDefault(require("../router"));
exports.FilestoreContext = (0, react_1.createContext)({
    files: [],
    filesLoading: false,
    filestoreConfig: {
        user_prefix: '',
        folders: [],
    },
    onSetFilestorePath: () => { },
});
const useFilestoreContext = () => {
    const api = (0, useApi_1.default)();
    const snackbar = (0, useSnackbar_1.default)();
    const loading = (0, useLoading_1.default)();
    const { name, meta, navigate, } = useContext(RouterContext);
    const [initialized, setInitialized] = (0, react_1.useState)(false);
    const [files, setFiles] = (0, react_1.useState)([]);
    const [filesLoading, setFilesLoading] = (0, react_1.useState)(false);
    const [filestoreConfig, setFilestoreConfig] = (0, react_1.useState)({
        user_prefix: '',
        folders: [],
    });
    const keycloak = (0, react_1.useMemo)(() => {
        return new keycloak_js_1.default({
            realm: REALM,
            url: KEYCLOAK_URL,
            clientId: CLIENT_ID,
        });
    }, []);
    const onSetFilestorePath = (0, react_1.useCallback)((path) => {
        const update = {};
        if (path) {
            update.path = path;
        }
        router_1.default.navigate('files', update);
    }, []);
    const loadModules = (0, react_1.useCallback)(() => __awaiter(void 0, void 0, void 0, function* () {
        const result = yield api.get('/api/v1/modules');
        if (!result)
            return;
        setModules(result);
    }), []);
    const loadJobs = (0, react_1.useCallback)(() => __awaiter(void 0, void 0, void 0, function* () {
        const result = yield api.get('/api/v1/jobs');
        if (!result)
            return;
        setJobs(result);
    }), []);
    const loadTransactions = (0, react_1.useCallback)(() => __awaiter(void 0, void 0, void 0, function* () {
        const result = yield api.get('/api/v1/transactions');
        if (!result)
            return;
        setTransactions(result);
    }), []);
    const loadStatus = (0, react_1.useCallback)(() => __awaiter(void 0, void 0, void 0, function* () {
        const statusResult = yield api.get('/api/v1/status');
        if (!statusResult)
            return;
        setCredits(statusResult.credits);
    }), []);
    const loadFilestoreConfig = (0, react_1.useCallback)(() => __awaiter(void 0, void 0, void 0, function* () {
        const configResult = yield api.get('/api/v1/filestore/config');
        if (!configResult)
            return;
        setFilestoreConfig(configResult);
    }), []);
    const loadFiles = (0, react_1.useCallback)((path) => __awaiter(void 0, void 0, void 0, function* () {
        setFilesLoading(true);
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
        setFilesLoading(false);
    }), []);
    const loadAll = (0, react_1.useCallback)(() => __awaiter(void 0, void 0, void 0, function* () {
        yield bluebird_1.default.all([
            loadModules(),
            loadFilestoreConfig(),
            loadJobs(),
            loadTransactions(),
            loadStatus(),
        ]);
    }), [
        loadModules,
        loadJobs,
        loadFilestoreConfig,
        loadTransactions,
        loadStatus,
    ]);
    const onLogin = (0, react_1.useCallback)(() => {
        keycloak.login();
    }, [
        keycloak,
    ]);
    const onLogout = (0, react_1.useCallback)(() => {
        keycloak.logout();
    }, [
        keycloak,
    ]);
    const initialize = (0, react_1.useCallback)(() => __awaiter(void 0, void 0, void 0, function* () {
        var _a, _b, _c, _d;
        loading.setLoading(true);
        try {
            const authenticated = yield keycloak.init({
                onLoad: 'check-sso',
                pkceMethod: 'S256',
            });
            if (authenticated) {
                if (!((_a = keycloak.tokenParsed) === null || _a === void 0 ? void 0 : _a.sub))
                    throw new Error(`no user id found from keycloak`);
                if (!((_b = keycloak.tokenParsed) === null || _b === void 0 ? void 0 : _b.preferred_username))
                    throw new Error(`no user email found from keycloak`);
                if (!keycloak.token)
                    throw new Error(`no user token found from keycloak`);
                const user = {
                    id: (_c = keycloak.tokenParsed) === null || _c === void 0 ? void 0 : _c.sub,
                    email: (_d = keycloak.tokenParsed) === null || _d === void 0 ? void 0 : _d.preferred_username,
                    token: keycloak.token,
                };
                api.setToken(keycloak.token);
                setUser(user);
                setInterval(() => __awaiter(void 0, void 0, void 0, function* () {
                    try {
                        const updated = yield keycloak.updateToken(10);
                        if (updated && keycloak.token) {
                            api.setToken(keycloak.token);
                            setUser(Object.assign({}, user, {
                                token: keycloak.token,
                            }));
                        }
                    }
                    catch (e) {
                        keycloak.login();
                    }
                }), 10 * 1000);
            }
        }
        catch (e) {
            const errorMessage = (0, useErrorCallback_1.extractErrorMessage)(e);
            console.error(errorMessage);
            snackbar.error(errorMessage);
        }
        loading.setLoading(false);
        setInitialized(true);
    }), []);
    (0, react_1.useEffect)(() => {
        initialize();
    }, []);
    (0, react_1.useEffect)(() => {
        if (!user)
            return;
        loadAll();
    }, [
        user,
    ]);
    (0, react_1.useEffect)(() => {
        if (!(user === null || user === void 0 ? void 0 : user.token))
            return;
        const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsHostname = window.location.hostname;
        const url = `${wsProtocol}//${wsHostname}/api/v1/ws?access_token=${user === null || user === void 0 ? void 0 : user.token}`;
        const rws = new reconnecting_websocket_1.default(url);
        rws.addEventListener('message', (event) => {
            const parsedData = JSON.parse(event.data);
            console.dir(parsedData);
            if (parsedData.type === 'job' && parsedData.job) {
                const newJob = parsedData.job;
                setJobs(jobs => jobs.map(existingJob => {
                    if (existingJob.id === newJob.id)
                        return newJob;
                    return existingJob;
                }));
            }
        });
        return () => rws.close();
    }, [
        user === null || user === void 0 ? void 0 : user.token,
    ]);
    (0, react_1.useEffect)(() => {
        if (!route.params.path)
            return;
        if (!user)
            return;
        loadFiles(route.params.path);
    }, [
        user,
        route.params.path,
    ]);
    const contextValue = (0, react_1.useMemo)(() => ({
        initialized,
        user,
        credits,
        jobs,
        modules,
        files,
        filesLoading,
        filestoreConfig,
        transactions,
        onLogin,
        onLogout,
        onSetFilestorePath,
    }), [
        initialized,
        user,
        credits,
        jobs,
        modules,
        files,
        filesLoading,
        filestoreConfig,
        transactions,
        onLogin,
        onLogout,
        onSetFilestorePath,
    ]);
    return contextValue;
};
exports.useFilestoreContext = useFilestoreContext;
const AccountContextProvider = ({ children }) => {
    const value = useAccountContext();
    return ((0, jsx_runtime_1.jsx)(AccountContext.Provider, Object.assign({ value: value }, { children: children })));
};
exports.AccountContextProvider = AccountContextProvider;
//# sourceMappingURL=filestore.js.map