# SPDX-FileCopyrightText: 2023-present deepset GmbH <info@deepset.ai> and HelixML, Inc <luke@helix.ml>
#
# SPDX-License-Identifier: Apache-2.0
import logging
from typing import Any, Dict, List, Literal, Optional, Union

from haystack import component, default_from_dict, default_to_dict
from haystack.dataclasses import Document
from haystack.document_stores.types import FilterPolicy
from haystack.document_stores.types.filter_policy import apply_filter_policy

from ..document_store import VectorchordDocumentStore
from ..document_store.document_store import VALID_VECTOR_FUNCTIONS

# Set up logging
logger = logging.getLogger(__name__)

@component
class VectorchordEmbeddingRetriever:
    """
    Retrieves documents from the `VectorchordDocumentStore`, based on their dense embeddings.

    Example usage:
    ```python
    from haystack.document_stores import DuplicatePolicy
    from haystack import Document, Pipeline
    from haystack.components.embedders import SentenceTransformersTextEmbedder, SentenceTransformersDocumentEmbedder

    # Using relative imports
    from ..document_store import VectorchordDocumentStore
    from . import VectorchordEmbeddingRetriever

    # Set an environment variable `PG_CONN_STR` with the connection string to your PostgreSQL database.
    # e.g., "postgresql://USER:PASSWORD@HOST:PORT/DB_NAME"

    document_store = VectorchordDocumentStore(
        embedding_dimension=768,
        vector_function="cosine_similarity",
        recreate_table=True,
    )

    documents = [Document(content="There are over 7,000 languages spoken around the world today."),
                 Document(content="Elephants have been observed to behave in a way that indicates..."),
                 Document(content="In certain places, you can witness the phenomenon of bioluminescent waves.")]

    document_embedder = SentenceTransformersDocumentEmbedder()
    document_embedder.warm_up()
    documents_with_embeddings = document_embedder.run(documents)

    document_store.write_documents(documents_with_embeddings.get("documents"), policy=DuplicatePolicy.OVERWRITE)

    query_pipeline = Pipeline()
    query_pipeline.add_component("text_embedder", SentenceTransformersTextEmbedder())
    query_pipeline.add_component("retriever", VectorchordEmbeddingRetriever(document_store=document_store))
    query_pipeline.connect("text_embedder.embedding", "retriever.query_embedding")

    query = "How many languages are there?"

    res = query_pipeline.run({"text_embedder": {"text": query}})

    assert res['retriever']['documents'][0].content == "There are over 7,000 languages spoken around the world today."
    ```
    """

    def __init__(
        self,
        *,
        document_store: VectorchordDocumentStore,
        filters: Optional[Dict[str, Any]] = None,
        top_k: int = 10,
        vector_function: Optional[Literal["cosine_similarity", "inner_product", "l2_distance"]] = None,
        filter_policy: Union[str, FilterPolicy] = FilterPolicy.REPLACE,
    ):
        """
        :param document_store: An instance of `VectorchordDocumentStore`.
        :param filters: Filters applied to the retrieved Documents.
        :param top_k: Maximum number of Documents to return.
        :param vector_function: The similarity function to use when searching for similar embeddings.
            Defaults to the one set in the `document_store` instance.
            `"cosine_similarity"` and `"inner_product"` are similarity functions and
            higher scores indicate greater similarity between the documents.
            `"l2_distance"` returns the straight-line distance between vectors,
            and the most similar documents are the ones with the smallest score.
            **Important**: if the document store is using the `"vchordrq"` search strategy, the vector function
            should match the one utilized during index creation to take advantage of the index.
        :param filter_policy: Policy to determine how filters are applied.
        :raises ValueError: If `document_store` is not an instance of `VectorchordDocumentStore` or if `vector_function`
            is not one of the valid options.
        """
        if not isinstance(document_store, VectorchordDocumentStore):
            msg = "document_store must be an instance of VectorchordDocumentStore"
            raise ValueError(msg)

        if vector_function and vector_function not in VALID_VECTOR_FUNCTIONS:
            msg = f"vector_function must be one of {VALID_VECTOR_FUNCTIONS}"
            raise ValueError(msg)

        self.document_store = document_store
        self.filters = filters or {}
        self.top_k = top_k
        self.vector_function = vector_function or document_store.vector_function
        self.filter_policy = (
            filter_policy if isinstance(filter_policy, FilterPolicy) else FilterPolicy.from_str(filter_policy)
        )

    def to_dict(self) -> Dict[str, Any]:
        """
        Serializes the component to a dictionary.

        :returns:
            Dictionary with serialized data.
        """
        return default_to_dict(
            self,
            filters=self.filters,
            top_k=self.top_k,
            vector_function=self.vector_function,
            filter_policy=self.filter_policy.value,
            document_store=self.document_store.to_dict(),
        )

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "VectorchordEmbeddingRetriever":
        """
        Deserializes the component from a dictionary.

        :param data:
            Dictionary to deserialize from.
        :returns:
            Deserialized component.
        """
        doc_store_params = data["init_parameters"]["document_store"]
        data["init_parameters"]["document_store"] = VectorchordDocumentStore.from_dict(doc_store_params)
        # Pipelines serialized with old versions of the component might not
        # have the filter_policy field.
        if filter_policy := data["init_parameters"].get("filter_policy"):
            data["init_parameters"]["filter_policy"] = FilterPolicy.from_str(filter_policy)
        return default_from_dict(cls, data)

    @component.output_types(documents=List[Document])
    def run(
        self,
        query_embedding: List[float],
        filters: Optional[Dict[str, Any]] = None,
        top_k: Optional[int] = None,
        vector_function: Optional[str] = None,
        **kwargs
    ):
        """
        Retrieve documents using dense embeddings.

        Args:
            query_embedding: The query embedding.
            filters: Optional filters to narrow down the search space.
            top_k: The maximum number of documents to retrieve. If None, use the top_k provided at initialization.
            vector_function: The similarity function to use. If None, use the vector_function provided at initialization.

        Returns:
            A dictionary containing the retrieved documents.
        """
        # Use values from initialization if not provided
        if top_k is None:
            top_k = self.top_k
        if vector_function is None:
            vector_function = self.vector_function

        # Apply filter policy to combine runtime filters with initialization filters
        filters = apply_filter_policy(self.filter_policy, self.filters, filters)

        logger.info(f"Vector retriever running with embedding length: {len(query_embedding)}")
        logger.info(f"Vector retriever filters: {filters}")
        
        # Retrieve documents using the embedding - returns a list of Documents directly
        documents = self.document_store._embedding_retrieval(
            query_embedding=query_embedding,
            filters=filters,
            top_k=top_k,
            vector_function=vector_function
        )
        
        logger.info(f"Vector retriever returned {len(documents)} documents")
        
        # Log scores for debugging and set directly on the Document object
        for i, doc in enumerate(documents):
            score = doc.meta.get('score', 'unknown') if hasattr(doc, 'meta') else 'unknown'
            if doc.meta.get('score') is not None:
                score = doc.meta.get('score')
                documents[i].score = score

            filename=doc.meta.get('filename', 'unknown') if hasattr(doc, 'meta') else 'unknown'
            logger.info(f"Vector result {i}: id={getattr(doc, 'id', 'unknown')}, score={score}, filename={filename}")
        
        return {"documents": documents}
