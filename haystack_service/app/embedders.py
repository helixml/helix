import time
from typing import Dict, List, Optional, Union, Any, Literal
import logging
import numpy as np
import base64
import json
import requests
import io
from pathlib import Path
from PIL import Image
from haystack import component, Document
from haystack.dataclasses import ByteStream
from copy import deepcopy
from openai.types.create_embedding_response import CreateEmbeddingResponse

logger = logging.getLogger(__name__)


@component
class MultimodalEmbedder:
    """
    A multimodal embedder that can process both text and images to create embeddings.

    This component creates embeddings using a specified model that can handle
    text, images, or a combination of both.
    """

    def __init__(
        self,
        api_key: str,
        model: str = "MrLight/dse-qwen2-2b-mrl-v1",
        api_base_url: str = "http://localhost:8000/v1",
        encoding_format: str = "float",
        user_prompt: str = "What is shown in this image?",
        text_prefix: str = "Query: ",
    ):
        """
        Initialize the MultimodalEmbedder.

        Args:
            api_key: API key for the embeddings service.
            model: Name of the model to use for embeddings.
            api_url: URL for the embeddings API endpoint.
            encoding_format: Format for encoding the embeddings (e.g., "float").
            user_prompt: Text prompt to use for image embedding.
            text_prefix: Prefix to add to text queries.
        """
        self.api_key = api_key
        self.model = model
        self.api_base_url = api_base_url
        self.encoding_format = encoding_format
        self.user_prompt = user_prompt
        self.text_prefix = text_prefix

        # Create a placeholder image once during initialization
        self.placeholder_image = self._create_placeholder_image()

    def _create_placeholder_image(self) -> str:
        """
        Create a placeholder image for text-only embeddings.

        Returns:
            Base64-encoded placeholder image.
        """
        buffer = io.BytesIO()
        image_placeholder = Image.new("RGB", (56, 56))
        image_placeholder.save(buffer, "png")
        buffer.seek(0)
        return base64.b64encode(buffer.read()).decode("utf-8")

    def _encode_image(self, image_path: str) -> str:
        """
        Encode an image as base64.

        Args:
            image_path: Path to the image file.

        Returns:
            Base64-encoded image string.
        """
        with open(image_path, "rb") as image_file:
            return base64.b64encode(image_file.read()).decode("utf-8")

    def _build_request_for_image(self, image_path: str) -> Dict:
        """
        Build the request payload for image embedding.

        Args:
            image_path: Path to the image to embed.

        Returns:
            Dictionary with the request payload.
        """
        image_base64 = self._encode_image(image_path)

        messages = [
            {
                "role": "user",
                "content": [
                    {
                        "type": "image_url",
                        "image_url": {
                            "url": f"data:image/png;base64,{image_base64}",
                        },
                    },
                    {"type": "text", "text": self.user_prompt},
                ],
            }
        ]

        return {
            "model": self.model,
            "messages": messages,
            "encoding_format": self.encoding_format,
        }

    def _build_request_for_text(self, text: str) -> Dict:
        """
        Build the request payload for text embedding.

        Args:
            text: Text to embed.

        Returns:
            Dictionary with the request payload.
        """
        messages = [
            {
                "role": "user",
                "content": [
                    {
                        "type": "image_url",
                        "image_url": {
                            "url": f"data:image/jpeg;base64,{self.placeholder_image}",
                        },
                    },
                    {"type": "text", "text": f"{self.text_prefix}{text}"},
                ],
            }
        ]

        return {
            "model": self.model,
            "messages": messages,
            "encoding_format": self.encoding_format,
        }

    def _get_embedding(
        self, text: Optional[str] = None, image_path: Optional[str] = None
    ) -> List[float]:
        """
        Get embedding for text, image, or both.

        Args:
            text: Optional text to embed.
            image_path: Optional path to an image to embed.

        Returns:
            The embedding as a list of floats.
        """
        if not text and not image_path:
            raise ValueError("Either text or image_path must be provided")

        # Prioritize image if both are provided
        if image_path:
            request_payload = self._build_request_for_image(image_path)
        else:
            request_payload = self._build_request_for_text(text)

        # Make the request using the requests library
        headers = {"Authorization": f"Bearer {self.api_key}"}
        response = requests.post(
            self.api_base_url + "/embeddings",
            headers=headers,
            json=request_payload,
        )
        response.raise_for_status()

        # Extract embedding from response
        response_json = response.json()
        return response_json["data"][0]["embedding"]

    @component.output_types(documents=List[Document])
    def run(self, documents: List[Document]):
        """
        Create embeddings for a batch of documents.

        Args:
            documents: List of documents to embed.

        Returns:
            Dictionary containing the documents with embeddings.
        """
        documents_with_embeddings = []

        time_start = time.time()

        # Process documents one by one
        for doc in documents:
            text = doc.content
            image_path = doc.meta.get("image_path")

            if not text and not image_path:
                logger.warning(
                    f"Document with id {doc.id} has no content or image path"
                )
            else:
                if image_path:
                    embedding = self._get_embedding(image_path)
                else:
                    embedding = self._get_embedding(text)

                embedding_doc = Document(
                    id=doc.id,
                    content=text,
                    blob=doc.blob,
                    meta=deepcopy(doc.meta),
                    score=doc.score,
                    embedding=embedding,
                    sparse_embedding=doc.sparse_embedding,
                )
                documents_with_embeddings.append(embedding_doc)

        logger.info(
            f"Created {len(documents_with_embeddings)} embeddings in {time.time() - time_start} seconds"
        )
        return {"documents": documents_with_embeddings}


@component
class MultimodalTextEmbedder:
    """
    A multimodal embedder that can process both text and images to create embeddings.

    This component creates embeddings using a specified model that can handle
    text, images, or a combination of both.
    """

    def __init__(
        self,
        api_key: str,
        model: str = "MrLight/dse-qwen2-2b-mrl-v1",
        api_base_url: str = "http://localhost:8000/v1",
        encoding_format: str = "float",
        user_prompt: str = "What is shown in this image?",
        text_prefix: str = "Query: ",
    ):
        """
        Initialize the MultimodalEmbedder.

        Args:
            api_key: API key for the embeddings service.
            model: Name of the model to use for embeddings.
            api_url: URL for the embeddings API endpoint.
            encoding_format: Format for encoding the embeddings (e.g., "float").
            user_prompt: Text prompt to use for image embedding.
            text_prefix: Prefix to add to text queries.
        """
        self.api_key = api_key
        self.model = model
        self.api_base_url = api_base_url
        self.encoding_format = encoding_format
        self.user_prompt = user_prompt
        self.text_prefix = text_prefix

        # Create a placeholder image once during initialization
        self.placeholder_image = self._create_placeholder_image()

    def _create_placeholder_image(self) -> str:
        """
        Create a placeholder image for text-only embeddings.

        Returns:
            Base64-encoded placeholder image.
        """
        buffer = io.BytesIO()
        image_placeholder = Image.new("RGB", (56, 56))
        image_placeholder.save(buffer, "png")
        buffer.seek(0)
        return base64.b64encode(buffer.read()).decode("utf-8")

    def _encode_image(self, image_path: str) -> str:
        """
        Encode an image as base64.

        Args:
            image_path: Path to the image file.

        Returns:
            Base64-encoded image string.
        """
        with open(image_path, "rb") as image_file:
            return base64.b64encode(image_file.read()).decode("utf-8")

    def _build_request_for_image(self, image_path: str) -> Dict:
        """
        Build the request payload for image embedding.

        Args:
            image_path: Path to the image to embed.

        Returns:
            Dictionary with the request payload.
        """
        image_base64 = self._encode_image(image_path)

        messages = [
            {
                "role": "user",
                "content": [
                    {
                        "type": "image_url",
                        "image_url": {
                            "url": f"data:image/png;base64,{image_base64}",
                        },
                    },
                    {"type": "text", "text": self.user_prompt},
                ],
            }
        ]

        return {
            "model": self.model,
            "messages": messages,
            "encoding_format": self.encoding_format,
        }

    def _build_request_for_text(self, text: str) -> Dict:
        """
        Build the request payload for text embedding.

        Args:
            text: Text to embed.

        Returns:
            Dictionary with the request payload.
        """
        messages = [
            {
                "role": "user",
                "content": [
                    {
                        "type": "image_url",
                        "image_url": {
                            "url": f"data:image/jpeg;base64,{self.placeholder_image}",
                        },
                    },
                    {"type": "text", "text": f"{self.text_prefix}{text}"},
                ],
            }
        ]

        return {
            "model": self.model,
            "messages": messages,
            "encoding_format": self.encoding_format,
        }

    def _get_embedding(
        self, text: Optional[str] = None, image_path: Optional[str] = None
    ) -> CreateEmbeddingResponse:
        """
        Get embedding for text, image, or both.

        Args:
            text: Optional text to embed.
            image_path: Optional path to an image to embed.

        Returns:
            The embedding as a list of floats.
        """
        if not text and not image_path:
            raise ValueError("Either text or image_path must be provided")

        # Prioritize image if both are provided
        if image_path:
            request_payload = self._build_request_for_image(image_path)
        else:
            request_payload = self._build_request_for_text(text)

        # Make the request using the requests library
        headers = {"Authorization": f"Bearer {self.api_key}"}
        response = requests.post(
            self.api_base_url + "/embeddings",
            headers=headers,
            json=request_payload,
        )
        response.raise_for_status()

        # Extract embedding from response into a ChatCompletion object
        return CreateEmbeddingResponse.model_validate_json(response.text)

    @component.output_types(embedding=List[float], meta=Dict[str, Any])
    def run(self, text: Optional[str] = None, image_path: Optional[str] = None):
        if not text and not image_path:
            raise ValueError("Either text or image_path must be provided")
        if image_path:
            response = self._get_embedding(image_path)
        else:
            response = self._get_embedding(text)

        meta = {"model": response.model, "usage": dict(response.usage)}

        return {"embedding": response.data[0].embedding, "meta": meta}
