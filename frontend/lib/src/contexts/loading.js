"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.LoadingContextProvider = exports.useLoadingContext = exports.LoadingContext = void 0;
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
exports.LoadingContext = (0, react_1.createContext)({
    loading: false,
    setLoading: () => { },
});
const useLoadingContext = () => {
    const [loading, setLoading] = (0, react_1.useState)(false);
    const contextValue = (0, react_1.useMemo)(() => ({
        loading,
        setLoading,
    }), [
        loading,
        setLoading,
    ]);
    return contextValue;
};
exports.useLoadingContext = useLoadingContext;
const LoadingContextProvider = ({ children }) => {
    const value = (0, exports.useLoadingContext)();
    return ((0, jsx_runtime_1.jsx)(exports.LoadingContext.Provider, Object.assign({ value: value }, { children: children })));
};
exports.LoadingContextProvider = LoadingContextProvider;
//# sourceMappingURL=loading.js.map