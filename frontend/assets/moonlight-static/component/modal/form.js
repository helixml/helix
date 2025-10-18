export class FormModal {
    constructor() {
        this.formElement = document.createElement("form");
        this.mounted = false;
        this.submitButton = document.createElement("button");
        this.cancelButton = document.createElement("button");
        this.submitButton.type = "submit";
        this.submitButton.innerText = "Ok";
        this.cancelButton.innerText = "Cancel";
        this.formElement.addEventListener("submit", (event) => event.preventDefault());
    }
    mount(parent) {
        if (!this.mounted) {
            this.mountForm(this.formElement);
            this.formElement.appendChild(this.submitButton);
            this.formElement.appendChild(this.cancelButton);
        }
        this.reset();
        parent.appendChild(this.formElement);
    }
    unmount(parent) {
        parent.removeChild(this.formElement);
    }
    onFinish(signal) {
        const abortController = new AbortController();
        signal.addEventListener("abort", abortController.abort.bind(abortController));
        return new Promise((resolve, reject) => {
            this.formElement.addEventListener("submit", event => {
                const output = this.submit();
                if (output == null) {
                    return;
                }
                abortController.abort();
                resolve(output);
            }, { signal: abortController.signal });
            this.cancelButton.addEventListener("click", event => {
                event.preventDefault();
                abortController.abort();
                resolve(null);
            }, { signal: abortController.signal });
        });
    }
}
