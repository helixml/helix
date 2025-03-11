#!/usr/bin/env python3
"""
Test script for VectorChord and VectorChord-bm25 components in Haystack.

This script:
1. Connects to a VectorChord-bm25 PostgreSQL database
2. Creates a document store with VectorchordDocumentStore
3. Indexes example documents
4. Tests vector search using VectorchordEmbeddingRetriever
5. Tests BM25 search using VectorchordBM25Retriever

Run with:
```
python -m app.vectorchord.test
```
From the top level of the haystack_service directory.
"""

import os
import time
import subprocess
import logging
from typing import Dict, Any, List, Optional

from haystack.utils.auth import Secret
from haystack.dataclasses import Document
from haystack import Pipeline
from haystack.document_stores.types import DuplicatePolicy
from haystack.components.embedders import SentenceTransformersTextEmbedder, SentenceTransformersDocumentEmbedder

# Import our custom components
from .document_store import VectorchordDocumentStore
from .components import VectorchordEmbeddingRetriever, VectorchordBM25Retriever

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Configuration
POSTGRES_PASSWORD = "mysecretpassword"
POSTGRES_USER = "postgres"
POSTGRES_HOST = "localhost"
POSTGRES_PORT = "5433"
POSTGRES_DB = "postgres"
CONTAINER_NAME = "vectorchord_test"
TABLE_NAME = "haystack_test_documents"
EMBEDDING_DIM = 384  # For all-MiniLM-L6-v2

# Connection string
PG_CONN_STR = f"postgresql://{POSTGRES_USER}:{POSTGRES_PASSWORD}@{POSTGRES_HOST}:{POSTGRES_PORT}/{POSTGRES_DB}"


def start_docker_container() -> bool:
    """Start a fresh VectorChord-bm25 Docker container for testing."""
    # Always stop and remove the container if it exists (for a fresh start)
    logger.info(f"Ensuring clean environment for testing...")
    subprocess.run(["docker", "rm", "-f", CONTAINER_NAME], 
                  stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    
    # Start a fresh container
    logger.info(f"Starting fresh container {CONTAINER_NAME}...")
    cmd = [
        "docker", "run",
        "--name", CONTAINER_NAME,
        "-e", f"POSTGRES_PASSWORD={POSTGRES_PASSWORD}",
        "-p", f"{POSTGRES_PORT}:5432",
        # Explicitly use ephemeral container with no volumes
        "--rm",
        "-d", "ghcr.io/tensorchord/vchord_bm25-postgres:pg17-v0.1.1"
    ]
    
    result = subprocess.run(cmd, capture_output=True, text=True)
    
    if result.returncode != 0:
        logger.error(f"Failed to start container: {result.stderr}")
        return False
    
    # Wait for container to be ready
    logger.info("Waiting for database to become available...")
    for _ in range(30):  # Wait up to 30 seconds
        time.sleep(1)
        try:
            # Try to connect to the database
            test_connection()
            logger.info("Database is available!")
            return True
        except Exception as e:
            pass  # Keep waiting
    
    logger.error("Timed out waiting for database to become available")
    return False


def test_connection():
    """Test the connection to the PostgreSQL database."""
    import psycopg
    conn = psycopg.connect(PG_CONN_STR)
    conn.close()
    return True


def setup_extensions():
    """Set up the required PostgreSQL extensions."""
    import psycopg
    
    conn = psycopg.connect(PG_CONN_STR)
    conn.autocommit = True
    
    with conn.cursor() as cur:
        # Create extensions
        cur.execute("CREATE EXTENSION IF NOT EXISTS vector;")
        cur.execute("CREATE EXTENSION IF NOT EXISTS vchord CASCADE;")
        cur.execute("CREATE EXTENSION IF NOT EXISTS vchord_bm25 CASCADE;")
        
        # Set up search path
        cur.execute("ALTER SYSTEM SET search_path TO \"$user\", public, bm25_catalog;")
        cur.execute("SELECT pg_reload_conf();")
    
    conn.close()
    logger.info("PostgreSQL extensions set up successfully")


def initialize_document_store() -> VectorchordDocumentStore:
    """Initialize and return a VectorchordDocumentStore."""
    document_store = VectorchordDocumentStore(
        connection_string=PG_CONN_STR,
        embedding_dimension=EMBEDDING_DIM,
        table_name=TABLE_NAME,
        language="english",
        vector_function="cosine_similarity",
        recreate_table=True,
        keyword_index_name=None  # Skip creating keyword index since we're using BM25
    )
    
    logger.info(f"VectorchordDocumentStore initialized with table {TABLE_NAME}")
    return document_store


def create_example_documents() -> List[Document]:
    """Create and return a list of example documents."""
    documents = [
        Document(content="VectorChord is a scalable, fast, and disk-friendly vector search in Postgres, the successor of pgvecto.rs."),
        Document(content="VectorChord-bm25 is a PostgreSQL extension that implements the BM25 ranking algorithm for text search."),
        Document(content="PostgreSQL is a powerful, free and open-source relational database system with over 30 years of active development."),
        Document(content="Haystack is an open-source framework for building NLP applications, especially for Retrieval Augmented Generation."),
        Document(content="BM25 ranking function is used by search engines to estimate the relevance of documents to a given search query."),
        Document(content="Vector search allows similarity queries based on embeddings rather than exact text matching."),
        Document(content="Postgres with vector extensions can handle both structured data and unstructured text with embeddings."),
        Document(content="Machine learning models can convert text into dense vector embeddings that capture semantic meaning."),
        Document(content="Retrieval Augmented Generation (RAG) combines retrieved documents with generative AI to produce more accurate responses."),
        Document(content="Hybrid search combines keyword matching with vector similarity for more relevant search results.")
    ]
    
    logger.info(f"Created {len(documents)} example documents")
    return documents


def embed_documents(documents: List[Document]) -> List[Document]:
    """Add embeddings to documents using SentenceTransformers."""
    embedder = SentenceTransformersDocumentEmbedder(model="all-MiniLM-L6-v2")
    
    # Warm up the embedder before using it
    embedder.warm_up()
    
    result = embedder.run(documents=documents)
    documents_with_embeddings = result["documents"]
    
    logger.info(f"Added embeddings to {len(documents_with_embeddings)} documents")
    return documents_with_embeddings


def index_documents(document_store: VectorchordDocumentStore, documents: List[Document]) -> None:
    """Index documents into the document store."""
    document_store.write_documents(documents, policy=DuplicatePolicy.OVERWRITE)
    logger.info(f"Indexed {len(documents)} documents into the document store")


def test_embedding_retriever(document_store: VectorchordDocumentStore):
    """Test the VectorchordEmbeddingRetriever."""
    # Create embedding retriever pipeline
    pipeline = Pipeline()
    text_embedder = SentenceTransformersTextEmbedder(model="all-MiniLM-L6-v2")
    retriever = VectorchordEmbeddingRetriever(document_store=document_store, top_k=3)
    
    pipeline.add_component("embedder", text_embedder)
    pipeline.add_component("retriever", retriever)
    pipeline.connect("embedder.embedding", "retriever.query_embedding")
    
    # Run queries
    queries = [
        "Tell me about vector search",
        "How does BM25 ranking work?",
        "What is Postgres used for?"
    ]
    
    for query in queries:
        logger.info(f"\nVector search query: '{query}'")
        results = pipeline.run({"embedder": {"text": query}})
        for i, doc in enumerate(results["retriever"]["documents"], 1):
            logger.info(f"Result {i}: {doc.content}")


def test_bm25_retriever(document_store: VectorchordDocumentStore):
    """Test the VectorchordBM25Retriever."""
    # Create BM25 retriever
    retriever = VectorchordBM25Retriever(document_store=document_store, top_k=3)
    
    # Run queries
    queries = [
        "vector search postgres",
        "BM25 ranking algorithm",
        "PostgreSQL database"
    ]
    
    all_scores_valid = True
    failed_queries = []
    
    for query in queries:
        logger.info(f"\nBM25 search query: '{query}'")
        results = retriever.run(query=query)
        
        query_has_valid_scores = True
        for i, doc in enumerate(results["documents"], 1):
            # Check if the score is None
            if doc.score is None:
                query_has_valid_scores = False
                all_scores_valid = False
                logger.error(f"Result {i} for query '{query}' has None score: {doc.content}")
            else:
                logger.info(f"Result {i}: {doc.content} (Score: {doc.score})")
        
        if not query_has_valid_scores:
            failed_queries.append(query)
    
    # Assert that all scores are valid (not None)
    if not all_scores_valid:
        raise AssertionError(f"BM25 search returned None scores for queries: {failed_queries}")
    
    logger.info("All BM25 search results have valid scores!")


def main():
    """Main function to run all test steps."""
    # Start the Docker container
    if not start_docker_container():
        logger.error("Failed to start Docker container. Exiting.")
        return
    
    # Set up PostgreSQL extensions
    setup_extensions()
    
    # Initialize document store
    document_store = initialize_document_store()
    
    # Create and embed example documents
    documents = create_example_documents()
    documents_with_embeddings = embed_documents(documents)
    
    # Index documents
    index_documents(document_store, documents_with_embeddings)
    
    # Test the retrievers
    logger.info("\n=== Testing VectorchordEmbeddingRetriever ===")
    test_embedding_retriever(document_store)
    
    logger.info("\n=== Testing VectorchordBM25Retriever ===")
    test_bm25_retriever(document_store)
    
    logger.info("\nAll tests completed successfully!")


if __name__ == "__main__":
    main()