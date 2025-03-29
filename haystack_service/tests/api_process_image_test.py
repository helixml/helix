import json
import os
import pytest
from fastapi.testclient import TestClient
from unittest.mock import AsyncMock, patch

from app.api import app, get_image_service

client = TestClient(app)


# Create test fixtures
@pytest.fixture
def sample_pdf_file():
    """Ensure a sample PDF file exists for testing"""
    test_dir = "test_files"
    pdf_path = os.path.join(test_dir, "sample.pdf")

    if not os.path.exists(test_dir):
        os.makedirs(test_dir)

    # If a real PDF doesn't exist, this is a placeholder
    if not os.path.exists(pdf_path):
        pytest.skip("Test PDF file not found. Please add a real PDF for testing.")

    yield pdf_path


class MockHaystackImageService:
    async def process_and_index(self, file_path, metadata):
        # Mock implementation to simulate successful processing
        return {"chunks": 5}


@pytest.fixture
def mock_image_service():
    """Create a mock image service for testing"""
    return MockHaystackImageService()


@pytest.fixture
def override_get_image_service(mock_image_service):
    app.dependency_overrides[get_image_service] = lambda: mock_image_service
    yield
    app.dependency_overrides.clear()


@patch("app.api.settings.VISION_ENABLED", True)
def test_process_vision_pdf_success(sample_pdf_file, override_get_image_service):
    """Test successful processing of a PDF file through vision endpoint"""
    # Prepare the file
    with open(sample_pdf_file, "rb") as f:
        file_content = f.read()

    # Prepare metadata
    metadata = {
        "author": "Test Author",
        "tags": ["test", "pdf"],
    }

    # Make the request
    response = client.post(
        "/process-vision",
        files={"file": ("test_document.pdf", file_content, "application/pdf")},
        data={"metadata": json.dumps(metadata)},
    )

    # Check the response
    assert response.status_code == 200
    result = response.json()
    assert result["status"] == "success"
    assert result["documents_processed"] == 1
    assert result["chunks_indexed"] == 5
    assert "Successfully processed test_document.pdf" in result["message"]


@patch("app.api.settings.VISION_ENABLED", False)
def test_process_vision_when_disabled(override_get_image_service):
    """Test that attempting to process when VISION_ENABLED is False returns 400"""
    # Create a dummy PDF content
    pdf_content = b"fake pdf content"

    # Make the request
    response = client.post(
        "/process-vision",
        files={"file": ("test.pdf", pdf_content, "application/pdf")},
        data={"metadata": json.dumps({"source": "test"})},
    )

    # Check the response
    assert response.status_code == 400
    result = response.json()
    assert "Vision RAG is not enabled" in result["detail"]
