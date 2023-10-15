"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.SnackbarContextProvider = exports.useSnackbarContext = exports.SnackbarContext = void 0;
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
exports.SnackbarContext = (0, react_1.createContext)({
    setSnackbar: () => { },
});
const useSnackbarContext = () => {
    const [snackbar, setRawSnackbar] = (0, react_1.useState)();
    const setSnackbar = (0, react_1.useCallback)((message, severity) => {
        if (!message) {
            setRawSnackbar(undefined);
        }
        else {
            setRawSnackbar({
                message,
                severity: severity || 'info',
            });
        }
    }, []);
    const contextValue = (0, react_1.useMemo)(() => ({
        snackbar,
        setSnackbar,
    }), [
        snackbar,
        setSnackbar,
    ]);
    return contextValue;
};
exports.useSnackbarContext = useSnackbarContext;
const SnackbarContextProvider = ({ children }) => {
    const value = (0, exports.useSnackbarContext)();
    return ((0, jsx_runtime_1.jsx)(exports.SnackbarContext.Provider, Object.assign({ value: value }, { children: children })));
};
exports.SnackbarContextProvider = SnackbarContextProvider;
//# sourceMappingURL=snackbar.js.map