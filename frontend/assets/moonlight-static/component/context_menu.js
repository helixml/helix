import { showErrorPopup } from "./error.js";
import { ListComponent } from "./list.js";
document.addEventListener("click", () => removeContextMenu());
const contextMenuElement = document.getElementById("context-menu");
const contextMenuList = new ListComponent([], {
    listClasses: ["context-menu-list"]
});
export function setContextMenu(event, init) {
    var _a;
    event.preventDefault();
    event.stopPropagation();
    if (contextMenuElement == null) {
        showErrorPopup("cannot find the context menu element");
        return;
    }
    contextMenuElement.style.setProperty("left", `${event.pageX}px`);
    contextMenuElement.style.setProperty("top", `${event.pageY}px`);
    contextMenuList.clear();
    for (const element of (_a = init === null || init === void 0 ? void 0 : init.elements) !== null && _a !== void 0 ? _a : []) {
        contextMenuList.append(new ContextMenuElementComponent(element));
    }
    contextMenuList.mount(contextMenuElement);
    contextMenuElement.classList.remove("context-menu-disabled");
}
export function removeContextMenu() {
    if (contextMenuElement == null) {
        showErrorPopup("cannot find the context menu element");
        return;
    }
    contextMenuElement.classList.add("context-menu-disabled");
}
class ContextMenuElementComponent {
    constructor(element) {
        this.nameElement = document.createElement("p");
        this.nameElement.innerText = element.name;
        this.nameElement.classList.add("context-menu-element");
        this.nameElement.addEventListener("click", event => {
            element.callback(event);
        });
    }
    mount(parent) {
        parent.appendChild(this.nameElement);
    }
    unmount(parent) {
        parent.removeChild(this.nameElement);
    }
}
