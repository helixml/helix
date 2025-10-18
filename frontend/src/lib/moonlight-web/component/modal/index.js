var __awaiter = (this && this.__awaiter) || function (thisArg, _arguments, P, generator) {
    function adopt(value) { return value instanceof P ? value : new P(function (resolve) { resolve(value); }); }
    return new (P || (P = Promise))(function (resolve, reject) {
        function fulfilled(value) { try { step(generator.next(value)); } catch (e) { reject(e); } }
        function rejected(value) { try { step(generator["throw"](value)); } catch (e) { reject(e); } }
        function step(result) { result.done ? resolve(result.value) : adopt(result.value).then(fulfilled, rejected); }
        step((generator = generator.apply(thisArg, _arguments || [])).next());
    });
};
import { showErrorPopup } from "../error.js";
import { FormModal } from "./form.js";
let modalAbort = null;
const modalBackground = document.getElementById("modal-overlay");
const modalParent = document.getElementById("modal-parent");
let previousModal = null;
// Don't allow context menu event through this background
modalBackground === null || modalBackground === void 0 ? void 0 : modalBackground.addEventListener("contextmenu", event => {
    event.stopImmediatePropagation();
});
export function getModalBackground() {
    return modalBackground;
}
export function showModal(modal) {
    return __awaiter(this, void 0, void 0, function* () {
        if (modalParent == null) {
            showErrorPopup("cannot find modal parent");
            return null;
        }
        if (modalBackground == null) {
            showErrorPopup("the modal overlay cannot be found");
        }
        if (modalAbort != null) {
            showErrorPopup("cannot mount 2 modals at the same time");
            modalAbort.abort();
            return null;
        }
        if (previousModal) {
            previousModal.unmount(modalParent);
        }
        previousModal = modal;
        const abortController = new AbortController();
        modalAbort = abortController;
        modal.mount(modalParent);
        modalBackground === null || modalBackground === void 0 ? void 0 : modalBackground.classList.remove("modal-disabled");
        const output = yield modal.onFinish(abortController.signal);
        modalBackground === null || modalBackground === void 0 ? void 0 : modalBackground.classList.add("modal-disabled");
        modalAbort.abort();
        modalAbort = null;
        return output;
    });
}
/// --- Helper Modals
export function showPrompt(prompt, promptInit) {
    return __awaiter(this, void 0, void 0, function* () {
        const modal = new PromptModal(prompt, promptInit);
        return yield showModal(modal);
    });
}
class PromptModal extends FormModal {
    constructor(prompt, init) {
        super();
        this.message = document.createElement("p");
        this.textInput = document.createElement("input");
        this.message.innerText = prompt;
        if (init === null || init === void 0 ? void 0 : init.type) {
            this.textInput.type = init === null || init === void 0 ? void 0 : init.type;
        }
        if (init === null || init === void 0 ? void 0 : init.defaultValue) {
            this.textInput.defaultValue = init === null || init === void 0 ? void 0 : init.defaultValue;
        }
        if (init === null || init === void 0 ? void 0 : init.name) {
            this.textInput.name = init === null || init === void 0 ? void 0 : init.name;
        }
    }
    reset() {
        this.textInput.value = "";
    }
    submit() {
        return this.textInput.value;
    }
    mountForm(form) {
        form.appendChild(this.message);
        form.appendChild(this.textInput);
    }
}
export function showMessage(message, init) {
    return __awaiter(this, void 0, void 0, function* () {
        const modal = new MessageModal(message, init);
        yield showModal(modal);
    });
}
class MessageModal {
    constructor(message, init) {
        this.textElement = document.createElement("p");
        this.okButton = document.createElement("button");
        this.textElement.innerText = message;
        this.okButton.innerText = "Ok";
        this.signal = init === null || init === void 0 ? void 0 : init.signal;
    }
    mount(parent) {
        parent.appendChild(this.textElement);
        parent.appendChild(this.okButton);
    }
    unmount(parent) {
        parent.removeChild(this.textElement);
        parent.removeChild(this.okButton);
    }
    onFinish(abort) {
        return new Promise((resolve, reject) => {
            let customController = null;
            if (this.signal) {
                customController = new AbortController();
                this.signal.addEventListener("abort", () => {
                    resolve();
                    customController === null || customController === void 0 ? void 0 : customController.abort();
                }, { once: true, signal: customController.signal });
            }
            this.okButton.addEventListener("click", () => {
                resolve();
                customController === null || customController === void 0 ? void 0 : customController.abort();
            }, { signal: customController === null || customController === void 0 ? void 0 : customController.signal });
            if (customController) {
                abort.addEventListener("abort", customController.abort.bind(customController));
            }
        });
    }
}
