# SPDX-FileCopyrightText: 2023-present deepset GmbH <info@deepset.ai> and HelixML, Inc <luke@helix.ml>
#
# SPDX-License-Identifier: Apache-2.0
from typing import Any, Dict, List, Optional, Union
import math
import logging

from haystack import component, default_from_dict, default_to_dict
from haystack.dataclasses import Document
from haystack.document_stores.types import FilterPolicy
from haystack.document_stores.types.filter_policy import apply_filter_policy

from ..document_store import VectorchordDocumentStore

# Configure logger
logger = logging.getLogger(__name__)

@component
class VectorchordBM25Retriever:
    """
    Retrieve documents from the `VectorchordDocumentStore` using BM25 ranking algorithm.

    This retriever utilizes VectorChord-bm25, a specialized PostgreSQL extension that implements
    the Block-WeakAnd algorithm for BM25 ranking. Unlike standard PostgreSQL keyword search,
    this approach provides more accurate document ranking based on relevance metrics similar to
    those used in modern search engines.

    Usage example:
    ```python
    from haystack.document_stores import DuplicatePolicy
    from haystack import Document

    # Using relative imports
    from ..document_store import VectorchordDocumentStore
    from . import VectorchordBM25Retriever

    # Set an environment variable `PG_CONN_STR` with the connection string to your PostgreSQL database.
    # e.g., "postgresql://USER:PASSWORD@HOST:PORT/DB_NAME"

    document_store = VectorchordDocumentStore(language="english", recreate_table=True)

    documents = [Document(content="There are over 7,000 languages spoken around the world today."),
        Document(content="Elephants have been observed to behave in a way that indicates..."),
        Document(content="In certain places, you can witness the phenomenon of bioluminescent waves.")]

    document_store.write_documents(documents, policy=DuplicatePolicy.OVERWRITE)

    # Create the BM25 index (this is needed for the BM25 retriever to work)
    # Note: This step is typically handled by the document store initialization

    retriever = VectorchordBM25Retriever(document_store=document_store)

    result = retriever.run(query="languages")

    assert result['documents'][0].content == "There are over 7,000 languages spoken around the world today."
    ```
    """

    def __init__(
        self,
        *,
        document_store: VectorchordDocumentStore,
        filters: Optional[Dict[str, Any]] = None,
        top_k: int = 10,
        filter_policy: Union[str, FilterPolicy] = FilterPolicy.REPLACE,
        tokenizer: str = "Bert",
    ):
        """
        :param document_store: An instance of `VectorchordDocumentStore`.
        :param filters: Filters applied to the retrieved Documents.
        :param top_k: Maximum number of Documents to return.
        :param filter_policy: Policy to determine how filters are applied.
        :param tokenizer: The tokenizer to use for BM25 search. Default is "Bert".
        :raises ValueError: If `document_store` is not an instance of `VectorchordDocumentStore`.
        """
        if not isinstance(document_store, VectorchordDocumentStore):
            msg = "document_store must be an instance of VectorchordDocumentStore"
            raise ValueError(msg)

        self.document_store = document_store
        self.filters = filters or {}
        self.top_k = top_k
        self.tokenizer = tokenizer
        self.filter_policy = (
            filter_policy if isinstance(filter_policy, FilterPolicy) else FilterPolicy.from_str(filter_policy)
        )
        
        # Initialize VectorChord BM25 (ensure index exists)
        self._initialize_bm25()

    def _initialize_bm25(self):
        """
        Initialize the BM25 index for the document store.
        This method ensures that all documents have been tokenized and the BM25 index is created.
        """
        # Ensure the vchord_bm25 extension is loaded and properly initialized
        try:
            # First, add the bm25vector column if it doesn't exist
            query = f"""
            ALTER TABLE {self.document_store.schema_name}.{self.document_store.table_name}
            ADD COLUMN IF NOT EXISTS content_bm25vector bm25vector;
            """
            self.document_store.cursor.execute(query)
            self.document_store.connection.commit()
            
            # Tokenize the content for all documents
            query = f"""
            UPDATE {self.document_store.schema_name}.{self.document_store.table_name}
            SET content_bm25vector = tokenize(content, '{self.tokenizer}')
            WHERE content IS NOT NULL;
            """
            self.document_store.cursor.execute(query)
            self.document_store.connection.commit()
            
            # Create the BM25 index on the content_bm25vector column
            index_name = f"{self.document_store.table_name}_bm25_idx"
            query = f"""
            CREATE INDEX IF NOT EXISTS {index_name} 
            ON {self.document_store.schema_name}.{self.document_store.table_name} 
            USING bm25 (content_bm25vector bm25_ops);
            """
            self.document_store.cursor.execute(query)
            self.document_store.connection.commit()
            
        except Exception as e:
            self.document_store.connection.rollback()
            logger.error(f"Failed to initialize BM25 index: {str(e)}")
            raise ValueError(f"Failed to initialize BM25 index: {str(e)}")

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
            filter_policy=self.filter_policy.value,
            tokenizer=self.tokenizer,
            document_store=self.document_store.to_dict(),
        )

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "VectorchordBM25Retriever":
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
        query: str,
        filters: Optional[Dict[str, Any]] = None,
        top_k: Optional[int] = None,
    ):
        """
        Retrieve documents from the `VectorchordDocumentStore`, using BM25 ranking.

        :param query: Search query text.
        :param filters: Filters applied to the retrieved Documents. The way runtime filters are applied depends on
                        the `filter_policy` chosen at retriever initialization. See init method docstring for more
                        details.
        :param top_k: Maximum number of Documents to return.

        :returns: A dictionary with the following keys:
            - `documents`: List of `Document`s that match the query.
        """
        filters = apply_filter_policy(self.filter_policy, self.filters, filters)
        top_k = top_k or self.top_k

        # Use VectorChord-bm25 for retrieval
        # First, we need to get the index name
        index_name = f"{self.document_store.table_name}_bm25_idx"
        
        # Build the query with filters
        query_base = f"""
        SELECT id, content, meta, embedding, blob_data, blob_meta, blob_mime_type,
            content_bm25vector <&> to_bm25query('{index_name}', %s, '{self.tokenizer}') AS bm25_score
        FROM {self.document_store.schema_name}.{self.document_store.table_name}
        """
        
        # Add filters if present
        params = [query]
        where_conditions = []
        
        if filters:
            for key, value in filters.items():
                if isinstance(value, (list, tuple)):
                    placeholders = ', '.join(['%s' for _ in value])
                    where_conditions.append(f"meta->'{key}' ? ANY(ARRAY[{placeholders}])")
                    params.extend(value)
                else:
                    where_conditions.append(f"meta->'{key}' ? %s")
                    params.append(value)
        
        if where_conditions:
            query_base += " WHERE " + " AND ".join(where_conditions)
        
        # Order by BM25 score (higher is better) and limit
        query_base += " ORDER BY bm25_score LIMIT %s"
        params.append(top_k)
        
        # Execute the query
        try:
            self.document_store.dict_cursor.execute(query_base, params)
            results = self.document_store.dict_cursor.fetchall()
            
            # Convert to Document objects
            docs = self.document_store._from_pg_to_haystack_documents(results)
            
            # Add the score to each document
            for i, result in enumerate(results):
                if i < len(docs):
                    # VectorChord-BM25 returns negative BM25 scores where:
                    # - More negative = less relevant (e.g., -5.0 is less relevant than -1.0)
                    # - To properly transform these while preserving the correct ordering:
                    #   We want the less negative value (-1.0) to become a higher positive score
                    #   than the more negative value (-5.0)
                    bm25_score = result.get("bm25_score", 0.0)
                    
                    # Simple transformation: just add a constant to make all scores positive
                    # The constant should be large enough to make all typical negative scores positive
                    # Since typical BM25 scores range from around -10 to 0, adding 10 works well
                    docs[i].score = bm25_score + 10.0
            
            return {"documents": docs}
        except Exception as e:
            logger.error(f"BM25 search failed: {str(e)}")
            raise ValueError(f"BM25 search failed: {str(e)}")
