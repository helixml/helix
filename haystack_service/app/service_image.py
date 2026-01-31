import logging
from typing import Any, Dict, List, Optional, Union

from haystack import Pipeline
from haystack.components.writers import DocumentWriter
from haystack.dataclasses import Document
from haystack.document_stores.types import DuplicatePolicy
from haystack.utils.auth import Secret

from .config import settings
from .converters import PDFToImagesConverter
from .embedders import MultimodalDocumentEmbedder, MultimodalTextEmbedder
from .unix_socket_embedders import (
    UnixSocketOpenAIDocumentEmbedder,
    UnixSocketOpenAITextEmbedder,
)
from .vectorchord.components import (
    VectorchordEmbeddingRetriever,
)
from .splitters import ImageSplitter
from .vectorchord.document_store import VectorchordDocumentStore

logger = logging.getLogger(__name__)


class HaystackImageService:
    """
    Provides image-based RAG (Retrieval Augmented Generation) service using Haystack.
    Handles document processing, indexing, and retrieval with support for vision models.
    """

    def __init__(self):
        """Initialize the Haystack service with vision model RAG capabilities"""
        logger.info("Initializing Vision Model RAG Service")
        self._init_document_store()
        self._init_indexing_pipeline()
        self._init_query_pipeline()
        logger.info("Vision Model RAG Service initialization complete")

    def _init_document_store(self):
        """Initialize the VectorchordDocumentStore with configured settings"""
        try:
            self.document_store = VectorchordDocumentStore(
                connection_string=Secret.from_token(settings.PGVECTOR_DSN),
                embedding_dimension=settings.VISION_EMBEDDING_DIM,
                table_name=settings.VISION_PGVECTOR_TABLE,
                vector_function="cosine_similarity",
                search_strategy="vchordrq",
                recreate_table=False,
            )
            logger.info(
                f"Connected to VectorchordDocumentStore: {settings.VISION_PGVECTOR_TABLE}"
            )
        except Exception as e:
            logger.error("Failed to connect to document store", exc_info=True)
            raise

    def _init_indexing_pipeline(self):
        """Initialize the document indexing pipeline with converter, splitter, embedder, and writer"""
        self.indexing_pipeline = Pipeline()

        components = {
            "converter": PDFToImagesConverter(),
            "splitter": ImageSplitter(),
            "embedder": self._get_embedder(for_documents=True),
            "vector_writer": DocumentWriter(
                document_store=self.document_store,
                policy=DuplicatePolicy.OVERWRITE,
            ),
        }

        # Add and connect components
        for name, component in components.items():
            self.indexing_pipeline.add_component(name, component)

        self.indexing_pipeline.connect("converter", "splitter")
        self.indexing_pipeline.connect("splitter", "embedder")
        self.indexing_pipeline.connect("embedder", "vector_writer")

        # Save converter for potential reuse
        self.converter = components["converter"]
        logger.info("Initialized vision indexing pipeline")

    def _init_query_pipeline(self):
        """Initialize the query pipeline with embedder and retriever"""
        self.query_pipeline = Pipeline()

        embedder = self._get_embedder(for_documents=False)
        self.vector_retriever = VectorchordEmbeddingRetriever(
            document_store=self.document_store, filters=None, top_k=5
        )

        self.query_pipeline.add_component("embedder", embedder)
        self.query_pipeline.add_component("vector_retriever", self.vector_retriever)
        self.query_pipeline.connect(
            "embedder.embedding", "vector_retriever.query_embedding"
        )

        logger.info("Initialized query pipeline")

    def _get_embedder(
        self, for_documents: bool = True
    ) -> Union[MultimodalDocumentEmbedder, MultimodalTextEmbedder]:
        """Get the appropriate embedder based on configuration"""
        return (
            MultimodalDocumentEmbedder if for_documents else MultimodalTextEmbedder
        )(
            api_key=settings.VISION_API_KEY,
            api_base_url=settings.VISION_BASE_URL,
            model=settings.VISION_EMBEDDINGS_MODEL,
            socket_path=settings.VISION_EMBEDDINGS_SOCKET,
        )

    def _format_filters(
        self, filters: Optional[Dict[str, Any]]
    ) -> Optional[Dict[str, Any]]:
        """Format filters into Haystack's expected structure"""
        if not filters:
            return None

        try:
            if not any(key in filters for key in ["operator", "conditions", "field"]):
                conditions = [
                    {"field": f"meta.{key}", "operator": "==", "value": value}
                    for key, value in filters.items()
                ]
                return {"operator": "AND", "conditions": conditions}
            return filters
        except Exception as e:
            logger.warning(
                f"Failed to format filters: {str(e)}. Continuing without filters."
            )
            return None

    async def process_and_index(
        self, file_path: str, metadata: Optional[Dict[str, Any]] = None
    ) -> Dict[str, Any]:
        """
        Process and index a document file.

        Args:
            file_path: Path to the file to process
            metadata: Optional metadata to attach to document chunks

        Returns:
            Dict containing indexing statistics
        """
        metadata = metadata or {}
        original_filename = metadata.get("filename")
        if not original_filename:
            raise ValueError("Original filename must be provided in metadata")

        logger.info(f"Processing and indexing vision document: {original_filename}")
        metadata["source"] = original_filename

        try:
            output = self.indexing_pipeline.run(
                {
                    "converter": {"paths": [file_path], "meta": metadata},
                }
            )

            return {
                "filename": original_filename,
                "indexed": True,
                "chunks": output.get("vector_writer", {}).get("documents_written", 0),
                "metadata": metadata,
            }
        except Exception:
            logger.error("Error processing document", exc_info=True)
            raise

    async def query(
        self, query_text: str, filters: Optional[Dict[str, Any]] = None, top_k: int = 5
    ) -> List[Dict[str, Any]]:
        """Query the document store for relevant passages"""
        # Sanitize query
        query_text = query_text.replace("\x00", "")
        if not query_text.strip():
            raise ValueError("Query text cannot be empty")

        logger.info(f"Querying with filters: {filters}, top_k: {top_k}")

        # Update retriever parameters
        formatted_filters = self._format_filters(filters)
        self.vector_retriever.top_k = top_k
        self.vector_retriever.filters = formatted_filters

        # Run query pipeline
        try:
            output = self.query_pipeline.run(
                {
                    "embedder": {"text": query_text},
                }
            )
            documents: List[Document] = output.get("vector_retriever", {}).get(
                "documents", []
            )

            # Clean and format results
            return [
                {
                    "id": doc.id,
                    "content": (doc.content or "").replace("\x00", ""),
                    "score": float(
                        doc.score
                        if hasattr(doc, "score") and doc.score is not None
                        else 0.0
                    ),
                    "metadata": doc.meta,
                    "rank": i + 1,
                }
                for i, doc in enumerate(documents)
            ]
        except Exception as e:
            logger.error(f"Error running query pipeline: {str(e)}")
            logger.exception("Query pipeline error details:")
            raise ValueError(f"Error querying document store: {str(e)}")

    async def delete(self, filters: Dict[str, Any]) -> Dict[str, Any]:
        """
        Delete documents matching the given filters.

        Args:
            filters: Query filters to identify documents for deletion

        Returns:
            Dict containing deletion status and count
        """
        logger.info(f"Deleting documents with filters: {filters}")

        try:
            matching_docs = self.document_store.filter_documents(filters=filters)
            if not matching_docs:
                return {"status": "success", "documents_deleted": 0}

            doc_ids = [doc.id for doc in matching_docs]
            self.document_store.delete_documents(doc_ids)

            return {"status": "success", "documents_deleted": len(doc_ids)}
        except Exception as e:
            logger.error("Error deleting documents", exc_info=True)
            raise
