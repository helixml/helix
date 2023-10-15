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
Object.defineProperty(exports, "__esModule", { value: true });
exports.useErrorCallback = exports.extractErrorMessage = void 0;
const react_1 = require("react");
const snackbar_1 = require("../contexts/snackbar");
const extractErrorMessage = (error) => {
    if (error.response && error.response.data && (error.response.data.message || error.response.data.error)) {
        return error.response.data.message || error.response.data.error;
    }
    else {
        return error.toString();
    }
};
exports.extractErrorMessage = extractErrorMessage;
function useErrorCallback(handler, snackbarActive = true) {
    const snackbar = (0, react_1.useContext)(snackbar_1.SnackbarContext);
    const callback = (0, react_1.useCallback)(() => __awaiter(this, void 0, void 0, function* () {
        try {
            const result = yield handler();
            return result;
        }
        catch (e) {
            const errorMessage = (0, exports.extractErrorMessage)(e);
            console.error(errorMessage);
            if (snackbarActive !== false)
                snackbar.setSnackbar(errorMessage, 'error');
        }
        return;
    }), [
        handler,
        snackbarActive,
    ]);
    return callback;
}
exports.useErrorCallback = useErrorCallback;
exports.default = useErrorCallback;
//# sourceMappingURL=useErrorCallback.js.map