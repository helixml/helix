var __awaiter = (this && this.__awaiter) || function (thisArg, _arguments, P, generator) {
    function adopt(value) { return value instanceof P ? value : new P(function (resolve) { resolve(value); }); }
    return new (P || (P = Promise))(function (resolve, reject) {
        function fulfilled(value) { try { step(generator.next(value)); } catch (e) { reject(e); } }
        function rejected(value) { try { step(generator["throw"](value)); } catch (e) { reject(e); } }
        function step(result) { result.done ? resolve(result.value) : adopt(result.value).then(fulfilled, rejected); }
        step((generator = generator.apply(thisArg, _arguments || [])).next());
    });
};
import { apiDeleteHost, apiGetHost, isDetailedHost, apiPostPair, apiWakeUp } from "../../api.js";
import { ComponentEvent } from "../index.js";
import { setContextMenu } from "../context_menu.js";
import { showErrorPopup } from "../error.js";
import { showMessage } from "../modal/index.js";
import { HOST_IMAGE, HOST_OVERLAY_LOCK, HOST_OVERLAY_NONE, HOST_OVERLAY_OFFLINE } from "../../resources/index.js";
export class Host {
    constructor(api, hostId, host) {
        this.cache = null;
        this.divElement = document.createElement("div");
        this.imageElement = document.createElement("img");
        this.imageOverlayElement = document.createElement("img");
        this.nameElement = document.createElement("p");
        this.api = api;
        this.hostId = hostId;
        this.cache = host;
        // Configure image
        this.imageElement.classList.add("host-image");
        this.imageElement.src = HOST_IMAGE;
        // Configure image overlay
        this.imageOverlayElement.classList.add("host-image-overlay");
        // Configure name
        this.nameElement.classList.add("host-name");
        // Append elements
        this.divElement.appendChild(this.imageElement);
        this.divElement.appendChild(this.imageOverlayElement);
        this.divElement.appendChild(this.nameElement);
        this.divElement.addEventListener("click", this.onClick.bind(this));
        this.divElement.addEventListener("contextmenu", this.onContextMenu.bind(this));
        // Update cache
        if (host != null) {
            this.updateCache(host);
        }
        else {
            this.forceFetch(false);
        }
    }
    forceFetch(forceServerRefresh) {
        return __awaiter(this, void 0, void 0, function* () {
            const newCache = yield apiGetHost(this.api, {
                host_id: this.hostId,
                force_refresh: forceServerRefresh || false
            });
            this.updateCache(newCache);
        });
    }
    getCurrentGame() {
        return __awaiter(this, void 0, void 0, function* () {
            yield this.forceFetch();
            if (this.cache && isDetailedHost(this.cache) && this.cache.current_game != 0) {
                return this.cache.current_game;
            }
            else {
                return null;
            }
        });
    }
    onClick(event) {
        return __awaiter(this, void 0, void 0, function* () {
            var _a, _b;
            if (((_a = this.cache) === null || _a === void 0 ? void 0 : _a.server_state) == null) {
                this.onContextMenu(event);
            }
            else if (((_b = this.cache) === null || _b === void 0 ? void 0 : _b.paired) == "Paired") {
                this.divElement.dispatchEvent(new ComponentEvent("ml-hostopen", this));
            }
            else {
                yield this.pair();
            }
        });
    }
    onContextMenu(event) {
        var _a, _b;
        const elements = [];
        if (((_a = this.cache) === null || _a === void 0 ? void 0 : _a.server_state) != null) {
            elements.push({
                name: "Show Details",
                callback: this.showDetails.bind(this),
            });
        }
        else {
            elements.push({
                name: "Send Wake Up Packet",
                callback: this.wakeUp.bind(this)
            });
        }
        elements.push({
            name: "Reload",
            callback: () => __awaiter(this, void 0, void 0, function* () { return this.forceFetch(true); })
        });
        if (((_b = this.cache) === null || _b === void 0 ? void 0 : _b.paired) == "NotPaired") {
            elements.push({
                name: "Pair",
                callback: this.pair.bind(this)
            });
        }
        elements.push({
            name: "Remove Host",
            callback: this.remove.bind(this)
        });
        setContextMenu(event, {
            elements
        });
    }
    showDetails() {
        return __awaiter(this, void 0, void 0, function* () {
            let host = this.cache;
            if (!host || !isDetailedHost(host)) {
                host = yield apiGetHost(this.api, {
                    host_id: this.hostId,
                    force_refresh: false
                });
            }
            if (!host || !isDetailedHost(host)) {
                showErrorPopup(`failed to get details for host ${this.hostId}`);
                return;
            }
            this.updateCache(host);
            yield showMessage(`Web Id: ${host.host_id}\n` +
                `Name: ${host.name}\n` +
                `Pair Status: ${host.paired}\n` +
                `State: ${host.server_state}\n` +
                `Address: ${host.address}\n` +
                `Http Port: ${host.http_port}\n` +
                `Https Port: ${host.https_port}\n` +
                `External Port: ${host.external_port}\n` +
                `Version: ${host.version}\n` +
                `Gfe Version: ${host.gfe_version}\n` +
                `Unique ID: ${host.unique_id}\n` +
                `MAC: ${host.mac}\n` +
                `Local IP: ${host.local_ip}\n` +
                `Current Game: ${host.current_game}\n` +
                `Max Luma Pixels Hevc: ${host.max_luma_pixels_hevc}\n` +
                `Server Codec Mode Support: ${host.server_codec_mode_support}`);
        });
    }
    addHostRemoveListener(listener, options) {
        this.divElement.addEventListener("ml-hostremove", listener, options);
    }
    removeHostRemoveListener(listener, options) {
        this.divElement.removeEventListener("ml-hostremove", listener, options);
    }
    addHostOpenListener(listener, options) {
        this.divElement.addEventListener("ml-hostopen", listener, options);
    }
    removeHostOpenListener(listener, options) {
        this.divElement.removeEventListener("ml-hostopen", listener, options);
    }
    remove() {
        return __awaiter(this, void 0, void 0, function* () {
            const success = yield apiDeleteHost(this.api, {
                host_id: this.getHostId()
            });
            if (!success) {
                showErrorPopup(`something went wrong whilst removing the host ${this.getHostId()}`);
            }
            this.divElement.dispatchEvent(new ComponentEvent("ml-hostremove", this));
        });
    }
    wakeUp() {
        return __awaiter(this, void 0, void 0, function* () {
            yield apiWakeUp(this.api, {
                host_id: this.getHostId()
            });
            yield showMessage("Sent Wake Up packet. It might take a moment for your pc to start.");
        });
    }
    pair() {
        return __awaiter(this, void 0, void 0, function* () {
            var _a, _b, _c;
            if (((_a = this.cache) === null || _a === void 0 ? void 0 : _a.paired) == "Paired") {
                yield this.forceFetch();
                if (((_b = this.cache) === null || _b === void 0 ? void 0 : _b.paired) == "Paired") {
                    showMessage("This host is already paired!");
                    return;
                }
            }
            const pinResponse = yield apiPostPair(this.api, {
                host_id: this.getHostId()
            });
            const messageAbort = new AbortController();
            showMessage(`Please pair your host ${(_c = this.getCache()) === null || _c === void 0 ? void 0 : _c.name} with this pin:\nPin: ${pinResponse.pin}`, { signal: messageAbort.signal });
            const resultResponse = yield pinResponse.result;
            messageAbort.abort();
            this.updateCache(resultResponse);
        });
    }
    getHostId() {
        return this.hostId;
    }
    getCache() {
        return this.cache;
    }
    updateCache(host) {
        if (this.getHostId() != host.host_id) {
            showErrorPopup(`tried to overwrite host ${this.getHostId()} with data from ${host.host_id}`);
            return;
        }
        if (this.cache == null) {
            this.cache = host;
        }
        else {
            // if server_state == null it means this host is offline
            // -> updating cache means setting it to offline
            if (this.cache.server_state != null) {
                Object.assign(this.cache, host);
            }
            else {
                this.cache = host;
            }
        }
        // Update Elements
        this.nameElement.innerText = this.cache.name;
        if (this.cache.server_state == null) {
            this.imageOverlayElement.src = HOST_OVERLAY_OFFLINE;
        }
        else if (this.cache.paired != "Paired") {
            this.imageOverlayElement.src = HOST_OVERLAY_LOCK;
        }
        else {
            this.imageOverlayElement.src = HOST_OVERLAY_NONE;
        }
    }
    mount(parent) {
        parent.appendChild(this.divElement);
    }
    unmount(parent) {
        parent.removeChild(this.divElement);
    }
}
