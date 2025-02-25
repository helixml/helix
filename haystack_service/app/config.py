import os
from typing import Optional

class Settings:
    # PostgreSQL connection
    PGVECTOR_DSN: str = os.getenv("PGVECTOR_DSN", "postgresql://postgres:postgres@postgres:5432/postgres")
    PGVECTOR_TABLE: str = os.getenv("PGVECTOR_TABLE", "haystack_documents")
    
    # Helix API connection
    HELIX_API_URL: str = os.getenv("HELIX_API_URL", "http://api:8080")
    
    # Document processing
    CHUNK_SIZE: int = int(os.getenv("CHUNK_SIZE", "500"))
    CHUNK_OVERLAP: int = int(os.getenv("CHUNK_OVERLAP", "50"))
    
    # Embedding model
    EMBEDDING_DIM: int = int(os.getenv("EMBEDDING_DIM", "384"))  # GTE-small is 384 dimensions
    RAG_HAYSTACK_EMBEDDINGS_MODEL: str = os.getenv("RAG_HAYSTACK_EMBEDDINGS_MODEL", "thenlper/gte-small")
    
    # Service settings
    LOG_LEVEL: str = os.getenv("LOG_LEVEL", "INFO")

settings = Settings() 