"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const react_dom_1 = __importDefault(require("react-dom"));
const App_1 = __importDefault(require("./App"));
const CssBaseline_1 = __importDefault(require("@mui/material/CssBaseline"));
let render = () => {
    react_dom_1.default.render((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsx)(CssBaseline_1.default, {}), (0, jsx_runtime_1.jsx)(App_1.default, {})] }), document.getElementById("root"));
};
render();
//# sourceMappingURL=index.js.map