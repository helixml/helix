export class ListComponent {
    constructor(list, init) {
        var _a, _b;
        this.mounted = 0;
        this.listElement = document.createElement("li");
        this.divElements = [];
        this.list = list !== null && list !== void 0 ? list : [];
        if (list) {
            this.internalMountFrom(0);
        }
        if (init === null || init === void 0 ? void 0 : init.listClasses) {
            this.listElement.classList.add(...init === null || init === void 0 ? void 0 : init.listClasses);
        }
        this.divClasses = (_a = init === null || init === void 0 ? void 0 : init.elementDivClasses) !== null && _a !== void 0 ? _a : [];
        this.remountIsInsertTransition = (_b = init === null || init === void 0 ? void 0 : init.remountIsInsert) !== null && _b !== void 0 ? _b : true;
    }
    divAt(index) {
        let div = this.divElements[index];
        if (!div) {
            div = document.createElement("div");
            div.classList.add(...this.divClasses);
            this.divElements[index] = div;
        }
        return div;
    }
    onAnimElementInserted(index) {
        const element = this.divElements[index];
        // let the element render and then add "list-show" for transitions :)
        setTimeout(() => {
            element.classList.add("list-show");
        }, 0);
    }
    onAnimElementRemoved(index) {
        let element;
        while ((element = this.divElements[index]).classList.contains("list-show")) {
            element.classList.remove("list-show");
        }
    }
    internalUnmountUntil(index) {
        for (let i = this.list.length - 1; i >= index; i--) {
            const divElement = this.divAt(i);
            this.listElement.removeChild(divElement);
            const element = this.list[i];
            element.unmount(divElement);
        }
    }
    internalMountFrom(index) {
        if (this.mounted <= 0) {
            return;
        }
        for (let i = index; i < this.list.length; i++) {
            let divElement = this.divAt(i);
            this.listElement.appendChild(divElement);
            const element = this.list[i];
            element.mount(divElement);
        }
    }
    insert(index, value) {
        if (index == this.list.length) {
            const divElement = this.divAt(index);
            this.list.push(value);
            value.mount(divElement);
            this.listElement.appendChild(divElement);
        }
        else {
            this.internalUnmountUntil(index);
            this.list.splice(index, 0, value);
            this.internalMountFrom(index);
        }
        this.onAnimElementInserted(index);
    }
    remove(index) {
        var _a;
        if (index == this.list.length - 1) {
            const element = this.list.pop();
            const divElement = this.divElements[index];
            if (element && divElement) {
                element.unmount(divElement);
                this.listElement.removeChild(divElement);
                return element;
            }
        }
        else {
            this.internalUnmountUntil(index);
            const element = this.list.splice(index, 1);
            this.internalMountFrom(index);
            return (_a = element[0]) !== null && _a !== void 0 ? _a : null;
        }
        this.onAnimElementRemoved(this.list.length + 1);
        return null;
    }
    append(value) {
        this.insert(this.get().length, value);
    }
    removeValue(value) {
        const index = this.get().indexOf(value);
        if (index != -1) {
            this.remove(index);
        }
    }
    clear() {
        this.internalUnmountUntil(0);
        this.list.splice(0, this.list.length);
    }
    get() {
        return this.list;
    }
    mount(parent) {
        this.mounted++;
        parent.appendChild(this.listElement);
        // Mount all elements
        if (this.mounted == 1) {
            this.internalMountFrom(0);
            if (this.remountIsInsertTransition) {
                for (let i = 0; i < this.list.length; i++) {
                    this.onAnimElementInserted(i);
                }
            }
        }
    }
    unmount(parent) {
        this.mounted--;
        parent.removeChild(this.listElement);
        // Unmount all elements
        if (this.mounted == 0) {
            this.internalUnmountUntil(0);
            if (this.remountIsInsertTransition) {
                for (let i = 0; i < this.list.length; i++) {
                    this.onAnimElementRemoved(i);
                }
            }
        }
    }
}
