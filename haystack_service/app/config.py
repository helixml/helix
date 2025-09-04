import os
from typing import Optional


def _build_pgvector_dsn() -> str:
    """Build PGVECTOR_DSN from individual components or use existing DSN."""
    # Check if individual components are provided
    host = os.getenv("PGVECTOR_HOST")
    port = os.getenv("PGVECTOR_PORT")
    user = os.getenv("PGVECTOR_USER")
    password = os.getenv("PGVECTOR_PASSWORD")
    database = os.getenv("PGVECTOR_DATABASE")

    # If all individual components are provided, construct DSN
    if all([host, user, password, database]):
        port = port or "5432"  # Default port if not provided
        return f"postgresql://{user}:{password}@{host}:{port}/{database}"

    # Otherwise, use the existing PGVECTOR_DSN or default
    return os.getenv("PGVECTOR_DSN", "postgresql://postgres:postgres@pgvector:5432/postgres")


class Settings:
    # PostgreSQL connection
    PGVECTOR_DSN: str = _build_pgvector_dsn()
    PGVECTOR_TABLE: str = os.getenv("PGVECTOR_TABLE", "haystack_documents")

    # Document processing
    CHUNK_SIZE: int = int(os.getenv("RAG_HAYSTACK_CHUNK_SIZE", "500"))
    CHUNK_OVERLAP: int = int(os.getenv("RAG_HAYSTACK_CHUNK_OVERLAP", "50"))
    CHUNK_UNIT: str = os.getenv("RAG_HAYSTACK_CHUNK_UNIT", "word")

    # Embedding model settings
    EMBEDDING_DIM: int = int(os.getenv("RAG_HAYSTACK_EMBEDDINGS_DIM", "1536"))
    EMBEDDINGS_MAX_TOKENS: int = int(
        os.getenv("RAG_HAYSTACK_EMBEDDINGS_MAX_TOKENS", "32768")
    )
    EMBEDDINGS_MODEL: str = os.getenv(
        "RAG_HAYSTACK_EMBEDDINGS_MODEL", "MrLight/dse-qwen2-2b-mrl-v1"
    )
    EMBEDDINGS_SOCKET: Optional[str] = os.getenv("HELIX_EMBEDDINGS_SOCKET", None)
    VLLM_BASE_URL: str = os.getenv(
        "VLLM_BASE_URL", None
    )  # optional, not used if socket is set
    VLLM_API_KEY: str = os.getenv("VLLM_API_KEY", "EMPTY")

    # Vision model settings
    VISION_ENABLED: bool = os.getenv("RAG_VISION_ENABLED", "true").lower() == "true"
    VISION_EMBEDDING_DIM: int = int(os.getenv("RAG_VISION_EMBEDDINGS_DIM", "1536"))
    VISION_EMBEDDINGS_MODEL: str = os.getenv(
        "RAG_VISION_EMBEDDINGS_MODEL", "MrLight/dse-qwen2-2b-mrl-v1"
    )
    VISION_EMBEDDINGS_SOCKET: Optional[str] = os.getenv(
        "RAG_VISION_EMBEDDINGS_SOCKET", None
    )
    VISION_BASE_URL: Optional[str] = os.getenv("RAG_VISION_BASE_URL", None)
    VISION_API_KEY: Optional[str] = os.getenv("RAG_VISION_API_KEY", None)
    VISION_PGVECTOR_TABLE: str = os.getenv(
        "RAG_VISION_PGVECTOR_TABLE", "haystack_documents_vision"
    )

    # Service settings
    LOG_LEVEL: str = os.getenv("LOG_LEVEL", "INFO")


settings = Settings()

# Check if LOG_LEVEL is set to TRACE. If it is, set it to DEBUG
if settings.LOG_LEVEL == "TRACE":
    settings.LOG_LEVEL = "DEBUG"
