import logging
from typing import Any, Dict, List

from haystack import Pipeline
from haystack.components.embedders import OpenAIDocumentEmbedder, OpenAITextEmbedder
from haystack.components.joiners import DocumentJoiner
from haystack.components.preprocessors import DocumentCleaner, DocumentSplitter
from haystack.components.writers import DocumentWriter
from haystack.dataclasses import Document
from haystack.document_stores.types import DuplicatePolicy
from haystack.utils.auth import Secret
from .config import settings
from .converters import PDFToImagesConverter
from .unix_socket_embedders import (
    UnixSocketOpenAIDocumentEmbedder,
    UnixSocketOpenAITextEmbedder,
)
from .vectorchord.components import (
    VectorchordBM25Retriever,
    VectorchordEmbeddingRetriever,
)
from .vectorchord.document_store import VectorchordDocumentStore
from .vectorchord.components.image_splitter import ImageSplitter
from .embedders import MultimodalTextEmbedder, MultimodalDocumentEmbedder

logger = logging.getLogger(__name__)


class HaystackImageService:
    """Provides image based RAG service"""

    def __init__(self):
        """Initialize the Haystack service"""
        logger.info("Initializing Vision Model RAG Service")

        # Initialize document stores
        try:
            # VectorChord store for both dense embeddings and BM25
            pgvector_secret = Secret.from_token(settings.PGVECTOR_DSN)
            self.document_store = VectorchordDocumentStore(
                connection_string=pgvector_secret,  # Pass Secret object directly
                embedding_dimension=settings.VISION_EMBEDDING_DIM,
                table_name=settings.VISION_PGVECTOR_TABLE,
                vector_function="cosine_similarity",
                search_strategy="vchordrq",  # Enable for faster vector search
                recreate_table=False,
            )
            logger.info(
                f"Connected to VectorchordDocumentStore: {settings.VISION_PGVECTOR_TABLE}"
            )

        except Exception as e:
            logger.error(f"Failed to connect to document stores: {str(e)}")
            raise

        # Initialize pipelines
        self._init_indexing_pipeline()
        self._init_query_pipeline()

        logger.info("Vision Model RAG Service initialization complete")

    def _init_indexing_pipeline(self):
        """Initialize the document indexing pipeline"""
        self.indexing_pipeline = Pipeline()

        # Create components for indexing pipeline
        if settings.EMBEDDINGS_SOCKET:
            logger.info(
                f"Using UNIX socket for document embeddings: {settings.EMBEDDINGS_SOCKET}"
            )
            embedder = UnixSocketOpenAIDocumentEmbedder(
                socket_path=settings.EMBEDDINGS_SOCKET,
                model=settings.VISION_EMBEDDINGS_MODEL,
                batch_size=1,
            )
        else:
            logger.info(f"Using API for document embeddings: {settings.VLLM_BASE_URL}")
            embedder = MultimodalDocumentEmbedder(
                api_key=Secret.from_token(settings.VLLM_API_KEY),
                api_base_url=settings.VLLM_BASE_URL,
                model=settings.VISION_EMBEDDINGS_MODEL,
            )

        logger.info(f"Using embedder: {embedder.api_base_url}")

        converter = PDFToImagesConverter()

        splitter = ImageSplitter()

        # Writer for the vector store (which now handles both embeddings and BM25)
        # NUL bytes are filtered out in VectorchordDocumentStore.write_documents method
        vector_writer = DocumentWriter(
            document_store=self.document_store,
            policy=DuplicatePolicy.OVERWRITE,  # Use overwrite policy to handle duplicate documents
        )

        # Add components
        self.indexing_pipeline.add_component("converter", converter)
        self.indexing_pipeline.add_component("splitter", splitter)
        self.indexing_pipeline.add_component("embedder", embedder)
        self.indexing_pipeline.add_component("vector_writer", vector_writer)

        # Connect components
        self.indexing_pipeline.connect("converter", "splitter")
        self.indexing_pipeline.connect("splitter", "embedder")
        self.indexing_pipeline.connect("embedder", "vector_writer")

        # Save converter instance for text extraction
        self.converter = converter

        logger.info("Initialized vision indexing pipeline")

    def _init_query_pipeline(self):
        """Initialize the query pipeline"""
        self.query_pipeline = Pipeline()

        # Create components for query pipeline
        if settings.EMBEDDINGS_SOCKET:
            logger.info(
                f"Using UNIX socket for text embeddings: {settings.EMBEDDINGS_SOCKET}"
            )
            embedder = UnixSocketOpenAITextEmbedder(
                socket_path=settings.EMBEDDINGS_SOCKET,
                model=settings.VISION_EMBEDDINGS_MODEL,
            )
        else:
            logger.info(f"Using API for document embeddings: {settings.VLLM_BASE_URL}")
            embedder = MultimodalTextEmbedder(
                api_key=Secret.from_token(settings.VLLM_API_KEY),
                api_base_url=settings.VLLM_BASE_URL,
                model=settings.VISION_EMBEDDINGS_MODEL,
            )

        # Dense vector retriever using VectorChord
        vector_retriever = VectorchordEmbeddingRetriever(
            document_store=self.document_store, filters=None, top_k=5
        )

        # # BM25 retriever using VectorChord-bm25
        # bm25_retriever = VectorchordBM25Retriever(
        #     document_store=self.document_store, filters=None, top_k=5
        # )

        # # Document joiner to combine results from both retrievers
        # document_joiner = DocumentJoiner(join_mode="reciprocal_rank_fusion", top_k=5)

        # Add components
        self.query_pipeline.add_component("embedder", embedder)
        self.query_pipeline.add_component("vector_retriever", vector_retriever)
        # self.query_pipeline.add_component("bm25_retriever", bm25_retriever)
        # self.query_pipeline.add_component("document_joiner", document_joiner)

        # Connect components
        # Vector retrieval - need to be explicit about field connections
        self.query_pipeline.connect(
            "embedder.embedding", "vector_retriever.query_embedding"
        )

        # Join documents from both retrievers - these are already correct
        # self.query_pipeline.connect("vector_retriever", "document_joiner")
        # self.query_pipeline.connect("bm25_retriever", "document_joiner")

        # Save retrievers for parameter updates
        self.vector_retriever = vector_retriever
        # self.bm25_retriever = bm25_retriever
        # self.document_joiner = document_joiner
        # self.document_joiner = vector_retriever  # temporary to test

        logger.info("Initialized query pipeline")

    async def process_and_index(
        self, file_path: str, metadata: Dict[str, Any] = None
    ) -> Dict[str, Any]:
        """
        Process a document file and index it

        Args:
            file_path: Path to the file to process
            metadata: Optional metadata to attach to the document chunks

        Returns:
            Dict containing processing stats
        """
        if metadata is None:
            metadata = {}

        # Get the original filename from metadata
        original_filename = metadata.get("filename")
        if not original_filename:
            raise ValueError(
                "Original filename must be provided in metadata for process_and_index"
            )

        logger.info(
            f"Processing and indexing for vision {original_filename} with metadata: {metadata}"
        )

        # Use the original filename as the source, never the temp path
        metadata["source"] = original_filename

        # Set up the parameters for the indexing pipeline
        params = {
            "converter": {"paths": [file_path], "meta": metadata},
        }

        try:
            output = self.indexing_pipeline.run(params)

            # Get the number of chunks created by looking at vector_writer output
            num_chunks = output.get("vector_writer", {}).get("documents_written", 0)

            # Return stats
            return {
                "filename": original_filename,
                "indexed": True,
                "chunks": num_chunks,
                "metadata": metadata,
            }

        except Exception as e:
            logger.error(f"Error processing document: {str(e)}")
            logger.exception("Processing pipeline error details:")
            raise

    async def query(
        self, query_text: str, filters: Dict[str, Any] = None, top_k: int = 5
    ) -> List[Dict[str, Any]]:
        """
        Query the document store for relevant passages

        Args:
            query_text: The query text to search for
            filters: Optional filters to apply to the search
            top_k: Number of results to return

        Returns:
            List of dictionaries with document data
        """
        # Remove NUL bytes from query if present
        if "\x00" in query_text:
            logger.warning("Query contained NUL bytes that will be removed")
            query_text = query_text.replace("\x00", "")

        # Validate query text after sanitizing
        if not query_text or query_text.strip() == "":
            logger.error("Empty query text received or contained only NUL bytes")
            raise ValueError("Query text cannot be empty")

        logger.info(
            f"Querying with: '{query_text}', filters: {filters}, top_k: {top_k}"
        )

        # Format filters correctly if they're provided
        formatted_filters = None
        if filters:
            try:
                # Simple filters might just be key-value pairs like {"data_entity_id": "some_id"}
                # We need to convert them to the format expected by Haystack with operator and conditions
                if not any(
                    key in filters for key in ["operator", "conditions", "field"]
                ):
                    # Simple key-value filters need to be converted to proper format
                    conditions = []
                    for key, value in filters.items():
                        conditions.append(
                            {"field": f"meta.{key}", "operator": "==", "value": value}
                        )

                    formatted_filters = {"operator": "AND", "conditions": conditions}
                else:
                    # Filters are already in the correct format
                    formatted_filters = filters

                logger.info(f"Using formatted filters: {formatted_filters}")
            except Exception as e:
                logger.warning(
                    f"Failed to format filters: {str(e)}. Continuing without filters."
                )

        # Update retriever parameters
        self.vector_retriever.top_k = top_k
        self.vector_retriever.filters = formatted_filters

        # self.bm25_retriever.top_k = top_k
        # self.bm25_retriever.filters = formatted_filters
        # # At this point there are 2*top_k results from the retrievers

        # logger.info(
        #     f"Chopping joined results in half to meet the request for top_k: {top_k}"
        # )
        # self.document_joiner.top_k = top_k

        # Set up the parameters for the query pipeline
        params = {
            "embedder": {"text": query_text},
            # "bm25_retriever": {"query": query_text},
        }

        # Run the query pipeline
        logger.info("Running full query pipeline with document joining")
        try:
            # Run the pipeline
            output = self.query_pipeline.run(params)

            # Get the results from the document joiner
            # documents = output.get("document_joiner", {}).get("documents", [])
            documents: List[Document] = output.get("vector_retriever", {}).get(
                "documents", []
            )
            logger.info(f"Document joiner returned {len(documents)} documents")

            # Filter out NUL bytes from document content
            for doc in documents:
                if doc.content and "\x00" in doc.content:
                    logger.warning(
                        f"Filtering NUL bytes from retrieval result document: {doc.id}"
                    )
                    doc.content = doc.content.replace("\x00", "")

            # Debug the joined results
            for i, doc in enumerate(documents):
                logger.info(
                    f"DEBUG: Final joined result {i + 1}: id={getattr(doc, 'id', 'unknown')}, "
                    f"score={getattr(doc, 'score', 'unknown')}"
                )
        except Exception as e:
            logger.error(f"Error running query pipeline: {str(e)}")
            logger.exception("Query pipeline error details:")
            raise ValueError(f"Error querying document store: {str(e)}")

        # Convert to dictionaries
        results = []
        for i, doc in enumerate(documents):
            results.append(
                {
                    "id": doc.id,
                    "content": doc.content if doc.content else "",
                    "score": float(doc.score)
                    if hasattr(doc, "score") and doc.score is not None
                    else 0.0,
                    "metadata": doc.meta,
                    "rank": i + 1,
                }
            )

        return results

    async def delete(self, filters: Dict[str, Any]) -> Dict[str, Any]:
        """
        Delete documents from the store that match the given filters

        Args:
            filters: Filters to identify documents to delete

        Returns:
            Dict with deletion status
        """
        logger.info(f"Deleting documents with filters: {filters}")

        try:
            # Get documents that match filters
            matching_docs = self.document_store.filter_documents(filters=filters)

            if not matching_docs:
                return {"status": "success", "documents_deleted": 0}

            # Get IDs of matching documents
            doc_ids = [doc.id for doc in matching_docs]

            # Delete from the document store
            self.document_store.delete_documents(doc_ids)

            return {"status": "success", "documents_deleted": len(doc_ids)}

        except Exception as e:
            logger.error(f"Error deleting documents: {str(e)}")
            raise
