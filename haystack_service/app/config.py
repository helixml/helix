import os
from typing import Optional

class Settings:
    # PostgreSQL connection
    PGVECTOR_DSN: str = os.getenv("PGVECTOR_DSN", "postgresql://postgres:postgres@pgvector:5432/postgres")
    PGVECTOR_TABLE: str = os.getenv("PGVECTOR_TABLE", "haystack_documents")
    SIMILARITY_FUNCTION: str = os.getenv("SIMILARITY_FUNCTION", "cosine_similarity")
    
    # Document processing
    CHUNK_SIZE: int = int(os.getenv("CHUNK_SIZE", "500"))
    CHUNK_OVERLAP: int = int(os.getenv("CHUNK_OVERLAP", "50"))
    
    # Embedding model settings
    EMBEDDING_DIM: int = int(os.getenv("RAG_HAYSTACK_EMBEDDINGS_DIM", "384"))  # GTE-small is 384 dimensions
    RAG_HAYSTACK_EMBEDDINGS_MODEL: str = os.getenv("RAG_HAYSTACK_EMBEDDINGS_MODEL", "thenlper/gte-small")
    VLLM_BASE_URL: str = os.environ["VLLM_BASE_URL"]
    VLLM_API_KEY: str = os.getenv("VLLM_API_KEY", "EMPTY")
    
    # Service settings
    LOG_LEVEL: str = os.getenv("LOG_LEVEL", "INFO")

settings = Settings() 