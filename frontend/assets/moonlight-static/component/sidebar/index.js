import { showErrorPopup } from "../error.js";
let sidebarMounted = false;
let sidebarExtended = false;
const sidebarRoot = document.getElementById("sidebar-root");
const sidebarParent = document.getElementById("sidebar-parent");
const sidebarButton = document.getElementById("sidebar-button");
sidebarButton === null || sidebarButton === void 0 ? void 0 : sidebarButton.addEventListener("click", toggleSidebar);
setSidebarStyle({
    edge: "left"
});
let sidebarComponent = null;
export function setSidebarStyle(style) {
    var _a;
    // Default values
    const edge = (_a = style.edge) !== null && _a !== void 0 ? _a : "left";
    // Set edge
    sidebarRoot === null || sidebarRoot === void 0 ? void 0 : sidebarRoot.classList.remove("sidebar-edge-left", "sidebar-edge-right", "sidebar-edge-up", "sidebar-edge-down");
    sidebarRoot === null || sidebarRoot === void 0 ? void 0 : sidebarRoot.classList.add(`sidebar-edge-${edge}`);
}
export function toggleSidebar() {
    setSidebarExtended(!isSidebarExtended());
}
export function setSidebarExtended(extended) {
    if (extended == sidebarExtended) {
        return;
    }
    if (extended) {
        sidebarRoot === null || sidebarRoot === void 0 ? void 0 : sidebarRoot.classList.add("sidebar-show");
    }
    else {
        sidebarRoot === null || sidebarRoot === void 0 ? void 0 : sidebarRoot.classList.remove("sidebar-show");
    }
    sidebarExtended = extended;
}
export function isSidebarExtended() {
    return sidebarExtended;
}
export function setSidebar(sidebar) {
    if (sidebarParent == null) {
        showErrorPopup("failed to get sidebar");
        return;
    }
    if (sidebarMounted) {
        // unmount
        sidebarComponent === null || sidebarComponent === void 0 ? void 0 : sidebarComponent.unmount(sidebarParent);
        sidebarMounted = false;
    }
    if (sidebar) {
        // mount
        sidebarComponent = sidebar;
        sidebar === null || sidebar === void 0 ? void 0 : sidebar.mount(sidebarParent);
    }
}
export function getSidebarRoot() {
    return sidebarRoot;
}
