"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || function (mod) {
    if (mod && mod.__esModule) return mod;
    var result = {};
    if (mod != null) for (var k in mod) if (k !== "default" && Object.prototype.hasOwnProperty.call(mod, k)) __createBinding(result, mod, k);
    __setModuleDefault(result, mod);
    return result;
};
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const react_router5_1 = require("react-router5");
const all_1 = __importDefault(require("./contexts/all"));
const Layout_1 = __importDefault(require("./pages/Layout"));
const router_1 = __importStar(require("./router"));
function App() {
    return ((0, jsx_runtime_1.jsx)(react_router5_1.RouterProvider, Object.assign({ router: router_1.default }, { children: (0, jsx_runtime_1.jsx)(all_1.default, { children: (0, jsx_runtime_1.jsx)(Layout_1.default, { children: (0, jsx_runtime_1.jsx)(router_1.RenderPage, {}) }) }) })));
}
exports.default = App;
//# sourceMappingURL=App.js.map