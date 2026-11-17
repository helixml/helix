import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useEffect, type ReactNode } from "react";

import { InteractionInference } from "./InteractionInference";
import { ModelSwitcherProvider, useModelSwitcher } from "../../contexts/modelSwitcher";

const { navigate, params, wallet } = vi.hoisted(() => ({
  navigate: vi.fn(),
  params: { org_id: "org_1", bot_id: "chief-of-staff" },
  wallet: { balance: 0 },
}));

// Only the chrome around the error block matters here, so the heavy content
// renderers are stubbed out. The error block itself is the real thing.
vi.mock("./Markdown", () => ({ default: () => <div /> }));
vi.mock("./WorkLog", () => ({ default: () => <div /> }));
vi.mock("./ActivitySummary", () => ({ default: () => <div /> }));
vi.mock("./PlanProgress", () => ({ SessionPlanProgress: () => <div /> }));
vi.mock("./ToolStepsWidget", () => ({ default: () => <div /> }));
vi.mock("./WorkspaceReviewMessage", () => ({ default: () => <div /> }));
vi.mock("./InteractionDebugCopyButton", () => ({ default: () => <div /> }));
vi.mock("./MessageReceivedTimestamp", () => ({ default: () => <div /> }));
vi.mock("./CopyButtonWithCheck", () => ({ default: () => <div /> }));
vi.mock("./ImageLightbox", () => ({ default: () => <div /> }));
vi.mock("../export/ExportDocument", () => ({ default: () => <div /> }));
vi.mock("../export/ToPDF", () => ({ default: () => <div /> }));
vi.mock("../../hooks/useAccount", () => ({
  default: () => ({ user: { id: "usr_1" }, admin: false, serverConfig: {} }),
}));
vi.mock("../../hooks/useRouter", () => ({
  default: () => ({ navigate, params }),
}));
vi.mock("../../services/interactionsService", () => ({
  useUpdateInteractionFeedback: () => ({ updateFeedback: vi.fn() }),
}));
vi.mock("../../services/useBilling", () => ({
  useGetWallet: () => ({ data: wallet }),
}));

// Stands in for the composer's model picker registering itself.
const RegisterOpener = ({ onOpen }: { onOpen: () => void }) => {
  const switcher = useModelSwitcher();
  const register = switcher?.register;
  useEffect(() => register?.(onOpen), [register, onOpen]);
  return null;
};

const ModelSwitcherHarness = ({ onOpen, children }: { onOpen: () => void; children: ReactNode }) => (
  <ModelSwitcherProvider>
    <RegisterOpener onOpen={onOpen} />
    {children}
  </ModelSwitcherProvider>
);

const baseProps = {
  serverConfig: {
    filestore_prefix: "/api/v1/filestore",
    minimum_inference_balance: 0.01,
  } as any,
  session: { id: "ses_1" } as any,
  interaction: { id: "int_1", prompt_message: "Do the work" } as any,
  isFromAssistant: true,
  onRegenerate: vi.fn(),
};

const vertexServiceDisabledError = String.raw`Anthropic API request failed with status 403: {\"error\":{\"code\":403,\"status\":\"PERMISSION_DENIED\",\"details\":[{\"@type\":\"type.googleapis.com/google.rpc.ErrorInfo\",\"reason\":\"SERVICE_DISABLED\",\"domain\":\"googleapis.com\",\"metadata\":{\"service\":\"aiplatform.googleapis.com\"}}]}}`;

describe("InteractionInference error display", () => {
  beforeEach(() => {
    params.org_id = "org_1";
    params.bot_id = "chief-of-staff";
    wallet.balance = 0;
    navigate.mockClear();
  });

  it("offers Add credits instead of Retry when the balance is empty", () => {
    render(
      <InteractionInference
        {...baseProps}
        error="agent turn aborted: insufficient balance"
      />,
    );

    expect(
      screen.getByText("More credits needed"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "Your organization doesn’t have enough credits. Add credits to continue.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("agent turn aborted: insufficient balance"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Retry/i }),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add credits" }));
    expect(navigate).toHaveBeenCalledWith("org_billing", { org_id: "org_1" });
  });

  it("offers Retry once credits are available", () => {
    wallet.balance = 5;

    render(
      <InteractionInference
        {...baseProps}
        error="agent turn aborted: insufficient balance"
      />,
    );

    expect(screen.getByRole("button", { name: /Retry/i })).toBeInTheDocument();
    expect(
      screen.getByText("Credits are available now. Retry to continue."),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Add credits" }),
    ).not.toBeInTheDocument();
  });

  it("keeps Add credits visible below the minimum inference balance", () => {
    wallet.balance = 0.005;

    render(
      <InteractionInference
        {...baseProps}
        error="agent turn aborted: insufficient balance"
      />,
    );

    expect(
      screen.queryByRole("button", { name: /Retry/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Add credits" }),
    ).toBeInTheDocument();
  });

  it("does not offer credits for unrelated failures", () => {
    render(
      <InteractionInference
        {...baseProps}
        error="configured NativeAgent model did not become available within 15s"
      />,
    );

    expect(
      screen.getByText("We couldn’t complete that request"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("configured NativeAgent model did not become available within 15s"),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Add credits" }),
    ).not.toBeInTheDocument();
  });

  it.each([
    "Agent startup failed: failed to fetch config: status 422: provider \"openai\" is not enabled for coding-agent runtime \"codex_cli\" in this organization",
    "This agent's provider is not available to the organization. Configure the provider for the organization, then select it again in the agent settings.",
    vertexServiceDisabledError,
    `failed to load thread: status 500: ${vertexServiceDisabledError}`,
    "Anthropic provider not configured: set ANTHROPIC_VERTEX_PROJECT_ID for Vertex AI, or ANTHROPIC_API_KEY for direct API access",
  ])("links bot provider errors to its settings", (error) => {
    render(<InteractionInference {...baseProps} error={error} />);

    expect(screen.getByText("Model provider unavailable")).toBeInTheDocument();
    expect(
      screen.getByText(
        "This bot's model provider is unavailable. Choose an available provider and model in bot settings.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Retry/i })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Configure provider" }));
    expect(navigate).toHaveBeenCalledWith("helix_org_bot_detail", {
      org_id: "org_1",
      bot_id: "chief-of-staff",
    });
  });

  it.each([
    "Anthropic API request failed with status 403: SERVICE_DISABLED for storage.googleapis.com",
    "Anthropic API request failed: SERVICE_DISABLED for aiplatform.googleapis.com",
    "HTTP 403: SERVICE_DISABLED for aiplatform.googleapis.com",
  ])("treats other permission failures as credential problems, not a missing bot provider", (error) => {
    render(<InteractionInference {...baseProps} error={error} />);

    expect(
      screen.getByText("The AI provider rejected the request’s credentials"),
    ).toBeInTheDocument();
    expect(screen.queryByText("Model provider unavailable")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Retry/i })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Switch model" }));
    expect(navigate).toHaveBeenCalledWith("helix_org_bot_detail", {
      org_id: "org_1",
      bot_id: "chief-of-staff",
    });
  });

  it.each([
    "provider \"openai\" is not enabled for coding-agent runtime \"codex_cli\" in this organization",
    "Anthropic provider not configured: set ANTHROPIC_API_KEY",
  ])("keeps provider errors unchanged outside bot chat", (error) => {
    params.bot_id = "";

    render(<InteractionInference {...baseProps} error={error} />);

    expect(screen.getByText("We couldn’t complete that request")).toBeInTheDocument();
    expect(screen.getByText(error)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Retry/i })).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Configure provider" }),
    ).not.toBeInTheDocument();
  });

  describe("model provider failures", () => {
    const modelNotFound = `agent turn aborted: Internal error: Internal Server Error: error, status code: 404, status: 404 Not Found, message: model not found for API profile: {
  "service": "session",
  "errorName": "APIError"
}`;

    beforeEach(() => {
      params.bot_id = "";
    });

    it("explains a missing model and opens the chat's model picker", () => {
      const openModelSwitcher = vi.fn();
      render(
        <ModelSwitcherHarness onOpen={openModelSwitcher}>
          <InteractionInference {...baseProps} error={modelNotFound} />
        </ModelSwitcherHarness>,
      );

      expect(
        screen.getByText("The AI provider couldn’t find this model"),
      ).toBeInTheDocument();
      expect(
        screen.getByText(/Provider response \(HTTP 404\): “model not found for API profile”/),
      ).toBeInTheDocument();
      expect(
        screen.getByText("Switch to a different model or provider, then send your message again."),
      ).toBeInTheDocument();
      // Re-sending to a model that doesn't exist cannot succeed.
      expect(screen.queryByRole("button", { name: /Retry/i })).not.toBeInTheDocument();
      // The raw wrapper chain is available, but not the headline.
      expect(screen.queryByText(modelNotFound)).not.toBeInTheDocument();
      fireEvent.click(screen.getByRole("button", { name: "Show details" }));
      expect(screen.getByText(/agent turn aborted: Internal error/)).toBeInTheDocument();

      fireEvent.click(screen.getByRole("button", { name: "Switch model" }));
      expect(openModelSwitcher).toHaveBeenCalledTimes(1);
    });

    it("offers no switch button when the chat has no model picker", () => {
      render(<InteractionInference {...baseProps} error={modelNotFound} />);

      expect(
        screen.getByText("The AI provider couldn’t find this model"),
      ).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Switch model" })).not.toBeInTheDocument();
    });

    it.each([
      ["error, status code: 429, status: 429 Too Many Requests, message: rate limit exceeded", "The AI provider is limiting requests"],
      ["Anthropic API request failed with status 529: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"Overloaded\"}}", "The AI provider is having problems"],
      ["agent turn aborted: context deadline exceeded", "The AI provider took too long to respond"],
    ])("keeps Retry for transient provider failures: %s", (error, title) => {
      const openModelSwitcher = vi.fn();
      render(
        <ModelSwitcherHarness onOpen={openModelSwitcher}>
          <InteractionInference {...baseProps} error={error} />
        </ModelSwitcherHarness>,
      );

      expect(screen.getByText(title)).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /Retry/i })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Switch model" })).toBeInTheDocument();
    });

    it("links credential failures to the providers page", () => {
      render(
        <InteractionInference
          {...baseProps}
          error="error, status code: 401, status: 401 Unauthorized, message: invalid x-api-key"
        />,
      );

      expect(
        screen.getByText("The AI provider rejected the request’s credentials"),
      ).toBeInTheDocument();
      fireEvent.click(screen.getByRole("button", { name: "Manage providers" }));
      expect(navigate).toHaveBeenCalledWith("org_providers", { org_id: "org_1" });
    });
  });

  it("withholds Retry once the session has recovered", () => {
    // Retry re-sends this interaction's prompt. After the session has moved on
    // and completed later work, that prompt is stale — offering the button
    // invites the user to re-run something the session already left behind,
    // and the red alert makes a working session look broken.
    render(
      <InteractionInference
        {...baseProps}
        error="agent turn aborted"
        errorIsHistorical
      />,
    );

    expect(
      screen.queryByRole("button", { name: /Retry/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText("We couldn’t complete that request"),
    ).not.toBeInTheDocument();
    // Not erased, though: the turn did fail and its work was abandoned.
    expect(
      screen.getByText(/This turn was interrupted and did not finish/),
    ).toBeInTheDocument();
    expect(screen.getByText("agent turn aborted")).toBeInTheDocument();
  });
});
