#!/usr/bin/env python3
"""
Script to debug the Unix socket embedders.

This script creates a Unix socket server that logs all requests it receives,
and then uses the Unix socket embedders to make requests to the server.

It helps debug issues with the Unix socket embedders making HTTPS connections
to api.openai.com instead of using the Unix socket.
"""

import os
import sys
import socket
import threading
import json
import time
import logging
from typing import List, Dict, Any

# Add the parent directory to the path so we can import the app module
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), '..')))

from haystack.utils import Secret
from haystack.dataclasses import Document
from app.unix_socket_embedders import (
    UnixSocketAdapter,
    UnixSocketOpenAITextEmbedder,
    UnixSocketOpenAIDocumentEmbedder
)

# Set up logging
logging.basicConfig(
    level=logging.DEBUG,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

class DebugSocketServer(threading.Thread):
    """A debug server that listens on a Unix socket and logs all requests."""
    
    def __init__(self, socket_path):
        super().__init__()
        self.socket_path = socket_path
        self.running = True
        self.requests_received: List[Dict[str, Any]] = []
        self.daemon = True  # Thread will exit when main thread exits
        
    def run(self):
        # Remove socket file if it exists
        if os.path.exists(self.socket_path):
            os.unlink(self.socket_path)
            
        # Create socket
        server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        server.bind(self.socket_path)
        server.listen(1)
        server.settimeout(0.1)  # Short timeout to allow for clean shutdown
        
        logger.info(f"Debug server listening on {self.socket_path}")
        
        while self.running:
            try:
                conn, _ = server.accept()
                data = conn.recv(4096).decode('utf-8')
                
                # Log the request
                logger.info(f"Received request: {data}")
                
                # Parse the HTTP request
                request_lines = data.split('\r\n')
                method, path, _ = request_lines[0].split(' ')
                
                # Extract headers
                headers = {}
                for line in request_lines[1:]:
                    if not line or line.isspace():
                        break
                    if ':' in line:
                        key, value = line.split(':', 1)
                        headers[key.strip()] = value.strip()
                
                # Extract body if present
                body = None
                if 'Content-Length' in headers:
                    content_length = int(headers['Content-Length'])
                    body_start = data.find('\r\n\r\n') + 4
                    body = data[body_start:body_start + content_length]
                    
                    # Try to parse JSON body
                    try:
                        body = json.loads(body)
                    except json.JSONDecodeError:
                        pass
                
                # Store the request
                request_info = {
                    'method': method,
                    'path': path,
                    'headers': headers,
                    'body': body,
                    'raw': data
                }
                self.requests_received.append(request_info)
                
                # Prepare mock response based on the path
                if '/embeddings' in path:
                    response = {
                        "object": "list",
                        "data": [
                            {
                                "object": "embedding",
                                "embedding": [0.1, 0.2, 0.3, 0.4, 0.5],
                                "index": 0
                            }
                        ],
                        "model": "text-embedding-ada-002",
                        "usage": {
                            "prompt_tokens": 5,
                            "total_tokens": 5
                        }
                    }
                    
                    # Create HTTP response
                    response_data = json.dumps(response).encode('utf-8')
                    http_response = (
                        b'HTTP/1.1 200 OK\r\n'
                        b'Content-Type: application/json\r\n'
                        b'Content-Length: ' + str(len(response_data)).encode('utf-8') + b'\r\n'
                        b'\r\n'
                    ) + response_data
                    
                    conn.sendall(http_response)
                    logger.info(f"Sent response: {response}")
                else:
                    # Return 404 for unknown paths
                    conn.sendall(b'HTTP/1.1 404 Not Found\r\n\r\n')
                    logger.info("Sent 404 response")
                
                conn.close()
            except socket.timeout:
                continue
            except Exception as e:
                logger.error(f"Error in debug server: {e}", exc_info=True)
                break
                
        server.close()
        if os.path.exists(self.socket_path):
            os.unlink(self.socket_path)
            
    def stop(self):
        self.running = False
        self.join(timeout=1.0)


def debug_text_embedder(socket_path):
    """Debug the UnixSocketOpenAITextEmbedder."""
    # Set up environment variable for testing
    os.environ["DEBUG_OPENAI_API_KEY"] = "dummy_key"
    
    logger.info("Creating UnixSocketOpenAITextEmbedder...")
    embedder = UnixSocketOpenAITextEmbedder(
        socket_path=socket_path,
        api_key=Secret.from_env_var("DEBUG_OPENAI_API_KEY"),
        model="text-embedding-ada-002"
    )
    
    logger.info("Embedder created. HTTP client type: %s", type(embedder.client.http_client).__name__)
    
    # Check if the HTTP client is a UnixSocketAdapter
    if not isinstance(embedder.client.http_client, UnixSocketAdapter):
        logger.error("HTTP client is not a UnixSocketAdapter! Type: %s", type(embedder.client.http_client).__name__)
    
    # Check the base URL
    logger.info("OpenAI client base URL: %s", embedder.client.base_url)
    
    # Try to embed some text
    logger.info("Embedding text...")
    try:
        # In Haystack 2.x, components use run() method
        result = embedder.run(text="test text")
        logger.info("Embedding result: %s", result)
    except Exception as e:
        logger.error("Error embedding text: %s", e, exc_info=True)


def debug_document_embedder(socket_path):
    """Debug the UnixSocketOpenAIDocumentEmbedder."""
    # Set up environment variable for testing
    os.environ["DEBUG_OPENAI_API_KEY"] = "dummy_key"
    
    logger.info("Creating UnixSocketOpenAIDocumentEmbedder...")
    embedder = UnixSocketOpenAIDocumentEmbedder(
        socket_path=socket_path,
        api_key=Secret.from_env_var("DEBUG_OPENAI_API_KEY"),
        model="text-embedding-ada-002"
    )
    
    logger.info("Embedder created. HTTP client type: %s", type(embedder.client.http_client).__name__)
    
    # Check if the HTTP client is a UnixSocketAdapter
    if not isinstance(embedder.client.http_client, UnixSocketAdapter):
        logger.error("HTTP client is not a UnixSocketAdapter! Type: %s", type(embedder.client.http_client).__name__)
    
    # Check the base URL
    logger.info("OpenAI client base URL: %s", embedder.client.base_url)
    
    # Try to embed a document
    logger.info("Embedding document...")
    try:
        # Create a document using Haystack 2.x Document class
        doc = Document(content="test document")
        # In Haystack 2.x, components use run() method
        result = embedder.run(documents=[doc])
        logger.info("Embedding result: %s", result)
    except Exception as e:
        logger.error("Error embedding document: %s", e, exc_info=True)


def main():
    """Main function."""
    # Create a socket path
    socket_path = "/tmp/debug_unix_socket.sock"
    
    # Start the debug server
    server = DebugSocketServer(socket_path)
    server.start()
    
    try:
        # Wait for server to start
        time.sleep(0.5)
        
        # Debug the text embedder
        debug_text_embedder(socket_path)
        
        # Debug the document embedder
        debug_document_embedder(socket_path)
        
        # Print summary of requests received
        logger.info("Summary of requests received:")
        for i, req in enumerate(server.requests_received):
            logger.info(f"Request {i+1}:")
            logger.info(f"  Method: {req['method']}")
            logger.info(f"  Path: {req['path']}")
            logger.info(f"  Headers: {json.dumps(req['headers'], indent=2)}")
            if req['body']:
                logger.info(f"  Body: {json.dumps(req['body'], indent=2)}")
        
    finally:
        # Stop the server
        server.stop()
        
        # Clean up environment variable
        if "DEBUG_OPENAI_API_KEY" in os.environ:
            del os.environ["DEBUG_OPENAI_API_KEY"]


if __name__ == "__main__":
    main() 