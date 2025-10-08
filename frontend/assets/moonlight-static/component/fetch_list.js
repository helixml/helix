import { ListComponent } from "./list.js";
export class FetchListComponent {
    constructor(listInit) {
        this.list = new ListComponent([], listInit);
    }
    updateCache(cache) {
        // Remove all non existing new data
        // Update all already existing components
        for (let i = 0; i < this.list.get().length; i++) {
            let component = this.list.get()[i];
            const dataId = this.getComponentDataId(component);
            const cacheIndex = cache.findIndex(data => this.getDataId(data) == dataId);
            if (cacheIndex == -1) {
                this.removeList(i);
                // removing an element will shift the array to the left
                // -> this means that we need to decr to get the next value because we incr in the loop
                i--;
            }
            else {
                this.updateComponentData(component, cache[cacheIndex]);
            }
        }
        // All all newly created data
        for (let i = 0; i < cache.length; i++) {
            let data = cache[i];
            const dataId = this.getDataId(data);
            const listIndex = this.list.get().findIndex(component => this.getComponentDataId(component) == dataId);
            if (listIndex == -1) {
                this.insertList(dataId, data);
            }
        }
    }
    removeList(listIndex) {
        this.list.remove(listIndex);
    }
    mount(parent) {
        this.list.mount(parent);
    }
    unmount(parent) {
        this.list.unmount(parent);
    }
}
