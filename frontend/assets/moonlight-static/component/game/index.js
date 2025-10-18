var __awaiter = (this && this.__awaiter) || function (thisArg, _arguments, P, generator) {
    function adopt(value) { return value instanceof P ? value : new P(function (resolve) { resolve(value); }); }
    return new (P || (P = Promise))(function (resolve, reject) {
        function fulfilled(value) { try { step(generator.next(value)); } catch (e) { reject(e); } }
        function rejected(value) { try { step(generator["throw"](value)); } catch (e) { reject(e); } }
        function step(result) { result.done ? resolve(result.value) : adopt(result.value).then(fulfilled, rejected); }
        step((generator = generator.apply(thisArg, _arguments || [])).next());
    });
};
import { ComponentEvent } from "../index.js";
import { apiGetAppImage, apiHostCancel } from "../../api.js";
import { setContextMenu } from "../context_menu.js";
import { showMessage } from "../modal/index.js";
import { APP_NO_IMAGE } from "../../resources/index.js";
import { buildUrl } from "../../config_.js";
export class Game {
    constructor(api, hostId, appId, cache) {
        this.mounted = 0;
        this.divElement = document.createElement("div");
        this.imageBlob = null;
        this.imageBlobUrl = null;
        this.imageElement = document.createElement("img");
        this.api = api;
        this.hostId = hostId;
        this.appId = appId;
        this.cache = cache;
        // Configure image
        this.imageElement.classList.add("app-image");
        this.imageElement.src = APP_NO_IMAGE;
        this.forceLoadImage(false);
        // Configure div
        this.divElement.classList.add("app");
        this.divElement.appendChild(this.imageElement);
        this.divElement.addEventListener("click", this.onClick.bind(this));
        this.divElement.addEventListener("contextmenu", this.onContextMenu.bind(this));
        this.updateCache(cache);
    }
    forceLoadImage(forceServerRefresh) {
        return __awaiter(this, void 0, void 0, function* () {
            this.imageBlob = yield apiGetAppImage(this.api, {
                host_id: this.hostId,
                app_id: this.appId,
                force_refresh: forceServerRefresh
            });
            this.updateImage();
        });
    }
    updateImage() {
        // generate and set url
        if (this.imageBlob && !this.imageBlobUrl && this.mounted > 0) {
            this.imageBlobUrl = URL.createObjectURL(this.imageBlob);
            this.imageElement.classList.add("app-image-loaded");
            this.imageElement.src = this.imageBlobUrl;
        }
        // revoke url
        if (this.imageBlobUrl && this.mounted <= 0) {
            URL.revokeObjectURL(this.imageBlobUrl);
            this.imageBlobUrl = null;
            this.imageElement.classList.remove("app-image-loaded");
            this.imageElement.src = "";
        }
    }
    updateCache(cache) {
        this.cache = cache;
        this.divElement.classList.remove("app-inactive");
        this.divElement.classList.remove("app-active");
        if (this.isActive()) {
            this.divElement.classList.add("app-active");
        }
        else if (this.cache.activeApp != null) {
            this.divElement.classList.add("app-inactive");
        }
    }
    onClick(event) {
        return __awaiter(this, void 0, void 0, function* () {
            if (this.cache.activeApp != null) {
                const elements = [];
                if (this.isActive()) {
                    elements.push({
                        name: "Resume Session",
                        callback: () => __awaiter(this, void 0, void 0, function* () {
                            this.startStream();
                            const event = new ComponentEvent("ml-gamereload", this);
                            this.divElement.dispatchEvent(event);
                        })
                    });
                }
                elements.push({
                    name: "Stop Current Session",
                    callback: () => __awaiter(this, void 0, void 0, function* () {
                        yield apiHostCancel(this.api, { host_id: this.hostId });
                        const event = new ComponentEvent("ml-gamereload", this);
                        this.divElement.dispatchEvent(event);
                    })
                });
                setContextMenu(event, {
                    elements
                });
            }
            else {
                this.startStream();
                yield new Promise(r => window.setTimeout(r, 6000));
                const event = new ComponentEvent("ml-gamereload", this);
                this.divElement.dispatchEvent(event);
            }
        });
    }
    startStream() {
        let query = new URLSearchParams({
            hostId: this.getHostId(),
            appId: this.getAppId(),
        });
        if (window.matchMedia('(display-mode: standalone)').matches) {
            // If we're in a pwa: open in the current tab
            // If we don't do this we might get a url bar at the top
            window.location.href = buildUrl(`/stream.html?${query}`);
        }
        else {
            window.open(buildUrl(`/stream.html?${query}`), "_blank");
        }
    }
    onContextMenu(event) {
        const elements = [];
        elements.push({
            name: "Show Details",
            callback: this.showDetails.bind(this),
        });
        setContextMenu(event, {
            elements
        });
    }
    showDetails() {
        return __awaiter(this, void 0, void 0, function* () {
            const app = this.cache;
            yield showMessage(`Title: ${app.title}\n` +
                `Id: ${app.app_id}\n` +
                `HDR Supported: ${app.is_hdr_supported}\n`);
        });
    }
    isActive() {
        return this.cache.activeApp == this.appId;
    }
    addForceReloadListener(listener) {
        this.divElement.addEventListener("ml-gamereload", listener);
    }
    removeForceReloadListener(listener) {
        this.divElement.removeEventListener("ml-gamereload", listener);
    }
    getHostId() {
        return this.hostId;
    }
    getAppId() {
        return this.appId;
    }
    mount(parent) {
        this.mounted++;
        this.updateImage();
        parent.appendChild(this.divElement);
    }
    unmount(parent) {
        parent.removeChild(this.divElement);
        this.mounted--;
        this.updateImage();
    }
}
