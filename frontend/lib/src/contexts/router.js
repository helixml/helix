"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || function (mod) {
    if (mod && mod.__esModule) return mod;
    var result = {};
    if (mod != null) for (var k in mod) if (k !== "default" && Object.prototype.hasOwnProperty.call(mod, k)) __createBinding(result, mod, k);
    __setModuleDefault(result, mod);
    return result;
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.RouterContextProvider = exports.useRouterContext = exports.RouterContext = void 0;
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const react_router5_1 = require("react-router5");
const router_1 = __importStar(require("../router"));
exports.RouterContext = (0, react_1.createContext)({
    name: '',
    params: {},
    render: () => (0, jsx_runtime_1.jsx)("div", { children: "Page Not Found" }),
    meta: {},
    navigate: () => { },
    setParams: () => { },
    removeParams: () => { },
});
const useRouterContext = () => {
    const { route } = (0, react_router5_1.useRoute)();
    const appRoute = (0, router_1.useApplicationRoute)();
    const meta = (0, react_1.useMemo)(() => {
        return appRoute.meta;
    }, [
        appRoute,
    ]);
    const navigate = (0, react_1.useCallback)((name, params) => {
        params ?
            router_1.default.navigate(name, params) :
            router_1.default.navigate(name);
    }, []);
    const setParams = (0, react_1.useCallback)((params, replace = false) => {
        router_1.default.navigate(route.name, replace ? params : Object.assign({}, route.params, params));
    }, [
        route,
    ]);
    const removeParams = (0, react_1.useCallback)((params) => {
        // reduce the current params and remove the parans list
        const newParams = Object.keys(route.params).reduce((acc, key) => {
            if (params.includes(key))
                return acc;
            acc[key] = route.params[key];
            return acc;
        }, {});
        router_1.default.navigate(route.name, newParams);
    }, [
        route,
    ]);
    const render = (0, react_1.useCallback)(() => {
        return appRoute.render();
    }, [
        appRoute,
    ]);
    const contextValue = (0, react_1.useMemo)(() => ({
        name: route.name,
        params: route.params,
        meta,
        navigate,
        setParams,
        removeParams,
        render,
    }), [
        route.name,
        route.params,
        meta,
        navigate,
        setParams,
        removeParams,
        render,
    ]);
    return contextValue;
};
exports.useRouterContext = useRouterContext;
const RouterContextProvider = ({ children }) => {
    const value = (0, exports.useRouterContext)();
    return ((0, jsx_runtime_1.jsx)(exports.RouterContext.Provider, Object.assign({ value: value }, { children: children })));
};
exports.RouterContextProvider = RouterContextProvider;
//# sourceMappingURL=router.js.map