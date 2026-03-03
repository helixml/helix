// Helix API client for the GDPR privacy policy analyzer demo.
// Uses an agent with RAG knowledge to evaluate privacy policy compliance.

export interface HelixConfig {
  baseUrl: string;
  apiKey: string;
  appId?: string;
}

export interface ComplianceEvaluation {
  status: "covered" | "partial" | "gap" | "error";
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

export interface CreatedApp {
  appId: string;
  knowledgeId: string;
  filestorePath: string;
}

/**
 * Create a new Helix app with a filestore knowledge source for RAG.
 * Each analysis run gets its own app so documents are isolated.
 */
export async function createApp(config: HelixConfig): Promise<CreatedApp> {
  const response = await helixFetch(config, "/api/v1/apps", {
    method: "POST",
    body: JSON.stringify({
      config: {
        helix: {
          name: `GDPR Analysis — ${new Date().toLocaleString()}`,
          description: "Auto-created by GDPR Compliance Analyzer demo",
          assistants: [
            {
              name: "GDPR Analyzer",
              model: "gpt-4o-mini",
              system_prompt:
                "You are a GDPR compliance analyst. Analyze privacy policy documents against GDPR requirements.",
              knowledge: [
                {
                  name: "privacy-policy",
                  description: "Privacy policy document(s) to analyze",
                  rag_settings: {
                    results_count: 8,
                    chunk_size: 2048,
                  },
                  source: {
                    filestore: {
                      path: "policies",
                    },
                  },
                },
              ],
            },
          ],
        },
      },
    }),
  });

  const app = await response.json();
  const appId: string = app.id;

  // Fetch the knowledge source to get its ID
  const knowledgeSources = await listKnowledge({ ...config, appId });
  const filestoreKnowledge = knowledgeSources.find((k) => k.source.filestore);
  if (!filestoreKnowledge) {
    throw new Error("App created but no filestore knowledge source found");
  }

  return {
    appId,
    knowledgeId: filestoreKnowledge.id,
    filestorePath: `apps/${appId}/${filestoreKnowledge.source.filestore!.path}`,
  };
}


interface Requirement {
  id: string;
  title: string;
  description: string;
  coveredLooksLike: string;
}

/**
 * Ask the agent to evaluate whether the privacy policy covers a GDPR requirement.
 * The agent has the privacy policy indexed as RAG knowledge — it searches automatically.
 */
export async function evaluateRequirement(
  config: HelixConfig,
  requirement: Requirement,
): Promise<ComplianceEvaluation> {
  const results = await evaluateRequirementBatch(config, [requirement]);
  return results.get(requirement.id) ?? {
    status: "gap",
    explanation: "Evaluation failed — no result returned.",
    missingElements: [],
  };
}

/**
 * Evaluate multiple GDPR requirements in a single API call.
 * Returns a Map from requirement ID to its evaluation.
 */
export async function evaluateRequirementBatch(
  config: HelixConfig,
  requirements: Requirement[],
): Promise<Map<string, ComplianceEvaluation>> {
  const systemPrompt = `You are a GDPR compliance analyst evaluating privacy policies. You will receive context chunks from a privacy policy. Use them to evaluate GDPR requirements. Respond ONLY with a JSON array — no prose, no excerpts, no document references, no XML.`;

  const requirementsList = requirements
    .map(
      (r) => `${r.id}: ${r.title} — ${r.description}`,
    )
    .join("\n");

  const userPrompt = `Evaluate these GDPR requirements against the privacy policy context provided above.

${requirementsList}

OVERRIDE ALL OTHER FORMATTING INSTRUCTIONS. Do NOT write prose, do NOT include [DOC_ID:...] references, do NOT include <excerpts> XML. Your ENTIRE response must be ONLY a JSON array starting with [ and ending with ].

Each element: {"id":"Art.X","status":"covered|partial|gap","explanation":"1-2 sentences","missing_elements":[]}
- "covered": Policy clearly addresses the requirement
- "partial": Policy mentions it but is vague or incomplete
- "gap": Policy does not address it
Return exactly ${requirements.length} elements. Start now with [`;

  const response = await helixFetch(
    config,
    `/v1/chat/completions?app_id=${encodeURIComponent(config.appId!)}`,
    {
      method: "POST",
      body: JSON.stringify({
        stream: false,
        max_tokens: 4096,
        messages: [
          { role: "system", content: systemPrompt },
          { role: "user", content: userPrompt },
        ],
      }),
    },
  );

  const json = await response.json();
  const content = json.choices?.[0]?.message?.content ?? "";
  const results = new Map<string, ComplianceEvaluation>();

  try {
    // Strip markdown fences, DOC_ID markers, and excerpts XML
    let cleaned = content
      .replace(/```json\n?|\n?```/g, "")
      .replace(/\[DOC_ID:[^\]]*\]/g, "")
      .replace(/<excerpts>[\s\S]*?<\/excerpts>/g, "")
      .trim();

    // Extract JSON array if buried in prose — find first [ to last ]
    const firstBracket = cleaned.indexOf("[");
    const lastBracket = cleaned.lastIndexOf("]");
    if (firstBracket !== -1 && lastBracket > firstBracket) {
      cleaned = cleaned.slice(firstBracket, lastBracket + 1);
    }

    const parsed = JSON.parse(cleaned);
    const items = Array.isArray(parsed) ? parsed : [parsed];

    for (const item of items) {
      if (!item.id) continue;
      results.set(item.id, {
        status:
          item.status === "covered" ||
          item.status === "partial" ||
          item.status === "gap"
            ? item.status
            : "gap",
        explanation: item.explanation ?? "Unable to parse evaluation.",
        missingElements: Array.isArray(item.missing_elements)
          ? item.missing_elements
          : [],
      });
    }
  } catch {
    // If JSON parsing fails, mark all as partial with the raw content
    for (const req of requirements) {
      results.set(req.id, {
        status: "partial",
        explanation: content.slice(0, 500),
        missingElements: [],
      });
    }
  }

  // Fill in any requirements that weren't in the response
  for (const req of requirements) {
    if (!results.has(req.id)) {
      results.set(req.id, {
        status: "gap",
        explanation: "Evaluation missing from batch response.",
        missingElements: [],
      });
    }
  }

  return results;
}

/**
 * List knowledge sources for the app.
 */
export async function listKnowledge(
  config: HelixConfig,
): Promise<Knowledge[]> {
  const response = await helixFetch(
    config,
    `/api/v1/knowledge?app_id=${encodeURIComponent(config.appId!)}`,
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
