import unittest
import os
import socket
import tempfile
import threading
import json
import time
import sys
from unittest.mock import patch, MagicMock

# Add the parent directory to the path so we can import the app module
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), '..')))

from haystack.utils import Secret
from haystack.dataclasses import Document
from app.unix_socket_embedders import (
    UnixSocketOpenAITextEmbedder,
    UnixSocketOpenAIDocumentEmbedder
)

class MockSocketServer(threading.Thread):
    """A mock server that listens on a Unix socket and responds to requests."""
    
    def __init__(self, socket_path):
        super().__init__()
        self.socket_path = socket_path
        self.running = True
        self.requests_received = []
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
        
        while self.running:
            try:
                conn, _ = server.accept()
                data = conn.recv(4096)
                data = data.decode('utf-8')
                self.requests_received.append(data)
                
                # Parse the HTTP request to get the path
                request_lines = data.split('\r\n')
                method, path, _ = request_lines[0].split(' ')
                
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
                else:
                    # Return 404 for unknown paths
                    conn.sendall(b'HTTP/1.1 404 Not Found\r\n\r\n')
                
                conn.close()
            except socket.timeout:
                continue
            except Exception as e:
                print(f"Error in mock server: {e}")
                break
                
        server.close()
        if os.path.exists(self.socket_path):
            os.unlink(self.socket_path)
            
    def stop(self):
        self.running = False
        self.join(timeout=1.0)


class TestUnixSocketEmbedders(unittest.TestCase):
    
    def setUp(self):
        # Create a temporary directory for the socket
        self.temp_dir = tempfile.TemporaryDirectory()
        self.socket_path = os.path.join(self.temp_dir.name, "test.sock")
        
        # Start the mock server
        self.server = MockSocketServer(self.socket_path)
        self.server.start()
        
        # Wait for server to start
        time.sleep(0.1)
        
    def tearDown(self):
        # Stop the server
        self.server.stop()
        
        # Clean up the temporary directory
        self.temp_dir.cleanup()

    
    def test_text_embedder_real_integration(self):
        """Test that the UnixSocketOpenAITextEmbedder can embed text using the mock server."""
        # Create the embedder
        embedder = UnixSocketOpenAITextEmbedder(
            socket_path=self.socket_path,
            model="text-embedding-ada-002"
        )

        # Call run (Haystack 2.x API)
        result = embedder.run(text="test text")
        
        # Check that the result contains embeddings
        self.assertIn("embedding", result)
        self.assertEqual(len(result["embedding"]), 5)


    def test_document_embedder_real_integration(self):
        """Test that the UnixSocketOpenAIDocumentEmbedder can embed documents using the mock server."""
        # Create the embedder
        embedder = UnixSocketOpenAIDocumentEmbedder(
            socket_path=self.socket_path,
            model="text-embedding-ada-002"
        )
        
        # Create a test document using Haystack 2.x Document class
        doc = Document(content="test document")
        
        # Call run (Haystack 2.x API)
        result = embedder.run(documents=[doc])
        
        # Check that the result contains documents with embeddings
        self.assertIn("documents", result)
        self.assertEqual(len(result["documents"]), 1)
        self.assertTrue(hasattr(result["documents"][0], "embedding"))
        self.assertEqual(len(result["documents"][0].embedding), 5)


if __name__ == "__main__":
    unittest.main() 
