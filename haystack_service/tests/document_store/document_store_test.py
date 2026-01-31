import time

import docker
import pytest
from app.vectorchord.document_store.document_store import VectorchordDocumentStore
from haystack.dataclasses.document import Document
from haystack.document_stores.types import DuplicatePolicy
import psycopg2
from psycopg2 import OperationalError


def wait_for_postgres(
    host="localhost",
    port=5432,
    dbname="test_db",
    user="test_user",
    password="test_pass",
    max_retries=30,
    retry_delay=0.2,
):
    """Wait for Postgres database to be ready"""
    retry_count = 0
    while retry_count < max_retries:
        try:
            conn = psycopg2.connect(
                dbname=dbname, user=user, password=password, host=host, port=port
            )
            conn.close()
            return True
        except OperationalError:
            retry_count += 1
            time.sleep(retry_delay)
    raise TimeoutError("Failed to connect to PostgreSQL after maximum retries")


@pytest.fixture(scope="session")
def docker_compose():
    """Spin up docker compose for the tests"""
    docker_client = docker.from_env()
    container = docker_client.containers.run(
        "ghcr.io/tensorchord/vchord_bm25-postgres:pg17-v0.1.1",
        environment=[
            "POSTGRES_DB=test_db",
            "POSTGRES_USER=test_user",
            "POSTGRES_PASSWORD=test_pass",
        ],
        ports={"5432/tcp": 5432},
        detach=True,
        remove=True,
    )

    # Wait for database to be ready
    wait_for_postgres()

    yield container

    # Cleanup
    container.stop()


@pytest.fixture
def test_store(docker_compose):
    """Create a test document store"""
    store = VectorchordDocumentStore(
        connection_string="postgresql://test_user:test_pass@localhost:5432/test_db",
        embedding_dimension=3,
        recreate_table=True,
    )
    yield store
    store.delete_table()


def test_embedding_retrieval_similarity(test_store):
    """Test that similarity scores are calculated correctly"""
    # Create test documents with known similarities
    docs = [
        Document(
            content="doc1",
            embedding=[1.0, 0.0, 0.0],  # Unit vector along x-axis
            id="1",
        ),
        Document(
            content="doc2",
            embedding=[0.0, 1.0, 0.0],  # Unit vector along y-axis (orthogonal to doc1)
            id="2",
        ),
        Document(
            content="doc3",
            embedding=[0.707, 0.707, 0.0],  # 45-degree vector (known cosine similarity)
            id="3",
        ),
    ]

    # Write documents to store
    test_store.write_documents(docs, DuplicatePolicy.OVERWRITE)

    # Test retrieval with different query vectors
    results = test_store._embedding_retrieval(
        query_embedding=[1.0, 0.0, 0.0],  # Same as doc1
        top_k=3,
        vector_function="cosine_similarity",
    )

    # Verify results
    assert len(results) == 3

    # Check first result (should be doc1 with perfect similarity)
    assert results[0].id == "1"
    assert results[0].meta["score"] == pytest.approx(1.0)

    # Check second result (should be doc3 with cos(45°) similarity)
    assert results[1].id == "3"
    assert results[1].meta["score"] == pytest.approx(0.707, abs=0.001)

    # Check third result (should be doc2 with 0 similarity)
    assert results[2].id == "2"
    assert results[2].meta["score"] == pytest.approx(0.0)


def test_embedding_retrieval_l2(test_store):
    """Test L2 distance calculations"""
    docs = [
        Document(content="doc1", embedding=[1.0, 0.0, 0.0], id="1"),
        Document(
            content="doc2",
            embedding=[2.0, 0.0, 0.0],  # Distance of 1.0 from doc1
            id="2",
        ),
    ]

    test_store.write_documents(docs, DuplicatePolicy.OVERWRITE)

    results = test_store._embedding_retrieval(
        query_embedding=[1.0, 0.0, 0.0], top_k=2, vector_function="l2_distance"
    )

    assert results[0].id == "1"
    assert results[0].meta["score"] == pytest.approx(0.0)  # Distance to self
    assert results[1].id == "2"
    assert results[1].meta["score"] == pytest.approx(1.0)  # L2 distance of 1.0
