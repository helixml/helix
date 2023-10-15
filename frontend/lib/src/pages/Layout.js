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
const react_1 = __importStar(require("react"));
const styles_1 = require("@mui/material/styles");
const useMediaQuery_1 = __importDefault(require("@mui/material/useMediaQuery"));
const CssBaseline_1 = __importDefault(require("@mui/material/CssBaseline"));
const Drawer_1 = __importDefault(require("@mui/material/Drawer"));
const Box_1 = __importDefault(require("@mui/material/Box"));
const AppBar_1 = __importDefault(require("@mui/material/AppBar"));
const Toolbar_1 = __importDefault(require("@mui/material/Toolbar"));
const Typography_1 = __importDefault(require("@mui/material/Typography"));
const Divider_1 = __importDefault(require("@mui/material/Divider"));
const Container_1 = __importDefault(require("@mui/material/Container"));
const List_1 = __importDefault(require("@mui/material/List"));
const ListItem_1 = __importDefault(require("@mui/material/ListItem"));
const ListItemButton_1 = __importDefault(require("@mui/material/ListItemButton"));
const ListItemIcon_1 = __importDefault(require("@mui/material/ListItemIcon"));
const ListItemText_1 = __importDefault(require("@mui/material/ListItemText"));
const Link_1 = __importDefault(require("@mui/material/Link"));
const Button_1 = __importDefault(require("@mui/material/Button"));
const IconButton_1 = __importDefault(require("@mui/material/IconButton"));
const MenuItem_1 = __importDefault(require("@mui/material/MenuItem"));
const Menu_1 = __importDefault(require("@mui/material/Menu"));
const Dashboard_1 = __importDefault(require("@mui/icons-material/Dashboard"));
const Login_1 = __importDefault(require("@mui/icons-material/Login"));
const Logout_1 = __importDefault(require("@mui/icons-material/Logout"));
const CloudUpload_1 = __importDefault(require("@mui/icons-material/CloudUpload"));
const Menu_2 = __importDefault(require("@mui/icons-material/Menu"));
const AccountCircle_1 = __importDefault(require("@mui/icons-material/AccountCircle"));
const AccountBox_1 = __importDefault(require("@mui/icons-material/AccountBox"));
const List_2 = __importDefault(require("@mui/icons-material/List"));
const useRouter_1 = __importDefault(require("../hooks/useRouter"));
const useAccount_1 = __importDefault(require("../hooks/useAccount"));
const Snackbar_1 = __importDefault(require("../components/system/Snackbar"));
const GlobalLoading_1 = __importDefault(require("../components/system/GlobalLoading"));
const useThemeConfig_1 = __importDefault(require("../hooks/useThemeConfig"));
const drawerWidth = 280;
const Logo = (0, styles_1.styled)('img')({
    height: '50px',
});
const AppBar = (0, styles_1.styled)(AppBar_1.default, {
    shouldForwardProp: (prop) => prop !== 'open',
})(({ theme, open }) => (Object.assign({ zIndex: theme.zIndex.drawer + 1, transition: theme.transitions.create(['width', 'margin'], {
        easing: theme.transitions.easing.sharp,
        duration: theme.transitions.duration.leavingScreen,
    }) }, (open && {
    marginLeft: drawerWidth,
    width: `calc(100% - ${drawerWidth}px)`,
    transition: theme.transitions.create(['width', 'margin'], {
        easing: theme.transitions.easing.sharp,
        duration: theme.transitions.duration.enteringScreen,
    }),
}))));
const Drawer = (0, styles_1.styled)(Drawer_1.default, { shouldForwardProp: (prop) => prop !== 'open' })(({ theme, open }) => ({
    '& .MuiDrawer-paper': Object.assign({ position: 'relative', whiteSpace: 'nowrap', width: drawerWidth, transition: theme.transitions.create('width', {
            easing: theme.transitions.easing.sharp,
            duration: theme.transitions.duration.enteringScreen,
        }), boxSizing: 'border-box' }, (!open && {
        overflowX: 'hidden',
        transition: theme.transitions.create('width', {
            easing: theme.transitions.easing.sharp,
            duration: theme.transitions.duration.leavingScreen,
        }),
        width: theme.spacing(7),
        [theme.breakpoints.up('sm')]: {
            width: theme.spacing(9),
        },
    })),
}));
const Layout = ({ children, }) => {
    const account = (0, useAccount_1.default)();
    const { name, meta, navigate, } = (0, useRouter_1.default)();
    const [accountMenuAnchorEl, setAccountMenuAnchorEl] = react_1.default.useState(null);
    const [mobileOpen, setMobileOpen] = (0, react_1.useState)(false);
    const theme = (0, styles_1.useTheme)();
    const themeConfig = (0, useThemeConfig_1.default)();
    const bigScreen = (0, useMediaQuery_1.default)(theme.breakpoints.up('md'));
    const handleAccountMenu = (event) => {
        setAccountMenuAnchorEl(event.currentTarget);
    };
    const handleCloseAccountMenu = () => {
        setAccountMenuAnchorEl(null);
    };
    const handleDrawerToggle = () => {
        setMobileOpen(!mobileOpen);
    };
    const drawer = ((0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsx)(Toolbar_1.default, Object.assign({ sx: {
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'flex-start',
                    px: [1],
                } }, { children: themeConfig.logo() })), (0, jsx_runtime_1.jsx)(Divider_1.default, {}), (0, jsx_runtime_1.jsx)(List_1.default, { children: account.user ? ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsx)(ListItem_1.default, Object.assign({ disablePadding: true, onClick: () => {
                                navigate('home');
                                setMobileOpen(false);
                            } }, { children: (0, jsx_runtime_1.jsxs)(ListItemButton_1.default, Object.assign({ selected: name == 'home' }, { children: [(0, jsx_runtime_1.jsx)(ListItemIcon_1.default, { children: (0, jsx_runtime_1.jsx)(Dashboard_1.default, { color: "primary" }) }), (0, jsx_runtime_1.jsx)(ListItemText_1.default, { primary: "Modules" })] })) })), (0, jsx_runtime_1.jsx)(ListItem_1.default, Object.assign({ disablePadding: true, onClick: () => {
                                navigate('jobs');
                                setMobileOpen(false);
                            } }, { children: (0, jsx_runtime_1.jsxs)(ListItemButton_1.default, Object.assign({ selected: name == 'jobs' }, { children: [(0, jsx_runtime_1.jsx)(ListItemIcon_1.default, { children: (0, jsx_runtime_1.jsx)(List_2.default, { color: "primary" }) }), (0, jsx_runtime_1.jsx)(ListItemText_1.default, { primary: "Jobs" })] })) })), (0, jsx_runtime_1.jsx)(ListItem_1.default, Object.assign({ disablePadding: true, onClick: () => {
                                navigate('files');
                                setMobileOpen(false);
                            } }, { children: (0, jsx_runtime_1.jsxs)(ListItemButton_1.default, Object.assign({ selected: name == 'files' }, { children: [(0, jsx_runtime_1.jsx)(ListItemIcon_1.default, { children: (0, jsx_runtime_1.jsx)(CloudUpload_1.default, { color: "primary" }) }), (0, jsx_runtime_1.jsx)(ListItemText_1.default, { primary: "Files" })] })) })), (0, jsx_runtime_1.jsx)(ListItem_1.default, Object.assign({ disablePadding: true, onClick: () => {
                                navigate('account');
                                setMobileOpen(false);
                            } }, { children: (0, jsx_runtime_1.jsxs)(ListItemButton_1.default, Object.assign({ selected: name == 'account' }, { children: [(0, jsx_runtime_1.jsx)(ListItemIcon_1.default, { children: (0, jsx_runtime_1.jsx)(AccountBox_1.default, { color: "primary" }) }), (0, jsx_runtime_1.jsx)(ListItemText_1.default, { primary: "Account" })] })) })), (0, jsx_runtime_1.jsx)(Divider_1.default, {}), (0, jsx_runtime_1.jsx)(ListItem_1.default, Object.assign({ disablePadding: true, onClick: () => {
                                account.onLogout();
                                setMobileOpen(false);
                            } }, { children: (0, jsx_runtime_1.jsxs)(ListItemButton_1.default, { children: [(0, jsx_runtime_1.jsx)(ListItemIcon_1.default, { children: (0, jsx_runtime_1.jsx)(Logout_1.default, { color: "primary" }) }), (0, jsx_runtime_1.jsx)(ListItemText_1.default, { primary: "Logout" })] }) }))] })) : ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsx)(ListItem_1.default, Object.assign({ disablePadding: true, onClick: () => {
                                navigate('home');
                                setMobileOpen(false);
                            } }, { children: (0, jsx_runtime_1.jsxs)(ListItemButton_1.default, Object.assign({ selected: name == 'home' }, { children: [(0, jsx_runtime_1.jsx)(ListItemIcon_1.default, { children: (0, jsx_runtime_1.jsx)(Dashboard_1.default, { color: "primary" }) }), (0, jsx_runtime_1.jsx)(ListItemText_1.default, { primary: "Modules" })] })) })), (0, jsx_runtime_1.jsx)(Divider_1.default, {}), (0, jsx_runtime_1.jsx)(ListItem_1.default, Object.assign({ disablePadding: true, onClick: () => {
                                account.onLogin();
                                setMobileOpen(false);
                            } }, { children: (0, jsx_runtime_1.jsxs)(ListItemButton_1.default, { children: [(0, jsx_runtime_1.jsx)(ListItemIcon_1.default, { children: (0, jsx_runtime_1.jsx)(Login_1.default, { color: "primary" }) }), (0, jsx_runtime_1.jsx)(ListItemText_1.default, { primary: "Login" })] }) }))] })) })] }));
    const container = window !== undefined ? () => document.body : undefined;
    return ((0, jsx_runtime_1.jsxs)(Box_1.default, Object.assign({ sx: { display: 'flex' }, component: "div" }, { children: [(0, jsx_runtime_1.jsx)(CssBaseline_1.default, {}), (0, jsx_runtime_1.jsx)(AppBar, Object.assign({ elevation: 0, position: "fixed", open: true, color: "default", sx: {
                    height: '64px',
                    width: { xs: '100%', sm: '100%', md: `calc(100% - ${drawerWidth}px)` },
                    ml: { xs: '0px', sm: '0px', md: `${drawerWidth}px` },
                } }, { children: (0, jsx_runtime_1.jsxs)(Toolbar_1.default, Object.assign({ sx: {
                        pr: '24px',
                        backgroundColor: '#fff'
                    } }, { children: [bigScreen ? ((0, jsx_runtime_1.jsx)(Typography_1.default, Object.assign({ component: "h1", variant: "h6", color: "inherit", noWrap: true, sx: {
                                flexGrow: 1,
                                ml: 1,
                                color: 'text.primary',
                            } }, { children: meta.title || '' }))) : ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsx)(IconButton_1.default, Object.assign({ color: "inherit", "aria-label": "open drawer", edge: "start", onClick: handleDrawerToggle, sx: {
                                        mr: 1,
                                        ml: 1,
                                    } }, { children: (0, jsx_runtime_1.jsx)(Menu_2.default, {}) })), themeConfig.logo()] })), (0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ sx: {
                                display: 'flex',
                                flexDirection: 'row',
                                alignItems: 'center',
                            } }, { children: account.user ? ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsxs)(Typography_1.default, Object.assign({ variant: "caption" }, { children: ["Signed in as ", account.user.email, " (", account.credits, " credits)"] })), (0, jsx_runtime_1.jsx)(IconButton_1.default, Object.assign({ size: "large", "aria-label": "account of current user", "aria-controls": "menu-appbar", "aria-haspopup": "true", onClick: handleAccountMenu, color: "inherit" }, { children: (0, jsx_runtime_1.jsx)(AccountCircle_1.default, {}) })), (0, jsx_runtime_1.jsxs)(Menu_1.default, Object.assign({ id: "menu-appbar", anchorEl: accountMenuAnchorEl, anchorOrigin: {
                                            vertical: 'top',
                                            horizontal: 'right',
                                        }, keepMounted: true, transformOrigin: {
                                            vertical: 'top',
                                            horizontal: 'right',
                                        }, open: Boolean(accountMenuAnchorEl), onClose: handleCloseAccountMenu }, { children: [(0, jsx_runtime_1.jsxs)(MenuItem_1.default, Object.assign({ onClick: () => {
                                                    handleCloseAccountMenu();
                                                    navigate('account');
                                                } }, { children: [(0, jsx_runtime_1.jsx)(ListItemIcon_1.default, { children: (0, jsx_runtime_1.jsx)(AccountBox_1.default, { fontSize: "small" }) }), "My account"] })), (0, jsx_runtime_1.jsxs)(MenuItem_1.default, Object.assign({ onClick: () => {
                                                    handleCloseAccountMenu();
                                                    account.onLogout();
                                                } }, { children: [(0, jsx_runtime_1.jsx)(ListItemIcon_1.default, { children: (0, jsx_runtime_1.jsx)(Logout_1.default, { fontSize: "small" }) }), "Logout"] }))] }))] })) : ((0, jsx_runtime_1.jsx)(jsx_runtime_1.Fragment, { children: (0, jsx_runtime_1.jsx)(Button_1.default, Object.assign({ variant: "outlined", endIcon: (0, jsx_runtime_1.jsx)(Login_1.default, {}), onClick: () => {
                                        account.onLogin();
                                    } }, { children: "Login" })) })) }))] })) })), (0, jsx_runtime_1.jsx)(Drawer_1.default, Object.assign({ container: container, variant: "temporary", open: mobileOpen, onClose: handleDrawerToggle, ModalProps: {
                    keepMounted: true, // Better open performance on mobile.
                }, sx: {
                    display: { sm: 'block', md: 'none' },
                    '& .MuiDrawer-paper': { boxSizing: 'border-box', width: drawerWidth },
                } }, { children: drawer })), (0, jsx_runtime_1.jsx)(Drawer, Object.assign({ variant: "permanent", sx: {
                    display: { xs: 'none', md: 'block' },
                    '& .MuiDrawer-paper': { boxSizing: 'border-box', width: drawerWidth },
                }, open: true }, { children: drawer })), (0, jsx_runtime_1.jsxs)(Box_1.default, Object.assign({ component: "main", sx: {
                    backgroundColor: (theme) => theme.palette.mode === 'light'
                        ? theme.palette.grey[100]
                        : theme.palette.grey[900],
                    flexGrow: 1,
                    height: '100vh',
                    overflow: 'auto',
                    display: 'flex',
                    flexDirection: 'column',
                } }, { children: [(0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ component: "div", sx: {
                            flexGrow: 0,
                            borderBottom: '1px solid rgba(0, 0, 0, 0.12)',
                        } }, { children: (0, jsx_runtime_1.jsx)(Toolbar_1.default, {}) })), (0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ component: "div", sx: {
                            flexGrow: 1,
                            py: 1,
                            px: 2,
                        } }, { children: children })), (0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ className: 'footer', component: "div", sx: {
                            flexGrow: 0,
                            backgroundColor: 'transparent',
                        } }, { children: (0, jsx_runtime_1.jsx)(Container_1.default, Object.assign({ maxWidth: 'xl', sx: { height: '5vh' } }, { children: (0, jsx_runtime_1.jsxs)(Typography_1.default, Object.assign({ variant: "body2", color: "text.secondary", align: "center" }, { children: ['Copyright © ', (0, jsx_runtime_1.jsx)(Link_1.default, Object.assign({ color: "inherit", href: themeConfig.url }, { children: themeConfig.company })), ' ', new Date().getFullYear(), '.'] })) })) }))] })), (0, jsx_runtime_1.jsx)(Snackbar_1.default, {}), (0, jsx_runtime_1.jsx)(GlobalLoading_1.default, {})] })));
};
exports.default = Layout;
//# sourceMappingURL=Layout.js.map