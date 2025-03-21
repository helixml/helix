import json
import os
import pytest
from fastapi.testclient import TestClient
from unittest.mock import AsyncMock, patch

from app.api import app, get_service

client = TestClient(app)

# Create test fixtures
@pytest.fixture
def sample_txt_file():
    """Create a sample text file for testing"""
    file_path = "sample_test.txt"
    with open(file_path, "w") as f:
        f.write("This is a sample test document.")
    yield file_path
    # Cleanup
    if os.path.exists(file_path):
        os.remove(file_path)

@pytest.fixture
def sample_pdf_file():
    """Ensure a sample PDF file exists for testing
    
    Note: For a real test, you should create or download an actual PDF file.
    This is a placeholder - replace with actual PDF creation logic or use a static test file.
    """
    # For this example, we'll check if a test PDF exists in a test_files directory
    # You should adjust this to match your project structure
    test_dir = "test_files"
    pdf_path = os.path.join(test_dir, "sample.pdf")
    
    if not os.path.exists(test_dir):
        os.makedirs(test_dir)
    
    # If a real PDF doesn't exist, this is a placeholder - in a real test you'd have a real PDF
    if not os.path.exists(pdf_path):
        pytest.skip("Test PDF file not found. Please add a real PDF for testing.")
    
    yield pdf_path

class MockHaystackService:
    async def process_and_index(self, file_path, metadata):
        # Mock implementation to simulate successful processing
        return {"chunks": 5}

@pytest.fixture
def mock_service():
    """Create a mock service for testing"""
    return MockHaystackService()

# Mock the dependency to use our mock service
@pytest.fixture
def override_get_service(mock_service):
    app.dependency_overrides[get_service] = lambda: mock_service
    yield
    app.dependency_overrides.clear()

def test_process_txt_file(sample_txt_file, override_get_service):
    """Test processing a text file"""
    # Prepare the file
    with open(sample_txt_file, "rb") as f:
        file_content = f.read()
    
    # Prepare metadata
    metadata = {
        "author": "Test Author",
        "subject": "Test Subject",
        "keywords": ["test", "document"]
    }
    
    # Make the request
    response = client.post(
        "/process",
        files={"file": ("test_document.txt", file_content, "text/plain")},
        data={"metadata": json.dumps(metadata)}
    )
    
    # Check the response
    assert response.status_code == 200
    result = response.json()
    assert result["status"] == "success"
    assert result["documents_processed"] == 1
    assert result["chunks_indexed"] == 5
    assert "Successfully processed test_document.txt" in result["message"]

def test_process_pdf_file(sample_pdf_file, override_get_service):
    """Test processing a PDF file"""
    # Prepare the file
    with open(sample_pdf_file, "rb") as f:
        file_content = f.read()
    
    # Prepare metadata
    metadata = {
        "author": "PDF Author",
        "created": "2023-01-01",
        "tags": ["pdf", "test"]
    }
    
    # Make the request
    response = client.post(
        "/process",
        files={"file": ("test_document.pdf", file_content, "application/pdf")},
        data={"metadata": json.dumps(metadata)}
    )
    
    # Check the response
    assert response.status_code == 200
    result = response.json()
    assert result["status"] == "success"
    assert result["documents_processed"] == 1
    assert result["chunks_indexed"] == 5
    assert "Successfully processed test_document.pdf" in result["message"]

def test_process_with_invalid_metadata(sample_txt_file, override_get_service):
    """Test processing with invalid metadata format"""
    # Prepare the file
    with open(sample_txt_file, "rb") as f:
        file_content = f.read()
    
    # Make the request with invalid JSON metadata
    response = client.post(
        "/process",
        files={"file": ("test_document.txt", file_content, "text/plain")},
        data={"metadata": "invalid json"}
    )
    
    # Check the response
    assert response.status_code == 400
    result = response.json()
    assert "Invalid metadata JSON" in result["detail"]

def test_process_with_empty_file(override_get_service):
    """Test processing an empty file"""
    # Make the request with an empty file
    response = client.post(
        "/process",
        files={"file": ("empty.txt", b"", "text/plain")},
        data={"metadata": json.dumps({"source": "test"})}
    )
    
    # Check the response
    assert response.status_code == 422
    result = response.json()
    assert "File content cannot be empty" in result["detail"]

def test_process_file_with_service_error(sample_txt_file, mock_service, override_get_service):
    """Test handling of service errors during processing"""
    # Make the mock service raise an exception
    mock_service.process_and_index = AsyncMock(side_effect=Exception("Service error"))
    
    # Prepare the file
    with open(sample_txt_file, "rb") as f:
        file_content = f.read()
    
    # Make the request
    response = client.post(
        "/process",
        files={"file": ("test_document.txt", file_content, "text/plain")},
        data={"metadata": json.dumps({"source": "test"})}
    )
    
    # Check the response
    assert response.status_code == 500
    result = response.json()
    assert "Error processing file" in result["detail"]
