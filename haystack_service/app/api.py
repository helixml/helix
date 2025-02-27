import os
import json
import tempfile
import logging
from typing import Dict, Any, List, Optional

from fastapi import FastAPI, UploadFile, File, Form, HTTPException, Depends
from pydantic import BaseModel

from .service import HaystackService
from .config import settings

# Configure logging
logger = logging.getLogger(__name__)

# API models
class QueryRequest(BaseModel):
    query: str
    filters: Optional[Dict[str, Any]] = None
    top_k: int = 5

class DeleteRequest(BaseModel):
    filters: Dict[str, Any]

class QueryResult(BaseModel):
    content: str
    metadata: Dict[str, Any]
    score: float

class QueryResponse(BaseModel):
    results: List[QueryResult]

class ProcessResponse(BaseModel):
    status: str
    documents_processed: Optional[int] = None
    chunks_indexed: Optional[int] = None
    message: Optional[str] = None

class DeleteResponse(BaseModel):
    status: str
    documents_deleted: int

class ExtractResponse(BaseModel):
    text: str

class HealthResponse(BaseModel):
    status: str = "healthy"
    version: str = "1.0.0"

# Create FastAPI app
app = FastAPI(
    title="Haystack RAG Service",
    description="RAG service using Haystack for document processing and retrieval",
    version="1.0.0"
)

# Service instance
service: Optional[HaystackService] = None

# Dependency to get service instance
async def get_service():
    global service
    if service is None:
        service = HaystackService()
    return service

@app.post("/process", response_model=ProcessResponse)
async def process_file(
    file: UploadFile = File(...),
    metadata: Optional[str] = Form(None),
    service: HaystackService = Depends(get_service)
):
    """Process and index a file"""
    logger.info(f"Received file: {file.filename}")
    
    # Parse metadata if provided
    meta_dict = {}
    if metadata:
        try:
            meta_dict = json.loads(metadata)
            logger.debug(f"Parsed metadata: {meta_dict}")
        except json.JSONDecodeError:
            logger.error("Invalid metadata JSON")
            raise HTTPException(status_code=400, detail="Invalid metadata JSON")
    
    # Add filename to metadata
    meta_dict["filename"] = file.filename
    
    # Get file extension
    _, ext = os.path.splitext(file.filename)
    
    # Save file temporarily
    with tempfile.NamedTemporaryFile(delete=False, suffix=ext) as temp:
        content = await file.read()
        temp.write(content)
        temp_path = temp.name
    
    try:
        # Process and index
        result = await service.process_and_index(temp_path, meta_dict)
        return result
    except Exception as e:
        logger.error(f"Error processing file: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Error processing file: {str(e)}")
    finally:
        # Clean up
        os.unlink(temp_path)

@app.post("/extract", response_model=ExtractResponse)
async def extract_text(
    file: UploadFile = File(...),
    service: HaystackService = Depends(get_service)
):
    """Extract text from a file without indexing"""
    logger.info(f"Extract request for file: {file.filename}")
    
    # Get file extension
    _, ext = os.path.splitext(file.filename)
    
    # Save file temporarily
    with tempfile.NamedTemporaryFile(delete=False, suffix=ext) as temp:
        content = await file.read()
        temp.write(content)
        temp_path = temp.name
    
    try:
        # Extract text
        text = await service.extract_text(temp_path)
        return {"text": text}
    except Exception as e:
        logger.error(f"Error extracting text: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Error extracting text: {str(e)}")
    finally:
        # Clean up
        os.unlink(temp_path)

@app.post("/query", response_model=QueryResponse)
async def query(
    request: QueryRequest,
    service: HaystackService = Depends(get_service)
):
    """Query for relevant documents"""
    
    try:
        results = await service.query(
            query_text=request.query,
            filters=request.filters,
            top_k=request.top_k
        )
        return {"results": results}
    except Exception as e:
        logger.error(f"Error querying: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Error querying: {str(e)}")

@app.post("/delete", response_model=DeleteResponse)
async def delete(
    request: DeleteRequest,
    service: HaystackService = Depends(get_service)
):
    """Delete documents based on filters"""
    logger.info(f"Delete request with filters: {request.filters}")
    
    try:
        result = await service.delete(filters=request.filters)
        return result
    except Exception as e:
        logger.error(f"Error deleting documents: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Error deleting documents: {str(e)}")

@app.get("/health", response_model=HealthResponse)
async def health():
    """Health check endpoint"""
    return {"status": "healthy", "version": "1.0.0"} 