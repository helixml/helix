import os
import tempfile
import logging
import traceback
from typing import List, Dict, Any, Optional, Union, BinaryIO

from haystack import Pipeline, Document
from haystack.utils import Secret
from haystack_integrations.document_stores.pgvector import PgvectorDocumentStore
from haystack.document_stores.in_memory import InMemoryDocumentStore
from haystack.components.preprocessors import DocumentSplitter, DocumentCleaner
from haystack_integrations.components.retrievers.pgvector import PgvectorEmbeddingRetriever
from haystack.components.retrievers.in_memory import InMemoryBM25Retriever
from haystack.components.embedders import OpenAIDocumentEmbedder, OpenAITextEmbedder
from haystack.components.writers import DocumentWriter
from haystack.components.joiners import DocumentJoiner

from .config import settings
from .converters import LocalUnstructuredConverter
# Import our custom embedders
from .unix_socket_embedders import UnixSocketOpenAIDocumentEmbedder, UnixSocketOpenAITextEmbedder

# Configure logging
logging.basicConfig(level=getattr(logging, settings.LOG_LEVEL))
logger = logging.getLogger(__name__)

# TODO: more work needed to get halfvec to work. Last error was
# Query failed: 'HalfVector' object has no attribute 'tolist' - on querying
#
# Monkeypatch pgvector's CREATE_TABLE_STATEMENT to use halfvec type so that we
# can support up to 4000 dimensions
# import haystack_integrations.document_stores.pgvector.document_store as pgvector_store
# pgvector_store.CREATE_TABLE_STATEMENT = """
# CREATE TABLE IF NOT EXISTS {schema_name}.{table_name} (
# id VARCHAR(128) PRIMARY KEY,
# embedding HALFVEC({embedding_dimension}),
# content TEXT,
# blob_data BYTEA,
# blob_meta JSONB,
# blob_mime_type VARCHAR(255),
# meta JSONB)
# """
# pgvector_store.VECTOR_FUNCTION_TO_POSTGRESQL_OPS = {
#     "cosine_similarity": "halfvec_cosine_ops",
#     "inner_product": "halfvec_ip_ops",
#     "l2_distance": "halfvec_l2_ops",
# }

class HaystackService:
    """Main service class for Haystack RAG operations"""
    
    def __init__(self):
        """Initialize the Haystack service"""
        logger.info("Initializing HaystackService")
        
        # Initialize document stores
        try:
            # PgVector store for dense embeddings
            self.document_store = PgvectorDocumentStore(
                connection_string=Secret.from_token(settings.PGVECTOR_DSN),
                embedding_dimension=settings.EMBEDDING_DIM,
                table_name=settings.PGVECTOR_TABLE,
                vector_function="cosine_similarity",
                # search_strategy="hnsw", # see above about halfvec
                recreate_table=True # XXX disable to avoid data loss?
            )
            logger.info(f"Connected to PgvectorDocumentStore: {settings.PGVECTOR_TABLE}")
            
            # In-memory store for BM25 retrieval
            self.bm25_document_store = InMemoryDocumentStore()
            logger.info("Initialized InMemoryDocumentStore for BM25 retrieval")
            
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
        
        # Writers for both document stores
        vector_writer = DocumentWriter(document_store=self.document_store)
        bm25_writer = DocumentWriter(document_store=self.bm25_document_store)
        
        # Add components
        self.indexing_pipeline.add_component("converter", converter)
        self.indexing_pipeline.add_component("cleaner", cleaner)
        self.indexing_pipeline.add_component("splitter", splitter)
        self.indexing_pipeline.add_component("embedder", embedder)
        self.indexing_pipeline.add_component("vector_writer", vector_writer)
        self.indexing_pipeline.add_component("bm25_writer", bm25_writer)
        
        # Connect components
        self.indexing_pipeline.connect("converter", "cleaner")
        self.indexing_pipeline.connect("cleaner", "splitter")
        self.indexing_pipeline.connect("splitter", "embedder")
        self.indexing_pipeline.connect("embedder", "vector_writer")
        # BM25 doesn't need embeddings, so connect splitter directly to BM25 writer
        self.indexing_pipeline.connect("splitter", "bm25_writer")
        
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
        
        # Dense vector retriever
        vector_retriever = PgvectorEmbeddingRetriever(
            document_store=self.document_store,
            filters=None,
            top_k=5
        )
        
        # BM25 retriever
        bm25_retriever = InMemoryBM25Retriever(
            document_store=self.bm25_document_store,
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
        self.document_joiner = document_joiner
        
        logger.info("Initialized hybrid query pipeline with vector and BM25 retrievers")

    async def extract_text(self, file_path: str) -> str:
        """Extract text from a file without indexing it"""
        logger.info(f"Extracting text from file: {file_path}")
        
        try:
            # Extract text using the converter
            result = self.converter.run(paths=[file_path])
            documents = result["documents"]
            if not documents:
                logger.warning("No text extracted from file")
                return ""
            return documents[0].content
        except Exception as e:
            logger.error(f"Text extraction error: {str(e)}")
            raise RuntimeError(f"Text extraction error: {str(e)}")

    async def process_and_index(self, file_path: str, metadata: Dict[str, Any] = None) -> Dict[str, Any]:
        """Process a document and index it in both document stores"""
        logger.info(f"Processing and indexing file with metadata: {metadata}")
        
        try:
            # Run the indexing pipeline
            result = self.indexing_pipeline.run(
                data={
                    "converter": {
                        "paths": [file_path],
                        "meta": metadata or {}  # Ensure meta is always a dict, even if None
                    }
                }
            )
            
            # Access the number of documents written (both vector and BM25)
            logger.info(f"Indexing pipeline result: {result}")
            num_vector_chunks = result.get("vector_writer", {}).get("documents_written", 0)
            num_bm25_chunks = result.get("bm25_writer", {}).get("documents_written", 0)
            
            logger.info(f"Successfully indexed document with {num_vector_chunks} vector chunks and {num_bm25_chunks} BM25 chunks")
            return {
                "status": "success",
                "documents_processed": 1,
                "vector_chunks_indexed": num_vector_chunks,
                "bm25_chunks_indexed": num_bm25_chunks
            }
            
        except Exception as e:
            error_details = {
                "type": type(e).__name__,
                "message": str(e),
                "traceback": traceback.format_exc()
            }
            logger.error(f"Failed to process and index document: {error_details}")
            return {
                "status": "error",
                "error_type": error_details["type"],
                "message": error_details["message"],
                "traceback": error_details["traceback"],
                "documents_processed": 0,
                "chunks_indexed": 0
            }
    
    async def query(self, query_text: str, filters: Dict[str, Any] = None, top_k: int = 5) -> List[Dict[str, Any]]:
        """Query both document stores for relevant documents using hybrid retrieval"""
        logger.info(f"Performing hybrid query; filters: {filters}, top_k: {top_k}")
        
        try:
            # Update retriever parameters if needed
            self.vector_retriever.top_k = top_k
            self.bm25_retriever.top_k = top_k
            self.document_joiner.top_k = top_k
            
            if filters:
                formatted_filters = {
                    "operator": "AND",
                    "conditions": [
                        {"field": f"meta.{key}", "operator": "==", "value": value}
                        for key, value in filters.items()
                    ]
                }
                self.vector_retriever.filters = formatted_filters
                self.bm25_retriever.filters = formatted_filters
            
            # Run the query pipeline
            result = self.query_pipeline.run(
                data={
                    "embedder": {"text": query_text},
                    "bm25_retriever": {"query": query_text}
                }
            )
            
            documents = result["document_joiner"]["documents"]
            logger.info(f"Retrieved {len(documents)} results from hybrid search")
            
            # Format results
            return [
                {
                    "content": doc.content,
                    "metadata": doc.meta,
                    "score": float(doc.score if doc.score is not None else 0.0)
                }
                for doc in documents
            ]
            
        except Exception as e:
            logger.error(f"Hybrid query failed: {str(e)}")
            raise
    
    async def delete(self, filters: Dict[str, Any]) -> Dict[str, Any]:
        """Delete documents from both document stores based on filters"""
        logger.info(f"Deleting documents with filters: {filters}")
        
        # Format filters
        formatted_filters = {
            "operator": "AND",
            "conditions": [
                {"field": f"meta.{key}", "operator": "==", "value": value}
                for key, value in filters.items()
            ]
        }
        
        try:
            # Find and delete matching documents from vector store
            matching_vector_docs = self.document_store.filter_documents(filters=formatted_filters)
            if matching_vector_docs:
                self.document_store.delete_documents(document_ids=[doc.id for doc in matching_vector_docs])
                deleted_vector = len(matching_vector_docs)
            else:
                deleted_vector = 0
            
            # Find and delete matching documents from BM25 store
            matching_bm25_docs = self.bm25_document_store.filter_documents(filters=formatted_filters)
            if matching_bm25_docs:
                self.bm25_document_store.delete_documents(document_ids=[doc.id for doc in matching_bm25_docs])
                deleted_bm25 = len(matching_bm25_docs)
            else:
                deleted_bm25 = 0
            
            logger.info(f"Deleted {deleted_vector} vector documents and {deleted_bm25} BM25 documents")
            return {
                "status": "success", 
                "vector_documents_deleted": deleted_vector,
                "bm25_documents_deleted": deleted_bm25
            }
            
        except Exception as e:
            logger.error(f"Failed to delete documents: {str(e)}")
            raise 