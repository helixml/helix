import { ComponentEvent } from "./index.js";
import { InputComponent, SelectComponent } from "./input.js";
export function defaultStreamSettings() {
    return {
        sidebarEdge: "left",
        bitrate: 10000,
        packetSize: 256,
        fps: 60,
        videoSampleQueueSize: 6,
        videoSize: "1080p",
        videoSizeCustom: {
            width: 1920,
            height: 1080,
        },
        dontForceH264: false,
        playAudioLocal: false,
        audioSampleQueueSize: 20,
        mouseScrollMode: "highres",
        controllerConfig: {
            invertAB: false,
            invertXY: false
        }
    };
}
export function getLocalStreamSettings() {
    let settings = null;
    try {
        const settingsLoadedJson = localStorage.getItem("mlSettings");
        if (settingsLoadedJson == null) {
            return null;
        }
        const settingsLoaded = JSON.parse(settingsLoadedJson);
        settings = defaultStreamSettings();
        Object.assign(settings, settingsLoaded);
    }
    catch (e) {
        localStorage.removeItem("mlSettings");
    }
    return settings;
}
export function setLocalStreamSettings(settings) {
    localStorage.setItem("mlSettings", JSON.stringify(settings));
}
export class StreamSettingsComponent {
    constructor(settings) {
        var _a, _b, _c, _d, _e, _f;
        this.divElement = document.createElement("div");
        this.sidebarHeader = document.createElement("h2");
        this.streamHeader = document.createElement("h2");
        this.audioHeader = document.createElement("h2");
        this.mouseHeader = document.createElement("h2");
        this.controllerHeader = document.createElement("h2");
        const defaultSettings = defaultStreamSettings();
        // Root div
        this.divElement.classList.add("settings");
        // Sidebar
        this.sidebarHeader.innerText = "Sidebar";
        this.divElement.appendChild(this.sidebarHeader);
        this.sidebarEdge = new SelectComponent("sidebarEdge", [
            { value: "left", name: "Left" },
            { value: "right", name: "Right" },
            { value: "up", name: "Up" },
            { value: "down", name: "Down" },
        ], {
            displayName: "Sidebar Edge",
            preSelectedOption: (_a = settings === null || settings === void 0 ? void 0 : settings.sidebarEdge) !== null && _a !== void 0 ? _a : defaultSettings.sidebarEdge,
        });
        this.sidebarEdge.addChangeListener(this.onSettingsChange.bind(this));
        this.sidebarEdge.mount(this.divElement);
        // Video
        this.streamHeader.innerText = "Video";
        this.divElement.appendChild(this.streamHeader);
        // Bitrate
        this.bitrate = new InputComponent("bitrate", "number", "Bitrate", {
            defaultValue: defaultSettings.bitrate.toString(),
            value: (_b = settings === null || settings === void 0 ? void 0 : settings.bitrate) === null || _b === void 0 ? void 0 : _b.toString(),
            step: "100",
        });
        this.bitrate.addChangeListener(this.onSettingsChange.bind(this));
        this.bitrate.mount(this.divElement);
        // Packet Size
        this.packetSize = new InputComponent("packetSize", "number", "Packet Size", {
            defaultValue: defaultSettings.packetSize.toString(),
            value: (_c = settings === null || settings === void 0 ? void 0 : settings.packetSize) === null || _c === void 0 ? void 0 : _c.toString(),
            step: "100"
        });
        this.packetSize.addChangeListener(this.onSettingsChange.bind(this));
        this.packetSize.mount(this.divElement);
        // Fps
        this.fps = new InputComponent("fps", "number", "Fps", {
            defaultValue: defaultSettings.fps.toString(),
            value: (_d = settings === null || settings === void 0 ? void 0 : settings.fps) === null || _d === void 0 ? void 0 : _d.toString(),
            step: "100"
        });
        this.fps.addChangeListener(this.onSettingsChange.bind(this));
        this.fps.mount(this.divElement);
        // Video Size
        this.videoSize = new SelectComponent("videoSize", [
            { value: "720p", name: "720p" },
            { value: "1080p", name: "1080p" },
            { value: "1440p", name: "1440p" },
            { value: "4k", name: "4k" },
            { value: "native", name: "native" },
            { value: "custom", name: "custom" }
        ], {
            displayName: "Video Size",
            preSelectedOption: (settings === null || settings === void 0 ? void 0 : settings.videoSize) || defaultSettings.videoSize
        });
        this.videoSize.addChangeListener(this.onSettingsChange.bind(this));
        this.videoSize.mount(this.divElement);
        this.videoSizeWidth = new InputComponent("videoSizeWidth", "number", "Video Width", {
            defaultValue: defaultSettings.videoSizeCustom.width.toString(),
            value: settings === null || settings === void 0 ? void 0 : settings.videoSizeCustom.width.toString()
        });
        this.videoSizeWidth.addChangeListener(this.onSettingsChange.bind(this));
        this.videoSizeWidth.mount(this.divElement);
        this.videoSizeHeight = new InputComponent("videoSizeHeight", "number", "Video Height", {
            defaultValue: defaultSettings.videoSizeCustom.height.toString(),
            value: settings === null || settings === void 0 ? void 0 : settings.videoSizeCustom.height.toString()
        });
        this.videoSizeHeight.addChangeListener(this.onSettingsChange.bind(this));
        this.videoSizeHeight.mount(this.divElement);
        // Video Sample Queue Size
        this.videoSampleQueueSize = new InputComponent("videoSampleQueueSize", "number", "Video Sample Queue Size", {
            defaultValue: defaultSettings.videoSampleQueueSize.toString(),
            value: (_e = settings === null || settings === void 0 ? void 0 : settings.videoSampleQueueSize) === null || _e === void 0 ? void 0 : _e.toString()
        });
        this.videoSampleQueueSize.addChangeListener(this.onSettingsChange.bind(this));
        this.videoSampleQueueSize.mount(this.divElement);
        // Force H264
        this.forceH264 = new InputComponent("dontForceH264", "checkbox", "Select Codec based on Support in Browser (Experimental)", {
            defaultValue: defaultSettings.dontForceH264.toString(),
            checked: settings === null || settings === void 0 ? void 0 : settings.dontForceH264
        });
        this.forceH264.addChangeListener(this.onSettingsChange.bind(this));
        this.forceH264.mount(this.divElement);
        // Audio local
        this.audioHeader.innerText = "Audio";
        this.divElement.appendChild(this.audioHeader);
        this.playAudioLocal = new InputComponent("playAudioLocal", "checkbox", "Play Audio Local", {
            checked: settings === null || settings === void 0 ? void 0 : settings.playAudioLocal
        });
        this.playAudioLocal.addChangeListener(this.onSettingsChange.bind(this));
        this.playAudioLocal.mount(this.divElement);
        // Audio Sample Queue Size
        this.audioSampleQueueSize = new InputComponent("audioSampleQueueSize", "number", "Audio Sample Queue Size", {
            defaultValue: defaultSettings.audioSampleQueueSize.toString(),
            value: (_f = settings === null || settings === void 0 ? void 0 : settings.audioSampleQueueSize) === null || _f === void 0 ? void 0 : _f.toString()
        });
        this.audioSampleQueueSize.addChangeListener(this.onSettingsChange.bind(this));
        this.audioSampleQueueSize.mount(this.divElement);
        // Mouse
        this.mouseHeader.innerText = "Mouse";
        this.divElement.appendChild(this.mouseHeader);
        this.mouseScrollMode = new SelectComponent("mouseScrollMode", [
            { value: "highres", name: "High Res" },
            { value: "normal", name: "Normal" }
        ], {
            displayName: "Scroll Mode",
            preSelectedOption: (settings === null || settings === void 0 ? void 0 : settings.mouseScrollMode) || defaultSettings.mouseScrollMode
        });
        this.mouseScrollMode.addChangeListener(this.onSettingsChange.bind(this));
        this.mouseScrollMode.mount(this.divElement);
        // Controller
        if (window.isSecureContext) {
            this.controllerHeader.innerText = "Controller";
        }
        else {
            this.controllerHeader.innerText = "Controller (Disabled: Secure Context Required)";
        }
        this.divElement.appendChild(this.controllerHeader);
        this.controllerInvertAB = new InputComponent("controllerInvertAB", "checkbox", "Invert A and B", {
            checked: settings === null || settings === void 0 ? void 0 : settings.controllerConfig.invertAB
        });
        this.controllerInvertAB.addChangeListener(this.onSettingsChange.bind(this));
        this.controllerInvertAB.mount(this.divElement);
        this.controllerInvertXY = new InputComponent("controllerInvertXY", "checkbox", "Invert X and Y", {
            checked: settings === null || settings === void 0 ? void 0 : settings.controllerConfig.invertXY
        });
        this.controllerInvertXY.addChangeListener(this.onSettingsChange.bind(this));
        this.controllerInvertXY.mount(this.divElement);
        if (!window.isSecureContext) {
            this.controllerInvertAB.setEnabled(false);
            this.controllerInvertXY.setEnabled(false);
        }
        this.onSettingsChange();
    }
    onSettingsChange() {
        if (this.videoSize.getValue() == "custom") {
            this.videoSizeWidth.setEnabled(true);
            this.videoSizeHeight.setEnabled(true);
        }
        else {
            this.videoSizeWidth.setEnabled(false);
            this.videoSizeHeight.setEnabled(false);
        }
        this.divElement.dispatchEvent(new ComponentEvent("ml-settingschange", this));
    }
    addChangeListener(listener) {
        this.divElement.addEventListener("ml-settingschange", listener);
    }
    removeChangeListener(listener) {
        this.divElement.removeEventListener("ml-settingschange", listener);
    }
    getStreamSettings() {
        const settings = defaultStreamSettings();
        settings.sidebarEdge = this.sidebarEdge.getValue();
        settings.bitrate = parseInt(this.bitrate.getValue());
        settings.packetSize = parseInt(this.packetSize.getValue());
        settings.fps = parseInt(this.fps.getValue());
        settings.videoSize = this.videoSize.getValue();
        settings.videoSizeCustom = {
            width: parseInt(this.videoSizeWidth.getValue()),
            height: parseInt(this.videoSizeHeight.getValue())
        };
        settings.videoSampleQueueSize = parseInt(this.videoSampleQueueSize.getValue());
        settings.dontForceH264 = this.forceH264.isChecked();
        settings.playAudioLocal = this.playAudioLocal.isChecked();
        settings.audioSampleQueueSize = parseInt(this.audioSampleQueueSize.getValue());
        settings.mouseScrollMode = this.mouseScrollMode.getValue();
        settings.controllerConfig.invertAB = this.controllerInvertAB.isChecked();
        settings.controllerConfig.invertXY = this.controllerInvertXY.isChecked();
        return settings;
    }
    mount(parent) {
        parent.appendChild(this.divElement);
    }
    unmount(parent) {
        parent.removeChild(this.divElement);
    }
}
