"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.useSnackbar = void 0;
const react_1 = require("react");
const snackbar_1 = require("../contexts/snackbar");
const useSnackbar = () => {
    const snackbar = (0, react_1.useContext)(snackbar_1.SnackbarContext);
    const error = (0, react_1.useCallback)((message) => {
        snackbar.setSnackbar(message, 'error');
    }, []);
    const info = (0, react_1.useCallback)((message) => {
        snackbar.setSnackbar(message, 'info');
    }, []);
    const success = (0, react_1.useCallback)((message) => {
        snackbar.setSnackbar(message, 'success');
    }, []);
    return {
        error,
        info,
        success,
    };
};
exports.useSnackbar = useSnackbar;
exports.default = exports.useSnackbar;
//# sourceMappingURL=useSnackbar.js.map