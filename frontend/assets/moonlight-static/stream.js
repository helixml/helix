var __awaiter = (this && this.__awaiter) || function (thisArg, _arguments, P, generator) {
    function adopt(value) { return value instanceof P ? value : new P(function (resolve) { resolve(value); }); }
    return new (P || (P = Promise))(function (resolve, reject) {
        function fulfilled(value) { try { step(generator.next(value)); } catch (e) { reject(e); } }
        function rejected(value) { try { step(generator["throw"](value)); } catch (e) { reject(e); } }
        function step(result) { result.done ? resolve(result.value) : adopt(result.value).then(fulfilled, rejected); }
        step((generator = generator.apply(thisArg, _arguments || [])).next());
    });
};
import { getApi } from "./api.js";
import { showErrorPopup } from "./component/error.js";
import { getStreamerSize, Stream } from "./stream/index.js";
import { getModalBackground, showMessage, showModal } from "./component/modal/index.js";
import { getSidebarRoot, setSidebar, setSidebarExtended, setSidebarStyle } from "./component/sidebar/index.js";
import { defaultStreamInputConfig } from "./stream/input.js";
import { defaultStreamSettings, getLocalStreamSettings } from "./component/settings_menu.js";
import { SelectComponent } from "./component/input.js";
import { getStandardVideoFormats, getSupportedVideoFormats } from "./stream/video.js";
import { StreamKeys } from "./api_bindings.js";
import { ScreenKeyboard } from "./screen_keyboard.js";
import { FormModal } from "./component/modal/form.js";
function startApp() {
    return __awaiter(this, void 0, void 0, function* () {
        const api = yield getApi();
        const rootElement = document.getElementById("root");
        if (rootElement == null) {
            showErrorPopup("couldn't find root element", true);
            return;
        }
        // Get Host and App via Query
        const queryParams = new URLSearchParams(location.search);
        const hostIdStr = queryParams.get("hostId");
        const appIdStr = queryParams.get("appId");
        if (hostIdStr == null || appIdStr == null) {
            yield showMessage("No Host or no App Id found");
            window.close();
            return;
        }
        const hostId = Number.parseInt(hostIdStr);
        const appId = Number.parseInt(appIdStr);
        // event propagation on overlays
        const sidebarRoot = getSidebarRoot();
        if (sidebarRoot) {
            stopPropagationOn(sidebarRoot);
        }
        const modalBackground = getModalBackground();
        if (modalBackground) {
            stopPropagationOn(modalBackground);
        }
        // Start and Mount App
        const app = new ViewerApp(api, hostId, appId);
        app.mount(rootElement);
    });
}
// Prevent starting transition
window.requestAnimationFrame(() => {
    var _a;
    // Note: elements is a live array
    const elements = document.getElementsByClassName("prevent-start-transition");
    while (elements.length > 0) {
        (_a = elements.item(0)) === null || _a === void 0 ? void 0 : _a.classList.remove("prevent-start-transition");
    }
});
startApp();
class ViewerApp {
    constructor(api, hostId, appId) {
        var _a;
        this.div = document.createElement("div");
        this.videoElement = document.createElement("video");
        this.stream = null;
        this.api = api;
        // Configure sidebar
        this.sidebar = new ViewerSidebar(this);
        setSidebar(this.sidebar);
        // Configure stream
        const settings = (_a = getLocalStreamSettings()) !== null && _a !== void 0 ? _a : defaultStreamSettings();
        let browserWidth = Math.max(document.documentElement.clientWidth || 0, window.innerWidth || 0);
        let browserHeight = Math.max(document.documentElement.clientHeight || 0, window.innerHeight || 0);
        this.startStream(hostId, appId, settings, [browserWidth, browserHeight]);
        this.streamerSize = getStreamerSize(settings, [browserWidth, browserHeight]);
        // Configure video element
        this.videoElement.classList.add("video-stream");
        this.videoElement.preload = "none";
        this.videoElement.controls = false;
        this.videoElement.autoplay = true;
        this.videoElement.disablePictureInPicture = true;
        this.videoElement.playsInline = true;
        this.videoElement.muted = true;
        this.videoElement.tabIndex = 0;
        this.div.tabIndex = 0;
        this.div.appendChild(this.videoElement);
        // Configure input
        document.addEventListener("keydown", this.onKeyDown.bind(this), { passive: false });
        document.addEventListener("keyup", this.onKeyUp.bind(this), { passive: false });
        document.addEventListener("mousedown", this.onMouseButtonDown.bind(this), { passive: false });
        document.addEventListener("mouseup", this.onMouseButtonUp.bind(this), { passive: false });
        document.addEventListener("mousemove", this.onMouseMove.bind(this), { passive: false });
        document.addEventListener("wheel", this.onMouseWheel.bind(this), { passive: false });
        document.addEventListener("contextmenu", this.onContextMenu.bind(this), { passive: false });
        document.addEventListener("touchstart", this.onTouchStart.bind(this), { passive: false });
        document.addEventListener("touchend", this.onTouchEnd.bind(this), { passive: false });
        document.addEventListener("touchcancel", this.onTouchCancel.bind(this), { passive: false });
        document.addEventListener("touchmove", this.onTouchMove.bind(this), { passive: false });
        window.addEventListener("gamepadconnected", this.onGamepadConnect.bind(this));
        window.addEventListener("gamepaddisconnected", this.onGamepadDisconnect.bind(this));
        // Connect all gamepads
        for (const gamepad of navigator.getGamepads()) {
            if (gamepad != null) {
                this.onGamepadAdd(gamepad);
            }
        }
    }
    startStream(hostId, appId, settings, browserSize) {
        return __awaiter(this, void 0, void 0, function* () {
            setSidebarStyle({
                edge: settings.sidebarEdge,
            });
            let supportedVideoFormats = getStandardVideoFormats();
            if (settings.dontForceH264) {
                supportedVideoFormats = yield getSupportedVideoFormats();
            }
            this.stream = new Stream(this.api, hostId, appId, settings, supportedVideoFormats, browserSize);
            // Add app info listener
            this.stream.addInfoListener(this.onInfo.bind(this));
            // Create connection info modal
            const connectionInfo = new ConnectionInfoModal();
            this.stream.addInfoListener(connectionInfo.onInfo.bind(connectionInfo));
            showModal(connectionInfo);
            // Set video
            this.videoElement.srcObject = this.stream.getMediaStream();
            // Start animation frame loop
            this.onTouchUpdate();
            this.onGamepadUpdate();
            this.stream.getInput().addScreenKeyboardVisibleEvent(this.onScreenKeyboardSetVisible.bind(this));
        });
    }
    onInfo(event) {
        return __awaiter(this, void 0, void 0, function* () {
            const data = event.detail;
            if (data.type == "app") {
                const app = data.app;
                document.title = `Stream: ${app.title}`;
            }
            else if (data.type == "connectionComplete") {
                this.sidebar.onCapabilitiesChange(data.capabilities);
            }
        });
    }
    onUserInteraction() {
        this.videoElement.muted = false;
    }
    onScreenKeyboardSetVisible(event) {
        console.info(event.detail);
        const screenKeyboard = this.sidebar.getScreenKeyboard();
        const newShown = event.detail.visible;
        if (newShown != screenKeyboard.isVisible()) {
            if (newShown) {
                screenKeyboard.show();
            }
            else {
                screenKeyboard.hide();
            }
        }
    }
    // Keyboard
    onKeyDown(event) {
        var _a;
        this.onUserInteraction();
        event.preventDefault();
        (_a = this.stream) === null || _a === void 0 ? void 0 : _a.getInput().onKeyDown(event);
    }
    onKeyUp(event) {
        var _a;
        this.onUserInteraction();
        event.preventDefault();
        (_a = this.stream) === null || _a === void 0 ? void 0 : _a.getInput().onKeyUp(event);
    }
    // Mouse
    onMouseButtonDown(event) {
        var _a;
        this.onUserInteraction();
        event.preventDefault();
        (_a = this.stream) === null || _a === void 0 ? void 0 : _a.getInput().onMouseDown(event, this.getStreamRect());
    }
    onMouseButtonUp(event) {
        var _a;
        this.onUserInteraction();
        event.preventDefault();
        (_a = this.stream) === null || _a === void 0 ? void 0 : _a.getInput().onMouseUp(event);
    }
    onMouseMove(event) {
        var _a;
        event.preventDefault();
        (_a = this.stream) === null || _a === void 0 ? void 0 : _a.getInput().onMouseMove(event, this.getStreamRect());
    }
    onMouseWheel(event) {
        var _a;
        event.preventDefault();
        (_a = this.stream) === null || _a === void 0 ? void 0 : _a.getInput().onMouseWheel(event);
    }
    onContextMenu(event) {
        event.preventDefault();
    }
    // Touch
    onTouchStart(event) {
        var _a;
        this.onUserInteraction();
        event.preventDefault();
        (_a = this.stream) === null || _a === void 0 ? void 0 : _a.getInput().onTouchStart(event, this.getStreamRect());
    }
    onTouchEnd(event) {
        var _a;
        this.onUserInteraction();
        event.preventDefault();
        (_a = this.stream) === null || _a === void 0 ? void 0 : _a.getInput().onTouchEnd(event, this.getStreamRect());
    }
    onTouchCancel(event) {
        var _a;
        this.onUserInteraction();
        event === null || event === void 0 ? void 0 : event.preventDefault();
        (_a = this.stream) === null || _a === void 0 ? void 0 : _a.getInput().onTouchCancel(event, this.getStreamRect());
    }
    onTouchUpdate() {
        var _a;
        (_a = this.stream) === null || _a === void 0 ? void 0 : _a.getInput().onTouchUpdate(this.getStreamRect());
        window.requestAnimationFrame(this.onTouchUpdate.bind(this));
    }
    onTouchMove(event) {
        var _a;
        event.preventDefault();
        (_a = this.stream) === null || _a === void 0 ? void 0 : _a.getInput().onTouchMove(event, this.getStreamRect());
    }
    // Gamepad
    onGamepadConnect(event) {
        this.onGamepadAdd(event.gamepad);
    }
    onGamepadAdd(gamepad) {
        var _a;
        (_a = this.stream) === null || _a === void 0 ? void 0 : _a.getInput().onGamepadConnect(gamepad);
    }
    onGamepadDisconnect(event) {
        var _a;
        (_a = this.stream) === null || _a === void 0 ? void 0 : _a.getInput().onGamepadDisconnect(event);
    }
    onGamepadUpdate() {
        var _a;
        (_a = this.stream) === null || _a === void 0 ? void 0 : _a.getInput().onGamepadUpdate();
        window.requestAnimationFrame(this.onGamepadUpdate.bind(this));
    }
    mount(parent) {
        parent.appendChild(this.div);
    }
    unmount(parent) {
        parent.removeChild(this.div);
    }
    getStreamRect() {
        // The bounding rect of the videoElement can be bigger than the actual video
        // -> We need to correct for this when sending positions, else positions are wrong
        var _a, _b;
        const videoSize = (_b = (_a = this.stream) === null || _a === void 0 ? void 0 : _a.getStreamerSize()) !== null && _b !== void 0 ? _b : this.streamerSize;
        const videoAspect = videoSize[0] / videoSize[1];
        const boundingRect = this.videoElement.getBoundingClientRect();
        const boundingRectAspect = boundingRect.width / boundingRect.height;
        let x = boundingRect.x;
        let y = boundingRect.y;
        let videoMultiplier;
        if (boundingRectAspect > videoAspect) {
            // How much is the video scaled up
            videoMultiplier = boundingRect.height / videoSize[1];
            // Note: Both in boundingRect / page scale
            const boundingRectHalfWidth = boundingRect.width / 2;
            const videoHalfWidth = videoSize[0] * videoMultiplier / 2;
            x += boundingRectHalfWidth - videoHalfWidth;
        }
        else {
            // Same as above but inverted
            videoMultiplier = boundingRect.width / videoSize[0];
            const boundingRectHalfHeight = boundingRect.height / 2;
            const videoHalfHeight = videoSize[1] * videoMultiplier / 2;
            y += boundingRectHalfHeight - videoHalfHeight;
        }
        return new DOMRect(x, y, videoSize[0] * videoMultiplier, videoSize[1] * videoMultiplier);
    }
    getElement() {
        return this.videoElement;
    }
    getStream() {
        return this.stream;
    }
}
class ConnectionInfoModal {
    constructor() {
        this.eventTarget = new EventTarget();
        this.root = document.createElement("div");
        this.text = document.createElement("p");
        this.debugDetailButton = document.createElement("button");
        this.debugDetail = ""; // We store this seperate because line breaks don't work when the element is not mounted on the dom
        this.debugDetailDisplay = document.createElement("div");
        this.root.classList.add("modal-video-connect");
        this.text.innerText = "Connecting";
        this.root.appendChild(this.text);
        this.debugDetailButton.innerText = "Show Logs";
        this.debugDetailButton.addEventListener("click", this.onDebugDetailClick.bind(this));
        this.root.appendChild(this.debugDetailButton);
        this.debugDetailDisplay.classList.add("textlike");
        this.debugDetailDisplay.classList.add("modal-video-connect-debug");
    }
    onDebugDetailClick() {
        let debugDetailCurrentlyShown = this.root.contains(this.debugDetailDisplay);
        if (debugDetailCurrentlyShown) {
            this.debugDetailButton.innerText = "Show Logs";
            this.root.removeChild(this.debugDetailDisplay);
        }
        else {
            this.debugDetailButton.innerText = "Hide Logs";
            this.root.appendChild(this.debugDetailDisplay);
            this.debugDetailDisplay.innerText = this.debugDetail;
        }
    }
    debugLog(line) {
        this.debugDetail += `${line}\n`;
        this.debugDetailDisplay.innerText = this.debugDetail;
        console.info(`[Stream]: ${line}`);
    }
    onInfo(event) {
        const data = event.detail;
        if (data.type == "stageStarting") {
            const text = `Server: Starting Stage: ${data.stage}`;
            this.text.innerText = text;
            this.debugLog(text);
        }
        else if (data.type == "stageComplete") {
            const text = `Server: Completed Stage: ${data.stage}`;
            this.text.innerText = text;
            this.debugLog(text);
        }
        else if (data.type == "stageFailed") {
            const text = `Server: Failed Stage: ${data.stage} with error ${data.errorCode}`;
            this.text.innerText = text;
            this.debugLog(text);
        }
        else if (data.type == "connectionComplete") {
            const text = `Connection Complete`;
            this.text.innerText = text;
            this.debugLog(text);
            this.eventTarget.dispatchEvent(new Event("ml-connected"));
        }
        else if (data.type == "addDebugLine") {
            this.debugLog(data.line);
        }
        // Reopen the modal cause we might already be closed at this point
        else if (data.type == "connectionTerminated") {
            const text = `Server: Connection Terminated with code ${data.errorCode}`;
            this.text.innerText = text;
            this.debugLog(text);
            showModal(this);
        }
        else if (data.type == "error") {
            const text = `Server: Error: ${data.message}`;
            this.text.innerText = text;
            this.debugLog(text);
            showModal(this);
        }
    }
    onFinish(abort) {
        return new Promise((resolve, reject) => {
            this.eventTarget.addEventListener("ml-connected", () => resolve(), { once: true, signal: abort });
        });
    }
    mount(parent) {
        parent.appendChild(this.root);
    }
    unmount(parent) {
        parent.removeChild(this.root);
    }
}
class ViewerSidebar {
    constructor(app) {
        this.div = document.createElement("div");
        this.buttonDiv = document.createElement("div");
        this.sendKeycodeButton = document.createElement("button");
        this.keyboardButton = document.createElement("button");
        this.screenKeyboard = new ScreenKeyboard();
        this.lockMouseButton = document.createElement("button");
        this.fullscreenButton = document.createElement("button");
        this.config = defaultStreamInputConfig();
        this.app = app;
        // Configure divs
        this.div.classList.add("sidebar-stream");
        this.buttonDiv.classList.add("sidebar-stream-buttons");
        this.div.appendChild(this.buttonDiv);
        // Send keycode
        this.sendKeycodeButton.innerText = "Send Keycode";
        this.sendKeycodeButton.addEventListener("click", () => __awaiter(this, void 0, void 0, function* () {
            var _a, _b;
            const key = yield showModal(new SendKeycodeModal());
            if (key == null) {
                return;
            }
            (_a = this.app.getStream()) === null || _a === void 0 ? void 0 : _a.getInput().sendKey(true, key, 0);
            (_b = this.app.getStream()) === null || _b === void 0 ? void 0 : _b.getInput().sendKey(false, key, 0);
        }));
        this.buttonDiv.appendChild(this.sendKeycodeButton);
        // Pointer Lock
        this.lockMouseButton.innerText = "Lock Mouse";
        this.lockMouseButton.addEventListener("click", () => __awaiter(this, void 0, void 0, function* () {
            setSidebarExtended(false);
            const root = document.getElementById("root");
            if (root) {
                if ("requestPointerLock" in root && typeof root.requestPointerLock == "function") {
                    yield root.requestPointerLock();
                }
                else {
                    yield showMessage("Pointer Lock not supported");
                }
            }
            else {
                console.warn("root element not found");
            }
        }));
        this.buttonDiv.appendChild(this.lockMouseButton);
        // Pop up keyboard
        this.keyboardButton.innerText = "Keyboard";
        this.keyboardButton.addEventListener("click", () => __awaiter(this, void 0, void 0, function* () {
            setSidebarExtended(false);
            this.screenKeyboard.show();
        }));
        this.buttonDiv.appendChild(this.keyboardButton);
        this.screenKeyboard.addKeyDownListener(this.onKeyDown.bind(this));
        this.screenKeyboard.addKeyUpListener(this.onKeyUp.bind(this));
        this.screenKeyboard.addTextListener(this.onText.bind(this));
        this.div.appendChild(this.screenKeyboard.getHiddenElement());
        // Fullscreen
        this.fullscreenButton.innerText = "Fullscreen";
        this.fullscreenButton.addEventListener("click", () => __awaiter(this, void 0, void 0, function* () {
            const root = document.getElementById("root");
            if (root) {
                yield root.requestFullscreen({
                    navigationUI: "hide"
                });
                if (this.mouseMode.getValue() == "relative") {
                    if ("requestPointerLock" in root && typeof root.requestPointerLock == "function") {
                        yield root.requestPointerLock();
                    }
                }
                else {
                    console.warn("failed to request pointer lock while requesting fullscreen");
                }
                try {
                    if (screen && "orientation" in screen) {
                        const orientation = screen.orientation;
                        if ("lock" in orientation && typeof orientation.lock == "function") {
                            yield orientation.lock("landscape");
                        }
                    }
                }
                catch (e) {
                    console.warn("failed to set orientation to landscape", e);
                }
            }
            else {
                console.warn("root element not found");
            }
        }));
        this.buttonDiv.appendChild(this.fullscreenButton);
        // Select Mouse Mode
        this.mouseMode = new SelectComponent("mouseMode", [
            { value: "relative", name: "Relative" },
            { value: "follow", name: "Follow" },
            { value: "pointAndDrag", name: "Point and Drag" }
        ], {
            displayName: "Mouse Mode",
            preSelectedOption: this.config.mouseMode
        });
        this.mouseMode.addChangeListener(this.onMouseModeChange.bind(this));
        this.mouseMode.mount(this.div);
        // Select Touch Mode
        this.touchMode = new SelectComponent("touchMode", [
            { value: "touch", name: "Touch" },
            { value: "mouseRelative", name: "Relative" },
            { value: "pointAndDrag", name: "Point and Drag" }
        ], {
            displayName: "Touch Mode",
            preSelectedOption: this.config.touchMode
        });
        this.touchMode.addChangeListener(this.onTouchModeChange.bind(this));
        this.touchMode.mount(this.div);
    }
    onCapabilitiesChange(capabilities) {
        this.touchMode.setOptionEnabled("touch", capabilities.touch);
    }
    getScreenKeyboard() {
        return this.screenKeyboard;
    }
    // -- Keyboard
    onText(event) {
        var _a;
        (_a = this.app.getStream()) === null || _a === void 0 ? void 0 : _a.getInput().sendText(event.detail.text);
    }
    onKeyDown(event) {
        var _a;
        (_a = this.app.getStream()) === null || _a === void 0 ? void 0 : _a.getInput().onKeyDown(event);
    }
    onKeyUp(event) {
        var _a;
        (_a = this.app.getStream()) === null || _a === void 0 ? void 0 : _a.getInput().onKeyUp(event);
    }
    // -- Mouse Mode
    onMouseModeChange() {
        var _a;
        this.config.mouseMode = this.mouseMode.getValue();
        (_a = this.app.getStream()) === null || _a === void 0 ? void 0 : _a.getInput().setConfig(this.config);
    }
    // -- Touch Mode
    onTouchModeChange() {
        var _a;
        this.config.touchMode = this.touchMode.getValue();
        (_a = this.app.getStream()) === null || _a === void 0 ? void 0 : _a.getInput().setConfig(this.config);
    }
    extended() {
    }
    unextend() {
    }
    mount(parent) {
        parent.appendChild(this.div);
    }
    unmount(parent) {
        parent.removeChild(this.div);
    }
}
class SendKeycodeModal extends FormModal {
    constructor() {
        super();
        const keyList = [];
        for (const keyName of Object.keys(StreamKeys)) {
            const keyValue = StreamKeys[keyName];
            const PREFIX = "VK_";
            let name = keyName;
            if (name.startsWith(PREFIX)) {
                name = name.slice(PREFIX.length);
            }
            keyList.push({
                value: keyValue.toString(),
                name
            });
        }
        this.dropdownSearch = new SelectComponent("winKeycode", keyList, {
            hasSearch: true,
            displayName: "Select Keycode"
        });
    }
    mountForm(form) {
        this.dropdownSearch.mount(form);
    }
    reset() {
        this.dropdownSearch.reset();
    }
    submit() {
        const keyString = this.dropdownSearch.getValue();
        if (keyString == null) {
            return null;
        }
        return parseInt(keyString);
    }
}
// Stop propagation so the stream doesn't get it
function stopPropagationOn(element) {
    element.addEventListener("keydown", onStopPropagation);
    element.addEventListener("keyup", onStopPropagation);
    element.addEventListener("keypress", onStopPropagation);
    element.addEventListener("click", onStopPropagation);
    element.addEventListener("mousedown", onStopPropagation);
    element.addEventListener("mouseup", onStopPropagation);
    element.addEventListener("mousemove", onStopPropagation);
    element.addEventListener("wheel", onStopPropagation);
    element.addEventListener("contextmenu", onStopPropagation);
    element.addEventListener("touchstart", onStopPropagation);
    element.addEventListener("touchmove", onStopPropagation);
    element.addEventListener("touchend", onStopPropagation);
    element.addEventListener("touchcancel", onStopPropagation);
}
function onStopPropagation(event) {
    event.stopPropagation();
}
