/**
 * Turns the raw error string a failed agent turn leaves on an interaction into
 * something a user can act on: what probably went wrong, and how to recover.
 *
 * The raw string is a stack of wrappers from every layer it passed through,
 * e.g. `agent turn aborted: Internal error: Internal Server Error: error,
 * status code: 404, status: 404 Not Found, message: model not found for API
 * profile: {"service": "session", "errorName": "APIError"}`. Only the innermost
 * provider status and message say anything useful.
 */

export type InferenceErrorKind =
  | "insufficient_balance"
  | "bot_provider_unavailable"
  | "model_not_found"
  | "auth"
  | "rate_limited"
  | "provider_unavailable"
  | "context_too_long"
  | "invalid_request"
  | "timeout"
  | "unknown";

export interface InferenceErrorDescription {
  kind: InferenceErrorKind;
  title: string;
  /** What probably went wrong, in plain language. */
  summary: string;
  /** Concrete recovery steps, in the order a user should try them. */
  steps: string[];
  /** Picking a different model or provider is likely to fix this. */
  suggestSwitchModel: boolean;
  /** Re-sending the same prompt has a reasonable chance of succeeding. */
  retryable: boolean;
  /** HTTP status the model provider returned, when the error carries one. */
  providerStatus?: number;
  /** The provider's own explanation, stripped of wrapper noise. */
  providerMessage?: string;
}

const WRAPPER_PREFIXES = [
  /^agent turn aborted:\s*/i,
  /^agent startup failed:\s*/i,
  /^internal error:\s*/i,
  /^internal server error:\s*/i,
  /^error:\s*/i,
];

// Agents append their structured log context to the message.
const TRAILING_LOG_CONTEXT = /:?\s*\{\s*"service"\s*:[\s\S]*\}\s*$/;

function stripWrappers(error: string): string {
  let text = error.trim().replace(TRAILING_LOG_CONTEXT, "").trim();
  let changed = true;
  while (changed) {
    changed = false;
    for (const prefix of WRAPPER_PREFIXES) {
      const next = text.replace(prefix, "");
      if (next !== text) {
        text = next.trim();
        changed = true;
      }
    }
  }
  return text;
}

const STATUS_PATTERNS = [
  /status code:\s*(\d{3})/i,
  /\bstatus[:\s]+(\d{3})\b/i,
  /\bHTTP\s*(\d{3})\b/i,
  /"code"\s*:\s*(\d{3})/,
  /\b(\d{3})\s+(?:Not Found|Unauthorized|Forbidden|Too Many Requests|Bad Request|Bad Gateway|Service Unavailable|Gateway Timeout)\b/i,
];

function extractStatus(text: string): number | undefined {
  for (const pattern of STATUS_PATTERNS) {
    const match = text.match(pattern);
    if (match) return Number(match[1]);
  }
  return undefined;
}

function extractProviderMessage(text: string): string | undefined {
  // go-openai: "error, status code: 404, status: 404 Not Found, message: …"
  const goOpenAI = text.match(/\bmessage:\s*([\s\S]+)$/i);
  if (goOpenAI) return goOpenAI[1].trim();
  // Provider JSON bodies: {"error":{"message":"…"}}
  const jsonMessage = text.match(/"message"\s*:\s*"((?:[^"\\]|\\.)*)"/);
  if (jsonMessage) return jsonMessage[1].replace(/\\"/g, '"');
  return undefined;
}

function isBotProviderUnavailable(error: string): boolean {
  return /provider is not available to the organization/i.test(error)
    || /provider ".+" is not enabled for coding-agent (?:runtime|harness) ".+" in this organization/i.test(error)
    || (/anthropic/i.test(error)
      && /\b403\b/.test(error)
      && /SERVICE_DISABLED/i.test(error)
      && /aiplatform\.googleapis\.com/i.test(error))
    || /anthropic provider not configured/i.test(error);
}

const SWITCH_STEP = "Switch to a different model or provider, then send your message again.";

export function describeInferenceError(
  error: string,
  options: { isBotChat?: boolean } = {},
): InferenceErrorDescription {
  const cleaned = stripWrappers(error);
  const providerStatus = extractStatus(cleaned);
  const providerMessage = extractProviderMessage(cleaned);
  const base = { providerStatus, providerMessage };

  if (/insufficient balance/i.test(error)) {
    return {
      ...base,
      kind: "insufficient_balance",
      title: "More credits needed",
      summary: "Your organization doesn’t have enough credits. Add credits to continue.",
      steps: [],
      suggestSwitchModel: false,
      retryable: false,
    };
  }

  if (options.isBotChat && isBotProviderUnavailable(error)) {
    return {
      ...base,
      kind: "bot_provider_unavailable",
      title: "Model provider unavailable",
      summary: "This bot's model provider is unavailable. Choose an available provider and model in bot settings.",
      steps: [],
      suggestSwitchModel: true,
      retryable: false,
    };
  }

  if (
    /context[_ ]length[_ ]exceeded|maximum context length|prompt is too long|context window|too many tokens|input is too long/i.test(cleaned)
  ) {
    return {
      ...base,
      kind: "context_too_long",
      title: "This conversation is too long for the model",
      summary: "The conversation no longer fits in the selected model’s context window, so the provider rejected it.",
      steps: [
        "Switch to a model with a larger context window.",
        "Or start a new session with a short summary of where you got to.",
      ],
      suggestSwitchModel: true,
      retryable: false,
    };
  }

  if (
    providerStatus === 404
    || /model[_ ]not[_ ]found|model not found|no such model|unknown model|model .* does not exist|does not exist or you do not have access/i.test(cleaned)
  ) {
    return {
      ...base,
      kind: "model_not_found",
      title: "The AI provider couldn’t find this model",
      summary: "The model provider says the selected model doesn’t exist for this account. It may have been retired or renamed, or it isn’t enabled on the provider.",
      steps: [
        SWITCH_STEP,
        "If you manage this provider, check that the model is enabled on its account.",
      ],
      suggestSwitchModel: true,
      retryable: false,
    };
  }

  if (
    providerStatus === 401
    || providerStatus === 403
    || /invalid[_ ]api[_ ]key|incorrect api key|invalid x-api-key|authentication[_ ]error|unauthori[sz]ed|permission[_ ]denied|SERVICE_DISABLED/i.test(cleaned)
  ) {
    return {
      ...base,
      kind: "auth",
      title: "The AI provider rejected the request’s credentials",
      summary: "The provider’s API key is missing, invalid or expired, or it doesn’t have permission to use this model.",
      steps: [
        SWITCH_STEP,
        "If you manage this provider, update its API key or permissions on the Providers page.",
      ],
      suggestSwitchModel: true,
      retryable: false,
    };
  }

  if (
    providerStatus === 429
    || /rate[_ ]limit|too many requests|quota|resource[_ ]exhausted/i.test(cleaned)
  ) {
    return {
      ...base,
      kind: "rate_limited",
      title: "The AI provider is limiting requests",
      summary: "The provider’s rate limit or usage quota has been reached.",
      steps: [
        "Wait a minute, then retry.",
        "If it keeps happening, switch to a different model or provider.",
      ],
      suggestSwitchModel: true,
      retryable: true,
    };
  }

  if (
    (providerStatus !== undefined && providerStatus >= 500 && providerStatus !== 500)
    || /overloaded|service unavailable|bad gateway|upstream connect error|server_error|api_error/i.test(cleaned)
  ) {
    return {
      ...base,
      kind: "provider_unavailable",
      title: "The AI provider is having problems",
      summary: "The model provider returned an error on its side. This is usually temporary.",
      steps: [
        "Retry in a moment.",
        "If it keeps failing, switch to a different model or provider.",
      ],
      suggestSwitchModel: true,
      retryable: true,
    };
  }

  if (
    /context deadline exceeded|timed out|timeout|gateway timeout/i.test(cleaned)
  ) {
    return {
      ...base,
      kind: "timeout",
      title: "The AI provider took too long to respond",
      summary: "The request to the model provider timed out before it finished.",
      steps: [
        "Retry — timeouts are often temporary.",
        "If it keeps happening, switch to a faster model or a different provider.",
      ],
      suggestSwitchModel: true,
      retryable: true,
    };
  }

  if (providerStatus === 400 || /reasoning[_ ]effort|invalid_request_error/i.test(cleaned)) {
    return {
      ...base,
      kind: "invalid_request",
      title: "The AI provider rejected the request",
      summary: /reasoning[_ ]effort/i.test(cleaned)
        ? "The selected model doesn’t support the chosen reasoning effort."
        : "The selected model doesn’t accept a setting in this request.",
      steps: [
        "Change the reasoning effort, or switch to a different model, then send your message again.",
      ],
      suggestSwitchModel: true,
      retryable: false,
    };
  }

  return {
    ...base,
    kind: "unknown",
    title: "We couldn’t complete that request",
    summary: cleaned || error,
    steps: [],
    suggestSwitchModel: false,
    retryable: true,
  };
}
