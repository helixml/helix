"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const Box_1 = __importDefault(require("@mui/material/Box"));
const useAccount_1 = __importDefault(require("../hooks/useAccount"));
const DataGridWithFilters_1 = __importDefault(require("../components/datagrid/DataGridWithFilters"));
const Account = () => {
    const account = (0, useAccount_1.default)();
    if (!account.user)
        return null;
    return ((0, jsx_runtime_1.jsx)(DataGridWithFilters_1.default, { datagrid: (0, jsx_runtime_1.jsx)(Box_1.default, { children: "account page" }) }));
};
exports.default = Account;
//# sourceMappingURL=Account.js.map