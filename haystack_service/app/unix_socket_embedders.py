# SPDX-FileCopyrightText: 2022-present deepset GmbH <info@deepset.ai>
#
# SPDX-License-Identifier: Apache-2.0

import os
from typing import Any, Dict, List, Optional

from openai import OpenAI

from haystack import component, default_from_dict, default_to_dict
from haystack.utils import Secret, deserialize_secrets_inplace

import httpx

@component
class UnixSocketOpenAITextEmbedder:

    def __init__(  # pylint: disable=too-many-positional-arguments
        self,
        socket_path: str,
        model: str = "text-embedding-ada-002",
        dimensions: Optional[int] = None,
        prefix: str = "",
        suffix: str = "",
        timeout: Optional[float] = None,
        max_retries: Optional[int] = None,
    ):
        self.model = model
        self.dimensions = dimensions
        self.socket_path = socket_path
        self.prefix = prefix
        self.suffix = suffix

        if timeout is None:
            timeout = float(os.environ.get("OPENAI_TIMEOUT", 30.0))
        if max_retries is None:
            max_retries = int(os.environ.get("OPENAI_MAX_RETRIES", 5))

        transport = httpx.HTTPTransport(uds=socket_path)
        http_client = httpx.Client(transport=transport)
        self.client = OpenAI(
            api_key="unused",
            timeout=timeout,
            max_retries=max_retries,
            http_client=http_client,
            base_url="http://localhost",
        )

    def _get_telemetry_data(self) -> Dict[str, Any]:
        """
        Data that is sent to Posthog for usage analytics.
        """
        return {"model": self.model}

    def to_dict(self) -> Dict[str, Any]:
        """
        Serializes the component to a dictionary.

        :returns:
            Dictionary with serialized data.
        """
        return default_to_dict(
            self,
            model=self.model,
            api_base_url=self.api_base_url,
            organization=self.organization,
            prefix=self.prefix,
            suffix=self.suffix,
            dimensions=self.dimensions,
            api_key=self.api_key.to_dict(),
        )

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "OpenAITextEmbedder":
        """
        Deserializes the component from a dictionary.

        :param data:
            Dictionary to deserialize from.
        :returns:
            Deserialized component.
        """
        deserialize_secrets_inplace(data["init_parameters"], keys=["api_key"])
        return default_from_dict(cls, data)

    @component.output_types(embedding=List[float], meta=Dict[str, Any])
    def run(self, text: str):
        """
        Embeds a single string.

        :param text:
            Text to embed.

        :returns:
            A dictionary with the following keys:
            - `embedding`: The embedding of the input text.
            - `meta`: Information about the usage of the model.
        """
        if not isinstance(text, str):
            raise TypeError(
                "OpenAITextEmbedder expects a string as an input."
                "In case you want to embed a list of Documents, please use the OpenAIDocumentEmbedder."
            )

        text_to_embed = self.prefix + text + self.suffix

        # copied from OpenAI embedding_utils (https://github.com/openai/openai-python/blob/main/openai/embeddings_utils.py)
        # replace newlines, which can negatively affect performance.
        text_to_embed = text_to_embed.replace("\n", " ")

        if self.dimensions is not None:
            response = self.client.embeddings.create(model=self.model, dimensions=self.dimensions, input=text_to_embed)
        else:
            response = self.client.embeddings.create(model=self.model, input=text_to_embed)

        meta = {"model": response.model, "usage": dict(response.usage)}

        return {"embedding": response.data[0].embedding, "meta": meta}



#########################################



# SPDX-FileCopyrightText: 2022-present deepset GmbH <info@deepset.ai>
#
# SPDX-License-Identifier: Apache-2.0

import os
from typing import Any, Dict, List, Optional, Tuple

from more_itertools import batched
from openai import APIError, OpenAI
from tqdm import tqdm

from haystack import Document, component, default_from_dict, default_to_dict, logging
from haystack.utils import Secret, deserialize_secrets_inplace

logger = logging.getLogger(__name__)


@component
class UnixSocketOpenAIDocumentEmbedder:
    """
    Computes document embeddings using OpenAI models.

    ### Usage example

    ```python
    from haystack import Document
    from haystack.components.embedders import OpenAIDocumentEmbedder

    doc = Document(content="I love pizza!")

    document_embedder = OpenAIDocumentEmbedder()

    result = document_embedder.run([doc])
    print(result['documents'][0].embedding)

    # [0.017020374536514282, -0.023255806416273117, ...]
    ```
    """

    def __init__(  # pylint: disable=too-many-positional-arguments
        self,
        socket_path: str,
        model: str = "text-embedding-ada-002",
        dimensions: Optional[int] = None,
        prefix: str = "",
        suffix: str = "",
        batch_size: int = 32,
        progress_bar: bool = True,
        meta_fields_to_embed: Optional[List[str]] = None,
        embedding_separator: str = "\n",
        timeout: Optional[float] = None,
        max_retries: Optional[int] = None,
    ):
        self.model = model
        self.dimensions = dimensions
        self.socket_path = socket_path
        self.prefix = prefix
        self.suffix = suffix
        self.batch_size = batch_size
        self.progress_bar = progress_bar
        self.meta_fields_to_embed = meta_fields_to_embed or []
        self.embedding_separator = embedding_separator

        if timeout is None:
            timeout = float(os.environ.get("OPENAI_TIMEOUT", 30.0))
        if max_retries is None:
            max_retries = int(os.environ.get("OPENAI_MAX_RETRIES", 5))

        transport = httpx.HTTPTransport(uds=socket_path)
        http_client = httpx.Client(transport=transport)
        self.client = OpenAI(
            api_key="unused",
            timeout=timeout,
            max_retries=max_retries,
            http_client=http_client,
            base_url="http://localhost",
        )

    def _get_telemetry_data(self) -> Dict[str, Any]:
        """
        Data that is sent to Posthog for usage analytics.
        """
        return {"model": self.model}

    def to_dict(self) -> Dict[str, Any]:
        """
        Serializes the component to a dictionary.

        :returns:
            Dictionary with serialized data.
        """
        return default_to_dict(
            self,
            model=self.model,
            dimensions=self.dimensions,
            organization=self.organization,
            api_base_url=self.api_base_url,
            prefix=self.prefix,
            suffix=self.suffix,
            batch_size=self.batch_size,
            progress_bar=self.progress_bar,
            meta_fields_to_embed=self.meta_fields_to_embed,
            embedding_separator=self.embedding_separator,
            api_key=self.api_key.to_dict(),
        )

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "OpenAIDocumentEmbedder":
        """
        Deserializes the component from a dictionary.

        :param data:
            Dictionary to deserialize from.
        :returns:
            Deserialized component.
        """
        deserialize_secrets_inplace(data["init_parameters"], keys=["api_key"])
        return default_from_dict(cls, data)

    def _prepare_texts_to_embed(self, documents: List[Document]) -> Dict[str, str]:
        """
        Prepare the texts to embed by concatenating the Document text with the metadata fields to embed.
        """
        texts_to_embed = {}
        for doc in documents:
            meta_values_to_embed = [
                str(doc.meta[key]) for key in self.meta_fields_to_embed if key in doc.meta and doc.meta[key] is not None
            ]

            text_to_embed = (
                self.prefix + self.embedding_separator.join(meta_values_to_embed + [doc.content or ""]) + self.suffix
            )

            # copied from OpenAI embedding_utils (https://github.com/openai/openai-python/blob/main/openai/embeddings_utils.py)
            # replace newlines, which can negatively affect performance.
            texts_to_embed[doc.id] = text_to_embed.replace("\n", " ")
        return texts_to_embed

    def _embed_batch(self, texts_to_embed: Dict[str, str], batch_size: int) -> Tuple[List[List[float]], Dict[str, Any]]:
        """
        Embed a list of texts in batches.
        """

        all_embeddings = []
        meta: Dict[str, Any] = {}
        for batch in tqdm(
            batched(texts_to_embed.items(), batch_size), disable=not self.progress_bar, desc="Calculating embeddings"
        ):
            args: Dict[str, Any] = {"model": self.model, "input": [b[1] for b in batch]}

            if self.dimensions is not None:
                args["dimensions"] = self.dimensions

            try:
                response = self.client.embeddings.create(**args)
            except APIError as exc:
                ids = ", ".join(b[0] for b in batch)
                msg = "Failed embedding of documents {ids} caused by {exc}"
                logger.exception(msg, ids=ids, exc=exc)
                continue

            embeddings = [el.embedding for el in response.data]
            all_embeddings.extend(embeddings)

            if "model" not in meta:
                meta["model"] = response.model
            if "usage" not in meta:
                meta["usage"] = dict(response.usage)
            else:
                meta["usage"]["prompt_tokens"] += response.usage.prompt_tokens
                meta["usage"]["total_tokens"] += response.usage.total_tokens

        return all_embeddings, meta

    @component.output_types(documents=List[Document], meta=Dict[str, Any])
    def run(self, documents: List[Document]):
        """
        Embeds a list of documents.

        :param documents:
            A list of documents to embed.

        :returns:
            A dictionary with the following keys:
            - `documents`: A list of documents with embeddings.
            - `meta`: Information about the usage of the model.
        """
        if not isinstance(documents, list) or documents and not isinstance(documents[0], Document):
            raise TypeError(
                "OpenAIDocumentEmbedder expects a list of Documents as input."
                "In case you want to embed a string, please use the OpenAITextEmbedder."
            )

        texts_to_embed = self._prepare_texts_to_embed(documents=documents)

        embeddings, meta = self._embed_batch(texts_to_embed=texts_to_embed, batch_size=self.batch_size)

        for doc, emb in zip(documents, embeddings):
            doc.embedding = emb

        return {"documents": documents, "meta": meta}







##########################################

# XXX Old implementation

# import logging
# import socket
# import http.client
# from typing import Optional, Dict, Any, Union, List, Callable
# import json
# from io import BytesIO
# import httpx

# from haystack.components.embedders import OpenAIDocumentEmbedder, OpenAITextEmbedder
# from haystack.utils import Secret
# from haystack.dataclasses import Document

# # Import json module to avoid name conflict with the json parameter

# logger = logging.getLogger(__name__)


# def _patch_openai_client(client, socket_path):
#     transport = httpx.HTTPTransport(uds=socket_path)
#     client = httpx.Client(transport=transport)
#     client._client = client

# class UnixSocketOpenAITextEmbedderOld(OpenAITextEmbedder):
#     """
#     A text embedder that uses the OpenAI API via a UNIX socket.
#     """
    
#     def __init__(self, socket_path: str, *args, **kwargs):
#         """
#         Initialize the UnixSocketOpenAITextEmbedder.
        
#         Args:
#             socket_path: Path to the UNIX socket.
#             *args, **kwargs: Arguments passed to OpenAITextEmbedder.
#         """
#         # Initialize the parent class
#         super().__init__(*args, **kwargs)
#         _patch_openai_client(self.client, socket_path)
#         logger.info(f"Initialized UnixSocketOpenAITextEmbedder with socket: {socket_path}")


# class UnixSocketOpenAIDocumentEmbedder(OpenAIDocumentEmbedder):
#     """
#     A document embedder that uses the OpenAI API via a UNIX socket.
#     """
    
#     def __init__(self, socket_path: str, *args, **kwargs):
#         """
#         Initialize the UnixSocketOpenAIDocumentEmbedder.
        
#         Args:
#             socket_path: Path to the UNIX socket.
#             *args, **kwargs: Arguments passed to OpenAIDocumentEmbedder.
#         """
#         # Initialize the parent class
#         super().__init__(*args, **kwargs)
#         _patch_openai_client(self.client, socket_path)


# # Should be not needed.
# #   def run(self, documents: List[Document]) -> Dict[str, List[Document]]:
# #       """
# #       Embed the given documents using the OpenAI API via a UNIX socket.
# #       
# #       Args:
# #           documents: The documents to embed.
# #           
# #       Returns:
# #           A dictionary with the embedded documents.
# #       """
# #       # Call the parent class's run method
# #       return super().run(documents=documents)

# # Should be not needed.
# #   def run(self, text: str) -> Dict[str, List[List[float]]]:
# #       """
# #       Embed the given text using the OpenAI API via a UNIX socket.
# #       
# #       Args:
# #           text: The text to embed.
# #           
# #       Returns:
# #           A dictionary with the embeddings.
# #       """
# #       # Call the parent class's run method
# #       return super().run(text=text)

# # Again, shouldn't be needed.
# #   class UnixSocketResponse:
# #       """A response object that mimics the httpx.Response interface used by OpenAI client"""
# #       
# #       def __init__(self, status_code, headers, content):
# #           self.status_code = status_code
# #           self.headers = headers
# #           self._content = content
# #           self._text = None
# #           
# #       def json(self) -> Dict[str, Any]:
# #           """Parse the response content as JSON"""
# #           return json.loads(self._content.decode('utf-8'))
# #       
# #       @property
# #       def content(self) -> bytes:
# #           """Return the raw content"""
# #           return self._content
# #       
# #       @property
# #       def text(self) -> str:
# #           """Return the content as text"""
# #           if self._text is None:
# #               self._text = self._content.decode("utf-8")
# #           return self._text
# #           
# #       def raise_for_status(self):
# #           """Raise an exception if the status code indicates an error"""
# #           if self.status_code >= 400:
# #               raise Exception(f"HTTP Error: {self.status_code}")


# #   class UnixSocketAdapter:
# #       """HTTP client adapter that uses a UNIX socket"""
# #       
# #       def __init__(self, socket_path, timeout=60):
# #           self.socket_path = socket_path
# #           self.timeout = timeout
# #       
# #       def request(self, method, url, headers=None, content=None, stream=False, auth=None, json=None, **kwargs) -> UnixSocketResponse:
# #           """Make an HTTP request using a UNIX socket"""
# #           # Create a socket
# #           sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
# #           sock.settimeout(self.timeout)
# #           sock.connect(self.socket_path)
# #           
# #           # Create a connection using the socket
# #           conn = http.client.HTTPConnection("localhost")
# #           conn.sock = sock
# #           
# #           # Extract just the path from the URL
# #           path = url
# #           if "://" in url:
# #               parts = url.split("://", 1)[1].split("/", 1)
# #               path = "/" + parts[1] if len(parts) > 1 else "/"
# #           
# #           # Prepare headers
# #           request_headers = {}
# #           if headers:
# #               request_headers.update(headers)
# #           
# #           # Prepare body
# #           body = None
# #           if json is not None:
# #               body = json_module.dumps(json).encode('utf-8')
# #               request_headers['Content-Type'] = 'application/json'
# #           elif content:
# #               body = content
# #           
# #           # Make request
# #           conn.request(method, path, body=body, headers=request_headers)
# #           http_response = conn.getresponse()
# #           
# #           # Read the response content
# #           content = http_response.read()
# #           
# #           # Convert headers to a dictionary
# #           headers_dict = {k.lower(): v for k, v in http_response.getheaders()}
# #           
# #           # Create and return a response object that mimics the httpx.Response interface
# #           response = UnixSocketResponse(
# #               status_code=http_response.status,
# #               headers=headers_dict,
# #               content=content
# #           )
# #           
# #           # For debugging
# #           logger.debug(f"Response status: {response.status_code}")
# #           logger.debug(f"Response headers: {response.headers}")
# #           logger.debug(f"Response content: {response.content[:100]}...")
# #           
# #           # If the content type is JSON, try to parse it
# #           if 'content-type' in headers_dict and 'application/json' in headers_dict['content-type']:
# #               try:
# #                   logger.debug(f"Parsed JSON: {response.json()}")
# #               except Exception as e:
# #                   logger.error(f"Failed to parse JSON: {e}")
# #           
# #           return response
