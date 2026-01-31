import os
from unittest.mock import MagicMock, patch

import pytest
from app.service_image import HaystackImageService
from app.vectorchord.components import VectorchordEmbeddingRetriever
from app.vectorchord.document_store import VectorchordDocumentStore
from haystack.dataclasses import Document

# Path to test sample file (you'll need to create this)
SAMPLE_IMAGE_PATH = os.path.join(
    os.path.dirname(__file__), "..", "test_files", "sample.jpg"
)


class TestHaystackImageService:
    """Test suite for the HaystackImageService class"""

    @pytest.fixture
    def mock_document_store(self):
        """Create a mock document store"""
        mock = MagicMock(spec=VectorchordDocumentStore)
        mock.write_documents.return_value = ["doc1", "doc2"]
        mock.filter_documents.return_value = [
            Document(id="doc1", content="test content 1", meta={"source": "test1.jpg"}),
            Document(id="doc2", content="test content 2", meta={"source": "test2.jpg"}),
        ]
        return mock

    @pytest.fixture
    def mock_converter(self):
        """Create a mock PDF to images converter"""
        mock = MagicMock()
        mock.run.return_value = {
            "documents": [
                Document(
                    id="doc1", content="test content 1", meta={"source": "test.jpg"}
                ),
                Document(
                    id="doc2", content="test content 2", meta={"source": "test.jpg"}
                ),
            ]
        }
        return mock

    @pytest.fixture
    def mock_vector_retriever(self, mock_document_store):
        """Create a mock vector retriever"""
        mock = MagicMock(spec=VectorchordEmbeddingRetriever)
        mock.document_store = mock_document_store
        return mock

    @pytest.fixture
    def service_with_mocks(
        self, mock_document_store, mock_converter, mock_vector_retriever
    ):
        """Create a service instance with mocked components"""
        with patch(
            "app.service_image.VectorchordDocumentStore",
            return_value=mock_document_store,
        ):
            with patch(
                "app.service_image.PDFToImagesConverter", return_value=mock_converter
            ):
                with patch("app.service_image.Pipeline"):
                    with patch(
                        "app.service_image.VectorchordEmbeddingRetriever",
                        return_value=mock_vector_retriever,
                    ):
                        # Skip actual initialization to avoid component errors
                        with patch.object(
                            HaystackImageService, "_init_indexing_pipeline"
                        ):
                            with patch.object(
                                HaystackImageService, "_init_query_pipeline"
                            ):
                                service = HaystackImageService()
                                # Manually set required properties
                                service.document_store = mock_document_store
                                service.converter = mock_converter
                                service.indexing_pipeline = MagicMock()
                                service.query_pipeline = MagicMock()
                                service.vector_retriever = mock_vector_retriever
                                return service

    def test_init(self, service_with_mocks):
        """Test that the service initializes properly"""
        assert service_with_mocks is not None
        assert service_with_mocks.document_store is not None
        assert service_with_mocks.indexing_pipeline is not None
        assert service_with_mocks.query_pipeline is not None

    @pytest.mark.asyncio
    async def test_process_and_index(self, service_with_mocks):
        """Test processing and indexing a document"""
        service_with_mocks.indexing_pipeline.run.return_value = {
            "vector_writer": {"documents_written": 2}
        }

        metadata = {"filename": "test.jpg", "custom_field": "custom_value"}
        result = await service_with_mocks.process_and_index("dummy_path.jpg", metadata)

        # Verify pipeline was called with correct parameters
        service_with_mocks.indexing_pipeline.run.assert_called_once()
        call_args = service_with_mocks.indexing_pipeline.run.call_args[0][0]
        assert "converter" in call_args
        assert call_args["converter"]["paths"] == ["dummy_path.jpg"]
        assert call_args["converter"]["meta"]["filename"] == "test.jpg"
        assert call_args["converter"]["meta"]["custom_field"] == "custom_value"
        assert call_args["converter"]["meta"]["source"] == "test.jpg"

        # Verify result
        assert result["filename"] == "test.jpg"
        assert result["indexed"] is True
        assert result["chunks"] == 2
        assert result["metadata"]["filename"] == "test.jpg"
        assert result["metadata"]["custom_field"] == "custom_value"

    @pytest.mark.asyncio
    async def test_process_and_index_missing_filename(self, service_with_mocks):
        """Test that process_and_index raises an error when filename is missing"""
        metadata = {"custom_field": "custom_value"}  # Missing filename

        with pytest.raises(ValueError, match="Original filename must be provided"):
            await service_with_mocks.process_and_index("dummy_path.jpg", metadata)

    @pytest.mark.asyncio
    async def test_query(self, service_with_mocks):
        """Test querying the document store"""
        # Mock the query pipeline output
        docs = [
            Document(
                id="doc1", content="content1", meta={"source": "test1.jpg"}, score=0.95
            ),
            Document(
                id="doc2", content="content2", meta={"source": "test2.jpg"}, score=0.85
            ),
        ]
        service_with_mocks.query_pipeline.run.return_value = {
            "vector_retriever": {"documents": docs}
        }

        # Perform the query
        results = await service_with_mocks.query(
            "test query", filters={"key": "value"}, top_k=2
        )

        # Check that retriever was updated with correct parameters
        assert service_with_mocks.vector_retriever.top_k == 2

        # Verify results format
        assert len(results) == 2
        assert results[0]["id"] == "doc1"
        assert results[0]["content"] == "content1"
        assert results[0]["score"] == 0.95
        assert results[0]["rank"] == 1
        assert results[1]["id"] == "doc2"
        assert results[1]["content"] == "content2"
        assert results[1]["score"] == 0.85
        assert results[1]["rank"] == 2

    @pytest.mark.asyncio
    async def test_query_with_empty_text(self, service_with_mocks):
        """Test that query raises an error with empty text"""
        with pytest.raises(ValueError, match="Query text cannot be empty"):
            await service_with_mocks.query("")

        with pytest.raises(ValueError, match="Query text cannot be empty"):
            await service_with_mocks.query("   ")

        with pytest.raises(ValueError, match="Query text cannot be empty"):
            await service_with_mocks.query("\x00")  # NUL byte

    @pytest.mark.asyncio
    async def test_query_sanitize_null_bytes(self, service_with_mocks):
        """Test that NUL bytes are removed from query text"""
        service_with_mocks.query_pipeline.run.return_value = {
            "vector_retriever": {"documents": []}
        }

        # Query with NUL bytes
        await service_with_mocks.query("test\x00query")

        # Get the actual query text passed to the pipeline
        calls = service_with_mocks.query_pipeline.run.call_args_list
        for call in calls:
            args = call[0][0]
            if "embedder" in args and "text" in args["embedder"]:
                # Check that NUL bytes were removed
                assert args["embedder"]["text"] == "testquery"
                break

    @pytest.mark.asyncio
    async def test_delete(self, service_with_mocks):
        """Test deleting documents from the store"""
        result = await service_with_mocks.delete({"source": "test.jpg"})

        # Verify document store methods were called
        service_with_mocks.document_store.filter_documents.assert_called_once_with(
            filters={"source": "test.jpg"}
        )
        service_with_mocks.document_store.delete_documents.assert_called_once_with(
            ["doc1", "doc2"]
        )

        # Verify result
        assert result["status"] == "success"
        assert result["documents_deleted"] == 2

    @pytest.mark.asyncio
    async def test_delete_no_matches(self, service_with_mocks):
        """Test deleting when no documents match the filter"""
        service_with_mocks.document_store.filter_documents.return_value = []

        result = await service_with_mocks.delete({"source": "nonexistent.jpg"})

        # Verify delete_documents was not called
        service_with_mocks.document_store.delete_documents.assert_not_called()

        # Verify result
        assert result["status"] == "success"
        assert result["documents_deleted"] == 0
