var __awaiter = (this && this.__awaiter) || function (thisArg, _arguments, P, generator) {
    function adopt(value) { return value instanceof P ? value : new P(function (resolve) { resolve(value); }); }
    return new (P || (P = Promise))(function (resolve, reject) {
        function fulfilled(value) { try { step(generator.next(value)); } catch (e) { reject(e); } }
        function rejected(value) { try { step(generator["throw"](value)); } catch (e) { reject(e); } }
        function step(result) { result.done ? resolve(result.value) : adopt(result.value).then(fulfilled, rejected); }
        step((generator = generator.apply(thisArg, _arguments || [])).next());
    });
};
import { apiGetApps } from "../../api.js";
import { FetchListComponent } from "../fetch_list.js";
import { ComponentEvent } from "../index.js";
import { Game } from "./index.js";
export class GameList extends FetchListComponent {
    constructor(api, hostId, cache) {
        super({
            listClasses: ["app-list"],
            elementDivClasses: ["animated-list-element", "app-element"]
        });
        this.eventTarget = new EventTarget();
        this.activeApp = null;
        this.api = api;
        this.hostId = hostId;
        // Update cache
        if (cache != null) {
            this.updateCache(cache);
        }
        else {
            this.forceFetch();
        }
    }
    setActiveGame(appId) {
        this.activeApp = appId;
        this.forceFetch();
    }
    forceFetch(forceServerRefresh) {
        return __awaiter(this, void 0, void 0, function* () {
            const apps = yield apiGetApps(this.api, {
                host_id: this.hostId,
                force_refresh: forceServerRefresh || false
            });
            this.updateCache(apps);
        });
    }
    createCache(data) {
        const cache = data;
        cache.activeApp = this.activeApp;
        return cache;
    }
    updateComponentData(component, data) {
        const cache = this.createCache(data);
        component.updateCache(cache);
    }
    getComponentDataId(component) {
        return component.getAppId();
    }
    getDataId(data) {
        return data.app_id;
    }
    insertList(dataId, data) {
        const cache = this.createCache(data);
        const game = new Game(this.api, this.hostId, dataId, cache);
        game.addForceReloadListener(this.onForceReload.bind(this));
        this.list.append(game);
    }
    onForceReload(event) {
        this.eventTarget.dispatchEvent(new ComponentEvent("ml-gamereload", event.component));
    }
    addForceReloadListener(listener) {
        this.eventTarget.addEventListener("ml-gamereload", listener);
    }
    removeForceReloadListener(listener) {
        this.eventTarget.removeEventListener("ml-gamereload", listener);
    }
    getHostId() {
        return this.hostId;
    }
    mount(parent) {
        this.list.mount(parent);
    }
    unmount(parent) {
        this.list.unmount(parent);
    }
}
