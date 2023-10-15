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
exports.usePollingApiData = void 0;
const react_1 = require("react");
const bluebird_1 = __importDefault(require("bluebird"));
const useApi_1 = __importDefault(require("./useApi"));
const debug_1 = require("../utils/debug");
function usePollingApiData({ url, defaultValue, active = true, reload = false, reloadInterval = 5000, jsonStringifyComparison = true, onChange = () => { }, }) {
    const api = (0, useApi_1.default)();
    const [data, setData] = (0, react_1.useState)(defaultValue);
    const fetchData = (0, react_1.useCallback)(() => __awaiter(this, void 0, void 0, function* () {
        const apiData = yield api.get(url);
        if (apiData === null)
            return;
        // only update the state if the data is different
        // this prevents re-renders whilst loading data in a loop
        setData(currentValue => {
            if (!jsonStringifyComparison)
                return apiData;
            const hasChanged = JSON.stringify(apiData) != JSON.stringify(currentValue);
            if (hasChanged && onChange)
                onChange(apiData);
            return hasChanged ?
                apiData :
                currentValue;
        });
    }), [
        url,
    ]);
    (0, react_1.useEffect)(() => {
        if (!active)
            return;
        if (!reload) {
            fetchData();
            return;
        }
        let loading = true;
        const doLoop = () => __awaiter(this, void 0, void 0, function* () {
            while (loading) {
                yield fetchData();
                yield bluebird_1.default.delay(reloadInterval);
            }
        });
        doLoop();
        return () => {
            loading = false;
        };
    }, [
        active,
        url,
    ]);
    (0, react_1.useEffect)(() => {
        (0, debug_1.logger)('useApiData', url, data);
    }, [
        data,
    ]);
    return [data, fetchData];
}
exports.usePollingApiData = usePollingApiData;
exports.default = usePollingApiData;
//# sourceMappingURL=usePollingApiData.js.map