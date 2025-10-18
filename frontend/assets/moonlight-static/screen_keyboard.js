export class ScreenKeyboard {
    constructor() {
        this.eventTarget = new EventTarget();
        this.fakeElement = document.createElement("input");
        this.visible = false;
        this.fakeElement.classList.add("hiddeninput");
        this.fakeElement.type = "text";
        this.fakeElement.name = "keyboard";
        this.fakeElement.autocomplete = "off";
        this.fakeElement.autocapitalize = "off";
        this.fakeElement.spellcheck = false;
        if ("autocorrect" in this.fakeElement) {
            this.fakeElement.autocorrect = false;
        }
        this.fakeElement.addEventListener("input", this.onKeyInput.bind(this));
        document.addEventListener("click", this.hide.bind(this));
        this.fakeElement.addEventListener("blur", this.hide.bind(this));
    }
    getHiddenElement() {
        return this.fakeElement;
    }
    show() {
        if (!this.visible) {
            this.visible = true;
            this.fakeElement.focus();
        }
    }
    hide() {
        if (this.visible) {
            this.visible = false;
            this.fakeElement.focus();
            this.fakeElement.blur();
        }
    }
    isVisible() {
        return this.visible;
    }
    addKeyDownListener(listener) {
        this.eventTarget.addEventListener("keydown", listener);
    }
    addKeyUpListener(listener) {
        this.eventTarget.addEventListener("keyup", listener);
    }
    addTextListener(listener) {
        this.eventTarget.addEventListener("ml-text", listener);
    }
    // -- Events
    onKeyInput(event) {
        if (!(event instanceof InputEvent)) {
            return;
        }
        if (event.isComposing) {
            return;
        }
        if ((event.inputType == "insertText" || event.inputType == "insertFromPaste") && event.data != null) {
            const customEvent = new CustomEvent("ml-text", {
                detail: { text: event.data }
            });
            this.eventTarget.dispatchEvent(customEvent);
        }
        else if (event.inputType == "deleteContentBackward" || event.inputType == "deleteByCut") {
            // these are handled by on key down / up on mobile
        }
        else if (event.inputType == "deleteContentForward") {
            // these are handled by on key down / up on mobile
        }
    }
}
