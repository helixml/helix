import base64
import io
import logging
from copy import deepcopy
from typing import Any, Dict, List, Optional
import requests
from requests_unixsocket import Session
from haystack import Document, component
from openai.types.create_embedding_response import CreateEmbeddingResponse
from PIL import Image

logger = logging.getLogger(__name__)

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
        socket_path: str = None,
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
            socket_path: Optional Unix socket path to use for requests.
        """
        self.api_key = api_key
        self.model = model
        self.api_base_url = api_base_url
        self.encoding_format = encoding_format
        self.user_prompt = user_prompt
        self.text_prefix = text_prefix
        self.socket_path = socket_path

        # Create placeholder image once during initialization
        buffer = io.BytesIO()
        Image.new("RGB", (28, 28)).save(buffer, "png")
        buffer.seek(0)
        self.placeholder_image = base64.b64encode(buffer.read()).decode("utf-8")

    def _build_request(
        self, text: Optional[str] = None, image_path: Optional[str] = None
    ) -> Dict:
        """Build request payload for either text or image embedding"""
        image_base64 = self.placeholder_image
        content_text = f"{self.text_prefix}{text}" if text else self.user_prompt

        if image_path:
            with open(image_path, "rb") as f:
                image_base64 = base64.b64encode(f.read()).decode("utf-8")

        messages = [
            {
                "role": "user",
                "content": [
                    {
                        "type": "image_url",
                        "image_url": {"url": f"data:image/png;base64,{image_base64}"},
                    },
                    {"type": "text", "text": content_text},
                ],
            }
        ]
        print("FISH: building request")

        return {
            "model": self.model,
            "messages": messages,
            "encoding_format": self.encoding_format,
        }

    def _get_embedding(
        self, text: Optional[str] = None, image_path: Optional[str] = None
    ) -> CreateEmbeddingResponse:
        """Get embedding for text, image, or both"""
        if not text and not image_path:
            raise ValueError("Either text or image_path must be provided")

        request_payload = self._build_request(text, image_path)
        print("FISH: api_base_url", self.api_base_url)

        # Use Unix socket if provided, otherwise use regular HTTP
        if self.socket_path:
            session = Session()
            url = f"http+unix://{self.socket_path.replace('/', '%2F')}/v1/embeddings"
        else:
            session = requests.Session()
            url = f"{self.api_base_url}/embeddings"

        response = session.post(
            url,
            headers={"Authorization": f"Bearer {self.api_key}"},
            json=request_payload,
        )

        if response.status_code != 200:
            raise ValueError(
                f"error from embedding API ({response.status_code}): {response.text}"
            )

        return CreateEmbeddingResponse.model_validate_json(response.text)

    @component.output_types(embedding=List[float], meta=Dict[str, Any])
    def run(self, text: Optional[str] = None, image_path: Optional[str] = None):
        if not text and not image_path:
            raise ValueError("Either text or image_path must be provided")

        response = self._get_embedding(text, image_path)
        return {
            "embedding": response.data[0].embedding,
            "meta": {"model": response.model, "usage": dict(response.usage)},
        }


@component
class MultimodalDocumentEmbedder(MultimodalTextEmbedder):
    """
    A document-based multimodal embedder that processes a list of documents.
    Inherits from MultimodalTextEmbedder to handle both text and image content within documents.
    """

    @component.output_types(documents=List[Document])
    def run(self, documents: List[Document]):
        """
        Create embeddings for a list of documents.

        Args:
            documents: List of Document objects to embed.

        Returns:
            List of Document objects with embeddings added to their metadata.
        """
        processed_documents = []
        print("FISH: called run")

        for doc in documents:
            # Create a deep copy to avoid modifying the original document
            processed_doc = deepcopy(doc)

            # Check if document has image content
            image_path = doc.meta.get("image_path")

            # Get embedding based on available content
            if image_path:
                embedding_result = self._get_embedding(image_path=image_path)
            else:
                embedding_result = self._get_embedding(text=doc.content)

            # Add embedding and metadata to the document
            processed_doc.embedding = embedding_result.data[0].embedding
            processed_doc.meta["embedding_model"] = embedding_result.model
            processed_doc.meta["embedding_usage"] = dict(embedding_result.usage)

            processed_documents.append(processed_doc)

        return {"documents": processed_documents}
