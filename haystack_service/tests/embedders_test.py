import json
from unittest.mock import Mock, patch

import pytest
from haystack import Document

from app.embedders import (
    MultimodalDocumentEmbedder,
    MultimodalTextEmbedder,
)

# Mock response data
MOCK_EMBEDDING = [0.1, 0.2, 0.3]
MOCK_RESPONSE_DATA = {
    "object": "list",
    "model": "test-model",
    "data": [{"object": "embedding", "embedding": MOCK_EMBEDDING, "index": 0}],
    "usage": {"prompt_tokens": 10, "total_tokens": 10},
}


@pytest.fixture
def mock_response():
    mock = Mock()
    mock.status_code = 200
    mock.text = json.dumps(MOCK_RESPONSE_DATA)
    return mock


@pytest.fixture
def text_embedder():
    return MultimodalTextEmbedder(
        api_key="test-key", model="test-model", api_base_url="http://test-url"
    )


@pytest.fixture
def document_embedder():
    return MultimodalDocumentEmbedder(
        api_key="test-key", model="test-model", api_base_url="http://test-url"
    )


class TestMultimodalTextEmbedder:
    def test_initialization(self, text_embedder):
        assert text_embedder.api_key == "test-key"
        assert text_embedder.model == "test-model"
        assert text_embedder.api_base_url == "http://test-url"
        assert isinstance(text_embedder.placeholder_image, str)

    @patch("requests.post")
    def test_text_embedding(self, mock_post, text_embedder, mock_response):
        mock_post.return_value = mock_response

        result = text_embedder.run(text="test text")

        assert "embedding" in result
        assert "meta" in result
        assert result["embedding"] == MOCK_EMBEDDING
        assert result["meta"]["model"] == "test-model"

        # Verify API call
        mock_post.assert_called_once()
        call_args = mock_post.call_args
        assert "Bearer test-key" in call_args[1]["headers"]["Authorization"]
        assert "test text" in str(call_args[1]["json"])

    @patch("requests.post")
    def test_image_embedding(self, mock_post, text_embedder, mock_response, tmp_path):
        mock_post.return_value = mock_response

        # Create a temporary test image
        test_image = tmp_path / "test_image.png"
        test_image.write_bytes(b"fake image content")

        result = text_embedder.run(image_path=str(test_image))

        assert "embedding" in result
        assert "meta" in result
        assert result["embedding"] == MOCK_EMBEDDING

        # Verify API call
        mock_post.assert_called_once()
        call_args = mock_post.call_args
        assert "Bearer test-key" in call_args[1]["headers"]["Authorization"]
        assert "image_url" in str(call_args[1]["json"])

    def test_invalid_input(self, text_embedder):
        with pytest.raises(ValueError):
            text_embedder.run()


class TestMultimodalDocumentEmbedder:
    @patch("requests.post")
    def test_document_embedding_text(self, mock_post, document_embedder, mock_response):
        mock_post.return_value = mock_response

        # Create test documents
        docs = [
            Document(content="test document 1"),
            Document(content="test document 2"),
        ]

        result = document_embedder.run(documents=docs)

        assert "documents" in result
        assert len(result["documents"]) == 2
        for doc in result["documents"]:
            assert doc.embedding == MOCK_EMBEDDING
            assert doc.meta["embedding_model"] == "test-model"
            assert "embedding_usage" in doc.meta

    @patch("requests.post")
    def test_document_embedding_image(
        self, mock_post, document_embedder, mock_response, tmp_path
    ):
        mock_post.return_value = mock_response

        # Create a temporary test image
        test_image = tmp_path / "test_image.png"
        test_image.write_bytes(b"fake image content")

        # Create test documents with image paths
        docs = [
            Document(content="", meta={"image_path": str(test_image)}),
            Document(content="", meta={"image_path": str(test_image)}),
        ]

        result = document_embedder.run(documents=docs)

        assert "documents" in result
        assert len(result["documents"]) == 2
        for doc in result["documents"]:
            assert doc.embedding == MOCK_EMBEDDING
            assert doc.meta["embedding_model"] == "test-model"
            assert "embedding_usage" in doc.meta

    def test_empty_document_list(self, document_embedder):
        result = document_embedder.run(documents=[])
        assert "documents" in result
        assert len(result["documents"]) == 0
