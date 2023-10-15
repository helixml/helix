"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.RenderPage = exports.useApplicationRoute = exports.router = exports.NOT_FOUND_ROUTE = void 0;
const jsx_runtime_1 = require("react/jsx-runtime");
const router5_1 = __importDefault(require("router5"));
const react_router5_1 = require("react-router5");
const router5_plugin_browser_1 = __importDefault(require("router5-plugin-browser"));
const Home_1 = __importDefault(require("./pages/Home"));
const Jobs_1 = __importDefault(require("./pages/Jobs"));
const Files_1 = __importDefault(require("./pages/Files"));
const Account_1 = __importDefault(require("./pages/Account"));
exports.NOT_FOUND_ROUTE = {
    name: 'notfound',
    path: '/notfound',
    meta: {
        title: 'Page Not Found',
    },
    render: () => (0, jsx_runtime_1.jsx)("div", { children: "Page Not Found" }),
};
const routes = [{
        name: 'home',
        path: '/',
        meta: {
            title: 'Home',
        },
        render: () => (0, jsx_runtime_1.jsx)(Home_1.default, {}),
    }, {
        name: 'jobs',
        path: '/jobs',
        meta: {
            title: 'Jobs',
        },
        render: () => (0, jsx_runtime_1.jsx)(Jobs_1.default, {}),
    }, {
        name: 'files',
        path: '/files',
        meta: {
            title: 'Files',
        },
        render: () => (0, jsx_runtime_1.jsx)(Files_1.default, {}),
    }, {
        name: 'account',
        path: '/account',
        meta: {
            title: 'Account',
        },
        render: () => (0, jsx_runtime_1.jsx)(Account_1.default, {}),
    }, exports.NOT_FOUND_ROUTE];
exports.router = (0, router5_1.default)(routes, {
    defaultRoute: 'notfound',
    queryParamsMode: 'loose',
});
exports.router.usePlugin((0, router5_plugin_browser_1.default)());
exports.router.start();
function useApplicationRoute() {
    const { route } = (0, react_router5_1.useRoute)();
    const fullRoute = routes.find(r => r.name == (route === null || route === void 0 ? void 0 : route.name)) || exports.NOT_FOUND_ROUTE;
    return fullRoute;
}
exports.useApplicationRoute = useApplicationRoute;
function RenderPage() {
    const route = useApplicationRoute();
    return route.render();
}
exports.RenderPage = RenderPage;
exports.default = exports.router;
//# sourceMappingURL=router.js.map