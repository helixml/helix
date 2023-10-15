"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const Box_1 = __importDefault(require("@mui/material/Box"));
const Button_1 = __importDefault(require("@mui/material/Button"));
const Add_1 = __importDefault(require("@mui/icons-material/Add"));
const router_1 = __importDefault(require("../router"));
const DataGridWithFilters_1 = __importDefault(require("../components/datagrid/DataGridWithFilters"));
const Job_1 = __importDefault(require("../components/datagrid/Job"));
const useAccount_1 = __importDefault(require("../hooks/useAccount"));
const Jobs = () => {
    const account = (0, useAccount_1.default)();
    if (!account.user)
        return null;
    return ((0, jsx_runtime_1.jsx)(DataGridWithFilters_1.default, { filters: (0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ sx: {
                width: '100%',
            } }, { children: (0, jsx_runtime_1.jsx)(Button_1.default, Object.assign({ sx: {
                    width: '100%',
                }, variant: "contained", color: "secondary", endIcon: (0, jsx_runtime_1.jsx)(Add_1.default, {}), onClick: () => {
                    router_1.default.navigate('/');
                } }, { children: "Create Job" })) })), datagrid: (0, jsx_runtime_1.jsx)(Job_1.default, { jobs: account.jobs, loading: account.initialized ? false : true }) }));
};
exports.default = Jobs;
//# sourceMappingURL=Jobs.js.map