import { ComponentEvent } from "./index.js";
export class ElementWithLabel {
    constructor(internalName, displayName) {
        this.div = document.createElement("div");
        this.label = document.createElement("label");
        if (displayName) {
            this.label.htmlFor = internalName;
            this.label.innerText = displayName;
            this.div.appendChild(this.label);
        }
    }
    mount(parent) {
        parent.appendChild(this.div);
    }
    unmount(parent) {
        parent.removeChild(this.div);
    }
}
export class InputComponent extends ElementWithLabel {
    constructor(internalName, type, displayName, init) {
        super(internalName, displayName);
        this.fileLabel = null;
        this.input = document.createElement("input");
        this.div.classList.add("input-div");
        this.input.id = internalName;
        this.input.type = type;
        if ((init === null || init === void 0 ? void 0 : init.defaultValue) != null) {
            this.input.defaultValue = init.defaultValue;
        }
        if ((init === null || init === void 0 ? void 0 : init.value) != null) {
            this.input.value = init.value;
        }
        if (init && init.checked != null) {
            this.input.checked = init.checked;
        }
        if (init && init.step != null) {
            this.input.step = init.step;
        }
        if (init && init.accept != null) {
            this.input.accept = init.accept;
        }
        if (init && init.inputMode != null) {
            this.input.inputMode = init.inputMode;
        }
        if (type == "file") {
            this.fileLabel = document.createElement("div");
            this.fileLabel.innerText = this.label.innerText;
            this.fileLabel.classList.add("file-label");
            this.label.innerText = "Open File";
            this.label.classList.add("file-button");
            this.div.insertBefore(this.fileLabel, this.label);
        }
        this.div.appendChild(this.input);
        this.input.addEventListener("change", () => {
            this.div.dispatchEvent(new ComponentEvent("ml-change", this));
        });
    }
    reset() {
        this.input.value = "";
    }
    getValue() {
        return this.input.value;
    }
    isChecked() {
        return this.input.checked;
    }
    getFiles() {
        return this.input.files;
    }
    setEnabled(enabled) {
        this.input.disabled = !enabled;
    }
    addChangeListener(listener, options) {
        this.div.addEventListener("ml-change", listener, options);
    }
    removeChangeListener(listener) {
        this.div.removeEventListener("ml-change", listener);
    }
}
export class SelectComponent extends ElementWithLabel {
    constructor(internalName, options, init) {
        super(internalName, init === null || init === void 0 ? void 0 : init.displayName);
        this.preSelectedOption = "";
        if (init && init.preSelectedOption) {
            this.preSelectedOption = init.preSelectedOption;
        }
        this.options = options;
        if (init && init.hasSearch && isElementSupported("datalist")) {
            this.strategy = "datalist";
            this.optionRoot = document.createElement("datalist");
            this.optionRoot.id = `${internalName}-list`;
            this.inputElement = document.createElement("input");
            this.inputElement.type = "text";
            this.inputElement.id = internalName;
            this.inputElement.setAttribute("list", this.optionRoot.id);
            if (init && init.preSelectedOption) {
                this.inputElement.defaultValue = init.preSelectedOption;
            }
            this.div.appendChild(this.inputElement);
            this.div.appendChild(this.optionRoot);
        }
        else {
            this.strategy = "select";
            this.inputElement = null;
            this.optionRoot = document.createElement("select");
            this.optionRoot.id = internalName;
            this.div.appendChild(this.optionRoot);
        }
        for (const option of options) {
            const optionElement = document.createElement("option");
            if (this.strategy == "datalist") {
                optionElement.value = option.name;
            }
            else if (this.strategy == "select") {
                optionElement.innerText = option.name;
                optionElement.value = option.value;
            }
            if (init && init.preSelectedOption == option.value) {
                optionElement.selected = true;
            }
            this.optionRoot.appendChild(optionElement);
        }
        this.optionRoot.addEventListener("change", () => {
            this.div.dispatchEvent(new ComponentEvent("ml-change", this));
        });
    }
    reset() {
        if (this.strategy == "datalist") {
            const inputElement = this.inputElement;
            inputElement.value = "";
        }
        else {
            const selectElement = this.optionRoot;
            selectElement.value = "";
        }
    }
    getValue() {
        var _a, _b;
        if (this.strategy == "datalist") {
            const name = this.inputElement.value;
            return (_b = (_a = this.options.find(option => option.name == name)) === null || _a === void 0 ? void 0 : _a.value) !== null && _b !== void 0 ? _b : "";
        }
        else if (this.strategy == "select") {
            return this.optionRoot.value;
        }
        throw "Invalid strategy for select input field";
    }
    setOptionEnabled(value, enabled) {
        for (const optionElement of this.optionRoot.options) {
            if (optionElement.value == value) {
                optionElement.disabled = !enabled;
            }
        }
    }
    addChangeListener(listener, options) {
        this.div.addEventListener("ml-change", listener, options);
    }
    removeChangeListener(listener) {
        this.div.removeEventListener("ml-change", listener);
    }
}
export function isElementSupported(tag) {
    // Create a test element for the tag
    const element = document.createElement(tag);
    // Check for support of custom elements registered via
    // `document.registerElement`
    if (tag.indexOf('-') > -1) {
        // Registered elements have their own constructor, while unregistered
        // ones use the `HTMLElement` or `HTMLUnknownElement` (if invalid name)
        // constructor (http://stackoverflow.com/a/28210364/1070244)
        return (element.constructor !== window.HTMLUnknownElement &&
            element.constructor !== window.HTMLElement);
    }
    // Obtain the element's internal [[Class]] property, if it doesn't 
    // match the `HTMLUnknownElement` interface than it must be supported
    return toString.call(element) !== '[object HTMLUnknownElement]';
}
;
