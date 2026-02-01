#!/usr/bin/env node
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import axios from "axios";
import * as fs from "fs";
import * as path from "path";
import * as os from "os";
import { z } from "zod";

// Parse command line arguments and environment variables
let serverUrl = process.env.DRONE_SERVER_URL;
let accessToken = process.env.DRONE_ACCESS_TOKEN;

for (const arg of process.argv.slice(2)) {
  if (arg.startsWith("--access-token=")) {
    accessToken = arg.split("=")[1];
  } else if (arg.startsWith("--server-url=")) {
    serverUrl = arg.split("=")[1];
  }
}

if (!accessToken) {
  console.error(
    "Error: No access token provided. Set DRONE_ACCESS_TOKEN or use --access-token="
  );
  process.exit(1);
}

if (!serverUrl) {
  console.error(
    "Error: No server URL provided. Set DRONE_SERVER_URL or use --server-url="
  );
  process.exit(1);
}

// Ensure temp directory exists
const LOGS_DIR = path.join(os.tmpdir(), "drone-ci-logs");
if (!fs.existsSync(LOGS_DIR)) {
  fs.mkdirSync(LOGS_DIR, { recursive: true });
}

// Drone API client
class DroneClient {
  constructor(
    private baseUrl: string,
    private token: string
  ) {}

  private async request<T>(endpoint: string): Promise<T> {
    const response = await axios.get(`${this.baseUrl}${endpoint}`, {
      headers: { Authorization: `Bearer ${this.token}` },
    });
    return response.data;
  }

  async getBuild(
    repoSlug: string,
    buildNumber: number
  ): Promise<DroneBuildInfo> {
    return this.request(`/api/repos/${repoSlug}/builds/${buildNumber}`);
  }

  async getStepLogs(
    repoSlug: string,
    buildNumber: number,
    stageNumber: number,
    stepNumber: number
  ): Promise<DroneLogLine[]> {
    return this.request(
      `/api/repos/${repoSlug}/builds/${buildNumber}/logs/${stageNumber}/${stepNumber}`
    );
  }

  async listBuilds(
    repoSlug: string,
    branch?: string,
    limit?: number
  ): Promise<DroneBuildListItem[]> {
    let endpoint = `/api/repos/${repoSlug}/builds?per_page=${limit || 10}`;
    if (branch) {
      endpoint += `&branch=${encodeURIComponent(branch)}`;
    }
    return this.request(endpoint);
  }
}

interface DroneBuildListItem {
  number: number;
  status: string;
  event: string;
  message: string;
  source: string;
  target: string;
  author_login: string;
  started: number;
  finished: number;
}

interface DroneLogLine {
  pos: number;
  out: string;
  time: number;
}

interface DroneStep {
  number: number;
  name: string;
  status: string;
  started?: number;
  stopped?: number;
}

interface DroneStage {
  number: number;
  name: string;
  status: string;
  steps: DroneStep[];
}

interface DroneBuildInfo {
  number: number;
  status: string;
  event: string;
  message: string;
  before: string;
  after: string;
  ref: string;
  source_repo: string;
  source: string;
  target: string;
  author_login: string;
  author_name: string;
  sender: string;
  started: number;
  finished: number;
  stages: DroneStage[];
}

// Create MCP server
const server = new McpServer({
  name: "Helix Drone CI",
  version: "1.0.0",
});

const client = new DroneClient(serverUrl, accessToken);

// Tool: List builds
server.tool(
  "drone_list_builds",
  "List recent builds for a repository. Use this to find build numbers, especially to check CI status for a branch after pushing.",
  {
    repo_slug: z.string().describe("Repository slug (e.g., 'helixml/helix')"),
    branch: z.string().optional().describe("Filter by branch name (e.g., 'main', 'fix/my-feature')"),
    limit: z.number().optional().default(10).describe("Number of builds to return (default: 10)"),
  },
  async ({ repo_slug, branch, limit }) => {
    try {
      const builds = await client.listBuilds(repo_slug, branch, limit ?? 10);

      if (builds.length === 0) {
        const branchMsg = branch ? ` for branch '${branch}'` : '';
        return { content: [{ type: "text", text: `No builds found${branchMsg}` }] };
      }

      let summary = `# Recent Builds for ${repo_slug}`;
      if (branch) {
        summary += ` (branch: ${branch})`;
      }
      summary += `\n\n`;

      summary += `| # | Status | Branch | Message | Author |\n`;
      summary += `|---|--------|--------|---------|--------|\n`;

      for (const build of builds) {
        const statusIcon = build.status === 'success' ? '✓' :
                          build.status === 'failure' ? '✗' :
                          build.status === 'running' ? '⟳' :
                          build.status === 'pending' ? '○' : '?';
        const msg = build.message.split('\n')[0].slice(0, 50);
        summary += `| ${build.number} | ${statusIcon} ${build.status} | ${build.source} | ${msg} | ${build.author_login} |\n`;
      }

      summary += `\nUse \`drone_build_info\` with a build number to see step details.`;

      return { content: [{ type: "text", text: summary }] };
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      return { content: [{ type: "text", text: `Error: ${msg}` }], isError: true };
    }
  }
);

// Tool: Get build info
server.tool(
  "drone_build_info",
  "Get information about a Drone CI build including status, stages, and steps. ALWAYS call this first to see which steps failed, then use drone_fetch_logs to get logs for failing steps.",
  {
    repo_slug: z.string().describe("Repository slug (e.g., 'helixml/helix')"),
    build_number: z.number().describe("Build number"),
  },
  async ({ repo_slug, build_number }) => {
    try {
      const build = await client.getBuild(repo_slug, build_number);

      // Format a concise summary
      let summary = `# Build #${build.number} - ${build.status.toUpperCase()}\n\n`;
      summary += `**Commit:** ${build.after?.slice(0, 8)} by ${build.author_name}\n`;
      summary += `**Message:** ${build.message}\n`;
      summary += `**Branch:** ${build.source} → ${build.target}\n\n`;

      summary += `## Steps\n\n`;
      summary += `| # | Name | Status |\n`;
      summary += `|---|------|--------|\n`;

      for (const stage of build.stages || []) {
        for (const step of stage.steps || []) {
          const statusIcon =
            step.status === "success"
              ? "✓"
              : step.status === "failure"
                ? "✗"
                : step.status === "running"
                  ? "⟳"
                  : "○";
          summary += `| ${step.number} | ${step.name} | ${statusIcon} ${step.status} |\n`;
        }
      }

      summary += `\nUse \`drone_fetch_logs\` with stage_number=1 and step_number=N to get logs.`;

      return { content: [{ type: "text", text: summary }] };
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      return { content: [{ type: "text", text: `Error: ${msg}` }], isError: true };
    }
  }
);

// Tool: Fetch logs to file
server.tool(
  "drone_fetch_logs",
  `Fetch build step logs and save to a temp file. Returns file path, line count, and issue summary.

IMPORTANT: CI logs can be very large (10,000+ lines). After fetching:
- If <100 lines: You can read the entire file with drone_read_logs
- If 100-1000 lines: Use drone_tail_logs to see the end, then drone_search_logs for 'FAIL:', 'panic:', 'error'
- If >1000 lines: NEVER read the whole file. Use drone_search_logs to find failures, then drone_read_logs for specific line ranges around matches`,
  {
    repo_slug: z.string().describe("Repository slug (e.g., 'helixml/helix')"),
    build_number: z.number().describe("Build number"),
    stage_number: z.number().describe("Stage number (usually 1)"),
    step_number: z.number().describe("Step number from drone_build_info"),
  },
  async ({ repo_slug, build_number, stage_number, step_number }) => {
    try {
      const logs = await client.getStepLogs(
        repo_slug,
        build_number,
        stage_number,
        step_number
      );

      // Write logs to temp file
      const filename = `${repo_slug.replace("/", "-")}-${build_number}-${stage_number}-${step_number}.log`;
      const filepath = path.join(LOGS_DIR, filename);

      const content = logs.map((l) => l.out).join("");
      fs.writeFileSync(filepath, content);

      // Count lines and check for common issues
      const lines = content.split("\n");
      const lineCount = lines.length;
      const fileSize = Buffer.byteLength(content, "utf8");

      // Quick scan for issues
      const failCount = (content.match(/^---\s+FAIL:/gm) || []).length;
      const panicCount = (content.match(/panic:/gi) || []).length;
      const errorCount = (content.match(/^Error:/gm) || []).length;

      let summary = `# Logs saved to: ${filepath}\n\n`;
      summary += `**Size:** ${(fileSize / 1024).toFixed(1)} KB (${lineCount} lines)\n\n`;

      if (failCount > 0 || panicCount > 0 || errorCount > 0) {
        summary += `## Issues Found\n`;
        if (failCount > 0) summary += `- **FAIL:** ${failCount} test failures\n`;
        if (panicCount > 0) summary += `- **panic:** ${panicCount} panics\n`;
        if (errorCount > 0) summary += `- **Error:** ${errorCount} errors\n`;
        summary += `\nUse \`drone_search_logs\` to find details.\n`;
      } else {
        summary += `No obvious failures detected.\n`;
      }

      // Provide guidance based on log size
      summary += `\n## Recommended Approach\n`;
      if (lineCount < 100) {
        summary += `Log is small (${lineCount} lines). You can read the entire file:\n`;
        summary += `\`drone_read_logs file_path="${filepath}" start_line=1 end_line=${lineCount}\`\n`;
      } else if (lineCount < 1000) {
        summary += `Log is medium-sized (${lineCount} lines). Recommended:\n`;
        summary += `1. Check the end: \`drone_tail_logs file_path="${filepath}" lines=100\`\n`;
        summary += `2. Search for failures: \`drone_search_logs file_path="${filepath}" pattern="FAIL:"\`\n`;
      } else {
        summary += `⚠️ Log is large (${lineCount} lines). Do NOT read the entire file.\n`;
        summary += `1. Search for test failures: \`drone_search_logs file_path="${filepath}" pattern="--- FAIL:"\`\n`;
        summary += `2. Search for panics: \`drone_search_logs file_path="${filepath}" pattern="panic:"\`\n`;
        summary += `3. Check the end: \`drone_tail_logs file_path="${filepath}" lines=50\`\n`;
        summary += `4. Use \`drone_read_logs\` to read specific line ranges around matches.\n`;
      }

      return { content: [{ type: "text", text: summary }] };
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      return { content: [{ type: "text", text: `Error: ${msg}` }], isError: true };
    }
  }
);

// Tool: Search logs
server.tool(
  "drone_search_logs",
  "Search log file for patterns. Returns matching lines with line numbers and context.",
  {
    file_path: z.string().describe("Path to log file (from drone_fetch_logs)"),
    pattern: z
      .string()
      .describe(
        "Search pattern (e.g., 'FAIL:', 'panic:', 'Error'). Case-insensitive regex."
      ),
    context_lines: z
      .number()
      .optional()
      .default(2)
      .describe("Number of context lines before/after match (default: 2)"),
    max_matches: z
      .number()
      .optional()
      .default(20)
      .describe("Maximum matches to return (default: 20)"),
  },
  async ({ file_path, pattern, context_lines, max_matches }) => {
    try {
      if (!fs.existsSync(file_path)) {
        return {
          content: [{ type: "text", text: `File not found: ${file_path}` }],
          isError: true,
        };
      }

      const content = fs.readFileSync(file_path, "utf8");
      const lines = content.split("\n");
      const regex = new RegExp(pattern, "gi");
      const ctx = context_lines ?? 2;
      const maxMatches = max_matches ?? 20;

      const matches: { lineNum: number; context: string[] }[] = [];

      for (let i = 0; i < lines.length && matches.length < maxMatches; i++) {
        if (regex.test(lines[i])) {
          const start = Math.max(0, i - ctx);
          const end = Math.min(lines.length - 1, i + ctx);
          const context: string[] = [];

          for (let j = start; j <= end; j++) {
            const prefix = j === i ? ">>>" : "   ";
            context.push(`${prefix} ${j + 1}: ${lines[j]}`);
          }

          matches.push({ lineNum: i + 1, context });
          // Skip ahead to avoid overlapping matches
          i += ctx;
        }
        // Reset regex state
        regex.lastIndex = 0;
      }

      if (matches.length === 0) {
        return {
          content: [
            {
              type: "text",
              text: `No matches found for pattern: ${pattern}`,
            },
          ],
        };
      }

      let output = `# Found ${matches.length} matches for "${pattern}"\n\n`;

      for (const match of matches) {
        output += `## Line ${match.lineNum}\n\`\`\`\n${match.context.join("\n")}\n\`\`\`\n\n`;
      }

      if (matches.length >= maxMatches) {
        output += `\n*Showing first ${maxMatches} matches. Increase max_matches for more.*\n`;
      }

      return { content: [{ type: "text", text: output }] };
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      return { content: [{ type: "text", text: `Error: ${msg}` }], isError: true };
    }
  }
);

// Tool: Read log lines
server.tool(
  "drone_read_logs",
  "Read specific line range from log file.",
  {
    file_path: z.string().describe("Path to log file (from drone_fetch_logs)"),
    start_line: z.number().describe("Starting line number (1-indexed)"),
    end_line: z.number().describe("Ending line number (inclusive)"),
  },
  async ({ file_path, start_line, end_line }) => {
    try {
      if (!fs.existsSync(file_path)) {
        return {
          content: [{ type: "text", text: `File not found: ${file_path}` }],
          isError: true,
        };
      }

      const content = fs.readFileSync(file_path, "utf8");
      const lines = content.split("\n");

      const start = Math.max(1, start_line);
      const end = Math.min(lines.length, end_line);

      if (start > lines.length) {
        return {
          content: [
            {
              type: "text",
              text: `Start line ${start} exceeds file length (${lines.length} lines)`,
            },
          ],
          isError: true,
        };
      }

      let output = `# Lines ${start}-${end} of ${lines.length}\n\n\`\`\`\n`;

      for (let i = start - 1; i < end; i++) {
        output += `${i + 1}: ${lines[i]}\n`;
      }

      output += `\`\`\`\n`;

      return { content: [{ type: "text", text: output }] };
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      return { content: [{ type: "text", text: `Error: ${msg}` }], isError: true };
    }
  }
);

// Tool: Get log tail
server.tool(
  "drone_tail_logs",
  "Get the last N lines of a log file (useful for seeing final output/errors).",
  {
    file_path: z.string().describe("Path to log file (from drone_fetch_logs)"),
    lines: z
      .number()
      .optional()
      .default(50)
      .describe("Number of lines to show (default: 50)"),
  },
  async ({ file_path, lines }) => {
    try {
      if (!fs.existsSync(file_path)) {
        return {
          content: [{ type: "text", text: `File not found: ${file_path}` }],
          isError: true,
        };
      }

      const content = fs.readFileSync(file_path, "utf8");
      const allLines = content.split("\n");
      const numLines = lines ?? 50;

      const start = Math.max(0, allLines.length - numLines);
      const tailLines = allLines.slice(start);

      let output = `# Last ${tailLines.length} lines (of ${allLines.length} total)\n\n\`\`\`\n`;

      for (let i = 0; i < tailLines.length; i++) {
        output += `${start + i + 1}: ${tailLines[i]}\n`;
      }

      output += `\`\`\`\n`;

      return { content: [{ type: "text", text: output }] };
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      return { content: [{ type: "text", text: `Error: ${msg}` }], isError: true };
    }
  }
);

// Start the server
async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
  console.error("Helix Drone CI MCP server running on stdio");
}

main().catch((err) => {
  console.error("Server error:", err);
  process.exit(1);
});
