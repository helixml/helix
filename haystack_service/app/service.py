import logging
import os
from typing import Any, Dict, List, Optional

from haystack import Pipeline
from haystack.components.joiners import DocumentJoiner
from haystack.components.preprocessors import DocumentSplitter, DocumentCleaner
from haystack.components.writers import DocumentWriter
from haystack.dataclasses import Document
from haystack.utils.auth import Secret

from haystack.components.embedders import OpenAIDocumentEmbedder, OpenAITextEmbedder
from .unix_socket_embedders import UnixSocketOpenAIDocumentEmbedder, UnixSocketOpenAITextEmbedder
from .converters import LocalUnstructuredConverter

from .vectorchord.document_store import VectorchordDocumentStore
from .vectorchord.components import VectorchordEmbeddingRetriever, VectorchordBM25Retriever

from .config import settings

logger = logging.getLogger(__name__)

class HaystackService:
    """Main service class for Haystack RAG operations"""
    
    def __init__(self):
        """Initialize the Haystack service"""
        logger.info("Initializing HaystackService")
        
        # Initialize document stores
        try:
            # VectorChord store for both dense embeddings and BM25
            self.document_store = VectorchordDocumentStore(
                connection_string=Secret.from_token(settings.PGVECTOR_DSN),
                embedding_dimension=settings.EMBEDDING_DIM,
                table_name=settings.PGVECTOR_TABLE,
                vector_function="cosine_similarity",
                # search_strategy="hnsw", # see above about halfvec
                recreate_table=True # XXX disable to avoid data loss?
            )
            logger.info(f"Connected to VectorchordDocumentStore: {settings.PGVECTOR_TABLE}")
            
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
            logger.info(f"Using UNIX socket for document embeddings: {settings.EMBEDDINGS_SOCKET}")
            embedder = UnixSocketOpenAIDocumentEmbedder(
                socket_path=settings.EMBEDDINGS_SOCKET,
                model=settings.EMBEDDINGS_MODEL,
                dimensions=settings.EMBEDDING_DIM
            )
        else:
            embedder = OpenAIDocumentEmbedder(
                api_key=Secret.from_token(settings.VLLM_API_KEY),
                api_base_url=settings.VLLM_BASE_URL,
                model=settings.EMBEDDINGS_MODEL,
                dimensions=settings.EMBEDDING_DIM
            )
        
        converter = LocalUnstructuredConverter()
        
        cleaner = DocumentCleaner(
            remove_empty_lines=False,
            remove_extra_whitespaces=False,
            remove_regex=r'\.{5,}'  # Remove runs of 5 or more dots
        )
        
        splitter = DocumentSplitter(
            split_length=settings.CHUNK_SIZE,
            split_overlap=settings.CHUNK_OVERLAP,
            split_by="word",
            respect_sentence_boundary=True
        )
        splitter.warm_up()
        
        # Writer for the vector store (which now handles both embeddings and BM25)
        vector_writer = DocumentWriter(document_store=self.document_store)
        
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
        
        logger.info(f"Initialized indexing pipeline with chunk_size={settings.CHUNK_SIZE}, overlap={settings.CHUNK_OVERLAP}")

    def _init_query_pipeline(self):
        """Initialize the query pipeline"""
        self.query_pipeline = Pipeline()
        
        # Create components for query pipeline
        if settings.EMBEDDINGS_SOCKET:
            logger.info(f"Using UNIX socket for text embeddings: {settings.EMBEDDINGS_SOCKET}")
            embedder = UnixSocketOpenAITextEmbedder(
                socket_path=settings.EMBEDDINGS_SOCKET,
                model=settings.EMBEDDINGS_MODEL,
                dimensions=settings.EMBEDDING_DIM
            )
        else:
            embedder = OpenAITextEmbedder(
                api_key=Secret.from_token(settings.VLLM_API_KEY),
                api_base_url=settings.VLLM_BASE_URL,
                model=settings.EMBEDDINGS_MODEL,
                dimensions=settings.EMBEDDING_DIM
            )
        
        # Dense vector retriever using VectorChord
        vector_retriever = VectorchordEmbeddingRetriever(
            document_store=self.document_store,
            filters=None,
            top_k=5
        )
        
        # BM25 retriever using VectorChord-bm25
        bm25_retriever = VectorchordBM25Retriever(
            document_store=self.document_store,
            filters=None,
            top_k=5
        )
        
        # Document joiner to combine results from both retrievers
        document_joiner = DocumentJoiner(
            join_mode="reciprocal_rank_fusion",
            top_k=5
        )
        
        # Add components
        self.query_pipeline.add_component("embedder", embedder)
        self.query_pipeline.add_component("vector_retriever", vector_retriever)
        self.query_pipeline.add_component("bm25_retriever", bm25_retriever)
        self.query_pipeline.add_component("document_joiner", document_joiner)
        
        # Connect components
        # Vector retrieval - need to be explicit about field connections
        self.query_pipeline.connect("embedder.embedding", "vector_retriever.query_embedding")
        
        # Join documents from both retrievers - these are already correct
        self.query_pipeline.connect("vector_retriever", "document_joiner")
        self.query_pipeline.connect("bm25_retriever", "document_joiner")
        
        # Save retrievers for parameter updates
        self.vector_retriever = vector_retriever
        self.bm25_retriever = bm25_retriever
        
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
    
    async def process_and_index(self, file_path: str, metadata: Dict[str, Any] = None) -> Dict[str, Any]:
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
            
        logger.info(f"Processing and indexing {os.path.basename(file_path)} with metadata: {metadata}")
        
        # Add the file path to metadata
        metadata["source"] = os.path.basename(file_path)
        
        # Set up the parameters for the indexing pipeline
        params = {
            "converter": {"paths": [file_path], "metadata": metadata},
        }
        
        try:
            output = self.indexing_pipeline.run(params)
            
            # Get the number of chunks created by looking at vector_writer output
            num_chunks = len(output.get("vector_writer", {}).get("written_documents", []))
            
            # Return stats
            return {
                "filename": os.path.basename(file_path),
                "indexed": True,
                "chunks": num_chunks,
                "metadata": metadata,
            }
            
        except Exception as e:
            logger.error(f"Error processing document: {str(e)}")
            raise
    
    async def query(self, query_text: str, filters: Dict[str, Any] = None, top_k: int = 5) -> List[Dict[str, Any]]:
        """
        Query the document store for relevant passages
        
        Args:
            query_text: The query text to search for
            filters: Optional filters to apply to the search
            top_k: Number of results to return
            
        Returns:
            List of dictionaries with document data
        """
        logger.info(f"Querying with: '{query_text}'")
        
        # Update retriever parameters
        self.vector_retriever.top_k = top_k
        self.vector_retriever.filters = filters
        
        self.bm25_retriever.top_k = top_k
        self.bm25_retriever.filters = filters
        
        # Set up the parameters for the query pipeline
        params = {
            "embedder": {"text": query_text},
            "bm25_retriever": {"query": query_text},
        }
        
        try:
            # Run the query
            output = self.query_pipeline.run(params)
            
            # Get the results from the document joiner
            documents = output.get("document_joiner", {}).get("documents", [])
            
            # Convert to dictionaries
            results = []
            for i, doc in enumerate(documents):
                results.append({
                    "id": doc.id,
                    "content": doc.content,
                    "score": float(doc.score) if hasattr(doc, "score") and doc.score is not None else 0.0,
                    "metadata": doc.meta,
                    "rank": i + 1
                })
                
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
                return {"deleted": False, "count": 0, "message": "No matching documents found"}
            
            # Get IDs of matching documents
            doc_ids = [doc.id for doc in matching_docs]
            
            # Delete from both document stores
            self.document_store.delete_documents(doc_ids)
            
            return {
                "deleted": True,
                "count": len(doc_ids),
                "message": f"Deleted {len(doc_ids)} documents",
            }
            
        except Exception as e:
            logger.error(f"Error deleting documents: {str(e)}")
            raise 