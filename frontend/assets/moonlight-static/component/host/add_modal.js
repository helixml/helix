import { InputComponent } from "../input.js";
import { FormModal } from "../modal/form.js";
export class AddHostModal extends FormModal {
    constructor() {
        super();
        this.header = document.createElement("h2");
        this.header.innerText = "Host";
        this.address = new InputComponent("address", "text", "Address");
        this.httpPort = new InputComponent("httpPort", "text", "Port", {
            inputMode: "numeric"
        });
    }
    reset() {
        this.address.reset();
        this.httpPort.reset();
    }
    submit() {
        const address = this.address.getValue();
        const httpPort = parseInt(this.httpPort.getValue());
        return {
            address,
            http_port: httpPort
        };
    }
    mountForm(form) {
        form.appendChild(this.header);
        this.address.mount(form);
        this.httpPort.mount(form);
    }
}
