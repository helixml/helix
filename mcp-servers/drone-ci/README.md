# Helix Drone CI MCP Server

An MCP server for interacting with Drone CI that handles large logs efficiently by saving them to temp files and providing navigation tools.

## Features

- **Smart log handling**: Logs are saved to temp files instead of returned directly, preventing context overflow
- **Search**: Find patterns like `FAIL:`, `panic:`, `Error` with context
- **Navigation**: Read specific line ranges, tail logs
- **Summary**: Quick scan for common issues when fetching logs

## Tools

| Tool | Description |
|------|-------------|
| `drone_build_info` | Get build status, stages, and steps |
| `drone_fetch_logs` | Fetch logs to temp file, returns path + summary |
| `drone_search_logs` | Search log file for patterns with context |
| `drone_read_logs` | Read specific line range from log file |
| `drone_tail_logs` | Get last N lines of log file |

## Installation

```bash
cd mcp-servers/drone-ci
npm install
npm run build
```

## Configuration

Set environment variables:
```bash
export DRONE_SERVER_URL=https://drone.example.com
export DRONE_ACCESS_TOKEN=your-token-here
```

Or pass as arguments:
```bash
node dist/index.js --server-url=https://drone.example.com --access-token=your-token
```

## Claude Code Integration

Add to your `.claude/settings.json`:

```json
{
  "mcpServers": {
    "drone-ci": {
      "type": "stdio",
      "command": "node",
      "args": ["/path/to/helix/mcp-servers/drone-ci/dist/index.js"],
      "env": {
        "DRONE_SERVER_URL": "https://drone.example.com",
        "DRONE_ACCESS_TOKEN": "your-token-here"
      }
    }
  }
}
```

## Usage Example

1. Get build info to find step numbers:
   ```
   drone_build_info repo_slug="helixml/helix" build_number=17173
   ```

2. Fetch logs for a failing step:
   ```
   drone_fetch_logs repo_slug="helixml/helix" build_number=17173 stage_number=1 step_number=10
   ```

3. Search for failures:
   ```
   drone_search_logs file_path="/tmp/drone-ci-logs/helixml-helix-17173-1-10.log" pattern="FAIL:"
   ```

4. Read specific lines around a failure:
   ```
   drone_read_logs file_path="/tmp/drone-ci-logs/..." start_line=1000 end_line=1050
   ```

5. Check the end of the log:
   ```
   drone_tail_logs file_path="/tmp/drone-ci-logs/..." lines=100
   ```
