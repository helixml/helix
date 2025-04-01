import os
import json
import tempfile
import logging
from typing import Dict, Any, List, Optional

from fastapi import FastAPI, UploadFile, File, Form, HTTPException, Depends
from pydantic import BaseModel

from .service import HaystackService
from .service_image import HaystackImageService
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
    version="1.0.0",
)


@app.on_event("startup")
async def startup_event():
    """Initialize services on startup"""
    app.state.haystack_service = HaystackService()

    app.state.image_service = None
    if settings.VISION_ENABLED:
        app.state.image_service = HaystackImageService()


# Dependency to get service instance
async def get_service():
    return app.state.haystack_service


# Dependency to get image service instance
async def get_image_service():
    return app.state.image_service


@app.post("/process-vision", response_model=ProcessResponse)
async def process_vision(
    file: UploadFile = File(...),
    metadata: Optional[str] = Form(None),
    image_service: HaystackImageService = Depends(get_image_service),
):
    """Process and index an image"""
    logger.info(f"Received image for vision processing: {file.filename}")
    if not settings.VISION_ENABLED:
        raise HTTPException(status_code=400, detail="Vision RAG is not enabled")

    # Parse metadata if provided
    meta_dict = {}
    if metadata:
        try:
            meta_dict = json.loads(metadata)
            logger.debug(f"Parsed metadata: {meta_dict}")
        except json.JSONDecodeError:
            logger.error("Invalid metadata JSON")
            raise HTTPException(status_code=400, detail="Invalid metadata JSON")

    # Get file extension
    _, ext = os.path.splitext(file.filename)

    # Read the file content
    content = await file.read()

    # Check for empty content
    if not content:
        logger.error("Empty file content received")
        raise HTTPException(
            status_code=422,
            detail="Input validation error: File content cannot be empty",
        )

    # For binary files like PDFs, we should NOT sanitize content as it will corrupt the file
    # PDF files and other binary formats may contain NUL bytes as part of their format
    # NUL bytes will be handled after text extraction in the converter

    # Only check if the content is ONLY NUL bytes (which would be invalid)
    if content == b"\x00" * len(content):
        logger.error("File contained only NUL bytes")
        raise HTTPException(
            status_code=422,
            detail="Input validation error: File content cannot be empty (contained only NUL bytes)",
        )

    # Save file temporarily with original binary content intact
    with tempfile.NamedTemporaryFile(delete=False, suffix=ext) as temp:
        temp.write(content)
        temp_path = temp.name

    try:
        # Set the original filename in metadata - this is the only field we need
        meta_dict["filename"] = file.filename

        # Process and index
        result = await image_service.process_and_index(temp_path, meta_dict)

        # Ensure response matches ProcessResponse schema with a status field
        response = {
            "status": "success",
            "documents_processed": 1,
            "chunks_indexed": result.get("chunks", 0),
            "message": f"Successfully processed {file.filename}",
        }
        return response
    except Exception as e:
        logger.error(f"Error processing file: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Error processing file: {str(e)}")
    finally:
        # Clean up
        os.unlink(temp_path)


@app.post("/process", response_model=ProcessResponse)
async def process_file(
    file: UploadFile = File(...),
    metadata: Optional[str] = Form(None),
    service: HaystackService = Depends(get_service),
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

    # Get file extension
    _, ext = os.path.splitext(file.filename)

    # Read the file content
    content = await file.read()

    # Check for empty content
    if not content:
        logger.error("Empty file content received")
        raise HTTPException(
            status_code=422,
            detail="Input validation error: File content cannot be empty",
        )

    # For binary files like PDFs, we should NOT sanitize content as it will corrupt the file
    # PDF files and other binary formats may contain NUL bytes as part of their format
    # NUL bytes will be handled after text extraction in the converter

    # Only check if the content is ONLY NUL bytes (which would be invalid)
    if content == b"\x00" * len(content):
        logger.error("File contained only NUL bytes")
        raise HTTPException(
            status_code=422,
            detail="Input validation error: File content cannot be empty (contained only NUL bytes)",
        )

    # Save file temporarily with original binary content intact
    with tempfile.NamedTemporaryFile(delete=False, suffix=ext) as temp:
        temp.write(content)
        temp_path = temp.name

    try:
        # Set the original filename in metadata - this is the only field we need
        meta_dict["filename"] = file.filename

        # Process and index
        result = await service.process_and_index(temp_path, meta_dict)

        # Ensure response matches ProcessResponse schema with a status field
        response = {
            "status": "success",
            "documents_processed": 1,
            "chunks_indexed": result.get("chunks", 0),
            "message": f"Successfully processed {file.filename}",
        }
        return response
    except Exception as e:
        logger.error(f"Error processing file: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Error processing file: {str(e)}")
    finally:
        # Clean up
        os.unlink(temp_path)


@app.post("/extract", response_model=ExtractResponse)
async def extract_text(
    file: UploadFile = File(...), service: HaystackService = Depends(get_service)
):
    """Extract text from a file without indexing"""
    logger.info(f"Extract request for file: {file.filename}")

    # Get file extension
    _, ext = os.path.splitext(file.filename)

    # Read the file content
    content = await file.read()

    # Check for empty content
    if not content:
        logger.error("Empty file content received")
        raise HTTPException(
            status_code=422,
            detail="Input validation error: File content cannot be empty",
        )

    # For binary files like PDFs, we should NOT sanitize content as it will corrupt the file
    # PDF files and other binary formats may contain NUL bytes as part of their format
    # NUL bytes will be handled after text extraction in the converter

    # Save file temporarily with original binary content intact
    with tempfile.NamedTemporaryFile(delete=False, suffix=ext) as temp:
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


@app.post("/query-vision", response_model=QueryResponse)
async def query_vision(
    request: QueryRequest,
    image_service: HaystackImageService = Depends(get_image_service),
):
    """Query for relevant documents"""

    if not settings.VISION_ENABLED:
        raise HTTPException(status_code=400, detail="Vision RAG is not enabled")

    try:
        # Check for empty query text
        if not request.query or request.query.strip() == "":
            raise HTTPException(
                status_code=422,
                detail="Input validation error: `query` cannot be empty",
            )

        # Remove NUL bytes from query if present
        sanitized_query = request.query.replace("\x00", "")
        if sanitized_query != request.query:
            logger.warning("Query contained NUL bytes that were removed")

        # Check again for emptiness after sanitizing
        if not sanitized_query or sanitized_query.strip() == "":
            logger.error("Query contained only NUL bytes")
            raise HTTPException(
                status_code=422,
                detail="Input validation error: `query` cannot be empty (contained only NUL bytes)",
            )

        results = await image_service.query(
            query_text=sanitized_query, filters=request.filters, top_k=request.top_k
        )
        return {"results": results}
    except HTTPException:
        # Re-raise HTTP exceptions
        raise
    except Exception as e:
        logger.error(f"Error querying: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Error querying: {str(e)}")


@app.post("/query", response_model=QueryResponse)
async def query(request: QueryRequest, service: HaystackService = Depends(get_service)):
    """Query for relevant documents"""

    try:
        # Check for empty query text
        if not request.query or request.query.strip() == "":
            raise HTTPException(
                status_code=422,
                detail="Input validation error: `query` cannot be empty",
            )

        # Remove NUL bytes from query if present
        sanitized_query = request.query.replace("\x00", "")
        if sanitized_query != request.query:
            logger.warning("Query contained NUL bytes that were removed")

        # Check again for emptiness after sanitizing
        if not sanitized_query or sanitized_query.strip() == "":
            logger.error("Query contained only NUL bytes")
            raise HTTPException(
                status_code=422,
                detail="Input validation error: `query` cannot be empty (contained only NUL bytes)",
            )

        results = await service.query(
            query_text=sanitized_query, filters=request.filters, top_k=request.top_k
        )
        return {"results": results}
    except HTTPException:
        # Re-raise HTTP exceptions
        raise
    except Exception as e:
        logger.error(f"Error querying: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Error querying: {str(e)}")


@app.post("/delete", response_model=DeleteResponse)
async def delete(
    request: DeleteRequest, service: HaystackService = Depends(get_service)
):
    """Delete documents based on filters"""
    logger.info(f"Delete request with filters: {request.filters}")

    try:
        result = await service.delete(filters=request.filters)
        return result
    except Exception as e:
        logger.error(f"Error deleting documents: {str(e)}")
        raise HTTPException(
            status_code=500, detail=f"Error deleting documents: {str(e)}"
        )


@app.get("/health", response_model=HealthResponse)
async def health():
    """Health check endpoint"""
    return {"status": "healthy", "version": "1.0.0"}
