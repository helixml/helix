import logging
import os
from typing import Any, Dict, List

from haystack import Pipeline
from haystack.components.embedders import OpenAIDocumentEmbedder, OpenAITextEmbedder
from haystack.components.joiners import DocumentJoiner
from haystack.components.preprocessors import DocumentCleaner, DocumentSplitter
from haystack.components.writers import DocumentWriter
from haystack.document_stores.types import DuplicatePolicy
from haystack.utils.auth import Secret

from .config import settings
from .converters import LocalUnstructuredConverter
from .unix_socket_embedders import (
    UnixSocketOpenAIDocumentEmbedder,
    UnixSocketOpenAITextEmbedder,
)
from .vectorchord.components import (
    VectorchordBM25Retriever,
    VectorchordEmbeddingRetriever,
)
from .vectorchord.document_store import VectorchordDocumentStore

logger = logging.getLogger(__name__)


class HaystackService:
    """Main service class for Haystack RAG operations"""

    def __init__(self):
        """Initialize the Haystack service"""
        logger.info("Initializing HaystackService")

        # Initialize document stores
        try:
            # VectorChord store for both dense embeddings and BM25
            pgvector_secret = Secret.from_token(settings.PGVECTOR_DSN)
            self.document_store = VectorchordDocumentStore(
                connection_string=pgvector_secret,  # Pass Secret object directly
                embedding_dimension=settings.EMBEDDING_DIM,
                table_name=settings.PGVECTOR_TABLE,
                vector_function="cosine_similarity",
                search_strategy="vchordrq",  # Enable for faster vector search
                recreate_table=False,
            )
            logger.info(
                f"Connected to VectorchordDocumentStore: {settings.PGVECTOR_TABLE}"
            )

            # We'll use VectorChord for BM25 as well, so no need for a separate in-memory store
            # Keep a reference for compatibility with existing code
            self.bm25_document_store = self.document_store

        except Exception as e:
            logger.error(f"Failed to connect to document stores: {str(e)}")
            raise

        # Initialize pipelines
        self._init_indexing_pipeline()
        self._init_query_pipeline()

        logger.info("HaystackService initialization complete")

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
                model=settings.EMBEDDINGS_MODEL,
            )
        elif settings.EMBEDDINGS_API_BASE_URL:
            logger.info(f"Using API for document embeddings: {settings.EMBEDDINGS_API_BASE_URL}")
            embedder = OpenAIDocumentEmbedder(
                api_key=Secret.from_token(settings.EMBEDDINGS_API_KEY),
                api_base_url=settings.EMBEDDINGS_API_BASE_URL,
                model=settings.EMBEDDINGS_MODEL,
            )
        else:
            raise ValueError(
                "No embeddings backend configured. Set HELIX_EMBEDDINGS_SOCKET or RAG_HAYSTACK_EMBEDDINGS_API_BASE_URL."
            )

        converter = LocalUnstructuredConverter()

        cleaner = DocumentCleaner(
            remove_empty_lines=False,
            remove_extra_whitespaces=False,
            remove_regex=r"\.{5,}",  # Remove runs of 5 or more dots
        )

        splitter = DocumentSplitter(
            split_length=settings.CHUNK_SIZE,
            split_overlap=settings.CHUNK_OVERLAP,
            split_by=settings.CHUNK_UNIT,
            respect_sentence_boundary=True,
        )
        splitter.warm_up()

        # Writer for the vector store (which now handles both embeddings and BM25)
        # NUL bytes are filtered out in VectorchordDocumentStore.write_documents method
        vector_writer = DocumentWriter(
            document_store=self.document_store,
            policy=DuplicatePolicy.OVERWRITE,  # Use overwrite policy to handle duplicate documents
        )

        # Add components
        self.indexing_pipeline.add_component("converter", converter)
        self.indexing_pipeline.add_component("cleaner", cleaner)
        self.indexing_pipeline.add_component("splitter", splitter)
        self.indexing_pipeline.add_component("embedder", embedder)
        self.indexing_pipeline.add_component("vector_writer", vector_writer)

        # Connect components
        self.indexing_pipeline.connect("converter", "cleaner")
        self.indexing_pipeline.connect("cleaner", "splitter")
        self.indexing_pipeline.connect("splitter", "embedder")
        self.indexing_pipeline.connect("embedder", "vector_writer")

        # Save converter instance for text extraction
        self.converter = converter

        logger.info(
            f"Initialized indexing pipeline with chunk_size={settings.CHUNK_SIZE}, overlap={settings.CHUNK_OVERLAP}"
        )

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
                model=settings.EMBEDDINGS_MODEL,
            )
        elif settings.EMBEDDINGS_API_BASE_URL:
            logger.info(f"Using API for text embeddings: {settings.EMBEDDINGS_API_BASE_URL}")
            embedder = OpenAITextEmbedder(
                api_key=Secret.from_token(settings.EMBEDDINGS_API_KEY),
                api_base_url=settings.EMBEDDINGS_API_BASE_URL,
                model=settings.EMBEDDINGS_MODEL,
            )
        else:
            raise ValueError(
                "No embeddings backend configured. Set HELIX_EMBEDDINGS_SOCKET or RAG_HAYSTACK_EMBEDDINGS_API_BASE_URL."
            )

        # Dense vector retriever using VectorChord
        vector_retriever = VectorchordEmbeddingRetriever(
            document_store=self.document_store, filters=None, top_k=5
        )

        # BM25 retriever using VectorChord-bm25
        bm25_retriever = VectorchordBM25Retriever(
            document_store=self.document_store, filters=None, top_k=5
        )

        # Document joiner to combine results from both retrievers
        document_joiner = DocumentJoiner(join_mode="reciprocal_rank_fusion", top_k=5)

        # Add components
        self.query_pipeline.add_component("embedder", embedder)
        self.query_pipeline.add_component("vector_retriever", vector_retriever)
        self.query_pipeline.add_component("bm25_retriever", bm25_retriever)
        self.query_pipeline.add_component("document_joiner", document_joiner)

        # Connect components
        # Vector retrieval - need to be explicit about field connections
        self.query_pipeline.connect(
            "embedder.embedding", "vector_retriever.query_embedding"
        )

        # Join documents from both retrievers - these are already correct
        self.query_pipeline.connect("vector_retriever", "document_joiner")
        self.query_pipeline.connect("bm25_retriever", "document_joiner")

        # Save retrievers for parameter updates
        self.vector_retriever = vector_retriever
        self.bm25_retriever = bm25_retriever
        self.document_joiner = document_joiner

        logger.info("Initialized query pipeline")

    async def extract_text(self, file_path: str) -> str:
        """
        Extract text from a file using the converter

        Args:
            file_path: Path to the file to extract text from

        Returns:
            Extracted text content
        """
        logger.info(f"Extracting text from {os.path.basename(file_path)}")

        # Use the converter to extract text
        try:
            documents = self.converter.run(paths=[file_path])
        except Exception as e:
            logger.error(f"Error extracting text: {str(e)}")
            raise

        # Return the concatenated text from all documents
        return "\n\n".join([doc.content for doc in documents.get("documents", [])])

    def _analyze_scores(self, vector_docs, bm25_docs):
        """
        Analyze the score distributions from both retrievers to identify potential normalization issues.

        Args:
            vector_docs: List of documents from the vector retriever
            bm25_docs: List of documents from the BM25 retriever

        Returns:
            Dictionary with score analysis information
        """
        # Initialize score collections - filter out None values
        vector_scores = [getattr(doc, "score", 0.0) for doc in vector_docs]
        vector_scores = [score for score in vector_scores if score is not None]

        bm25_scores = [getattr(doc, "score", 0.0) for doc in bm25_docs]
        bm25_scores = [score for score in bm25_scores if score is not None]

        # Skip empty score lists
        if not vector_scores and not bm25_scores:
            logger.warning("No valid scores available for analysis")
            return {}

        # Compute statistics
        stats = {
            "vector": {
                "count": len(vector_scores),
                "min": min(vector_scores) if vector_scores else None,
                "max": max(vector_scores) if vector_scores else None,
                "mean": sum(vector_scores) / len(vector_scores)
                if vector_scores
                else None,
                "range": max(vector_scores) - min(vector_scores)
                if vector_scores
                else None,
            },
            "bm25": {
                "count": len(bm25_scores),
                "min": min(bm25_scores) if bm25_scores else None,
                "max": max(bm25_scores) if bm25_scores else None,
                "mean": sum(bm25_scores) / len(bm25_scores) if bm25_scores else None,
                "range": max(bm25_scores) - min(bm25_scores) if bm25_scores else None,
            },
        }

        # Log findings
        logger.info(
            f"Score analysis - Vector scores: count={stats['vector']['count']}, "
            f"min={stats['vector']['min']}, max={stats['vector']['max']}, "
            f"mean={stats['vector']['mean']}, range={stats['vector']['range']}"
        )
        logger.info(
            f"Score analysis - BM25 scores: count={stats['bm25']['count']}, "
            f"min={stats['bm25']['min']}, max={stats['bm25']['max']}, "
            f"mean={stats['bm25']['mean']}, range={stats['bm25']['range']}"
        )

        # Analyze potential issues
        if vector_scores and bm25_scores:
            vector_range = stats["vector"]["range"]
            bm25_range = stats["bm25"]["range"]

            # Check if one retriever has much larger score range than the other
            if vector_range and bm25_range and vector_range > 5 * bm25_range:
                logger.warning(
                    "Vector score range is much larger than BM25 score range - "
                    "this could lead to vector results dominating the ranking"
                )
            elif bm25_range and vector_range and bm25_range > 5 * vector_range:
                logger.warning(
                    "BM25 score range is much larger than vector score range - "
                    "this could lead to BM25 results dominating the ranking"
                )

            # Check if the mean scores are very different
            if stats["vector"]["mean"] and stats["bm25"]["mean"]:
                if stats["vector"]["mean"] > 5 * stats["bm25"]["mean"]:
                    logger.warning(
                        "Vector mean score is much higher than BM25 mean score - "
                        "this could lead to vector results dominating the ranking"
                    )
                elif stats["bm25"]["mean"] > 5 * stats["vector"]["mean"]:
                    logger.warning(
                        "BM25 mean score is much higher than vector mean score - "
                        "this could lead to BM25 results dominating the ranking"
                    )

        return stats

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
            f"Processing and indexing {original_filename} with metadata: {metadata}"
        )

        # Preserve the source from the caller (e.g. a full URL for web pages).
        # Only fall back to the filename when no source was provided.
        if not metadata.get("source"):
            metadata["source"] = original_filename

        # Set up the parameters for the indexing pipeline
        params = {
            "converter": {"paths": [file_path], "meta": metadata},
        }

        try:
            output = self.indexing_pipeline.run(params)

            # Get the number of chunks created by looking at vector_writer output
            num_chunks = len(
                output.get("vector_writer", {}).get("written_documents", [])
            )

            # Return stats
            return {
                "filename": original_filename,
                "indexed": True,
                "chunks": num_chunks,
                "metadata": metadata,
            }

        except Exception as e:
            logger.error(f"Error processing document: {str(e)}")
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

        self.bm25_retriever.top_k = top_k
        self.bm25_retriever.filters = formatted_filters
        # At this point there are 2*top_k results from the retrievers

        logger.info(
            f"Chopping joined results in half to meet the request for top_k: {top_k}"
        )
        self.document_joiner.top_k = top_k

        # Set up the parameters for the query pipeline
        params = {
            "embedder": {"text": query_text},
            "bm25_retriever": {"query": query_text},
        }

        try:
            # Debug individual retriever operation first
            # Test BM25 retriever directly to see results before joining
            logger.info("DEBUG: Testing BM25 retriever directly")
            try:
                bm25_results = self.bm25_retriever.run(
                    query=query_text, filters=formatted_filters, top_k=top_k
                )
                logger.info(
                    f"DEBUG: BM25 retriever returned {len(bm25_results.get('documents', []))} documents"
                )
                for i, doc in enumerate(bm25_results.get("documents", [])):
                    logger.info(
                        f"DEBUG: BM25 result {i + 1}: id={doc.id}, score={doc.score}"
                    )
            except Exception as e:
                logger.error(f"DEBUG: BM25 retriever error: {str(e)}")

            # Test the vector retriever directly
            logger.info("DEBUG: Testing vector retriever directly")
            try:
                # Get the query embedding
                embeddings = self.query_pipeline.get_component("embedder").run(
                    text=query_text
                )
                query_embedding = embeddings["embedding"]

                # Call the retriever directly - use the component interface
                vector_results = self.vector_retriever.run(
                    query_embedding=query_embedding,
                    filters=formatted_filters,
                    top_k=top_k,
                )

                # Extract documents from the result dictionary
                vector_docs = vector_results.get("documents", [])
                logger.info(
                    f"DEBUG: Vector retriever returned {len(vector_docs)} documents"
                )
                for vector_doc in vector_docs:
                    logger.info(
                        f"DEBUG: Vector result: id={getattr(vector_doc, 'id', 'unknown')}, "
                        f"score={getattr(vector_doc, 'score', 'unknown')}"
                    )
            except Exception as e:
                logger.error(f"DEBUG: Vector retriever error: {str(e)}")
                # Initialize with empty results so subsequent code doesn't fail
                vector_results = {"documents": []}

            # Special debug: analyze score distributions
            logger.info("DEBUG: Analyzing score distributions from both retrievers")
            try:
                bm25_docs = bm25_results.get("documents", [])
                vector_docs = vector_results.get("documents", [])

                # Analyze the scores
                score_analysis = self._analyze_scores(vector_docs, bm25_docs)

                # Check for potential modifications to improve ranking
                if score_analysis:
                    logger.info(
                        "DEBUG: Consider these potential improvements based on score analysis:"
                    )

                    # Check if score normalization might help
                    if score_analysis.get("vector", {}).get(
                        "range"
                    ) and score_analysis.get("bm25", {}).get("range"):
                        logger.info(
                            "DEBUG: One potential fix is to normalize scores before joining:"
                        )
                        logger.info(
                            "DEBUG: For vector scores: score_norm = (score - min_score) / score_range"
                        )
                        logger.info(
                            "DEBUG: For BM25 scores: score_norm = (score - min_score) / score_range"
                        )
                        logger.info(
                            "DEBUG: This would make both retrievers contribute more equally to ranking"
                        )
            except Exception as e:
                logger.error(f"DEBUG: Score analysis failed: {str(e)}")
                logger.exception("Score analysis error details:")

            # Special debug: manually simulate the reciprocal rank fusion to understand how the DocumentJoiner works
            logger.info(
                "DEBUG: Manually simulating reciprocal rank fusion to understand document joiner behavior"
            )
            try:
                # Get documents from both retrievers
                bm25_docs = bm25_results.get("documents", [])
                vector_docs = vector_results.get("documents", [])

                # Combine all unique document IDs
                all_doc_ids = set(doc.id for doc in bm25_docs if hasattr(doc, "id"))
                all_doc_ids.update(doc.id for doc in vector_docs if hasattr(doc, "id"))
                logger.info(
                    f"DEBUG: Found {len(all_doc_ids)} unique document IDs across both retrievers"
                )

                # Track rankings in each result list
                bm25_ranks = {
                    doc.id: i + 1
                    for i, doc in enumerate(bm25_docs)
                    if hasattr(doc, "id")
                }
                vector_ranks = {
                    doc.id: i + 1
                    for i, doc in enumerate(vector_docs)
                    if hasattr(doc, "id")
                }

                # Default constant k for RRF formula
                k = 60  # Standard value used in RRF

                # Calculate RRF scores
                rrf_scores = {}
                for doc_id in all_doc_ids:
                    # For documents not in a result set, rank is considered "infinity"
                    # which effectively means their contribution from that source is 0
                    bm25_rank = bm25_ranks.get(doc_id, float("inf"))
                    vector_rank = vector_ranks.get(doc_id, float("inf"))

                    # RRF formula: score = sum of 1/(k + rank) across all sources
                    rrf_score = 0
                    if bm25_rank != float("inf"):
                        rrf_score += 1 / (k + bm25_rank)
                    if vector_rank != float("inf"):
                        rrf_score += 1 / (k + vector_rank)

                    rrf_scores[doc_id] = rrf_score

                # Sort documents by RRF score (higher is better)
                sorted_doc_ids = sorted(
                    rrf_scores.keys(),
                    key=lambda doc_id: rrf_scores[doc_id],
                    reverse=True,
                )

                # Log the top results and their source ranks
                logger.info("DEBUG: Top documents after RRF fusion:")
                for i, doc_id in enumerate(
                    sorted_doc_ids[: min(10, len(sorted_doc_ids))], 1
                ):
                    buzzwang = any(
                        doc.content and "SmartBuzz" in doc.content
                        for doc in bm25_docs + vector_docs
                        if doc.id == doc_id
                    )
                    logger.info(
                        f"DEBUG: Rank {i}: doc_id={doc_id}, "
                        f"RRF score={rrf_scores[doc_id]:.6f}, "
                        f"BM25 rank={bm25_ranks.get(doc_id, 'not in results')}, "
                        f"Vector rank={vector_ranks.get(doc_id, 'not in results')}, "
                        f"buzzwang={buzzwang}"
                    )
            except Exception as e:
                logger.error(f"DEBUG: Manual RRF simulation failed: {str(e)}")
                logger.exception("RRF simulation error details:")

            # Log comparison with actual pipeline results
            result = self.query_pipeline.run(
                {
                    "bm25_retriever": {
                        "query": query_text,
                        "filters": formatted_filters,
                        "top_k": top_k,
                    },
                    "embedder": {"text": query_text},
                    "vector_retriever": {"filters": formatted_filters, "top_k": top_k},
                }
            )
            documents = result.get("document_joiner", {}).get("documents", [])
            logger.info(f"DEBUG: Query pipeline returned {len(documents)} documents")

            # Compare with our manual RRF
            if documents:
                logger.info(
                    "DEBUG: Comparing pipeline results with manual RRF calculation:"
                )
                for i, doc in enumerate(documents[: min(len(documents), top_k)]):
                    if hasattr(doc, "id") and doc.id in rrf_scores:
                        # Find this document's position in our sorted RRF results
                        manual_rank = (
                            sorted_doc_ids.index(doc.id) + 1
                            if doc.id in sorted_doc_ids
                            else "not found"
                        )
                        logger.info(
                            f"DEBUG: Document {doc.id} - actual joiner rank: {i + 1}, "
                            f"manual RRF rank: {manual_rank}, "
                            f"actual joiner score: {getattr(doc, 'score', 'unknown')}, "
                            f"manual RRF score: {rrf_scores.get(doc.id, 'unknown')}"
                        )
                    else:
                        logger.info(
                            f"DEBUG: Document at position {i + 1} has no ID or wasn't in manual results"
                        )

            # Run the query pipeline
            logger.info("Running full query pipeline with document joining")
            try:
                # Run the pipeline
                output = self.query_pipeline.run(params)

                # Get the results from the document joiner
                documents = output.get("document_joiner", {}).get("documents", [])
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
                        "content": doc.content,
                        "score": float(doc.score)
                        if hasattr(doc, "score") and doc.score is not None
                        else 0.0,
                        "metadata": doc.meta,
                        "rank": i + 1,
                    }
                )

            return results

        except Exception as e:
            logger.error(f"Error querying document store: {str(e)}")
            raise

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
