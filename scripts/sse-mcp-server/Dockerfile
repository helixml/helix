FROM node:20-slim@sha256:f93745c153377ee2fbbdd6e24efcd03cd2e86d6ab1d8aa9916a3790c40313a55

WORKDIR /app

# Install the official MCP server-everything package
RUN npm install @modelcontextprotocol/server-everything

# The server runs on port 3001 by default for SSE
EXPOSE 3001

# Run the server in SSE mode
CMD ["npx", "@modelcontextprotocol/server-everything", "sse"]