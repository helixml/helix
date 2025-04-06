# SPDX-FileCopyrightText: 2023-present deepset GmbH <info@deepset.ai> and HelixML, Inc <luke@helixml.tech>
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
from ..document_store.filters import _convert_filters_to_where_clause_and_params

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
        
        # Ensure the BM25 index exists in the document store
        logger.info("Verifying BM25 index exists in document store")
        self._verify_bm25_index()

    def _verify_bm25_index(self):
        """
        Verify that the BM25 index exists in the document store.
        If not, it might indicate that the document store was not properly initialized.
        """
        try:
            # Check if the index exists
            index_name = f"{self.document_store.table_name}_bm25_idx"
            query = f"""
            SELECT EXISTS (
                SELECT 1 FROM pg_indexes 
                WHERE schemaname = '{self.document_store.schema_name}' 
                AND tablename = '{self.document_store.table_name}'
                AND indexname = '{index_name}'
            );
            """
            self.document_store.cursor.execute(query)
            index_exists = self.document_store.cursor.fetchone()[0]
            
            if not index_exists:
                logger.warning(f"BM25 index '{index_name}' not found. Asking document store to create it.")
                # Call the document store method to create the BM25 index
                self.document_store._create_bm25_index_if_not_exists()
                
        except Exception as e:
            logger.error(f"Error verifying BM25 index: {str(e)}")
            raise ValueError(f"Failed to verify BM25 index: {str(e)}")

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
        try:
            logger.info(f"BM25Retriever: Running query '{query}' with filters={filters}, top_k={top_k}")
            filters = apply_filter_policy(self.filter_policy, self.filters, filters)
            top_k = top_k or self.top_k
            logger.info(f"BM25Retriever: After filter policy applied: filters={filters}, top_k={top_k}")

            # Use VectorChord-bm25 for retrieval
            # First, we need to get the index name
            index_name = f"{self.document_store.table_name}_bm25_idx"
            
            # Build the query with filters
            query_base = f"""
            SELECT id, content, meta, embedding, blob_data, blob_meta, blob_mime_type,
                content_bm25vector <&> to_bm25query('{index_name}', %s, '{self.tokenizer}') AS bm25_score
            FROM {self.document_store.schema_name}.{self.document_store.table_name}
            """
            
            # Add WHERE clause for filters
            if filters:
                try:
                    logger.info(f"BM25Retriever: Applying filters: {filters}")
                    # Import the filter handling function from the filters module
                    from ..document_store.filters import _convert_filters_to_where_clause_and_params
                    from psycopg.sql import SQL, Composed
                    
                    # Convert filters to SQL WHERE clause
                    where_clause, filter_params = _convert_filters_to_where_clause_and_params(filters)
                    logger.info(f"BM25Retriever: Generated filter clause: {where_clause}, params: {filter_params}")
                    
                    # Create SQL objects for each part of the query
                    base_sql = SQL(query_base)
                    order_limit_sql = SQL(" ORDER BY bm25_score LIMIT %s")
                    
                    # Build params properly - first the query text, then filter params, then top_k
                    params = [query]
                    if isinstance(filter_params, tuple):
                        # Handle tuple of filter params - add them one by one to the list
                        if filter_params:
                            params.extend(filter_params)
                    else:
                        # Handle single filter parameter or other types
                        params.append(filter_params)
                    params.append(top_k)
                    logger.info(f"BM25Retriever: Final query parameters: {params}")
                    
                    # Compose the final query with properly formatted SQL objects
                    final_query = Composed([base_sql, where_clause, order_limit_sql])
                    logger.info(f"BM25Retriever: Final query: {final_query}")
                    
                    self.document_store.dict_cursor.execute(final_query, params)
                    results = self.document_store.dict_cursor.fetchall()
                    logger.info(f"BM25Retriever: SQL query returned {len(results)} results")
                except Exception as e:
                    # Make filter errors fatal
                    logger.error(f"BM25Retriever: Failed to apply filters: {str(e)}")
                    logger.exception("BM25Retriever filter error details")
                    raise ValueError(f"BM25Retriever: Failed to apply filters: {str(e)}")
            else:
                # No filters, just execute the query
                clean_query_base = query_base + " ORDER BY bm25_score LIMIT %s"
                clean_params = [query, top_k]
                logger.info(f"BM25Retriever: Executing query without filters: {clean_query_base} with params {clean_params}")
                self.document_store.dict_cursor.execute(clean_query_base, clean_params)
                results = self.document_store.dict_cursor.fetchall()
                logger.info(f"BM25Retriever: Query returned {len(results)} results")
            
            # Convert to Document objects
            docs = self.document_store._from_pg_to_haystack_documents(results)
            logger.info(f"BM25Retriever: Converted {len(docs)} results to Document objects")
            
            # Debug the raw scores from the database
            for i, result in enumerate(results):
                bm25_score = result.get("bm25_score", 0.0)
                logger.info(f"BM25Retriever: Raw result {i+1}: id={result.get('id')}, raw_bm25_score={bm25_score}")
            
            # Add the score to each document
            for i, result in enumerate(results):
                if i < len(docs):
                    # VectorChord-BM25 returns negative BM25 scores where:
                    # - More negative = less relevant (e.g., -5.0 is less relevant than -1.0)
                    # - To properly transform these while preserving the correct ordering:
                    #   We want the less negative value (-1.0) to become a higher positive score
                    #   than the more negative value (-5.0)
                    bm25_score = result.get("bm25_score", 0.0)
                    
                    # Handle None values safely
                    if bm25_score is None:
                        logger.warning(f"BM25Retriever: Document {i+1} (id: {docs[i].id}) had None score, defaulting to -20.0")
                        bm25_score = -20.0
                    
                    # Simple transformation: just add a constant to make all scores positive
                    # The constant should be large enough to make all typical negative scores positive
                    # Since typical BM25 scores range from around -20 to 0, adding 20 works well
                    transformed_score = bm25_score + 20.0
                    docs[i].score = transformed_score
                    logger.info(f"BM25Retriever: Document {i+1} (id: {docs[i].id}): raw_score={bm25_score}, transformed_score={transformed_score}")
            
            return {"documents": docs}
            
        except Exception as e:
            logger.error(f"BM25 search failed: {str(e)}")
            logger.exception("BM25 search error details:")
            raise ValueError(f"BM25 search failed: {str(e)}")
