var __awaiter = (this && this.__awaiter) || function (thisArg, _arguments, P, generator) {
    function adopt(value) { return value instanceof P ? value : new P(function (resolve) { resolve(value); }); }
    return new (P || (P = Promise))(function (resolve, reject) {
        function fulfilled(value) { try { step(generator.next(value)); } catch (e) { reject(e); } }
        function rejected(value) { try { step(generator["throw"](value)); } catch (e) { reject(e); } }
        function step(result) { result.done ? resolve(result.value) : adopt(result.value).then(fulfilled, rejected); }
        step((generator = generator.apply(thisArg, _arguments || [])).next());
    });
};
import { getApi, apiPutHost, FetchError } from "./api.js";
import { AddHostModal } from "./component/host/add_modal.js";
import { HostList } from "./component/host/list.js";
import { showErrorPopup } from "./component/error.js";
import { showModal } from "./component/modal/index.js";
import { setContextMenu } from "./component/context_menu.js";
import { GameList } from "./component/game/list.js";
import { getLocalStreamSettings, setLocalStreamSettings, StreamSettingsComponent } from "./component/settings_menu.js";
import { setTouchContextMenuEnabled } from "./ios_right_click.js";
function startApp() {
    return __awaiter(this, void 0, void 0, function* () {
        setTouchContextMenuEnabled(true);
        const api = yield getApi();
        const rootElement = document.getElementById("root");
        if (rootElement == null) {
            showErrorPopup("couldn't find root element", true);
            return;
        }
        const app = new MainApp(api);
        app.mount(rootElement);
        app.forceFetch();
        window.addEventListener("popstate", event => {
            app.setAppState(event.state);
        });
    });
}
startApp();
function pushAppState(state) {
    history.pushState(state, "");
}
class MainApp {
    constructor(api) {
        var _a;
        this.divElement = document.createElement("div");
        this.moonlightTextElement = document.createElement("h1");
        this.actionElement = document.createElement("div");
        this.backToHostsButton = document.createElement("button");
        this.hostAddButton = document.createElement("button");
        this.settingsButton = document.createElement("button");
        this.currentDisplay = null;
        this.gameList = null;
        this.api = api;
        // Moonlight text
        this.moonlightTextElement.innerHTML = "Moonlight Web";
        // Actions
        this.actionElement.classList.add("actions-list");
        // Back button
        this.backToHostsButton.innerText = "Back";
        this.backToHostsButton.addEventListener("click", () => this.setCurrentDisplay("hosts"));
        // Host add button
        this.hostAddButton.classList.add("host-add");
        this.hostAddButton.addEventListener("click", this.addHost.bind(this));
        // Host list
        this.hostList = new HostList(api);
        this.hostList.addHostOpenListener(this.onHostOpen.bind(this));
        // Settings Button
        this.settingsButton.classList.add("open-settings");
        this.settingsButton.addEventListener("click", () => this.setCurrentDisplay("settings"));
        // Settings
        this.settings = new StreamSettingsComponent((_a = getLocalStreamSettings()) !== null && _a !== void 0 ? _a : undefined);
        this.settings.addChangeListener(this.onSettingsChange.bind(this));
        // Append default elements
        this.divElement.appendChild(this.moonlightTextElement);
        this.divElement.appendChild(this.actionElement);
        this.setCurrentDisplay("hosts");
        // Context Menu
        document.body.addEventListener("contextmenu", this.onContextMenu.bind(this), { passive: false });
    }
    setAppState(state) {
        if (state.display == "hosts") {
            this.setCurrentDisplay("hosts");
        }
        else if (state.display == "games" && state.hostId != null) {
            this.setCurrentDisplay("games", state.hostId);
        }
        else if (state.display == "settings") {
            this.setCurrentDisplay("settings");
        }
    }
    addHost() {
        return __awaiter(this, void 0, void 0, function* () {
            const modal = new AddHostModal();
            let host = yield showModal(modal);
            if (host) {
                let newHost;
                try {
                    newHost = yield apiPutHost(this.api, host);
                }
                catch (e) {
                    if (e instanceof FetchError) {
                        const response = e.getResponse();
                        if (response && response.status == 400) {
                            showErrorPopup("couldn't add host: not found");
                            return;
                        }
                    }
                    throw e;
                }
                this.hostList.insertList(newHost.host_id, newHost);
            }
        });
    }
    onContextMenu(event) {
        if (this.currentDisplay == "hosts" || this.currentDisplay == "games") {
            const elements = [
                {
                    name: "Reload",
                    callback: this.forceFetch.bind(this)
                }
            ];
            setContextMenu(event, {
                elements
            });
        }
    }
    onHostOpen(event) {
        return __awaiter(this, void 0, void 0, function* () {
            const hostId = event.component.getHostId();
            this.setCurrentDisplay("games", hostId);
        });
    }
    onSettingsChange() {
        const newSettings = this.settings.getStreamSettings();
        setLocalStreamSettings(newSettings);
    }
    setCurrentDisplay(display, hostId, hostCache) {
        var _a, _b, _c, _d;
        if (display == "games" && hostId == null) {
            // invalid input state
            return;
        }
        // Check if we need to change
        if (this.currentDisplay == display) {
            if (this.currentDisplay == "games" && ((_a = this.gameList) === null || _a === void 0 ? void 0 : _a.getHostId()) != hostId) {
                // fall through
            }
            else {
                return;
            }
        }
        // Unmount the current display
        if (this.currentDisplay == "hosts") {
            this.actionElement.removeChild(this.hostAddButton);
            this.actionElement.removeChild(this.settingsButton);
            this.hostList.unmount(this.divElement);
        }
        else if (this.currentDisplay == "games") {
            this.actionElement.removeChild(this.backToHostsButton);
            (_b = this.gameList) === null || _b === void 0 ? void 0 : _b.unmount(this.divElement);
        }
        else if (this.currentDisplay == "settings") {
            this.actionElement.removeChild(this.backToHostsButton);
            this.settings.unmount(this.divElement);
        }
        // Mount the new display
        if (display == "hosts") {
            this.actionElement.appendChild(this.hostAddButton);
            this.actionElement.appendChild(this.settingsButton);
            this.hostList.mount(this.divElement);
            pushAppState({ display: "hosts" });
        }
        else if (display == "games" && hostId != null) {
            this.actionElement.appendChild(this.backToHostsButton);
            if (((_c = this.gameList) === null || _c === void 0 ? void 0 : _c.getHostId()) != hostId) {
                this.gameList = new GameList(this.api, hostId, hostCache !== null && hostCache !== void 0 ? hostCache : null);
                this.gameList.addForceReloadListener(this.forceFetch.bind(this));
            }
            this.gameList.mount(this.divElement);
            this.refreshGameListActiveGame();
            pushAppState({ display: "games", hostId: (_d = this.gameList) === null || _d === void 0 ? void 0 : _d.getHostId() });
        }
        else if (display == "settings") {
            this.actionElement.appendChild(this.backToHostsButton);
            this.settings.mount(this.divElement);
            pushAppState({ display: "settings" });
        }
        this.currentDisplay = display;
    }
    forceFetch() {
        return __awaiter(this, void 0, void 0, function* () {
            var _a;
            yield Promise.all([
                this.hostList.forceFetch(),
                (_a = this.gameList) === null || _a === void 0 ? void 0 : _a.forceFetch(true)
            ]);
            if (this.currentDisplay == "games"
                && this.gameList
                && !this.hostList.getHost(this.gameList.getHostId())) {
                // The newly fetched list doesn't contain the hosts game view we're in -> go to hosts
                this.setCurrentDisplay("hosts");
            }
            yield this.refreshGameListActiveGame();
        });
    }
    refreshGameListActiveGame() {
        return __awaiter(this, void 0, void 0, function* () {
            const gameList = this.gameList;
            const hostId = gameList === null || gameList === void 0 ? void 0 : gameList.getHostId();
            if (hostId == null) {
                return;
            }
            const host = this.hostList.getHost(hostId);
            if (host == null) {
                return;
            }
            const currentGame = yield host.getCurrentGame();
            if (currentGame != null) {
                gameList === null || gameList === void 0 ? void 0 : gameList.setActiveGame(currentGame);
            }
            else {
                gameList === null || gameList === void 0 ? void 0 : gameList.setActiveGame(null);
            }
        });
    }
    mount(parent) {
        parent.appendChild(this.divElement);
    }
    unmount(parent) {
        parent.removeChild(this.divElement);
    }
}
