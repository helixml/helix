import os
from typing import Any, Dict, List, Optional, Tuple
import time

from openai import OpenAI

from haystack import component, default_from_dict, default_to_dict
from haystack.utils import Secret, deserialize_secrets_inplace

import httpx

from haystack import logging

logger = logging.getLogger(__name__)

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
            timeout = float(os.environ.get("OPENAI_TIMEOUT", 300.0))
        if max_retries is None:
            max_retries = int(os.environ.get("OPENAI_MAX_RETRIES", 5))

        transport = httpx.HTTPTransport(uds=socket_path)
        http_client = httpx.Client(transport=transport, timeout=timeout)
        self.client = OpenAI(
            api_key="unused",
            timeout=timeout,
            max_retries=max_retries,
            http_client=http_client,
            base_url="http://localhost/v1", # needed to stop it using TLS
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
    def from_dict(cls, data: Dict[str, Any]) -> "UnixSocketOpenAITextEmbedder":
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

        # Log the request
        logger.info(f"🧩 TEXT EMBEDDING REQUEST [socket={self.socket_path}, model={self.model}] input_length={len(text_to_embed)} chars")

        # Log detailed request information
        endpoint = "/embeddings"
        req_body_summary = {
            "model": self.model,
            "input_type": "string",
            "input_length": len(text_to_embed),
            "dimensions": self.dimensions,
        }
        logger.info(
            f"🔍 EMBEDDING REQUEST DETAILS [socket={self.socket_path}] "
            f"endpoint=POST {endpoint} "
            f"request_body={req_body_summary}"
        )

        # Log a sample of the text (truncated)
        sample = text_to_embed[:100] + "..." if len(text_to_embed) > 100 else text_to_embed
        logger.info(f"📄 EMBEDDING INPUT SAMPLE: '{sample}'")

        start_time = time.time()
        try:
            args = {"model": self.model, "input": text_to_embed}
            if self.dimensions is not None:
                args["dimensions"] = self.dimensions

            response = self.client.embeddings.create(**args)

            duration_ms = int((time.time() - start_time) * 1000)

            # Log response information
            resp_summary = {
                "status": "success",
                "data_count": len(response.data) if hasattr(response, "data") else 0,
                "model": response.model if hasattr(response, "model") else "unknown",
            }

            if hasattr(response, "usage") and hasattr(response.usage, "prompt_tokens"):
                resp_summary["prompt_tokens"] = response.usage.prompt_tokens

            meta = {"model": response.model, "usage": dict(response.usage)}

            # Log the response
            logger.info(
                f"✅ TEXT EMBEDDING RESPONSE [socket={self.socket_path}, model={self.model}] "
                f"duration_ms={duration_ms} response={resp_summary} dimensions={len(response.data[0].embedding)}"
            )

            return {"embedding": response.data[0].embedding, "meta": meta}
        except Exception as e:
            duration_ms = int((time.time() - start_time) * 1000)
            error_details = {
                "error_type": type(e).__name__,
                "error_message": str(e),
                "endpoint": "/embeddings",
                "model": self.model,
                "input_length": len(text_to_embed),
            }
            logger.error(
                f"❌ TEXT EMBEDDING ERROR [socket={self.socket_path}, model={self.model}] "
                f"duration_ms={duration_ms} error_details={error_details}"
            )
            # Include exception traceback for debugging
            import traceback
            logger.error(f"TRACEBACK: {traceback.format_exc()}")
            raise



#########################################


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
        verify_connection: bool = True,
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
            timeout = float(os.environ.get("OPENAI_TIMEOUT", 300.0))
        if max_retries is None:
            max_retries = int(os.environ.get("OPENAI_MAX_RETRIES", 5))

        transport = httpx.HTTPTransport(uds=socket_path)
        http_client = httpx.Client(transport=transport, timeout=timeout)
        self.client = OpenAI(
            api_key="unused",
            timeout=timeout,
            max_retries=max_retries,
            http_client=http_client,
            base_url="http://localhost/v1", # needed to stop it using TLS
        )

        # Verify the socket connection if requested
        if verify_connection:
            self._verify_socket_connection()

    def _verify_socket_connection(self):
        """
        Verify that the socket connection is working by sending a small test request.
        This helps identify socket connectivity issues early.

        Will retry for up to 5 minutes with exponential backoff before giving up.
        """
        logger.info(f"Verifying socket connection to {self.socket_path}...")

        start_time = time.time()
        max_wait_time = 5 * 60  # 5 minutes in seconds
        attempt = 0
        retry_delay = 1  # Start with 1 second delay

        while time.time() - start_time < max_wait_time:
            attempt += 1
            try:
                # Send a request to the models endpoint, which should be lightweight
                # and doesn't actually require a model to be loaded
                logger.info(f"Socket connection attempt {attempt} (elapsed: {int(time.time() - start_time)}s)")
                response = self.client.models.list()

                if hasattr(response, "data") and len(response.data) > 0:
                    available_models = [model.id for model in response.data]
                    logger.info(f"Socket connection verified after {int(time.time() - start_time)}s. Available models: {available_models}")

                    # Check if our model is available
                    if self.model not in available_models:
                        logger.warning(
                            f"⚠️ Requested model '{self.model}' not found in available models. "
                            f"This may cause errors when creating embeddings."
                        )
                    return True  # Connection successful
                else:
                    logger.warning("Socket connection verified but no models were returned.")
                    return True  # Connection successful but empty response

            except httpx.ConnectError as e:
                elapsed = int(time.time() - start_time)
                remaining = max_wait_time - elapsed

                if remaining <= 0:
                    logger.error(f"❌ Socket connection failed after {elapsed}s: {str(e)}")
                    logger.error(
                        f"The socket path '{self.socket_path}' may not exist or the service isn't running. "
                        f"Embedding requests will fail until this is fixed."
                    )
                    break

                logger.warning(
                    f"⏳ Socket not ready ({elapsed}s elapsed). Will retry in {retry_delay}s. "
                    f"Waiting up to {remaining}s more."
                )
                time.sleep(retry_delay)
                # Exponential backoff with a max of 20 seconds between retries
                retry_delay = min(retry_delay * 1.5, 20)

            except Exception as e:
                logger.error(f"❌ Error verifying socket connection: {str(e)}")
                import traceback
                logger.error(f"TRACEBACK: {traceback.format_exc()}")
                break

        logger.warning(
            f"Socket verification timed out after {int(time.time() - start_time)}s. "
            f"Continuing without verification. Embedding requests may fail."
        )
        return False  # Connection could not be verified

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
    def from_dict(cls, data: Dict[str, Any]) -> "UnixSocketOpenAIDocumentEmbedder":
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
            text_to_embed = text_to_embed.replace("\n", " ")

            # DEBUG: Log individual document text length
            text_length = len(text_to_embed)
            logger.warning(f"🐛 DEBUG CHUNK SIZE: doc_id={doc.id} content_length={len(doc.content or '')} chars, final_text_length={text_length} chars")
            if text_length > 50000:  # Log huge chunks
                logger.error(f"🚨 MASSIVE CHUNK DETECTED: doc_id={doc.id} text_length={text_length} chars - this will likely fail embedding!")
                # Log first 200 chars to see what kind of content this is
                preview = text_to_embed[:200] + "..." if len(text_to_embed) > 200 else text_to_embed
                logger.error(f"🚨 MASSIVE CHUNK PREVIEW: '{preview}'")

            # Ensure document ID is a string - this prevents unhashable type errors
            # if doc.id is not a string but a slice or other unhashable type
            doc_id = str(doc.id) if doc.id is not None else f"doc_{id(doc)}"
            texts_to_embed[doc_id] = text_to_embed
        return texts_to_embed

    def _embed_batch(self, texts_to_embed: List[str], batch_size: int) -> Tuple[List[List[float]], Dict[str, Any]]:
        """Embed a batch of texts.

        Args:
            texts_to_embed: A list of texts to embed.
            batch_size: The batch size.

        Returns:
            A tuple of (embeddings, meta). The first element is a list of embeddings,
            each embedding being a list of floats. The second element is a dictionary
            with meta information about the API response.
        """
        all_embeddings: List[List[float]] = []
        total_prompt_tokens = 0
        total_batches = len(texts_to_embed) // batch_size + (1 if len(texts_to_embed) % batch_size > 0 else 0)
        logger.info(f"🔄 EMBEDDING BATCH PROCESS [socket={self.socket_path}] total_batches={total_batches} batch_size={batch_size}")

        for i in range(0, len(texts_to_embed), batch_size):
            batch_start_time = time.time()
            batch = texts_to_embed[i : i + batch_size]
            batch_number = i // batch_size + 1
            logger.info(f"⏳ EMBEDDING BATCH [{batch_number}/{total_batches}] [socket={self.socket_path}] size={len(batch)}")

            try:
                # Create embeddings using the OpenAI client directly
                args = {"model": self.model, "input": batch}
                if self.dimensions is not None:
                    args["dimensions"] = self.dimensions

                # Log detailed request information
                endpoint = "/embeddings"
                req_body_summary = {
                    "model": self.model,
                    "input_type": type(batch).__name__,
                    "input_length": len(batch),
                    "dimensions": self.dimensions,
                }
                logger.info(
                    f"🔍 EMBEDDING REQUEST DETAILS [socket={self.socket_path}] "
                    f"endpoint=POST {endpoint} "
                    f"request_body={req_body_summary}"
                )
                if len(batch) > 0:
                    # Log a sample of the first text (truncated)
                    sample = batch[0][:100] + "..." if len(batch[0]) > 100 else batch[0]
                    logger.info(f"📄 EMBEDDING INPUT SAMPLE: '{sample}'")

                response = self.client.embeddings.create(**args)
                batch_duration_ms = int((time.time() - batch_start_time) * 1000)

                # Log response information
                resp_summary = {
                    "status": "success",
                    "data_count": len(response.data) if hasattr(response, "data") else 0,
                    "model": response.model if hasattr(response, "model") else "unknown",
                }

                if hasattr(response, "usage") and hasattr(response.usage, "prompt_tokens"):
                    total_prompt_tokens += response.usage.prompt_tokens
                    resp_summary["prompt_tokens"] = response.usage.prompt_tokens
                    logger.info(
                        f"✅ EMBEDDING BATCH RESPONSE [{batch_number}/{total_batches}] [socket={self.socket_path}] "
                        f"duration_ms={batch_duration_ms} response={resp_summary}"
                    )
                else:
                    logger.warning(
                        f"⚠️ EMBEDDING BATCH RESPONSE [{batch_number}/{total_batches}] [socket={self.socket_path}] "
                        f"duration_ms={batch_duration_ms} missing_usage_data=True response={resp_summary}"
                    )

                # Process each embedding response
                for data in response.data:
                    try:
                        # Get the embedding data
                        emb = data.embedding

                        # Check embedding type and convert to list as needed
                        if emb is None:
                            # Handle None case
                            all_embeddings.append(None)
                        elif isinstance(emb, list):
                            # Ensure it's a proper list of float values
                            all_embeddings.append([float(val) for val in emb])
                        else:
                            # For any other type (numpy arrays, slices, etc.), convert explicitly
                            # This will handle cases where the embedding is returned as a special type
                            embedding_list = list(map(float, emb))
                            all_embeddings.append(embedding_list)
                            logger.debug(f"Converted embedding of type {type(emb).__name__} to list: {type(embedding_list)}")
                    except Exception as conversion_error:
                        logger.error(f"Error converting embedding: {conversion_error}")
                        # Include the object type in the log
                        logger.error(f"Embedding object type: {type(data.embedding).__name__}")
                        # Fallback to None for this embedding
                        all_embeddings.append(None)

                # Log first embedding dimensions if available
                if len(response.data) > 0 and response.data[0].embedding is not None:
                    try:
                        embedding_dim = len(response.data[0].embedding)
                        logger.info(f"📊 EMBEDDING DIMENSIONS: {embedding_dim}")
                    except Exception as dim_error:
                        logger.warning(f"Could not determine embedding dimensions: {dim_error}")

            except httpx.HTTPStatusError as e:
                batch_duration_ms = int((time.time() - batch_start_time) * 1000)
                # Check for specific status codes that indicate model not available
                if e.response.status_code == 404:
                    error_message = "Model not found: The embedding model is not available or loaded. Check vLLM configuration."
                elif e.response.status_code == 503:
                    error_message = "Service unavailable: vLLM may be overloaded or unable to start the embedding model."
                else:
                    error_message = f"HTTP error {e.response.status_code}: {e.response.text}"

                logger.error(
                    f"🚨 EMBEDDING MODEL ERROR [socket={self.socket_path}, model={self.model}] "
                    f"duration_ms={batch_duration_ms} error={error_message}"
                )
                # This is a serious error that indicates the model isn't available
                raise ValueError(f"Embedding model not available: {error_message}") from e

            except httpx.ConnectError as e:
                batch_duration_ms = int((time.time() - batch_start_time) * 1000)
                error_message = f"Connection error: Could not connect to vLLM socket at {self.socket_path}"
                logger.error(
                    f"🚨 EMBEDDING SOCKET ERROR [socket={self.socket_path}] "
                    f"duration_ms={batch_duration_ms} error={error_message}"
                )
                raise ConnectionError(f"Embedding socket connection failed: {str(e)}") from e

            except Exception as e:
                batch_duration_ms = int((time.time() - batch_start_time) * 1000)
                error_details = {
                    "error_type": type(e).__name__,
                    "error_message": str(e),
                    "endpoint": "/embeddings",
                    "model": self.model,
                    "batch_size": len(batch),
                }
                logger.error(
                    f"❌ EMBEDDING BATCH ERROR [{batch_number}/{total_batches}] [socket={self.socket_path}] "
                    f"duration_ms={batch_duration_ms} error_details={error_details}"
                )
                # Include exception traceback for debugging
                import traceback
                logger.error(f"TRACEBACK: {traceback.format_exc()}")
                raise

        meta = {"usage": {"prompt_tokens": total_prompt_tokens}}
        logger.info(f"✅ EMBEDDING COMPLETE [socket={self.socket_path}] total_batches={total_batches} total_tokens={total_prompt_tokens}")
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

        logger.info(f"🧩 DOCUMENT EMBEDDING REQUEST [socket={self.socket_path}, model={self.model}] documents={len(documents)}")
        start_time = time.time()

        try:
            # First ensure all documents have valid IDs
            for i, doc in enumerate(documents):
                if doc.id is None or not isinstance(doc.id, str):
                    doc.id = f"doc_{i}_{id(doc)}"
                    logger.debug(f"Assigned generated ID to document: {doc.id}")

            # Now prepare texts to embed
            texts_to_embed = self._prepare_texts_to_embed(documents=documents)
            logger.info(f"🔄 DOCUMENT EMBEDDING PROCESSING [socket={self.socket_path}] prepared_texts={len(texts_to_embed)}")

            # Convert dict values to a list for batch processing
            doc_ids = list(texts_to_embed.keys())
            texts_list = [texts_to_embed[doc_id] for doc_id in doc_ids]

            logger.debug(f"Document IDs: {doc_ids[:5]}{'...' if len(doc_ids) > 5 else ''}")

            # Get embeddings from the API
            embeddings, meta = self._embed_batch(texts_to_embed=texts_list, batch_size=self.batch_size)

            # Validate embeddings before assigning
            if len(embeddings) != len(documents):
                logger.warning(
                    f"Mismatch between number of embeddings ({len(embeddings)}) and documents ({len(documents)}). "
                    f"Some documents may not receive embeddings."
                )

            # Create a mapping from document ID to index position in the documents list
            doc_index_map = {doc.id: i for i, doc in enumerate(documents)}

            # Assign embeddings back to documents
            for i, doc_id in enumerate(doc_ids):
                if i < len(embeddings):
                    # Find the document in our list that matches this ID
                    if doc_id in doc_index_map:
                        doc_idx = doc_index_map[doc_id]
                        # Convert embedding to a list if it's not already one
                        embedding = embeddings[i]
                        if embedding is not None:
                            documents[doc_idx].embedding = list(embedding)
                        logger.debug(f"Assigned embedding to document {doc_id} at index {doc_idx}")
                    else:
                        logger.warning(f"Document ID {doc_id} not found in document list")

            duration_ms = int((time.time() - start_time) * 1000)
            embedding_dim = len(embeddings[0]) if embeddings and len(embeddings) > 0 else 0
            logger.info(
                f"✅ DOCUMENT EMBEDDING RESPONSE [socket={self.socket_path}, model={self.model}] "
                f"duration_ms={duration_ms} documents={len(documents)} "
                f"tokens={meta['usage']['prompt_tokens']} dimensions={embedding_dim}"
            )

            return {"documents": documents, "meta": meta}
        except Exception as e:
            duration_ms = int((time.time() - start_time) * 1000)
            logger.error(
                f"❌ DOCUMENT EMBEDDING ERROR [socket={self.socket_path}, model={self.model}] "
                f"duration_ms={duration_ms} documents={len(documents)} error={str(e)}"
            )
            # Include traceback for better debugging
            import traceback
            logger.error(f"TRACEBACK: {traceback.format_exc()}")
            raise
