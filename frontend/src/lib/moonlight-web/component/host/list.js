var __awaiter = (this && this.__awaiter) || function (thisArg, _arguments, P, generator) {
    function adopt(value) { return value instanceof P ? value : new P(function (resolve) { resolve(value); }); }
    return new (P || (P = Promise))(function (resolve, reject) {
        function fulfilled(value) { try { step(generator.next(value)); } catch (e) { reject(e); } }
        function rejected(value) { try { step(generator["throw"](value)); } catch (e) { reject(e); } }
        function step(result) { result.done ? resolve(result.value) : adopt(result.value).then(fulfilled, rejected); }
        step((generator = generator.apply(thisArg, _arguments || [])).next());
    });
};
import { apiGetHosts } from "../../api.js";
import { ComponentEvent } from "../index.js";
import { Host } from "./index.js";
import { FetchListComponent } from "../fetch_list.js";
export class HostList extends FetchListComponent {
    constructor(api) {
        super({
            listClasses: ["host-list"],
            elementDivClasses: ["animated-list-element", "host-element"]
        });
        this.eventTarget = new EventTarget();
        this.api = api;
    }
    forceFetch() {
        return __awaiter(this, void 0, void 0, function* () {
            const hosts = yield apiGetHosts(this.api);
            this.updateCache(hosts);
        });
    }
    updateComponentData(component, data) {
        component.updateCache(data);
    }
    getComponentDataId(component) {
        return component.getHostId();
    }
    getDataId(data) {
        return data.host_id;
    }
    insertList(dataId, data) {
        const newHost = new Host(this.api, dataId, data);
        this.list.append(newHost);
        newHost.addHostRemoveListener(this.removeHostListener.bind(this));
        newHost.addHostOpenListener(this.onHostOpenEvent.bind(this));
    }
    removeList(listIndex) {
        const hostComponent = this.list.remove(listIndex);
        hostComponent === null || hostComponent === void 0 ? void 0 : hostComponent.addHostOpenListener(this.onHostOpenEvent.bind(this));
        hostComponent === null || hostComponent === void 0 ? void 0 : hostComponent.removeHostRemoveListener(this.removeHostListener.bind(this));
    }
    removeHostListener(event) {
        const listIndex = this.list.get().findIndex(component => component.getHostId() == event.component.getHostId());
        this.removeList(listIndex);
    }
    getHost(hostId) {
        return this.list.get().find(host => host.getHostId() == hostId);
    }
    onHostOpenEvent(event) {
        this.eventTarget.dispatchEvent(new ComponentEvent("ml-hostopen", event.component));
    }
    addHostOpenListener(listener, options) {
        this.eventTarget.addEventListener("ml-hostopen", listener, options);
    }
    removeHostOpenListener(listener, options) {
        this.eventTarget.removeEventListener("ml-hostopen", listener, options);
    }
    mount(parent) {
        this.list.mount(parent);
    }
    unmount(parent) {
        this.list.unmount(parent);
    }
}
