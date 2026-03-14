# Requirements: Deprecate Haystack RAG, Replace with Kodit

## Background

Helix currently uses a Python FastAPI microservice ("haystack_service") as the primary RAG provider. It indexes arbitrary documents (PDFs, DOCX, TXT, images) into VectorChord (PostgreSQL + pgvector + BM25) and performs hybrid search. Kodit is an existing Go library (`github.com/helixml/kodit v1.1.8`) already embedded in helix for code intelligence, also using VectorChord with local ONNX embeddings.

The goal is to eliminate the haystack Python microservice and have kodit serve the same RAG capabilities, reducing operational complexity and unifying the embedding/storage stack.

## User Stories

**US-1 — Operator: No haystack service to run**
As an operator, I want to deploy helix without the haystack Python microservice, so that I have fewer containers to manage and no Python dependency.

**US-2 — End user: Knowledge base still works**
As an end user, I want to upload PDFs, Word docs, and text files to a knowledge base and have them returned as context during chat, so that helix answers questions using my documents.

**US-3 — End user: Web-crawled knowledge works**
As an end user, I want to create a knowledge base from a URL and have it indexed and queryable, exactly as before.

**US-4 — Admin: Vision/image RAG (stretch goal)**
As an admin, I want image-based documents to still be queryable via vision embeddings. (This may be deferred if kodit does not support it initially.)

**US-5 — Developer: No upstream API changes**
As a developer integrating with helix, I want the public API for knowledge bases to remain unchanged so that no client changes are required.

## Acceptance Criteria

- AC-1: The `haystack_service/` directory is removed from the repository (or marked deprecated and unused).
- AC-2: The `rag.RAG` interface (`Index`, `Query`, `Delete`) is implemented by a new `KoditRAG` adapter that delegates to the kodit library.
- AC-3: All existing `SessionRAGIndexChunk`, `SessionRAGQuery`, `SessionRAGResult`, `DeleteIndexRequest` types are preserved unchanged.
- AC-4: Documents of types TXT, MD, PDF, DOCX, PPTX, HTML can be indexed and queried via kodit.
- AC-5: Hybrid search (semantic + BM25) is supported, matching or exceeding haystack's ranking quality.
- AC-6: Metadata filtering by `data_entity_id` and `document_id` works for queries and deletes.
- AC-7: A re-indexing path exists so that existing knowledge base content is re-indexed via kodit (existing haystack vector data need not be migrated).
- AC-8: The `docker-compose.yaml` `haystack` service is removed; VectorChord continues to serve both kodit (code) and kodit-RAG (documents), ideally sharing the same PostgreSQL instance with separate tables.
- AC-9: Configuration switches cleanly: `RAG_DEFAULT_PROVIDER=kodit` selects the new implementation.

## Out of Scope

- Vision/image RAG pipeline — defer unless kodit gains native image embedding support.
- Migration of existing vector data from haystack tables — re-indexing from filestore is sufficient.
- Changes to the external helix HTTP API.
