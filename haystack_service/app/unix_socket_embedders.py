import logging
import socket
import http.client
from typing import Optional, Dict, Any, Union, List, Callable
import json
from io import BytesIO
import httpx

from haystack.components.embedders import OpenAIDocumentEmbedder, OpenAITextEmbedder
from haystack.utils import Secret
from haystack.dataclasses import Document

# Import json module to avoid name conflict with the json parameter

logger = logging.getLogger(__name__)


def _patch_openai_client(client, socket_path):
    transport = httpx.HTTPTransport(uds=socket_path)
    client = httpx.Client(transport=transport)
    client._client = client


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
        _patch_openai_client(self.client, socket_path)
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
        _patch_openai_client(self.client, socket_path)


# Should be not needed.
#   def run(self, documents: List[Document]) -> Dict[str, List[Document]]:
#       """
#       Embed the given documents using the OpenAI API via a UNIX socket.
#       
#       Args:
#           documents: The documents to embed.
#           
#       Returns:
#           A dictionary with the embedded documents.
#       """
#       # Call the parent class's run method
#       return super().run(documents=documents)

# Should be not needed.
#   def run(self, text: str) -> Dict[str, List[List[float]]]:
#       """
#       Embed the given text using the OpenAI API via a UNIX socket.
#       
#       Args:
#           text: The text to embed.
#           
#       Returns:
#           A dictionary with the embeddings.
#       """
#       # Call the parent class's run method
#       return super().run(text=text)

# Again, shouldn't be needed.
#   class UnixSocketResponse:
#       """A response object that mimics the httpx.Response interface used by OpenAI client"""
#       
#       def __init__(self, status_code, headers, content):
#           self.status_code = status_code
#           self.headers = headers
#           self._content = content
#           self._text = None
#           
#       def json(self) -> Dict[str, Any]:
#           """Parse the response content as JSON"""
#           return json.loads(self._content.decode('utf-8'))
#       
#       @property
#       def content(self) -> bytes:
#           """Return the raw content"""
#           return self._content
#       
#       @property
#       def text(self) -> str:
#           """Return the content as text"""
#           if self._text is None:
#               self._text = self._content.decode("utf-8")
#           return self._text
#           
#       def raise_for_status(self):
#           """Raise an exception if the status code indicates an error"""
#           if self.status_code >= 400:
#               raise Exception(f"HTTP Error: {self.status_code}")


#   class UnixSocketAdapter:
#       """HTTP client adapter that uses a UNIX socket"""
#       
#       def __init__(self, socket_path, timeout=60):
#           self.socket_path = socket_path
#           self.timeout = timeout
#       
#       def request(self, method, url, headers=None, content=None, stream=False, auth=None, json=None, **kwargs) -> UnixSocketResponse:
#           """Make an HTTP request using a UNIX socket"""
#           # Create a socket
#           sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
#           sock.settimeout(self.timeout)
#           sock.connect(self.socket_path)
#           
#           # Create a connection using the socket
#           conn = http.client.HTTPConnection("localhost")
#           conn.sock = sock
#           
#           # Extract just the path from the URL
#           path = url
#           if "://" in url:
#               parts = url.split("://", 1)[1].split("/", 1)
#               path = "/" + parts[1] if len(parts) > 1 else "/"
#           
#           # Prepare headers
#           request_headers = {}
#           if headers:
#               request_headers.update(headers)
#           
#           # Prepare body
#           body = None
#           if json is not None:
#               body = json_module.dumps(json).encode('utf-8')
#               request_headers['Content-Type'] = 'application/json'
#           elif content:
#               body = content
#           
#           # Make request
#           conn.request(method, path, body=body, headers=request_headers)
#           http_response = conn.getresponse()
#           
#           # Read the response content
#           content = http_response.read()
#           
#           # Convert headers to a dictionary
#           headers_dict = {k.lower(): v for k, v in http_response.getheaders()}
#           
#           # Create and return a response object that mimics the httpx.Response interface
#           response = UnixSocketResponse(
#               status_code=http_response.status,
#               headers=headers_dict,
#               content=content
#           )
#           
#           # For debugging
#           logger.debug(f"Response status: {response.status_code}")
#           logger.debug(f"Response headers: {response.headers}")
#           logger.debug(f"Response content: {response.content[:100]}...")
#           
#           # If the content type is JSON, try to parse it
#           if 'content-type' in headers_dict and 'application/json' in headers_dict['content-type']:
#               try:
#                   logger.debug(f"Parsed JSON: {response.json()}")
#               except Exception as e:
#                   logger.error(f"Failed to parse JSON: {e}")
#           
#           return response
