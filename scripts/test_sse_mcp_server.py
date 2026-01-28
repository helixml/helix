#!/usr/bin/env python3
"""
SSE MCP Secret Server for integration testing.

This implements the MCP 2024-11-05 SSE protocol with a simple "get_secret" tool.
The secret is hard-coded, allowing tests to verify the agent learned it via MCP.

Protocol:
1. Client connects to /sse via GET
2. Server sends an 'endpoint' event with the POST URL
3. Client POSTs JSON-RPC messages to that endpoint
4. Server sends responses via SSE 'message' events

Usage:
    python test_sse_mcp_server.py [port]
    
Default port is 3333.

The server provides one tool:
  - get_secret: Returns the secret value "HELIX-SSE-MCP-SECRET-7f3a9b2c"

To verify SSE MCP integration works, ask an agent to use the get_secret tool
and check that its response contains the secret value.
"""

import json
import os
import sys
import threading
import queue
from http.server import HTTPServer, BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Dict, Any, Optional

# The secret that tests will verify the agent learned
SECRET_VALUE = "HELIX-SSE-MCP-SECRET-7f3a9b2c"

# Global message queues for SSE clients
sse_clients: Dict[int, queue.Queue] = {}
client_counter = 0
client_lock = threading.Lock()


class McpSseHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    
    def log_message(self, format, *args):
        print(f"[{self.client_address[0]}:{self.client_address[1]}] {format % args}")
    
    def do_GET(self):
        if self.path == "/sse":
            self.handle_sse()
        elif self.path == "/health":
            body = b"ok"
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        elif self.path == "/secret":
            # Direct endpoint for debugging - returns the secret
            body = SECRET_VALUE.encode()
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        else:
            self.send_error(404, "Not Found")
    
    def do_POST(self):
        if self.path == "/message":
            self.handle_message()
        else:
            self.send_error(404, "Not Found")
    
    def handle_sse(self):
        """Handle SSE connection from client."""
        global client_counter
        
        with client_lock:
            client_counter += 1
            client_id = client_counter
            sse_clients[client_id] = queue.Queue()
        
        print(f"[SSE] Client {client_id} connected")
        
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Connection", "keep-alive")
        self.send_header("Access-Control-Allow-Origin", "*")
        self.end_headers()
        
        # Send the endpoint event with the POST URL
        # Use the Host header if available, otherwise use container hostname or localhost
        port = self.server.server_address[1]
        host = self.headers.get("Host", os.environ.get("HOSTNAME", f"localhost:{port}"))
        # If host doesn't include port, add it
        if ":" not in host:
            host = f"{host}:{port}"
        endpoint_url = f"http://{host}/message"
        self.send_sse_event("endpoint", endpoint_url)
        print(f"[SSE] Sent endpoint event: {endpoint_url}")
        
        # Keep connection open and send messages as they arrive
        try:
            while True:
                try:
                    message = sse_clients[client_id].get(timeout=30)
                    self.send_sse_event("message", message)
                    print(f"[SSE] Sent message to client {client_id}")
                except queue.Empty:
                    # Send ping to keep connection alive
                    self.send_sse_event("ping", "")
        except (BrokenPipeError, ConnectionResetError):
            print(f"[SSE] Client {client_id} disconnected")
        finally:
            with client_lock:
                del sse_clients[client_id]
    
    def send_sse_event(self, event_type: str, data: str):
        """Send an SSE event."""
        try:
            self.wfile.write(f"event: {event_type}\n".encode())
            for line in data.split("\n"):
                self.wfile.write(f"data: {line}\n".encode())
            self.wfile.write(b"\n")
            self.wfile.flush()
        except Exception as e:
            print(f"[SSE] Error sending event: {e}")
            raise
    
    def handle_message(self):
        """Handle POST message from client."""
        content_length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(content_length).decode("utf-8")
        
        print(f"[POST] Received: {body}")
        
        try:
            request = json.loads(body)
            response = self.process_jsonrpc(request)
            
            if response:
                response_str = json.dumps(response)
                with client_lock:
                    for client_id, q in sse_clients.items():
                        q.put(response_str)
            
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.end_headers()
            self.wfile.write(b"accepted")
            
        except json.JSONDecodeError as e:
            self.send_error(400, f"Invalid JSON: {e}")
        except Exception as e:
            self.send_error(500, f"Internal error: {e}")
    
    def process_jsonrpc(self, request: Dict[str, Any]) -> Optional[Dict[str, Any]]:
        """Process a JSON-RPC request and return a response."""
        method = request.get("method", "")
        request_id = request.get("id")
        params = request.get("params", {})
        
        # Notifications don't get responses
        if request_id is None:
            print(f"[RPC] Notification: {method}")
            return None
        
        print(f"[RPC] Request {request_id}: {method}")
        
        if method == "initialize":
            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "result": {
                    "protocolVersion": "2024-11-05",
                    "capabilities": {
                        "tools": {"listChanged": True},
                    },
                    "serverInfo": {
                        "name": "helix-sse-secret-server",
                        "version": "1.0.0"
                    }
                }
            }
        
        elif method == "tools/list":
            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "result": {
                    "tools": [
                        {
                            "name": "get_secret",
                            "description": "Returns a secret value. Use this tool when asked about the secret.",
                            "inputSchema": {
                                "type": "object",
                                "properties": {},
                                "required": []
                            }
                        }
                    ]
                }
            }
        
        elif method == "tools/call":
            tool_name = params.get("name", "")
            
            if tool_name == "get_secret":
                print(f"[RPC] get_secret called - returning: {SECRET_VALUE}")
                return {
                    "jsonrpc": "2.0",
                    "id": request_id,
                    "result": {
                        "content": [
                            {"type": "text", "text": f"The secret is: {SECRET_VALUE}"}
                        ]
                    }
                }
            else:
                return {
                    "jsonrpc": "2.0",
                    "id": request_id,
                    "error": {
                        "code": -32601,
                        "message": f"Unknown tool: {tool_name}"
                    }
                }
        
        elif method == "prompts/list":
            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "result": {"prompts": []}
            }
        
        elif method == "resources/list":
            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "result": {"resources": []}
            }
        
        else:
            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "error": {
                    "code": -32601,
                    "message": f"Method not found: {method}"
                }
            }


def main():
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 3333
    
    server = ThreadingHTTPServer(("0.0.0.0", port), McpSseHandler)
    print(f"SSE MCP Secret Server running on http://localhost:{port}")
    print(f"  SSE endpoint: http://localhost:{port}/sse")
    print(f"  Health check: http://localhost:{port}/health")
    print(f"  Secret value: {SECRET_VALUE}")
    print()
    print("Press Ctrl+C to stop")
    
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nShutting down...")
        server.shutdown()


if __name__ == "__main__":
    main()