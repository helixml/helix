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
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const axios_1 = __importDefault(require("axios"));
const Button_1 = __importDefault(require("@mui/material/Button"));
const TextField_1 = __importDefault(require("@mui/material/TextField"));
const Typography_1 = __importDefault(require("@mui/material/Typography"));
const Grid_1 = __importDefault(require("@mui/material/Grid"));
const Container_1 = __importDefault(require("@mui/material/Container"));
const Dashboard = () => {
    const [loading, setLoading] = (0, react_1.useState)(false);
    const [inputValue, setInputValue] = (0, react_1.useState)('');
    const handleInputChange = (event) => {
        setInputValue(event.target.value);
    };
    const runJob = (0, react_1.useCallback)(() => __awaiter(void 0, void 0, void 0, function* () {
        setLoading(true);
        try {
            const statusResult = yield axios_1.default.post('/api/v1/jobs', {
                module: 'cowsay:v0.0.1',
                inputs: {
                    Message: inputValue,
                }
            });
            console.log('--------------------------------------------');
            console.dir(statusResult.data);
        }
        catch (e) {
            alert(e.message);
        }
        setLoading(false);
    }), [
        inputValue
    ]);
    return ((0, jsx_runtime_1.jsx)(Container_1.default, Object.assign({ maxWidth: 'xl', sx: { mt: 4, mb: 4 } }, { children: (0, jsx_runtime_1.jsxs)(Grid_1.default, Object.assign({ container: true, spacing: 3 }, { children: [(0, jsx_runtime_1.jsx)(Grid_1.default, Object.assign({ item: true, xs: 12, md: 12 }, { children: (0, jsx_runtime_1.jsx)(Typography_1.default, { children: "Run cowsay..." }) })), (0, jsx_runtime_1.jsx)(Grid_1.default, Object.assign({ item: true, xs: 12, md: 12 }, { children: (0, jsx_runtime_1.jsx)(TextField_1.default, { fullWidth: true, label: "Type something here", value: inputValue, disabled: loading, onChange: handleInputChange }) })), (0, jsx_runtime_1.jsx)(Grid_1.default, Object.assign({ item: true, xs: 12, md: 12 }, { children: (0, jsx_runtime_1.jsx)(Button_1.default, Object.assign({ variant: 'contained', disabled: loading, onClick: runJob }, { children: "Run" })) }))] })) })));
};
exports.default = Dashboard;
//# sourceMappingURL=Home.js.map