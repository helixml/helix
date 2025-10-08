var __awaiter = (this && this.__awaiter) || function (thisArg, _arguments, P, generator) {
    function adopt(value) { return value instanceof P ? value : new P(function (resolve) { resolve(value); }); }
    return new (P || (P = Promise))(function (resolve, reject) {
        function fulfilled(value) { try { step(generator.next(value)); } catch (e) { reject(e); } }
        function rejected(value) { try { step(generator["throw"](value)); } catch (e) { reject(e); } }
        function step(result) { result.done ? resolve(result.value) : adopt(result.value).then(fulfilled, rejected); }
        step((generator = generator.apply(thisArg, _arguments || [])).next());
    });
};
import { showErrorPopup } from "./component/error.js";
import { InputComponent } from "./component/input.js";
import { FormModal } from "./component/modal/form.js";
import { showMessage, showModal } from "./component/modal/index.js";
import { buildUrl } from "./config_.js";
// IMPORTANT: this should be a bit bigger than the moonlight-common reqwest backend timeout if some hosts are offline!
const API_TIMEOUT = 6000;
let currentApi = null;
export function getApi(host_url) {
    return __awaiter(this, void 0, void 0, function* () {
        if (currentApi) {
            return currentApi;
        }
        if (!host_url) {
            host_url = buildUrl("/api");
        }
        let credentials = sessionStorage.getItem("mlCredentials");
        while (credentials == null) {
            const prompt = new ApiCredentialsPrompt();
            const testCredentials = yield showModal(prompt);
            if (testCredentials == null) {
                continue;
            }
            let api = { host_url, credentials: testCredentials };
            if (yield apiAuthenticate(api)) {
                sessionStorage.setItem("mlCredentials", testCredentials);
                credentials = api.credentials;
                break;
            }
            else {
                yield showMessage("Credentials are not Valid");
            }
        }
        currentApi = { host_url, credentials };
        return currentApi;
    });
}
class ApiCredentialsPrompt extends FormModal {
    constructor() {
        super();
        this.text = document.createElement("h3");
        this.text.innerText = "Enter Credentials";
        this.credentials = new InputComponent("ml-api-credentials", "password", "Credentials");
        this.credentialsFile = new InputComponent("ml-api-credentials-file", "file", "Credentials as File", { accept: ".txt" });
    }
    reset() {
        this.credentials.reset();
    }
    submit() {
        return this.credentials.getValue();
    }
    onFinish(abort) {
        const abortController = new AbortController();
        abort.addEventListener("abort", abortController.abort.bind(abortController));
        return new Promise((resolve, reject) => {
            this.credentialsFile.addChangeListener(() => {
                const files = this.credentialsFile.getFiles();
                if (files && files.length >= 1) {
                    const file = files[0];
                    file.text().then((credentials) => {
                        abortController.abort();
                        resolve(credentials);
                    });
                }
            }, { signal: abortController.signal });
            super.onFinish(abortController.signal).then((data) => {
                abortController.abort();
                resolve(data);
            }, (data) => {
                abortController.abort();
                reject(data);
            });
        });
    }
    mountForm(form) {
        form.appendChild(this.text);
        this.credentials.mount(form);
        this.credentialsFile.mount(form);
    }
}
export function isDetailedHost(host) {
    return host.https_port !== undefined;
}
function buildRequest(api, endpoint, method, init) {
    const query = new URLSearchParams(init === null || init === void 0 ? void 0 : init.query);
    const queryString = query.size > 0 ? `?${query.toString()}` : "";
    const url = `${api.host_url}${endpoint}${queryString}`;
    const headers = {
        "Authorization": `Bearer ${api.credentials}`,
    };
    if (init === null || init === void 0 ? void 0 : init.json) {
        headers["Content-Type"] = "application/json";
    }
    const request = {
        method: method,
        headers,
        body: (init === null || init === void 0 ? void 0 : init.json) && JSON.stringify(init.json)
    };
    return [url, request];
}
export class FetchError extends Error {
    constructor(type, endpoint, method, response) {
        if (type == "timeout") {
            super(`failed to fetch ${method} at ${endpoint} because of timeout`);
        }
        else {
            super(`failed to fetch ${method} at ${endpoint} with code ${response === null || response === void 0 ? void 0 : response.status}`);
        }
        this.response = response;
    }
    getResponse() {
        var _a;
        return (_a = this.response) !== null && _a !== void 0 ? _a : null;
    }
}
export function fetchApi(api_1, endpoint_1) {
    return __awaiter(this, arguments, void 0, function* (api, endpoint, method = "get", init) {
        const [url, request] = buildRequest(api, endpoint, method, init);
        const timeoutAbort = new AbortController();
        request.signal = timeoutAbort.signal;
        if (!(init === null || init === void 0 ? void 0 : init.noTimeout)) {
            setTimeout(() => timeoutAbort.abort(new FetchError("timeout", endpoint, method)), API_TIMEOUT);
        }
        const response = yield fetch(url, request);
        if (!response.ok) {
            throw new FetchError("failed", endpoint, method, response);
        }
        if ((init === null || init === void 0 ? void 0 : init.response) == "ignore") {
            return response;
        }
        if ((init === null || init === void 0 ? void 0 : init.response) == undefined || init.response == "json") {
            const json = yield response.json();
            return json;
        }
    });
}
export function apiAuthenticate(api) {
    return __awaiter(this, void 0, void 0, function* () {
        let response;
        try {
            response = yield fetchApi(api, "/authenticate", "get", { response: "ignore" });
        }
        catch (e) {
            if (e instanceof FetchError) {
                const response = e.getResponse();
                if (response && response.status == 403) {
                    return false;
                }
                else {
                    showErrorPopup(e.message);
                    return false;
                }
            }
            throw e;
        }
        return response != null;
    });
}
export function apiGetHosts(api) {
    return __awaiter(this, void 0, void 0, function* () {
        const response = yield fetchApi(api, "/hosts", "get");
        return response.hosts;
    });
}
export function apiGetHost(api, query) {
    return __awaiter(this, void 0, void 0, function* () {
        const response = yield fetchApi(api, "/host", "get", { query });
        return response.host;
    });
}
export function apiPutHost(api, data) {
    return __awaiter(this, void 0, void 0, function* () {
        const response = yield fetchApi(api, "/host", "put", { json: data });
        return response.host;
    });
}
export function apiDeleteHost(api, query) {
    return __awaiter(this, void 0, void 0, function* () {
        try {
            yield fetchApi(api, "/host", "delete", { query, response: "ignore" });
        }
        catch (e) {
            return false;
        }
        return true;
    });
}
export function apiPostPair(api, request) {
    return __awaiter(this, void 0, void 0, function* () {
        const response = yield fetchApi(api, "/pair", "post", {
            json: request,
            response: "ignore",
            noTimeout: true
        });
        if (!response.body) {
            throw "no response body in pair response";
        }
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        const read1 = yield reader.read();
        const response1 = JSON.parse(decoder.decode(read1.value));
        if (typeof response1 == "string") {
            throw `failed to pair: ${response1}`;
        }
        if (read1.done) {
            throw "failed to pair: InternalServerError";
        }
        return {
            pin: response1.Pin,
            result: (() => __awaiter(this, void 0, void 0, function* () {
                const read2 = yield reader.read();
                const response2 = JSON.parse(decoder.decode(read2.value));
                if (response2 == "PairError") {
                    throw "failed to pair";
                }
                else {
                    return response2.Paired;
                }
            }))()
        };
    });
}
export function apiWakeUp(api, request) {
    return __awaiter(this, void 0, void 0, function* () {
        yield fetchApi(api, "/host/wake", "post", {
            json: request,
            response: "ignore"
        });
    });
}
export function apiGetApps(api, query) {
    return __awaiter(this, void 0, void 0, function* () {
        const response = yield fetchApi(api, "/apps", "get", { query });
        return response.apps;
    });
}
export function apiGetAppImage(api, query) {
    return __awaiter(this, void 0, void 0, function* () {
        const response = yield fetchApi(api, "/app/image", "get", {
            query,
            response: "ignore"
        });
        return yield response.blob();
    });
}
export function apiHostCancel(api, request) {
    return __awaiter(this, void 0, void 0, function* () {
        const response = yield fetchApi(api, "/host/cancel", "POST", {
            json: request
        });
        return response;
    });
}
