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
exports.useLoadingErrorHandler = void 0;
const useLoading_1 = __importDefault(require("./useLoading"));
const useSnackbar_1 = __importDefault(require("./useSnackbar"));
const useLoadingErrorHandler = ({ withSnackbar = true, withLoading = true, } = {}) => {
    const loading = (0, useLoading_1.default)();
    const snackbar = (0, useSnackbar_1.default)();
    return (handler) => {
        return () => __awaiter(void 0, void 0, void 0, function* () {
            let sawError = false;
            if (withLoading)
                loading.setLoading(true);
            try {
                yield handler();
            }
            catch (e) {
                sawError = true;
                // if(e.response) console.error(e.response.body)
                // if(withSnackbar) snackbar.error(e.toString())
            }
            if (withLoading)
                loading.setLoading(false);
            return sawError;
        });
    };
};
exports.useLoadingErrorHandler = useLoadingErrorHandler;
exports.default = exports.useLoadingErrorHandler;
//# sourceMappingURL=useLoadingErrorHandler.js.map