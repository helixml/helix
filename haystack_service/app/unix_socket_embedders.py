import logging
import socket
import http.client
from typing import Optional

from haystack.components.embedders import OpenAIDocumentEmbedder, OpenAITextEmbedder
from haystack.utils import Secret

logger = logging.getLogger(__name__)

class UnixSocketAdapter:
    """HTTP client adapter that uses a UNIX socket"""
    
    def __init__(self, socket_path, timeout=60):
        self.socket_path = socket_path
        self.timeout = timeout
    
    def request(self, method, url, *args, **kwargs):
        # Create a socket
        sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        sock.settimeout(self.timeout)
        sock.connect(self.socket_path)
        
        # Create a connection using the socket
        conn = http.client.HTTPConnection("localhost")
        conn.sock = sock
        
        # Extract just the path from the URL
        path = url
        if "://" in url:
            parts = url.split("://", 1)[1].split("/", 1)
            path = "/" + parts[1] if len(parts) > 1 else "/"
        
        # Make request
        conn.request(method, path, *args, **kwargs)
        return conn.getresponse()

class UnixSocketOpenAITextEmbedder(OpenAITextEmbedder):
    """
    A text embedder that uses the OpenAI API via a UNIX socket.
    """
    
    def __init__(self, socket_path: str, *args, **kwargs):
        """
        Initialize the UnixSocketOpenAITextEmbedder.
        
        Args:
            socket_path: Path to the UNIX socket.
            *args, **kwargs: Arguments passed to OpenAITextEmbedder.
        """
        # Initialize the parent class
        super().__init__(*args, **kwargs)
        
        # Store socket path for reference
        self.socket_path = socket_path
        
        # Replace the HTTP client in the OpenAI client
        self.client.http_client = UnixSocketAdapter(socket_path)
        logger.info(f"Initialized UnixSocketOpenAITextEmbedder with socket: {socket_path}")


class UnixSocketOpenAIDocumentEmbedder(OpenAIDocumentEmbedder):
    """
    A document embedder that uses the OpenAI API via a UNIX socket.
    """
    
    def __init__(self, socket_path: str, *args, **kwargs):
        """
        Initialize the UnixSocketOpenAIDocumentEmbedder.
        
        Args:
            socket_path: Path to the UNIX socket.
            *args, **kwargs: Arguments passed to OpenAIDocumentEmbedder.
        """
        # Initialize the parent class
        super().__init__(*args, **kwargs)
        
        # Store socket path for reference
        self.socket_path = socket_path
        
        # Replace the HTTP client in the OpenAI client
        self.client.http_client = UnixSocketAdapter(socket_path)
        logger.info(f"Initialized UnixSocketOpenAIDocumentEmbedder with socket: {socket_path}") 