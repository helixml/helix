FROM node:20-slim@sha256:7129e1780341f8dff603243d2b0cb9179c1716291ff6a86706946b629d3c544a

WORKDIR /app

# Install the official MCP server-everything package
RUN npm install @modelcontextprotocol/server-everything

# The server runs on port 3001 by default for SSE
EXPOSE 3001

# Run the server in SSE mode
CMD ["npx", "@modelcontextprotocol/server-everything", "sse"]