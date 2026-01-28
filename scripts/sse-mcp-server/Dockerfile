FROM node:20-slim

WORKDIR /app

# Install the official MCP server-everything package
RUN npm install @modelcontextprotocol/server-everything

# The server runs on port 3001 by default for SSE
EXPOSE 3001

# Run the server in SSE mode
CMD ["npx", "@modelcontextprotocol/server-everything", "sse"]