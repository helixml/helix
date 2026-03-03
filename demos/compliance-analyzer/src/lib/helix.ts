// Helix API client for the GDPR privacy policy analyzer demo.
// Uses an agent with RAG knowledge to evaluate privacy policy compliance.

export interface HelixConfig {
  baseUrl: string;
  apiKey: string;
  appId: string;
}

export interface ComplianceEvaluation {
  status: "covered" | "partial" | "gap";
  explanation: string;
  missingElements: string[];
}

export type KnowledgeState =
  | "preparing"
  | "pending"
  | "indexing"
  | "ready"
  | "error";

export interface KnowledgeProgress {
  step: string;
  progress: number;
  message: string;
}

export interface Knowledge {
  id: string;
  name: string;
  state: KnowledgeState;
  message: string;
  progress: KnowledgeProgress;
  source: {
    filestore?: { path: string };
  };
}

async function helixFetch(
  config: HelixConfig,
  path: string,
  options?: RequestInit,
): Promise<Response> {
  const url = `${config.baseUrl}${path}`;
  const response = await fetch(url, {
    ...options,
    credentials: "omit",
    signal: options?.signal ?? AbortSignal.timeout(120_000),
    headers: {
      Authorization: `Bearer ${config.apiKey}`,
      "Content-Type": "application/json",
      ...options?.headers,
    },
  });

  if (!response.ok) {
    const body = await response.text().catch(() => "(unable to read body)");
    throw new Error(
      `Helix API error: ${response.status} ${response.statusText} — ${body}`,
    );
  }

  return response;
}

/**
 * Test connectivity to Helix by hitting the status endpoint.
 */
export async function testConnection(config: HelixConfig): Promise<void> {
  await helixFetch(config, "/api/v1/status");
}

/**
 * Read an SSE (Server-Sent Events) stream from a chat completions response
 * and return the accumulated content string.
 */
async function readSSEStream(response: Response): Promise<string> {
  const reader = response.body?.getReader();
  if (!reader) throw new Error("No response body");

  const decoder = new TextDecoder();
  let content = "";
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });

    // Process complete SSE lines
    const lines = buffer.split("\n");
    // Keep the last potentially incomplete line in the buffer
    buffer = lines.pop() ?? "";

    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed || !trimmed.startsWith("data: ")) continue;

      const data = trimmed.slice(6);
      if (data === "[DONE]") continue;

      try {
        const parsed = JSON.parse(data);
        const delta = parsed.choices?.[0]?.delta?.content;
        if (delta) content += delta;
      } catch {
        // Skip malformed chunks
      }
    }
  }

  return content;
}

/**
 * Ask the agent to evaluate whether the privacy policy covers a GDPR requirement.
 * The agent has the privacy policy indexed as RAG knowledge — it searches automatically.
 */
export async function evaluateRequirement(
  config: HelixConfig,
  requirement: { id: string; title: string; description: string; coveredLooksLike: string },
): Promise<ComplianceEvaluation> {
  const systemPrompt = `You are a GDPR compliance analyst. You have access to a company's privacy policy. For each GDPR requirement, search the privacy policy and evaluate whether it is adequately addressed.

You must respond with ONLY a JSON object (no markdown, no code fences) in this exact format:
{
  "status": "covered" | "partial" | "gap",
  "explanation": "2-3 sentence analysis referencing specific policy text you found",
  "missing_elements": ["specific thing missing 1", "specific thing missing 2"]
}

Rules:
- "covered": The policy clearly and specifically addresses the requirement.
- "partial": The policy touches on the topic but is vague, incomplete, or missing key elements.
- "gap": The policy does not address this requirement at all.
- Be specific — quote or reference actual policy text when available.
- For missing_elements, list concrete additions needed. Use empty array [] if fully covered.`;

  const userPrompt = `Evaluate GDPR ${requirement.id} — ${requirement.title}:

${requirement.description}

What compliance looks like: ${requirement.coveredLooksLike}

Search the privacy policy and evaluate whether this requirement is met.`;

  const response = await helixFetch(
    config,
    `/v1/chat/completions?app_id=${encodeURIComponent(config.appId)}`,
    {
      method: "POST",
      body: JSON.stringify({
        stream: true,
        messages: [
          { role: "system", content: systemPrompt },
          { role: "user", content: userPrompt },
        ],
        temperature: 0.1,
      }),
    },
  );

  // Read SSE stream and accumulate content
  const content = await readSSEStream(response);

  try {
    const cleaned = content.replace(/```json\n?|\n?```/g, "").trim();
    const parsed = JSON.parse(cleaned);
    return {
      status:
        parsed.status === "covered" ||
        parsed.status === "partial" ||
        parsed.status === "gap"
          ? parsed.status
          : "gap",
      explanation: parsed.explanation ?? "Unable to parse evaluation.",
      missingElements: Array.isArray(parsed.missing_elements)
        ? parsed.missing_elements
        : [],
    };
  } catch {
    // If JSON parsing fails, return raw content as explanation
    return {
      status: "partial",
      explanation: content.slice(0, 500),
      missingElements: [],
    };
  }
}

/**
 * List knowledge sources for the app.
 */
export async function listKnowledge(
  config: HelixConfig,
): Promise<Knowledge[]> {
  const response = await helixFetch(
    config,
    `/api/v1/knowledge?app_id=${encodeURIComponent(config.appId)}`,
  );
  return response.json();
}

/**
 * Get a single knowledge source (used for polling indexing status).
 */
export async function getKnowledge(
  config: HelixConfig,
  knowledgeId: string,
): Promise<Knowledge> {
  const response = await helixFetch(
    config,
    `/api/v1/knowledge/${encodeURIComponent(knowledgeId)}`,
  );
  return response.json();
}

/**
 * Upload files to a knowledge source's filestore path.
 */
export async function uploadFiles(
  config: HelixConfig,
  filestorePath: string,
  files: File[],
): Promise<void> {
  const formData = new FormData();
  for (const file of files) {
    formData.append("files", file);
  }

  const url = `${config.baseUrl}/api/v1/filestore/upload?path=${encodeURIComponent(filestorePath)}`;
  const response = await fetch(url, {
    method: "POST",
    credentials: "omit",
    headers: {
      Authorization: `Bearer ${config.apiKey}`,
    },
    body: formData,
  });

  if (!response.ok) {
    const body = await response.text().catch(() => "(unable to read body)");
    throw new Error(
      `Upload failed: ${response.status} ${response.statusText} — ${body}`,
    );
  }
}

/**
 * Complete the preparation phase — moves knowledge from "preparing" to "pending".
 * Use this after the first file upload when knowledge hasn't been indexed yet.
 */
export async function completeKnowledge(
  config: HelixConfig,
  knowledgeId: string,
): Promise<void> {
  await helixFetch(
    config,
    `/api/v1/knowledge/${encodeURIComponent(knowledgeId)}/complete`,
    { method: "POST" },
  );
}

/**
 * Trigger re-indexing of a knowledge source after uploading new files.
 * Use this when knowledge is already in "ready" state and you want to re-index.
 */
export async function refreshKnowledge(
  config: HelixConfig,
  knowledgeId: string,
): Promise<void> {
  await helixFetch(
    config,
    `/api/v1/knowledge/${encodeURIComponent(knowledgeId)}/refresh`,
    { method: "POST" },
  );
}
