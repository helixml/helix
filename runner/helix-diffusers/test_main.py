from unittest.mock import Mock, patch

import PIL.Image
import pytest
from fastapi.testclient import TestClient

from main import app


@pytest.fixture
def mock_pipeline():
    with patch("main.shared_pipeline") as mock:
        # Create a mock image
        mock_image = Mock(spec=PIL.Image.Image)
        mock_image.save = Mock()

        # Set up pipeline mock
        mock.pipeline = Mock()
        mock.generate.return_value = [mock_image]

        yield mock


@pytest.fixture
def client(mock_pipeline):
    return TestClient(app)


def test_generate_image_endpoint(client):
    request_data = {"model": "test-model", "prompt": "a test prompt"}
    response = client.post("/v1/images/generations", json=request_data)

    assert response.status_code == 200
    assert "data" in response.json()
    assert len(response.json()["data"]) == 1
    assert "url" in response.json()["data"][0]
    assert response.json()["data"][0]["url"].startswith("http://")
    assert response.json()["data"][0]["url"].endswith(".png")


def test_healthz_endpoint(client):
    response = client.get("/healthz")
    assert response.status_code == 200
    assert response.json() == {"status": "ok"}


def test_pipeline_error_handling(client):
    request_data = {"model": "test-model", "prompt": "test prompt"}

    with patch("main.shared_pipeline.generate", side_effect=Exception("Test error")):
        response = client.post("/v1/images/generations", json=request_data)

        assert response.status_code == 500
        assert "Test error" in response.json()["detail"]


def test_pipeline_not_initialized_error(client):
    request_data = {"model": "test-model", "prompt": "test prompt"}

    with patch("main.shared_pipeline.pipeline", None):
        response = client.post("/v1/images/generations", json=request_data)

        assert response.status_code == 500
        assert "Pipeline not initialized" in response.json()["detail"]
