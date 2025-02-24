import os
import tempfile
import logging
import httpx
from typing import List, Dict, Any, Optional, Union

from haystack import Document, Pipeline
from haystack_integrations.document_stores.pgvector import PgvectorDocumentStore
from haystack.components import TextSplitter
from haystack.components.retrievers import EmbeddingRetriever
from unstructured.partition.auto import partition

from .config import settings

# Configure logging
logging.basicConfig(level=getattr(logging, settings.LOG_LEVEL))
logger = logging.getLogger(__name__)

class HelixEmbedder:
    """Custom embedder that uses Helix API for embeddings"""
    
    def __init__(self, api_url: str):
        self.api_url = api_url
        self.client = httpx.AsyncClient(timeout=60.0)
        logger.info(f"Initialized HelixEmbedder with API URL: {api_url}")
    
    async def embed(self, texts: List[str]) -> List[List[float]]:
        """Generate embeddings for a list of texts using Helix API"""
        logger.debug(f"Generating embeddings for {len(texts)} texts")
        
        try:
            response = await self.client.post(
                f"{self.api_url}/embeddings",
                json={"input": texts}
            )
            response.raise_for_status()
            
            result = response.json()
            embeddings = [item["embedding"] for item in result["data"]]
            
            logger.debug(f"Successfully generated {len(embeddings)} embeddings")
            return embeddings
            
        except Exception as e:
            logger.error(f"Error generating embeddings: {str(e)}")
            raise RuntimeError(f"Embedding API error: {str(e)}")

class UnstructuredConverter:
    """Converts documents to text using unstructured"""
    
    def convert(self, file_path: str, metadata: Dict[str, Any] = None) -> List[Document]:
        """Convert a file to text using unstructured"""
        logger.info(f"Converting file: {file_path}")
        
        try:
            elements = partition(filename=file_path)
            text = "\n\n".join(str(el) for el in elements if str(el).strip())
            
            if not text.strip():
                logger.warning(f"No text extracted from file: {file_path}")
                return []
            
            logger.info(f"Extracted {len(text)} characters from file")
            return [Document(content=text, metadata=metadata or {})]
            
        except Exception as e:
            logger.error(f"Document conversion error: {str(e)}")
            raise RuntimeError(f"Document conversion error: {str(e)}")
    
    async def extract_text_from_url(self, url: str) -> str:
        """Extract text from a URL"""
        logger.info(f"Extracting text from URL: {url}")
        
        try:
            async with httpx.AsyncClient(timeout=30.0) as client:
                response = await client.get(url)
                response.raise_for_status()
                
                # Save content to a temporary file
                with tempfile.NamedTemporaryFile(delete=False) as temp:
                    temp.write(response.content)
                    temp_path = temp.name
                
                try:
                    # Use the converter to extract text
                    docs = self.convert(temp_path)
                    if not docs:
                        return ""
                    return docs[0].content
                finally:
                    # Clean up
                    os.unlink(temp_path)
                    
        except Exception as e:
            logger.error(f"URL extraction error: {str(e)}")
            raise RuntimeError(f"URL extraction error: {str(e)}")

class HaystackService:
    """Main service class for Haystack RAG operations"""
    
    def __init__(self):
        """Initialize the Haystack service"""
        logger.info("Initializing HaystackService")
        
        # Initialize document store
        try:
            self.document_store = PgvectorDocumentStore(
                connection_string=settings.PGVECTOR_DSN,
                embedding_dimension=settings.EMBEDDING_DIM,
                table_name=settings.PGVECTOR_TABLE
            )
            logger.info(f"Connected to PgvectorDocumentStore: {settings.PGVECTOR_TABLE}")
        except Exception as e:
            logger.error(f"Failed to connect to PgvectorDocumentStore: {str(e)}")
            raise
        
        # Initialize components
        self.embedder = HelixEmbedder(settings.HELIX_API_URL)
        self.converter = UnstructuredConverter()
        self.splitter = TextSplitter(
            split_by="word",
            split_length=settings.CHUNK_SIZE,
            split_overlap=settings.CHUNK_OVERLAP
        )
        
        logger.info(f"Initialized TextSplitter with chunk_size={settings.CHUNK_SIZE}, overlap={settings.CHUNK_OVERLAP}")
        
        # Initialize retriever
        self.retriever = EmbeddingRetriever(
            document_store=self.document_store,
            embedding_model=self.embedder
        )
        
        logger.info("HaystackService initialization complete")
    
    async def extract_text(self, file_path: Optional[str] = None, url: Optional[str] = None) -> str:
        """Extract text from a file or URL without indexing it"""
        logger.info(f"Extracting text from file_path={file_path}, url={url}")
        
        if file_path:
            # Extract from file
            documents = self.converter.convert(file_path)
            if not documents:
                return ""
            return documents[0].content
        elif url:
            # Extract from URL
            return await self.converter.extract_text_from_url(url)
        else:
            raise ValueError("Either file_path or url must be provided")
    
    async def process_and_index(self, file_path: str, metadata: Dict[str, Any] = None) -> Dict[str, Any]:
        """Process a document and index it in the document store"""
        logger.info(f"Processing and indexing file: {file_path} with metadata: {metadata}")
        
        # Convert document
        documents = self.converter.convert(file_path, metadata)
        
        if not documents:
            logger.warning("No documents to index")
            return {"status": "warning", "message": "No content extracted from file"}
        
        # Split into chunks
        chunks = []
        for doc in documents:
            chunks.extend(self.splitter.split(doc))
        
        logger.info(f"Split document into {len(chunks)} chunks")
        
        # Generate embeddings and store
        embeddings = await self.embedder.embed([chunk.content for chunk in chunks])
        for chunk, embedding in zip(chunks, embeddings):
            chunk.embedding = embedding
        
        # Store in database
        self.document_store.write_documents(chunks)
        
        logger.info(f"Successfully indexed {len(chunks)} chunks")
        return {
            "status": "success",
            "documents_processed": len(documents),
            "chunks_indexed": len(chunks)
        }
    
    async def query(self, query_text: str, filters: Dict[str, Any] = None, top_k: int = 5) -> List[Dict[str, Any]]:
        """Query the document store for relevant documents"""
        logger.info(f"Querying with: '{query_text}', filters: {filters}, top_k: {top_k}")
        
        # Generate query embedding
        query_embedding = await self.embedder.embed([query_text])
        
        # Retrieve documents
        results = self.retriever.retrieve(
            query_embedding=query_embedding[0],
            filters=filters,
            top_k=top_k
        )
        
        logger.info(f"Retrieved {len(results)} results")
        
        # Format results
        formatted_results = [
            {
                "content": doc.content,
                "metadata": doc.metadata,
                "score": doc.score
            }
            for doc in results
        ]
        
        return formatted_results
    
    async def delete(self, filters: Dict[str, Any]) -> Dict[str, Any]:
        """Delete documents from the document store based on filters"""
        logger.info(f"Deleting documents with filters: {filters}")
        
        deleted = self.document_store.delete_documents(filters=filters)
        
        logger.info(f"Deleted {deleted} documents")
        return {"status": "success", "documents_deleted": deleted} 