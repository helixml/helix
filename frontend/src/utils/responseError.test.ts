import { describe, expect, it } from "vitest";

import { readResponseError } from "./responseError";

describe("readResponseError", () => {
  it("preserves JSON error fields", async () => {
    const response = new Response(
      JSON.stringify({ error: "oauth_required", provider_type: "github" }),
      { status: 422 },
    );

    await expect(readResponseError(response)).resolves.toEqual({
      error: "oauth_required",
      provider_type: "github",
    });
  });

  it("uses a plain-text error body", async () => {
    const response = new Response("failed to stop agent session\n", {
      status: 500,
    });

    await expect(readResponseError(response)).resolves.toEqual({
      message: "failed to stop agent session",
    });
  });

  it("falls back to the status code when the response is empty", async () => {
    const response = new Response("", { status: 500, statusText: "" });

    await expect(readResponseError(response)).resolves.toEqual({
      message: "HTTP 500",
    });
  });
});
