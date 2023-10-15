"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const router_1 = require("./router");
const snackbar_1 = require("./snackbar");
const loading_1 = require("./loading");
const theme_1 = require("./theme");
const account_1 = require("./account");
const AllContextProvider = ({ children }) => {
    return ((0, jsx_runtime_1.jsx)(router_1.RouterContextProvider, { children: (0, jsx_runtime_1.jsx)(snackbar_1.SnackbarContextProvider, { children: (0, jsx_runtime_1.jsx)(loading_1.LoadingContextProvider, { children: (0, jsx_runtime_1.jsx)(theme_1.ThemeProviderWrapper, { children: (0, jsx_runtime_1.jsx)(account_1.AccountContextProvider, { children: children }) }) }) }) }));
};
exports.default = AllContextProvider;
//# sourceMappingURL=all.js.map