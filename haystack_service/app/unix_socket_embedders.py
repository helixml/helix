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
            texts_to_embed[doc.id] = text_to_embed.replace("\n", " ")
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
                
                for data in response.data:
                    all_embeddings.append(data.embedding)
                    
                # Log first embedding dimensions if available
                if len(response.data) > 0:
                    embedding_dim = len(response.data[0].embedding)
                    logger.info(f"📊 EMBEDDING DIMENSIONS: {embedding_dim}")
                
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
            texts_to_embed = self._prepare_texts_to_embed(documents=documents)
            logger.info(f"🔄 DOCUMENT EMBEDDING PROCESSING [socket={self.socket_path}] prepared_texts={len(texts_to_embed)}")

            embeddings, meta = self._embed_batch(texts_to_embed=texts_to_embed, batch_size=self.batch_size)
            
            for doc, emb in zip(documents, embeddings):
                doc.embedding = emb

            duration_ms = int((time.time() - start_time) * 1000)
            logger.info(
                f"✅ DOCUMENT EMBEDDING RESPONSE [socket={self.socket_path}, model={self.model}] "
                f"duration_ms={duration_ms} documents={len(documents)} "
                f"tokens={meta['usage']['prompt_tokens']} dimensions={len(embeddings[0]) if embeddings else 0}"
            )
            
            return {"documents": documents, "meta": meta}
        except Exception as e:
            duration_ms = int((time.time() - start_time) * 1000)
            logger.error(
                f"❌ DOCUMENT EMBEDDING ERROR [socket={self.socket_path}, model={self.model}] "
                f"duration_ms={duration_ms} documents={len(documents)} error={str(e)}"
            )
            raise