import { FetchError } from "../api.js";
import { ERROR_IMAGE, WARN_IMAGE } from "../resources/index.js";
import { ListComponent } from "./list.js";
const ERROR_REMOVAL_TIME_MS = 10000;
const errorListElement = document.getElementById("error-list");
const errorListComponent = new ListComponent([], { listClasses: ["error-list"], elementDivClasses: ["error-element"] });
if (errorListElement) {
    errorListComponent.mount(errorListElement);
}
let alertedErrorListNotFound = false;
export function showErrorPopup(message, fatal = false, errorObject) {
    console.error(message, errorObject);
    if (!errorListElement) {
        if (!alertedErrorListNotFound) {
            alert("couldn't find the error element");
            alertedErrorListNotFound = true;
        }
        alert(message);
        return;
    }
    let error;
    if (fatal) {
        error = new ErrorComponent(message, ERROR_IMAGE);
    }
    else {
        error = new ErrorComponent(message, WARN_IMAGE);
    }
    errorListComponent.append(error);
    setTimeout(() => {
        errorListComponent.removeValue(error);
    }, ERROR_REMOVAL_TIME_MS);
}
function handleError(event) {
    const fatal = event instanceof FetchError;
    showErrorPopup(`${event.error}`, fatal, event);
}
function handleRejection(event) {
    const fatal = event instanceof FetchError;
    showErrorPopup(`${event.reason}`, fatal, event);
}
window.addEventListener("error", handleError);
window.addEventListener("unhandledrejection", handleRejection);
class ErrorComponent {
    constructor(message, image) {
        this.messageElement = document.createElement("p");
        this.imageElement = document.createElement("img");
        this.messageElement.innerText = message;
        this.messageElement.classList.add("error-message");
        this.imageElement.src = image;
        this.imageElement.classList.add("error-image");
    }
    mount(parent) {
        parent.appendChild(this.imageElement);
        parent.appendChild(this.messageElement);
    }
    unmount(parent) {
        parent.removeChild(this.imageElement);
        parent.removeChild(this.messageElement);
    }
}
