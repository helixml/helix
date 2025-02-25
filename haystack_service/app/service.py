import os
import tempfile
import logging
import httpx
from typing import List, Dict, Any, Optional, Union, BinaryIO

from haystack import Document, Pipeline
from haystack_integrations.document_stores.pgvector import PgvectorDocumentStore
from haystack.components.preprocessors import DocumentSplitter
from haystack_integrations.components.retrievers.pgvector import PgvectorEmbeddingRetriever
from haystack.components.embedders import OpenAIDocumentEmbedder
from unstructured.partition.auto import partition
from unstructured.documents.elements import (
    Title, ListItem, Header, Footer, Table, Image
)

from .config import settings

# Configure logging
logging.basicConfig(level=getattr(logging, settings.LOG_LEVEL))
logger = logging.getLogger(__name__)

class UnstructuredConverter:
    """Converts documents to text using unstructured"""
    
    def _element_to_markdown(self, element) -> str:
        """Convert an unstructured element to markdown format"""
        if not str(element).strip():
            return ""
            
        text = str(element).strip()
        
        if isinstance(element, Title):
            return f"# {text}"
        elif isinstance(element, Header):
            return f"## {text}"
        elif isinstance(element, ListItem):
            return f"- {text}"
        elif isinstance(element, Table):
            # Basic table formatting - could be enhanced
            return f"**Table**: {text}"
        elif isinstance(element, Image):
            return f"![Image]{text}"
        elif isinstance(element, Footer):
            return f"*{text}*"
        else:  # NarrativeText, Text, etc
            return text
    
    def convert(self, file: Union[str, BinaryIO], metadata: Dict[str, Any] = None) -> List[Document]:
        """Convert a file to text using unstructured"""
        logger.info("Converting file to text")
        
        try:
            # If file is a string (path), use it directly, otherwise save to temp file
            if isinstance(file, str):
                elements = partition(filename=file)
            else:
                with tempfile.NamedTemporaryFile(delete=False) as temp:
                    # If file is a BytesIO, get its content
                    if hasattr(file, 'read'):
                        content = file.read()
                        if isinstance(content, str):
                            content = content.encode('utf-8')
                        temp.write(content)
                    else:
                        temp.write(file)
                    temp_path = temp.name
                try:
                    elements = partition(filename=temp_path)
                finally:
                    os.unlink(temp_path)
            
            markdown_elements = [
                self._element_to_markdown(el) 
                for el in elements
            ]
            # Filter out empty strings and join with double newlines
            text = "\n\n".join(el for el in markdown_elements if el)
            
            if not text.strip():
                logger.warning("No text extracted from file")
                return []
            
            logger.info(f"Extracted {len(text)} characters")
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
                table_name=settings.PGVECTOR_TABLE,
                vector_function="cosine_similarity",
                search_strategy="hnsw",
                recreate_table=True
            )
            logger.info(f"Connected to PgvectorDocumentStore: {settings.PGVECTOR_TABLE}")
        except Exception as e:
            logger.error(f"Failed to connect to PgvectorDocumentStore: {str(e)}")
            raise
        
        # Initialize components
        self.embedder = OpenAIDocumentEmbedder(
            api_key=settings.VLLM_API_KEY,
            api_base_url=settings.VLLM_BASE_URL,
            model_name=settings.RAG_HAYSTACK_EMBEDDINGS_MODEL
        )
        self.converter = UnstructuredConverter()
        self.splitter = DocumentSplitter(
            split_length=settings.CHUNK_SIZE,
            split_overlap=settings.CHUNK_OVERLAP,
            add_split_info=True
        )
        
        logger.info(f"Initialized DocumentSplitter with chunk_size={settings.CHUNK_SIZE}, overlap={settings.CHUNK_OVERLAP}")
        
        # Initialize retriever
        self.retriever = PgvectorEmbeddingRetriever(
            document_store=self.document_store,
            filters=None,
            top_k=5
        )
        
        logger.info("HaystackService initialization complete")
    
    async def extract_text(self, file: Optional[Union[str, BinaryIO]] = None, url: Optional[str] = None) -> str:
        """Extract text from a file or URL without indexing it"""
        logger.info(f"Extracting text from file or URL")
        
        if file is not None:
            # Extract from file
            documents = self.converter.convert(file)
            if not documents:
                return ""
            return documents[0].content
        elif url:
            # Extract from URL
            return await self.converter.extract_text_from_url(url)
        else:
            raise ValueError("Either file or url must be provided")
    
    async def process_and_index(self, file: Union[str, BinaryIO], metadata: Dict[str, Any] = None) -> Dict[str, Any]:
        """Process a document and index it in the document store"""
        logger.info(f"Processing and indexing file with metadata: {metadata}")
        
        # Convert document
        documents = self.converter.convert(file, metadata)
        
        if not documents:
            logger.warning("No documents to index")
            return {"status": "warning", "message": "No content extracted from file"}
        
        # Split into chunks
        chunks = []
        for doc in documents:
            chunks.extend(self.splitter.split(doc))
        
        logger.info(f"Split document into {len(chunks)} chunks")
        
        # Generate embeddings and store
        embeddings = await self.embedder.run(documents=chunks)
        chunks = embeddings["documents"]
        
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
        query_result = await self.embedder.run(text=query_text)
        query_embedding = query_result["embedding"]
        
        # Retrieve documents
        results = self.retriever.retrieve(
            query_embedding=query_embedding,
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