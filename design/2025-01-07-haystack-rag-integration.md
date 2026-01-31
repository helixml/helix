# Haystack RAG Integration Design Document

**Date:** 2025-01-07
**Status:** Active/Production
**Version:** 1.0

## Executive Summary

Haystack is a Python-based RAG (Retrieval Augmented Generation) framework integrated into Helix as a microservice. It provides semantic search and document retrieval capabilities for knowledge management, supporting hybrid search (semantic + keyword BM25), vision-based document retrieval, and GPU-accelerated embeddings.

## 1. System Overview

### 1.1 Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                         Helix API Server (Go)                    │
│                                                                   │
│  Knowledge Indexing Flow    │    Query Flow                      │
│  ┌───────────────────────┐  │  ┌──────────────────┐             │
│  │ Knowledge Reconciler  │  │  │ RAG Query Handler│             │
│  │ (knowledge_indexer.go)│  │  │  (rag.go)        │             │
│  └──────────┬────────────┘  │  └────────┬─────────┘             │
│             │                │          │                        │
│  ┌──────────▼────────────┐  │  ┌───────▼─────────┐             │
│  │ Document Extraction   │  │  │ HaystackRAG     │             │
│  │ (text extraction)     │  │  │ Client (Go)     │             │
│  └──────────┬────────────┘  │  │ (rag_haystack.go)             │
│             │                │  └────────┬────────┘             │
└─────────────┼────────────────┼──────────┼──────────────────────┘
              │                │          │
              │  HTTP POST     │  HTTP POST
              │  /process      │  /query
              │                │
        ┌─────▼────────────────▼──────────────────────┐
        │     Haystack Service (Python FastAPI)       │
        │                                              │
        │  ┌─────────────────────────────────────┐   │
        │  │ Haystack Pipelines                  │   │
        │  │ ├─ Indexing Pipeline                │   │
        │  │ │  ├─ Converters (PDF, DOCX, etc) │   │
        │  │ │  ├─ Splitters (chunking)        │   │
        │  │ │  ├─ Embedders (local/VLLM)     │   │
        │  │ │  └─ Document Store (VectorChord)│   │
        │  │ │                                   │   │
        │  │ └─ Query Pipeline                   │   │
        │  │    ├─ Query Embedding              │   │
        │  │    ├─ Hybrid Search (Semantic+BM25)│   │
        │  │    └─ Result Ranking               │   │
        │  │                                     │   │
        │  │ ┌─────────────────────────────────┐│   │
        │  │ │ Vision Support (Optional)        ││   │
        │  │ │ ├─ Image Indexing               ││   │
        │  │ │ └─ Vision Query Pipeline        ││   │
        │  │ └─────────────────────────────────┘│   │
        │  └─────────────────────────────────────┘   │
        │                                              │
        └─────────────┬───────────────────────────────┘
                      │
        ┌─────────────▼───────────────┐
        │  VectorChord / PostgreSQL    │
        │  (Vector DB + BM25 Indexing) │
        │                              │
        │  Tables:                     │
        │  ├─ haystack_documents       │
        │  └─ haystack_documents_vision│
        │  (pgvector + pg17 + BM25)    │
        │                              │
        │  + UNIX Socket for embeddings│
        └──────────────────────────────┘
```

### 1.2 Component Roles

| Component | Technology | Purpose | Location |
|-----------|-----------|---------|----------|
| **API Server** | Go | Helix core server, orchestrates RAG operations | `/api/pkg/server/` |
| **RAG Client** | Go | HTTP client for Haystack API | `/api/pkg/rag/rag_haystack.go` |
| **Haystack Service** | Python FastAPI | Standalone RAG microservice | `/haystack_service/` |
| **Document Store** | VectorChord (PostgreSQL 17) | Vector storage + BM25 indexing | `pgvector` container |
| **Embeddings** | VLLM or UNIX Socket | Vector generation for documents/queries | VLLM service or embedded |
| **Text Extraction** | Unstructured + PyMuPDF | Parse PDF, DOCX, TXT, images | Haystack service |

## 2. Haystack Service Architecture

### 2.1 Microservice Structure

```
haystack_service/
├── main.py                          # FastAPI app initialization
├── pyproject.toml                   # Python dependencies
├── app/
│   ├── api.py                       # REST endpoints
│   ├── service.py                   # Core HaystackService class
│   ├── service_image.py             # Vision RAG support
│   ├── config.py                    # Configuration management
│   ├── embedders.py                 # Embedding configurations
│   ├── unix_socket_embedders.py     # Local socket embeddings
│   ├── converters.py                # Document format converters
│   ├── splitters.py                 # Text chunking strategies
│   ├── vectorchord/                 # Custom VectorChord integration
│   └── Dockerfile                   # Container image
└── docker-compose.override.yaml     # Optional local override
```

### 2.2 Key Classes and Methods

#### HaystackService (service.py)

```python
class HaystackService:
    def __init__(self, config: Config)
        # Initialize document store, pipelines, embedders

    def build_indexing_pipeline(self) -> Pipeline
        # Creates: Converter → Splitter → Embedder → DocumentStore

    def build_query_pipeline(self) -> Pipeline
        # Creates: Query Embedder → Retriever → Ranker

    async def index_documents(self, data: IndexRequest) -> IndexResponse
        # Process and store documents

    async def query_documents(self, query: QueryRequest) -> QueryResponse
        # Retrieve relevant documents

    async def delete_documents(self, request: DeleteRequest) -> DeleteResponse
        # Remove documents by filter
```

#### VectorChord Document Store

- **Type:** PostgreSQL 17 with pgvector and BM25 indexing
- **Hybrid Search:** Combines semantic (embedding) + keyword (BM25) scoring
- **Metadata:** Rich filtering via SQL WHERE clauses
- **Schema:**
  ```sql
  haystack_documents (
      id TEXT PRIMARY KEY,
      embedding pgvector(1536),     -- Vector field
      content TEXT,                  -- Full text for BM25
      metadata JSONB,               -- {document_id, data_entity_id, ...}
      created_at TIMESTAMP,
      updated_at TIMESTAMP
  )
  ```

### 2.3 REST API Endpoints

#### 1. POST `/process` - Index Documents
**Purpose:** Upload and index documents

```json
Request:
{
  "file": <multipart file>,
  "metadata": {
    "data_entity_id": "uuid",
    "document_id": "id",
    "source": "filename",
    "tags": ["tag1", "tag2"]
  }
}

Response:
{
  "status": "success",
  "indexed_count": 42,
  "document_id": "doc-123",
  "chunks": 42
}
```

**Haystack Pipeline:**
1. **Converter** → Detect format (PDF, DOCX, TXT, images)
2. **Splitter** → Chunk text (500 char chunks, 50 char overlap)
3. **Embedder** → Generate vectors via VLLM
4. **Document Store** → Store in VectorChord with metadata

#### 2. POST `/query` - Semantic Search
**Purpose:** Retrieve relevant documents

```json
Request:
{
  "query": "How to deploy on Kubernetes?",
  "filters": {
    "data_entity_id": ["uuid1", "uuid2"],
    "document_id": ["doc1", "doc2"]
  },
  "top_k": 5
}

Response:
{
  "results": [
    {
      "content": "Kubernetes deployment requires...",
      "metadata": {
        "document_id": "doc-123",
        "data_entity_id": "uuid",
        "source": "deployment.pdf"
      },
      "score": 0.87
    }
  ]
}
```

**Query Pipeline:**
1. **Embedder** → Convert query to vector
2. **Retriever** → Hybrid search (semantic + BM25)
3. **Filter** → Apply metadata filters
4. **Ranker** → Sort by relevance scores

#### 3. POST `/process-vision` - Index Images
**Purpose:** Index images for vision-based retrieval

```json
Request:
{
  "file": <image file>,
  "metadata": { "data_entity_id": "uuid", ... }
}
```

#### 4. POST `/query-vision` - Vision Search
**Purpose:** Query indexed images

```json
Request:
{
  "query": "Diagrams showing architecture",
  "top_k": 5
}
```

#### 5. POST `/extract` - Extract Text Only
**Purpose:** Extract text without indexing

```json
Request: { "file": <multipart file> }
Response: { "text": "extracted content..." }
```

#### 6. POST `/delete` - Remove Documents
**Purpose:** Delete documents by filter

```json
Request:
{
  "filters": {
    "data_entity_id": "uuid"  # Delete all docs for this entity
  }
}
```

#### 7. GET `/health` - Health Check
**Purpose:** Service readiness check

## 3. Integration with Helix API

### 3.1 Initialization Flow

**File:** `api/cmd/helix/serve.go` (lines 426-428)

```go
// During API startup, RAG provider is selected based on configuration
switch cfg.RAG.Provider {
case config.RAGProviderHaystack:
    ragClient = rag.NewHaystackRAG(cfg.RAG.Haystack.URL)
    log.Info().Msgf("Using Haystack for RAG")
case config.RAGProviderTypesense:
    ragClient = rag.NewTypesenseRAG(...)
case config.RAGProviderLlamaindex:
    ragClient = rag.NewLlamaindexRAG(...)
}
```

### 3.2 Document Indexing Workflow

**Triggered by:** Knowledge reconciliation process

```
User uploads knowledge source
    ↓
api/pkg/controller/knowledge/knowledge_indexer.go
    ↓
Extract text (Tika, Unstructured, or Haystack extractor)
    ↓
api/pkg/rag/rag_haystack.go → HaystackRAG.Index()
    ↓
HTTP POST to http://haystack:8000/process
    ↓
Haystack processes and indexes documents
    ↓
Documents stored in VectorChord with metadata
    ↓
Helix knowledge store records mapping
```

### 3.3 Query Workflow

**Triggered by:** RAG context retrieval during conversation

```
User sends message (agent/assistant query)
    ↓
api/pkg/rag/rag.go checks if RAG context needed
    ↓
api/pkg/rag/rag_haystack.go → HaystackRAG.Query()
    ↓
HTTP POST to http://haystack:8000/query
    ├─ Query text
    ├─ Filters (document IDs, data entity IDs)
    └─ Top-K results count
    ↓
Haystack performs hybrid search (semantic + BM25)
    ↓
Results returned with metadata and relevance scores
    ↓
Results added to LLM context window
    ↓
LLM generates response using RAG context
```

### 3.4 Go RAG Client Implementation

**File:** `api/pkg/rag/rag_haystack.go`

```go
type HaystackRAG struct {
    client   *http.Client
    endpoint string  // e.g., "http://haystack:8000"
}

// Create new client
func NewHaystackRAG(endpoint string) *HaystackRAG

// Index documents
func (h *HaystackRAG) Index(ctx context.Context, chunks []RAGChunk) error

// Query documents
func (h *HaystackRAG) Query(ctx context.Context, q RAGQuery) (*RAGQueryResult, error)

// Delete documents
func (h *HaystackRAG) Delete(ctx context.Context, req DeleteRequest) error
```

**Type Definitions** (`haystack_types.go`):
- `HaystackIndexRequest` - Index request structure
- `HaystackQueryRequest` - Query request structure
- `HaystackQueryResponse` - Query results structure
- `HaystackResult` - Individual search result

## 4. Configuration and Environment Variables

### 4.1 API Server Configuration

**File:** `api/pkg/config/config.go`

```go
type RAGConfig struct {
    Enabled   bool
    Provider  RAGProvider  // "haystack", "typesense", "llamaindex"
    Haystack  HaystackConfig
}

type HaystackConfig struct {
    Enabled bool
    URL     string  // "http://haystack:8000"
}
```

### 4.2 Environment Variables

#### API Server (docker-compose.dev.yaml)

| Variable | Default | Purpose |
|----------|---------|---------|
| `RAG_HAYSTACK_ENABLED` | `false` | Enable Haystack as RAG provider |
| `RAG_DEFAULT_PROVIDER` | `haystack` | Primary RAG provider selection |
| `RAG_HAYSTACK_URL` | `http://localhost:8000` | Haystack service endpoint |
| `TEXT_EXTRACTION_PROVIDER` | `tika` | Document extraction (tika, unstructured, haystack) |

#### Haystack Service (docker-compose.dev.yaml - lines 513-556)

| Variable | Default | Purpose |
|----------|---------|---------|
| `PGVECTOR_DSN` | - | PostgreSQL connection string |
| `LOG_LEVEL` | `INFO` | Python logging level |
| `VLLM_BASE_URL` | - | VLLM server for embeddings |
| `RAG_HAYSTACK_EMBEDDINGS_MODEL` | `MrLight/dse-qwen2-2b-mrl-v1` | Embedding model name |
| `RAG_HAYSTACK_EMBEDDINGS_DIM` | `1536` | Embedding vector dimensions |
| `RAG_HAYSTACK_CHUNK_SIZE` | `500` | Document chunk size (characters) |
| `RAG_HAYSTACK_CHUNK_OVERLAP` | `50` | Chunk overlap (characters) |
| `RAG_VISION_ENABLED` | `true` | Enable vision RAG |
| `RAG_VISION_BASE_URL` | - | Vision API endpoint |
| `RAG_VISION_EMBEDDINGS_MODEL` | `MrLight/dse-qwen2-2b-mrl-v1` | Vision embedding model |
| `HELIX_EMBEDDINGS_SOCKET` | `/socket/embeddings.sock` | UNIX socket for local embeddings |

### 4.3 Configuration via Installation Script

**File:** `install.sh`

When user runs `./install.sh --haystack`:
```bash
COMPOSE_PROFILES="haystack"
RAG_DEFAULT_PROVIDER="haystack"
RAG_HAYSTACK_ENABLED=true
```

Automatically enables Haystack with VectorChord backend.

## 5. Data Flow: Document Indexing

### 5.1 Detailed Indexing Sequence

```
1. User uploads knowledge source (PDF, DOCX, etc.)
   └─ Helix API receives file

2. Knowledge reconciliation triggered
   └─ api/pkg/controller/knowledge/knowledge_indexer.go

3. Document extraction phase
   └─ api/pkg/extract/haystack_extractor.go
   └─ Calls: HaystackRAG.Extract() → /extract endpoint
   └─ Returns: Raw text content

4. Prepare indexing request
   └─ Create RAGChunk with:
      ├─ content: extracted text
      ├─ metadata: {
      │   └─ data_entity_id: UUID (Helix knowledge entity)
      │   └─ document_id: unique doc identifier
      │   └─ source: original filename
      │   └─ custom_fields: user metadata
      └─ }

5. Index documents
   └─ HaystackRAG.Index(ctx, chunks)
   └─ HTTP POST: /process

6. Haystack processing pipeline
   └─ Converter: Parse format → text/images
   └─ Splitter: Chunk text (500 chars, 50 overlap)
   └─ Embedder: Generate vectors
      ├─ Option 1: VLLM server (RAG_HAYSTACK_EMBEDDINGS_BASE_URL)
      ├─ Option 2: UNIX socket (HELIX_EMBEDDINGS_SOCKET)
      └─ Model: MrLight/dse-qwen2-2b-mrl-v1 (1536 dims)
   └─ Store: VectorChord PostgreSQL

7. VectorChord storage
   └─ Table: haystack_documents
   └─ Columns:
      ├─ id: document chunk ID
      ├─ embedding: pgvector(1536)
      ├─ content: full text (BM25 indexing)
      ├─ metadata: JSON {data_entity_id, document_id, source, ...}
      └─ timestamps: created_at, updated_at

8. Helix knowledge store update
   └─ Record: document ID → Haystack mapping
   └─ Enable later deletion when knowledge removed
```

### 5.2 Metadata Structure

**Automatic Metadata (Set by Haystack):**
```json
{
  "document_id": "unique-doc-123",
  "document_group_id": "doc-group-456",
  "source": "deployment_guide.pdf",
  "filename": "deployment_guide.pdf",
  "original_filename": "/uploads/2024-12-01_deployment_guide.pdf",
  "content_offset": 0,
  "page_number": 1
}
```

**System Metadata (Set by Helix):**
```json
{
  "data_entity_id": "knowledge-uuid-789",
  "created_by": "user-id",
  "knowledge_source_id": "source-456"
}
```

**User Metadata:**
```json
{
  "tags": ["deployment", "kubernetes", "production"],
  "category": "documentation",
  "version": "v2.1",
  "custom_field": "custom_value"
}
```

## 6. Data Flow: Document Querying

### 6.1 Detailed Query Sequence

```
1. User sends message to assistant/agent
   └─ Text: "How do I deploy to Kubernetes?"

2. RAG context needed?
   └─ api/pkg/rag/rag.go checks if knowledge needed
   └─ Decision: Query RAG system

3. Build query request
   └─ Query text: "How do I deploy to Kubernetes?"
   └─ Filters (optional):
      ├─ data_entity_id: [uuid1, uuid2]  (which knowledge sources?)
      ├─ document_id: [doc1, doc2]       (specific documents?)
      └─ tags: ["deployment", ...]       (metadata filters)
   └─ top_k: 5  (return top 5 results)

4. Query Haystack
   └─ HaystackRAG.Query(ctx, query_request)
   └─ HTTP POST: /query

5. Haystack query pipeline
   └─ Embed query text
      ├─ VLLM: http://vllm:8000/v1/embeddings
      ├─ Socket: /socket/embeddings.sock
      └─ Model: Same as indexing (MrLight/dse-qwen2-2b-mrl-v1)

   └─ Retrieve via hybrid search
      ├─ Semantic search: Vector similarity (cosine)
      ├─ Keyword search: BM25 on document content
      └─ Hybrid score: Combination of both

   └─ Filter results
      ├─ data_entity_id IN (uuid1, uuid2)
      ├─ document_id IN (doc1, doc2)
      └─ tags CONTAINS ["deployment"]

   └─ Rank and limit
      ├─ Sort by relevance score (descending)
      ├─ Apply top_k limit
      └─ Return with metadata

6. Parse results
   └─ For each result:
      ├─ content: "Kubernetes deployment requires..."
      ├─ metadata: {data_entity_id, document_id, source, ...}
      ├─ score: 0.87 (similarity score)
      └─ chunk_id: "doc-123-chunk-5"

7. Add to LLM context
   └─ Format results as system/assistant message:
      "Based on the knowledge base:

       Source: deployment_guide.pdf (section 3)
       Kubernetes deployment requires..."

8. Generate response
   └─ LLM uses RAG context + user query
   └─ Produces informed answer
```

### 6.2 Filtering Examples

**Filter: Single data entity**
```json
{
  "filters": {
    "data_entity_id": ["abc-123-def"]
  }
}
```
→ Only documents from this knowledge source

**Filter: Multiple document IDs**
```json
{
  "filters": {
    "document_id": ["doc-1", "doc-2", "doc-3"]
  }
}
```
→ Only from these specific documents (OR logic)

**Filter: Complex (data_entity AND document_ids)**
```json
{
  "filters": {
    "data_entity_id": ["uuid"],
    "document_id": ["doc-1", "doc-2"]
  }
}
```
→ AND: From this entity AND from these specific documents

## 7. Document Store: VectorChord

### 7.1 Database Schema

**Container:** `pgvector` service
**Database:** PostgreSQL 17 with pgvector + BM25 extensions

```sql
-- Main documents table
CREATE TABLE haystack_documents (
    id TEXT PRIMARY KEY,
    embedding pgvector(1536) NOT NULL,      -- Vector representation
    content TEXT NOT NULL,                   -- Full text for BM25
    metadata JSONB DEFAULT '{}',             -- JSON metadata
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Indexes for performance
CREATE INDEX idx_embedding ON haystack_documents
    USING ivfflat (embedding vector_cosine_ops);  -- Vector similarity

CREATE INDEX idx_bm25 ON haystack_documents
    USING GIN(to_tsvector('english', content));   -- Full text search

CREATE INDEX idx_metadata ON haystack_documents
    USING GIN(metadata);                           -- JSON metadata

-- Vision documents (optional)
CREATE TABLE haystack_documents_vision (
    id TEXT PRIMARY KEY,
    embedding pgvector(1536),
    image_data BYTEA,
    metadata JSONB,
    created_at TIMESTAMP DEFAULT NOW()
);
```

### 7.2 Hybrid Search Mechanics

**Haystack's Hybrid Retriever:**
1. **Semantic Search (Vector Similarity)**
   - Query embedding vs document embeddings
   - Distance metric: cosine similarity
   - Normalized score: [0, 1]

2. **Keyword Search (BM25)**
   - Full-text search on document content
   - Term frequency + inverse document frequency
   - Normalized score: [0, 1]

3. **Hybrid Scoring**
   ```
   final_score = (semantic_score * weight1) + (bm25_score * weight2)
   Default: weight1 = 0.6, weight2 = 0.4
   ```

4. **Filtering**
   - SQL WHERE clause on metadata JSONB
   - Applied AFTER retrieval for efficiency

## 8. Embedding Generation

### 8.1 Embedding Options

#### Option 1: VLLM Server (GPU-Accelerated)
**Configuration:**
```yaml
RAG_HAYSTACK_EMBEDDINGS_BASE_URL: "http://vllm:8000"
RAG_HAYSTACK_EMBEDDINGS_MODEL: "MrLight/dse-qwen2-2b-mrl-v1"
RAG_HAYSTACK_EMBEDDINGS_DIM: 1536
```

**Flow:**
```
Document/Query text
    ↓
Haystack embedder (via requests library)
    ↓
HTTP request to VLLM /v1/embeddings
    ↓
VLLM (running on GPU)
    ↓
Vector (1536 dimensions)
```

**Benefits:**
- GPU acceleration
- Shared with other VLLM workloads
- Scalable to multiple inference servers

#### Option 2: UNIX Socket (Local Embedding)
**Configuration:**
```yaml
HELIX_EMBEDDINGS_SOCKET: "/socket/embeddings.sock"
```

**Flow:**
```
Document/Query text
    ↓
Haystack unix_socket_embedder
    ↓
UNIX socket to local embedding service
    ↓
Embedded Helix runner (on GPU)
    ↓
Vector (1536 dimensions)
```

**Benefits:**
- No network overhead
- Lower latency
- Integrated deployment

### 8.2 Embedding Model: MrLight/dse-qwen2-2b-mrl-v1

- **Type:** Dense Small Encoder (DSE)
- **Base:** Qwen2 2B parameters
- **Dimension:** 1536
- **Training:** Matryoshka Representation Learning
- **Strengths:** Lightweight + good quality
- **Max tokens:** 32,768 context window

## 9. Document Extraction

### 9.1 Extraction Process

**File:** `api/pkg/extract/haystack_extractor.go`

```go
type HaystackExtractor struct {
    haystackURL string
}

func (he *HaystackExtractor) Extract(ctx context.Context, file io.Reader,
    filename string) (string, error)
    // Calls: POST /extract
    // Returns: Raw text content
```

**Supported Formats:**
- **Documents:** PDF, DOCX, PPTX, XLS, ODS, RTF, TXT
- **Images:** PNG, JPG, GIF, BMP, WebP
- **Archives:** ZIP (for multi-file extraction)
- **Code:** Source files (with syntax preservation)

**Extraction Pipeline:**
1. **Format Detection** → Determine file type
2. **Format-Specific Parsing** → Use appropriate parser (PyMuPDF for PDF, python-docx for DOCX, etc.)
3. **Layout Preservation** → Maintain structure where meaningful
4. **Text Cleaning** → Remove artifacts, normalize whitespace
5. **Return** → Raw text to Helix

## 10. Vision RAG (Optional)

### 10.1 Vision-Specific Features

**Enabled by:** `RAG_VISION_ENABLED=true`

**Capabilities:**
- Index images and diagrams
- Query with natural language
- Retrieve visually similar images
- Extract text from images (OCR)

**Vision Service Components:**

```python
# service_image.py
class VisionHaystackService(HaystackService):
    def __init__(self, vision_config: VisionConfig)
        # Initialize vision embedder
        # Create haystack_documents_vision table

    async def index_vision(self, request: VisionIndexRequest):
        # Process images → generate vision embeddings

    async def query_vision(self, request: VisionQueryRequest):
        # Query indexed images
```

**Vision API Endpoints:**
- `POST /process-vision` - Index images
- `POST /query-vision` - Query images
- `POST /extract` - OCR text from images

**Vision Embeddings:**
- Model: `RAG_VISION_EMBEDDINGS_MODEL` (default: MrLight/dse-qwen2-2b-mrl-v1)
- Base URL: `RAG_VISION_BASE_URL`
- Support: Multimodal queries ("Show diagrams of...")

## 11. Data Management: Deletion Workflow

### 11.1 Document Deletion

**Trigger:** Knowledge source removed from Helix

**Flow:**
```
User deletes knowledge source
    ↓
api/pkg/store/store_knowledge.go
    ↓
Delete knowledge record from Helix DB
    ↓
Call HaystackRAG.Delete()
    ↓
HTTP POST: /delete
{
  "filters": {
    "data_entity_id": "uuid-of-deleted-knowledge"
  }
}
    ↓
Haystack deletes ALL documents with matching data_entity_id
    ↓
SQL DELETE from haystack_documents
    WHERE metadata->>'data_entity_id' = 'uuid'
    ↓
VectorChord removes vectors + metadata
```

**Deletion Strategy:**
- **Primary filter:** `data_entity_id` (bulk delete entire knowledge sources)
- **Secondary filter:** `document_id` (delete specific documents)
- **Implementation:** SQL WHERE clause on JSONB metadata

## 12. Docker Compose Integration

### 12.1 Service Definition

**File:** `docker-compose.dev.yaml` (lines 513-556)

```yaml
haystack:
    build:
        context: ./haystack_service
        dockerfile: Dockerfile
    restart: always
    environment:
        # Database
        - PGVECTOR_DSN=postgresql://postgres:postgres@pgvector:5432/postgres

        # Embeddings
        - VLLM_BASE_URL=${RAG_HAYSTACK_EMBEDDINGS_BASE_URL:-}
        - RAG_HAYSTACK_EMBEDDINGS_MODEL=${RAG_HAYSTACK_EMBEDDINGS_MODEL:-MrLight/dse-qwen2-2b-mrl-v1}
        - RAG_HAYSTACK_EMBEDDINGS_DIM=${RAG_HAYSTACK_EMBEDDINGS_DIM:-1536}

        # Chunking
        - RAG_HAYSTACK_CHUNK_SIZE=${RAG_HAYSTACK_CHUNK_SIZE:-500}
        - RAG_HAYSTACK_CHUNK_OVERLAP=${RAG_HAYSTACK_CHUNK_OVERLAP:-50}

        # Vision
        - RAG_VISION_ENABLED=${RAG_VISION_ENABLED:-true}
        - RAG_VISION_BASE_URL=${RAG_VISION_BASE_URL:-}

        # Socket
        - HELIX_EMBEDDINGS_SOCKET=${HELIX_EMBEDDINGS_SOCKET:-/socket/embeddings.sock}

        # Logging
        - LOG_LEVEL=INFO

    volumes:
        - ./haystack_service/app:/app/app
        - ./haystack_service/main.py:/app/main.py
        - helix-socket:/socket

    depends_on:
        - pgvector
        - api

    networks:
        - helix_default

    # Port commented out for production
    # ports:
    #   - 8001:8000
```

### 12.2 Network and Dependencies

**Network:** `helix_default` (shared with api, pgvector services)

**Accessible as:** `http://haystack:8000` from other containers

**Dependencies:**
- `pgvector` - PostgreSQL 17 with pgvector + BM25
- `api` - Helix API server (for WebSocket embeddings if needed)

**Volumes:**
- `./haystack_service/app:/app/app` - Hot reload Python code
- `helix-socket:/socket` - Shared UNIX socket for embeddings
- Docker volume `helix-socket` defined at docker-compose level

### 12.3 Docker Compose Profiles

When using `--haystack` flag:
```bash
COMPOSE_PROFILES="haystack"
```

This enables Haystack + pgvector services in docker-compose.

## 13. Dependencies and Build

### 13.1 Python Dependencies (haystack_service/pyproject.toml)

```
Core:
  - haystack-ai==2.11.2                  # Haystack framework
  - pgvector-haystack==3.1.0             # PostgreSQL integration
  - fastapi>=0.115.11                    # Web API
  - uvicorn>=0.34.0                      # ASGI server

Document Processing:
  - unstructured[all-docs]>=0.17.2       # Format parsing
  - pymupdf>=1.25.4                      # PDF extraction
  - python-docx>=1.0.0                   # DOCX parsing
  - python-pptx>=0.6.21                  # PPTX parsing

Embeddings & AI:
  - sentence-transformers>=2.3.0         # Local embeddings
  - requests>=2.31.0                     # HTTP client

NLP & Text:
  - nltk>=3.9.1                          # Text utilities
  - tiktoken>=0.5.2                      # Tokenization

Utils:
  - python-dotenv>=1.0.0                 # Environment config
  - pydantic>=2.4.2                      # Data validation
```

### 13.2 Go Dependencies

**In `api/go.mod`:**
```
Standard Go libraries only
(HTTP client, JSON marshaling, logging)

No direct Haystack dependency needed
(Communication via HTTP REST API)
```

## 14. Operational Considerations

### 14.1 Prerequisites

**Before enabling Haystack:**
1. PostgreSQL 17 (pgvector + BM25 available)
2. VectorChord container running
3. VLLM service OR UNIX socket embeddings available
4. 100GB+ disk for pgvector database (for large knowledge bases)
5. GPU memory: Embeddings models need ~4GB VRAM

### 14.2 Performance Characteristics

| Operation | Time | Notes |
|-----------|------|-------|
| Index 100 pages (PDF) | 30-60s | Depends on GPU availability |
| Query 1M documents | <100ms | VectorChord is highly optimized |
| Delete 1000 documents | <1s | Single SQL DELETE |
| Embedding generation | 5-10ms per 500-token chunk | VLLM with GPU |

### 14.3 Scaling Considerations

**Vertical Scaling:**
- Increase pgvector server memory (currently moderate)
- Multiple embeddings workers (VLLM replicas)
- GPU allocation for Haystack container

**Horizontal Scaling:**
- Multiple Haystack service instances
- Load balancer (reverse proxy)
- Shared pgvector backend
- Cache query results (Redis)

## 15. Testing and Quality Assurance

### 15.1 Unit Tests

**File:** `api/pkg/rag/rag_haystack_test.go`

Tests cover:
- Single document ID filtering
- Multiple document ID filtering (OR logic)
- Data entity filtering
- Metadata preservation in results
- Query normalization
- NUL byte handling

### 15.2 Integration Tests

Recommended test scenarios:
1. Upload PDF → Index → Query → Verify results
2. Multiple documents with filters
3. Delete and verify removal
4. Vision indexing and retrieval (if enabled)
5. Network failure handling (Haystack service down)

### 15.3 Logging and Monitoring

**Haystack logs:**
```bash
docker compose -f docker-compose.dev.yaml logs haystack
```

**Key log patterns:**
- `Indexing pipeline started` - Indexing begun
- `Embedding generation completed` - Vectors created
- `Documents stored: N` - Storage complete
- `Query processed: N results` - Query complete
- Errors: Connection failures, parsing issues

## 16. Known Issues and Limitations

### 16.1 Current Limitations

1. **Vision embedding model:** Uses text model (MrLight), not specialized vision model
2. **Chunk size tuning:** Fixed to 500 chars, not adaptive per document type
3. **Metadata search:** Only filtering, not semantic metadata search
4. **Result deduplication:** No automatic duplicate removal
5. **Ranking customization:** Limited control over semantic vs BM25 weighting

### 16.2 Future Improvements

1. **Specialized vision embeddings** → Better image retrieval
2. **Dynamic chunking** → Optimize per document type
3. **Semantic metadata search** → Query by topics/tags
4. **Caching layer** → Redis for frequent queries
5. **Multi-language support** → Non-English documents
6. **Custom ranking** → User-defined relevance tuning

## 17. Troubleshooting

### 17.1 Common Issues

**Issue: Haystack service won't start**
```
Solution: Check pgvector connectivity
docker compose -f docker-compose.dev.yaml logs haystack | grep "connection"
Verify PGVECTOR_DSN environment variable is correct
```

**Issue: Embedding generation fails**
```
Solution: Check VLLM service availability
docker compose -f docker-compose.dev.yaml exec haystack curl http://vllm:8000/health
Or check UNIX socket: ls -la /path/to/embeddings.sock
```

**Issue: Query returns no results**
```
Solutions:
1. Verify documents were indexed: Check pgvector database directly
2. Check filter criteria are correct
3. Ensure embedding model matches during query
4. Check Haystack logs for errors
```

**Issue: Performance is slow**
```
Solutions:
1. Check pgvector IVFFlat index: ANALYZE table
2. Increase work_mem in PostgreSQL
3. Add more replicas of Haystack service
4. Cache frequent queries
```

## 18. Architecture Decisions

### 18.1 Why Haystack as Microservice?

**Decision:** Separate Python service instead of Go library

**Rationale:**
1. **Language fit:** Document processing libraries better in Python
2. **Isolation:** Failures in extraction don't crash API server
3. **Scalability:** Haystack can be replicated independently
4. **Simplicity:** REST API is language-agnostic
5. **Operational:** Can update Haystack without redeploying API

### 18.2 Why VectorChord (PostgreSQL) as Document Store?

**Decision:** Instead of pure vector DB (Pinecone, Milvus, etc.)

**Rationale:**
1. **Hybrid search:** BM25 + semantic in same database
2. **Metadata filtering:** JSONB provides flexible filtering
3. **Integration:** Single database with application data
4. **Cost:** Self-hosted, no vendor lock-in
5. **Compliance:** Data stays on-premise/controlled

### 18.3 Why Hybrid Search (Semantic + BM25)?

**Decision:** Instead of pure semantic search

**Rationale:**
1. **Recall:** BM25 catches exact keyword matches semantic misses
2. **Precision:** Semantic catches meaning semantic BM25 misses
3. **Robustness:** Graceful degradation if one fails
4. **Best of both:** Combines strengths

## 19. References and Resources

### Documentation
- Haystack: https://docs.haystack.deepset.ai/
- VectorChord: https://vectorchord.io/
- pgvector: https://github.com/pgvector/pgvector
- FastAPI: https://fastapi.tiangolo.com/

### Code Files
- Haystack service: `/haystack_service/`
- Helix RAG client: `/api/pkg/rag/rag_haystack.go`
- Knowledge indexing: `/api/pkg/controller/knowledge/`
- Tests: `/api/pkg/rag/rag_haystack_test.go`

### Configuration
- docker-compose: `/docker-compose.dev.yaml`
- Installation: `/install.sh`
- Helix config: `/api/pkg/config/config.go`

## 20. Conclusion

Haystack provides Helix with a production-grade RAG system capable of:
- **Semantic search** via embedding-based retrieval
- **Keyword search** via BM25 full-text indexing
- **Hybrid ranking** combining both approaches
- **Rich metadata** filtering on JSONB
- **Vision support** for image indexing/querying
- **GPU acceleration** for embeddings
- **Enterprise scalability** with PostgreSQL backend

The integration as a microservice maintains separation of concerns while providing a clean HTTP API for seamless Helix integration.
