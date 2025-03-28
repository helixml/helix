import os
from typing import Optional


class Settings:
    # PostgreSQL connection
    PGVECTOR_DSN: str = os.getenv(
        "PGVECTOR_DSN", "postgresql://postgres:postgres@pgvector:5432/postgres"
    )
    PGVECTOR_TABLE: str = os.getenv("PGVECTOR_TABLE", "haystack_documents")

    # Document processing
    CHUNK_SIZE: int = int(os.getenv("RAG_HAYSTACK_CHUNK_SIZE", "1000"))
    CHUNK_OVERLAP: int = int(os.getenv("RAG_HAYSTACK_CHUNK_OVERLAP", "50"))
    CHUNK_UNIT: str = os.getenv("RAG_HAYSTACK_CHUNK_UNIT", "word")

    # Embedding model settings
    EMBEDDING_DIM: int = int(os.getenv("RAG_HAYSTACK_EMBEDDINGS_DIM", "3584"))
    EMBEDDINGS_MAX_TOKENS: int = int(
        os.getenv("RAG_HAYSTACK_EMBEDDINGS_MAX_TOKENS", "32768")
    )
    EMBEDDINGS_MODEL: str = os.getenv(
        "RAG_HAYSTACK_EMBEDDINGS_MODEL", "Alibaba-NLP/gte-Qwen2-7B-instruct"
    )
    EMBEDDINGS_SOCKET: Optional[str] = os.getenv("HELIX_EMBEDDINGS_SOCKET", None)
    VLLM_BASE_URL: str = os.getenv(
        "VLLM_BASE_URL", None
    )  # optional, not used if socket is set
    VLLM_API_KEY: str = os.getenv("VLLM_API_KEY", "EMPTY")

    # Vision model settings
    VISION_ENABLED: bool = os.getenv("RAG_VISION_ENABLED", "false").lower() == "true"
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
