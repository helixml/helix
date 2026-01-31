import { TypesSession } from "../api/api";
import {
    IApp,
    IDataPrepChunkWithFilename,
    IDataPrepStats,
    IModelInstanceState,
    IPageBreadcrumb,
    ISessionMode,
    ISessionSummary,
    ISessionType,
    ITextDataPrepStage,
    SESSION_MODE_FINETUNE,
    SESSION_MODE_INFERENCE,
    SESSION_TYPE_IMAGE,
    TEXT_DATA_PREP_DISPLAY_STAGES,
    TEXT_DATA_PREP_STAGE_NONE,
    TEXT_DATA_PREP_STAGES,
} from "../types";

import { getAppName } from "./apps";

const NO_DATE = "0001-01-01T00:00:00Z";

const COLORS: Record<string, string> = {
    // Diffusers/image models
    diffusers_inference: "#D183C9", // Purple for Diffusers inference
    diffusers_finetune: "#E3879E", // Pink for Diffusers finetune

    // Text models
    vllm_inference: "#72C99A", // Green for VLLM inference
    vllm_finetune: "#50B37D", // Darker green for VLLM finetune

    // Ollama models
    ollama_inference: "#F4D35E", // Yellow for Ollama inference
    ollama_finetune: "#EE964B", // Orange-yellow for Ollama finetune

    // Axolotl models
    axolotl_inference: "#FF6B6B", // Red for Axolotl inference
    axolotl_finetune: "#CC5151", // Darker red for Axolotl finetune

    // Legacy model mappings (keeping for compatibility)
    sdxl_inference: "#D183C9", // Map to diffusers
    sdxl_finetune: "#E3879E", // Map to diffusers
    mistral_inference: "#FF6B6B", // Map to axolotl
    mistral_finetune: "#CC5151", // Map to axolotl
    text_inference: "#FF6B6B", // Map to axolotl
    image_inference: "#D183C9", // Map to diffusers
    image_finetune: "#E3879E", // Map to diffusers
};

export const hasDate = (dt?: string): boolean => {
    if (!dt) return false;
    return dt != NO_DATE;
};

export const getColor = (modelName: string): string => {
    // Get the model type first
    const modelType = getModelName(modelName);

    // Build the key to look up in COLORS record
    const key = `${modelType}`;

    // Return the corresponding color
    return COLORS[key];
};

export const getModelName = (model_name: string): string => {
    // Diffusers/image models
    if (
        model_name.indexOf("stabilityai") >= 0 ||
        model_name.indexOf("diffusers") >= 0 ||
        model_name === "image" ||
        model_name.indexOf("sdxl") >= 0
    )
        return "diffusers";

    // Ollama models - check before VLLM since qwen models use colon format
    if (model_name.indexOf(":") >= 0 || model_name.indexOf("ollama") >= 0)
        return "ollama";

    // VLLM models
    if (
        model_name.indexOf("vllm") >= 0 ||
        model_name.toLowerCase().indexOf("qwen") >= 0
    )
        return "vllm";

    // Axolotl models
    if (
        model_name.indexOf("mistral") >= 0 ||
        model_name.indexOf("axolotl") >= 0 ||
        model_name === "text"
    )
        return "axolotl";

    return "";
};

export const getHeadline = (
    modelName: string,
    mode: string,
    loraDir = "",
): string => {
    let loraString = "";
    if (loraDir) {
        const parts = loraDir.split("/");
        const id = parts[parts.length - 2];
        loraString = ` - ${id.split("-").pop()}`;
    }
    return `${getModelName(modelName)} ${mode} ${loraString}`;
};

export const getSessionHeadline = (session: ISessionSummary): string => {
    return `${getHeadline(session.model_name, session.mode, session.lora_dir)} : ${shortID(session.session_id)} : ${getTiming(session)}`;
};

export const getModelInstanceNoSessionHeadline = (
    modelInstance: IModelInstanceState,
): string => {
    return `${getHeadline(modelInstance.model_name, modelInstance.mode, modelInstance.lora_dir)} : ${getModelInstanceIdleTime(modelInstance)}`;
};

export const getSummaryCaption = (session: ISessionSummary): string => {
    return session.summary;
};

export const getModelInstanceIdleTime = (
    modelInstance: IModelInstanceState,
): string => {
    if (!modelInstance.last_activity) return "";
    const idleFor = Date.now() - modelInstance.last_activity * 1000;
    const idleForSeconds = Math.floor(idleFor / 1000);
    return `idle for ${idleForSeconds} secs, timeout is ${modelInstance.timeout} secs, stale = ${modelInstance.stale}`;
};

export const shortID = (id: string): string => {
    return id.split("-").shift() || "";
};

export const getTiming = (session: ISessionSummary): string => {
    if (hasDate(session?.scheduled)) {
        const runningFor =
            Date.now() - new Date(session?.scheduled || "").getTime();
        const runningForSeconds = Math.floor(runningFor / 1000);
        return `${runningForSeconds} secs`;
    } else if (hasDate(session?.created)) {
        const waitingFor =
            Date.now() - new Date(session?.created || "").getTime();
        const waitingForSeconds = Math.floor(waitingFor / 1000);
        return `${waitingForSeconds} secs`;
    } else {
        return "";
    }
};

export const getSessionSummary = (session: TypesSession): ISessionSummary => {
    let summary = "";
    if (session.mode == SESSION_MODE_INFERENCE) {
        summary = session.interactions?.[0]?.prompt_message || "";
    }
    return {
        session_id: session.id || "",
        name: session.name || "",
        interaction_id: session.interactions?.[0]?.id || "",
        mode: session.mode || "",
        type: session.type || "",
        model_name: session.model_name || "",
        owner: session.owner || "",
        lora_dir: session.lora_dir,
        created: session.interactions?.[0]?.created || "",
        updated: session.interactions?.[0]?.updated || "",
        scheduled: session.interactions?.[0]?.scheduled || "",
        completed: session.interactions?.[0]?.completed || "",
        summary,
    };
};

/**
 * Helper function to escape special characters in a string for use in RegExp
 * This is exported for use by MessageProcessor in Markdown.tsx
 */
export function escapeRegExp(string: string): string {
    return string.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"); // $& means the whole matched string
}

export const getNewSessionBreadcrumbs = ({
    mode,
    type,
    ragEnabled,
    finetuneEnabled,
    app,
}: {
    mode: ISessionMode;
    type: ISessionType;
    ragEnabled: boolean;
    finetuneEnabled: boolean;
    app?: IApp;
}): IPageBreadcrumb[] => {
    if (mode == SESSION_MODE_FINETUNE) {
        let txt = "Add Documents";
        if (type == SESSION_TYPE_IMAGE) {
            txt += " (image style and objects)";
        } else if (ragEnabled && finetuneEnabled) {
            txt += " (hybrid RAG + Fine-tuning)";
        } else if (ragEnabled) {
            txt += " (RAG)";
        } else if (finetuneEnabled) {
            txt += " (Fine-tuning on knowledge)";
        }
        return [
            {
                title: txt,
            },
        ];
    } else if (app) {
        return [
            {
                title: 'Agents',
                routeName: 'apps',
            },
            {
                title: getAppName(app),
            },
        ];
    }

    return [];
};
