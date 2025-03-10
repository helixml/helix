# SPDX-FileCopyrightText: 2023-present deepset GmbH <info@deepset.ai> and HelixML, Inc <luke@helix.ml>
#
# SPDX-License-Identifier: Apache-2.0
import logging
from typing import Any, Dict, List, Literal, Optional

from haystack import default_from_dict, default_to_dict
from haystack.dataclasses.document import ByteStream, Document
from haystack.document_stores.errors import DocumentStoreError, DuplicateDocumentError
from haystack.document_stores.types import DuplicatePolicy
from haystack.utils.auth import Secret, deserialize_secrets_inplace
from psycopg import Error, IntegrityError, connect
from psycopg.abc import Query
from psycopg.cursor import Cursor
from psycopg.rows import dict_row
from psycopg.sql import SQL, Identifier
from psycopg.sql import Literal as SQLLiteral
from psycopg.types.json import Jsonb

# Import from pgvector for compatibility
from pgvector.psycopg import register_vector

from .filters import _convert_filters_to_where_clause_and_params

logger = logging.getLogger(__name__)

CREATE_TABLE_STATEMENT = """
CREATE TABLE IF NOT EXISTS {schema_name}.{table_name} (
id VARCHAR(128) PRIMARY KEY,
embedding VECTOR({embedding_dimension}),
content TEXT,
blob_data BYTEA,
blob_meta JSONB,
blob_mime_type VARCHAR(255),
meta JSONB)
"""

INSERT_STATEMENT = """
INSERT INTO {schema_name}.{table_name}
(id, embedding, content, blob_data, blob_meta, blob_mime_type, meta)
VALUES (%(id)s, %(embedding)s, %(content)s, %(blob_data)s, %(blob_meta)s, %(blob_mime_type)s, %(meta)s)
"""

UPDATE_STATEMENT = """
ON CONFLICT (id) DO UPDATE SET
embedding = EXCLUDED.embedding,
content = EXCLUDED.content,
blob_data = EXCLUDED.blob_data,
blob_meta = EXCLUDED.blob_meta,
blob_mime_type = EXCLUDED.blob_mime_type,
meta = EXCLUDED.meta
"""

DELETE_STATEMENT = """
DELETE FROM {schema_name}.{table_name} WHERE id = ANY(%(ids)s)
"""

TRUNCATE_STATEMENT = """
TRUNCATE {schema_name}.{table_name}
"""

SELECT_STATEMENT = """
SELECT * FROM {schema_name}.{table_name}
"""

COUNT_STATEMENT = """
SELECT COUNT(*) FROM {schema_name}.{table_name}
"""

CHECK_VALID_CONNECTION_STATEMENT = """
SELECT 1
"""

# Mapping from PostgreSQL functions to their corresponding SQL operators
VECTOR_FUNCTION_TO_OPERATOR = {
    "cosine_similarity": "<=>",
    "inner_product": "<#>",
    "l2_distance": "<->",
}

VALID_VECTOR_FUNCTIONS = list(VECTOR_FUNCTION_TO_OPERATOR.keys())

# Dictionary to map vector functions to order directions in the final SQL query
VECTOR_FUNCTION_TO_ORDER_DIRECTION = {
    "cosine_similarity": "DESC",  # Higher cosine similarity means more similar
    "inner_product": "DESC",  # Higher inner product means more similar
    "l2_distance": "ASC",  # Lower L2 distance means more similar
}

CREATE_KEYWORD_INDEX_STATEMENT = """
CREATE INDEX IF NOT EXISTS {index_name}
ON {schema_name}.{table_name} USING GIN (to_tsvector({language}, content))
"""

KEYWORD_SELECT_STATEMENT = """
SELECT *,
    ts_rank_cd(to_tsvector({language}, content),
               to_tsquery({language}, %(query)s)) AS rank
FROM {schema_name}.{table_name}
WHERE to_tsvector({language}, content) @@ to_tsquery({language}, %(query)s)
ORDER BY rank DESC
LIMIT %(top_k)s
"""

VECTOR_SELECT_STATEMENT = """
SELECT *
FROM {schema_name}.{table_name}
ORDER BY embedding {operator} %(query_embedding)s {direction}
LIMIT %(top_k)s
"""

CREATE_HNSW_INDEX_STATEMENT = """
CREATE INDEX IF NOT EXISTS {index_name}
ON {schema_name}.{table_name}
USING hnsw(embedding {operator})
{with_clause}
"""

HNSW_WITH_CLAUSE_TEMPLATE = """WITH (
    ef_construction = {ef_construction}, 
    m = {m}
)"""

HNSW_SET_EF_SEARCH_STATEMENT = """
SET vchord.ef_search = {ef_search}
"""

class VectorchordDocumentStore:
    """
    Store for documents using VectorChord (PostgreSQL with vector search capability).

    VectorChord is a scalable, fast, and disk-friendly vector search extension for Postgres, 
    the successor of pgvecto.rs.

    It allows to store and search for documents by their embeddings, as well as content-based keyword search.
    The document store can be used to create various retrieval components like `VectorchordEmbeddingRetriever` and
    `VectorchordBM25Retriever`.

    Usage example:
    ```python
    from haystack.document_stores import DuplicatePolicy
    from haystack import Document
    from haystack_service.app.vectorchord.document_store import VectorchordDocumentStore

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

    document_store.write_documents(documents, policy=DuplicatePolicy.OVERWRITE)
    ```
    """

    def __init__(
        self,
        *,
        connection_string: Secret = Secret.from_env_var("PG_CONN_STR"),
        create_extension: bool = True,
        schema_name: str = "public",
        table_name: str = "haystack_documents",
        language: str = "english",
        embedding_dimension: int = 768,
        vector_function: Literal["cosine_similarity", "inner_product", "l2_distance"] = "cosine_similarity",
        recreate_table: bool = False,
        search_strategy: Literal["exact_nearest_neighbor", "hnsw"] = "exact_nearest_neighbor",
        hnsw_recreate_index_if_exists: bool = False,
        hnsw_index_creation_kwargs: Optional[Dict[str, int]] = None,
        hnsw_index_name: str = "haystack_hnsw_index",
        hnsw_ef_search: Optional[int] = None,
        keyword_index_name: str = "haystack_keyword_index",
    ):
        """
        Create a new document store using VectorChord.

        :param connection_string: Database connection string in the format
                                `postgresql://USER:PASSWORD@HOST:PORT/DB_NAME`.
        :param create_extension: Whether to create the VectorChord extension if it doesn't exist.
        :param schema_name: Name of the database schema.
        :param table_name: Name of the document table.
        :param language: Language used for full-text search using PostgreSQL's ts_vector.
        :param embedding_dimension: Dimensionality of the embeddings.
        :param vector_function: Similarity function to use for vector search.
            `"cosine_similarity"` and `"inner_product"` are similarity functions - higher scores
            indicate greater similarity between the documents.
            `"l2_distance"` returns the straight-line distance between vectors, and the most similar
            documents are the ones with the smallest score.
        :param recreate_table: Whether to recreate the table if it already exists. If set to `True`, the table
                             will be dropped and recreated. Warning: this will delete all data in the table.
        :param search_strategy: The strategy to use for vector search. If set to "exact_nearest_neighbor", the search
                            will use the exact nearest neighbor algorithm. If set to "hnsw", the search will use the
                            Hierarchical Navigable Small World algorithm, which is faster but may be less accurate.
        :param hnsw_recreate_index_if_exists: Whether to recreate the HNSW index if it already exists. Ignored if
                                            `search_strategy` is not `"hnsw"`.
        :param hnsw_index_creation_kwargs: Dictionary of parameters for creating the HNSW index. The keys are
                                        `"ef_construction"` and `"m"`. See the VectorChord documentation for details.
        :param hnsw_index_name: Name of the HNSW index. Ignored if `search_strategy` is not `"hnsw"`.
        :param hnsw_ef_search: The size of the dynamic list for the nearest neighbors. Ignored if `search_strategy`
                             is not `"hnsw"`. Higher values give more accurate but slower results.
        :param keyword_index_name: Name of the keyword index.
        """
        self.language = language
        self.connection_string = connection_string
        self.create_extension = create_extension
        self.schema_name = schema_name
        self.table_name = table_name
        self.embedding_dimension = embedding_dimension
        self.vector_function = vector_function
        self.recreate_table = recreate_table
        self.search_strategy = search_strategy
        self.hnsw_recreate_index_if_exists = hnsw_recreate_index_if_exists
        self.hnsw_index_creation_kwargs = hnsw_index_creation_kwargs or {}
        self.hnsw_index_name = hnsw_index_name
        self.hnsw_ef_search = hnsw_ef_search
        self.keyword_index_name = keyword_index_name

        # Initialize the connection and set up the database
        self._conn = None
        self._cursor = None
        self._dict_cursor = None

        try:
            self._create_connection()
            self._initialize_table()
        except Error as err:
            self._conn = None
            error_msg = f"Failed to initialize VectorchordDocumentStore: {err}"
            logger.error(error_msg)
            raise DocumentStoreError(error_msg) from err

    @property
    def cursor(self):
        """Return a cursor for executing SQL queries."""
        if not self._connection_is_valid(self._conn):
            self._create_connection()
        if not self._cursor:
            self._cursor = self._conn.cursor()
        return self._cursor

    @property
    def dict_cursor(self):
        """Return a cursor for executing SQL queries with dictionary-like results."""
        if not self._connection_is_valid(self._conn):
            self._create_connection()
        if not self._dict_cursor:
            self._dict_cursor = self._conn.cursor(row_factory=dict_row)
        return self._dict_cursor

    @property
    def connection(self):
        """Return the database connection."""
        if not self._connection_is_valid(self._conn):
            self._create_connection()
        return self._conn

    def _create_connection(self):
        """Create a database connection."""
        try:
            # If connection already exists, close it first
            if self._conn:
                try:
                    self._conn.close()
                except Error:
                    pass

            # Create a new connection
            # Use the Secret object's value property to get the actual connection string
            if isinstance(self.connection_string, Secret):
                conn_str = self.connection_string.resolve_value()
            else:
                conn_str = str(self.connection_string)
                
            self._conn = connect(conn_str)
            register_vector(self._conn)

            # Create the VectorChord extension if needed
            if self.create_extension:
                try:
                    with self._conn.cursor() as cur:
                        # First ensure we have the required extensions
                        cur.execute("CREATE EXTENSION IF NOT EXISTS vector;")
                        cur.execute("CREATE EXTENSION IF NOT EXISTS vchord CASCADE;")
                        
                        # Add the VectorChord-BM25 extension
                        cur.execute("CREATE EXTENSION IF NOT EXISTS vchord_bm25 CASCADE;")
                        
                        self._conn.commit()
                except Error as err:
                    error_msg = f"Failed to create VectorChord extension: {err}"
                    logger.error(error_msg)
                    raise DocumentStoreError(error_msg) from err

            # Reset cursors
            self._cursor = None
            self._dict_cursor = None

        except Error as err:
            error_msg = f"Failed to connect to database: {err}"
            logger.error(error_msg)
            self._conn = None
            raise DocumentStoreError(error_msg) from err

    def _initialize_table(self):
        """Initialize the document table and indexes."""
        try:
            if self.recreate_table:
                # Delete the existing table if recreate_table is True
                self.delete_table()

            # Create table
            self._create_table_if_not_exists()

            # Create keyword index
            self._create_keyword_index_if_not_exists()

            # Handle HNSW index creation if needed
            if self.search_strategy == "hnsw":
                self._handle_hnsw()

        except Error as err:
            error_msg = f"Failed to initialize table: {err}"
            logger.error(error_msg)
            raise DocumentStoreError(error_msg) from err

    @staticmethod
    def _connection_is_valid(connection):
        """Check if the database connection is valid."""
        if not connection:
            return False
        try:
            # Try to execute a simple query to check the connection
            connection.execute(CHECK_VALID_CONNECTION_STATEMENT)
            return True
        except Error:
            return False

    def to_dict(self) -> Dict[str, Any]:
        """
        Serializes the document store to a dictionary.

        :returns:
            Dictionary with serialized data.
        """
        data = default_to_dict(
            self,
            connection_string=self.connection_string,
            create_extension=self.create_extension,
            schema_name=self.schema_name,
            table_name=self.table_name,
            language=self.language,
            embedding_dimension=self.embedding_dimension,
            vector_function=self.vector_function,
            recreate_table=self.recreate_table,
            search_strategy=self.search_strategy,
            hnsw_recreate_index_if_exists=self.hnsw_recreate_index_if_exists,
            hnsw_index_creation_kwargs=self.hnsw_index_creation_kwargs,
            hnsw_index_name=self.hnsw_index_name,
            hnsw_ef_search=self.hnsw_ef_search,
            keyword_index_name=self.keyword_index_name,
        )
        return data

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "VectorchordDocumentStore":
        """
        Deserializes the document store from a dictionary.

        :param data:
            Dictionary to deserialize from.
        :returns:
            Deserialized document store.
        """
        init_params = data.get("init_parameters", {})
        deserialize_secrets_inplace(init_params, "connection_string")
        return default_from_dict(cls, data)

    def _execute_sql(
        self, sql_query: Query, params: Optional[tuple] = None, error_msg: str = "", cursor: Optional[Cursor] = None
    ):
        """Execute a SQL query."""
        try:
            cursor_to_use = cursor or self.cursor
            if params:
                cursor_to_use.execute(sql_query, params)
            else:
                cursor_to_use.execute(sql_query)
            self.connection.commit()
        except Error as err:
            self.connection.rollback()
            error_msg = error_msg or f"Failed to execute query: {err}"
            logger.error(error_msg)
            raise DocumentStoreError(error_msg) from err

    def _create_table_if_not_exists(self):
        """Create the document table if it doesn't exist."""
        query = SQL(CREATE_TABLE_STATEMENT).format(
            schema_name=Identifier(self.schema_name),
            table_name=Identifier(self.table_name),
            embedding_dimension=SQLLiteral(self.embedding_dimension),
        )
        self._execute_sql(query, error_msg=f"Failed to create table {self.schema_name}.{self.table_name}")

    def delete_table(self):
        """
        Delete the document table.
        Warning: This will delete all data in the table.
        """
        query = SQL("DROP TABLE IF EXISTS {schema_name}.{table_name}").format(
            schema_name=Identifier(self.schema_name),
            table_name=Identifier(self.table_name),
        )
        self._execute_sql(query, error_msg=f"Failed to delete table {self.schema_name}.{self.table_name}")

    def _create_keyword_index_if_not_exists(self):
        """Create the keyword index if it doesn't exist."""
        # Skip index creation if keyword_index_name is None
        if self.keyword_index_name is None:
            return
            
        index_name = Identifier(f"{self.table_name}_{self.keyword_index_name}")

        # Format language as 'english'::regconfig for PostgreSQL
        language_param = SQL("{}::regconfig").format(SQLLiteral(self.language))

        query = SQL(CREATE_KEYWORD_INDEX_STATEMENT).format(
            index_name=index_name,
            schema_name=Identifier(self.schema_name),
            table_name=Identifier(self.table_name),
            language=language_param,
        )
        self._execute_sql(query, error_msg=f"Failed to create keyword index {index_name}")

    def _handle_hnsw(self):
        """Handle the creation of the HNSW index."""
        # Get the list of existing indexes for the table
        query = SQL(
            """
            SELECT indexname
            FROM pg_indexes
            WHERE schemaname = %(schema_name)s AND tablename = %(table_name)s
            """
        )
        params = {"schema_name": self.schema_name, "table_name": self.table_name}
        self.dict_cursor.execute(query, params)
        existing_indexes = [row["indexname"] for row in self.dict_cursor.fetchall()]

        # Check if the HNSW index already exists
        full_index_name = f"{self.table_name}_{self.hnsw_index_name}"
        if full_index_name in existing_indexes:
            if self.hnsw_recreate_index_if_exists:
                # If it exists and we want to recreate it, drop the index first
                drop_query = SQL("DROP INDEX IF EXISTS {schema_name}.{index_name}").format(
                    schema_name=Identifier(self.schema_name),
                    index_name=Identifier(full_index_name),
                )
                self._execute_sql(drop_query, error_msg=f"Failed to drop HNSW index {full_index_name}")
                # Create a new index
                self._create_hnsw_index()
            # If the index exists and we don't want to recreate it, set the ef_search if provided
            elif self.hnsw_ef_search is not None:
                self._set_hnsw_ef_search()
        else:
            # If the index doesn't exist, create it
            self._create_hnsw_index()

    def _create_hnsw_index(self):
        """Create the HNSW index."""
        # Use default values if not provided
        ef_construction = self.hnsw_index_creation_kwargs.get("ef_construction", 128)
        m = self.hnsw_index_creation_kwargs.get("m", 16)

        # Set ef_search if provided
        if self.hnsw_ef_search is not None:
            self._set_hnsw_ef_search()

        # Create the with clause
        with_clause = SQL(HNSW_WITH_CLAUSE_TEMPLATE).format(
            ef_construction=SQLLiteral(ef_construction),
            m=SQLLiteral(m),
        )

        # Get the operator for the vector function
        operator = VECTOR_FUNCTION_TO_OPERATOR.get(self.vector_function, VECTOR_FUNCTION_TO_OPERATOR["cosine_similarity"])

        # Create the HNSW index
        query = SQL(CREATE_HNSW_INDEX_STATEMENT).format(
            index_name=Identifier(f"{self.table_name}_{self.hnsw_index_name}"),
            schema_name=Identifier(self.schema_name),
            table_name=Identifier(self.table_name),
            operator=SQL(operator),
            with_clause=with_clause,
        )
        self._execute_sql(query, error_msg="Failed to create HNSW index")

    def _set_hnsw_ef_search(self):
        """Set the ef_search parameter for HNSW index."""
        if self.hnsw_ef_search is not None:
            query = SQL(HNSW_SET_EF_SEARCH_STATEMENT).format(ef_search=SQLLiteral(self.hnsw_ef_search))
            self._execute_sql(query, error_msg="Failed to set ef_search for HNSW index")

    def count_documents(self) -> int:
        """
        Return the count of all documents in the document store.

        :returns:
            Number of documents.
        """
        query = SQL(COUNT_STATEMENT).format(
            schema_name=Identifier(self.schema_name),
            table_name=Identifier(self.table_name),
        )
        self.cursor.execute(query)
        count = self.cursor.fetchone()[0]
        return count

    def filter_documents(self, filters: Optional[Dict[str, Any]] = None) -> List[Document]:
        """
        Get documents from the document store using filters.

        :param filters: Filters to narrow down the documents to retrieve.
        :returns: List of `Document`s that match the given filters.
        """
        # Start with the base SELECT statement
        query_parts = [
            SQL(SELECT_STATEMENT).format(
                schema_name=Identifier(self.schema_name),
                table_name=Identifier(self.table_name),
            )
        ]

        params = {}

        # If filters are provided, add a WHERE clause
        if filters:
            # Convert the filters to a SQL WHERE clause
            where_clause, where_params = _convert_filters_to_where_clause_and_params(filters)
            query_parts.append(SQL(" WHERE ") + where_clause)
            params.update(where_params)

        # Combine the query parts
        query = query_parts[0]
        for part in query_parts[1:]:
            query = query + part

        # Execute the query
        self.dict_cursor.execute(query, params)
        results = self.dict_cursor.fetchall()

        # Convert the results to Document objects
        return self._from_pg_to_haystack_documents(results)

    def write_documents(self, documents: List[Document], policy: DuplicatePolicy = DuplicatePolicy.NONE) -> int:
        """
        Write documents to the document store.

        :param documents: List of `Document`s to index.
        :param policy: How to handle duplicates.
        :returns: Number of documents indexed.
        """
        if not documents:
            return 0

        # Convert Document objects to Postgres compatible format
        pg_documents = self._from_haystack_to_pg_documents(documents)

        try:
            # Prepare the query based on the duplicate policy
            if policy == DuplicatePolicy.SKIP:
                base_query = SQL(INSERT_STATEMENT).format(
                    schema_name=Identifier(self.schema_name),
                    table_name=Identifier(self.table_name),
                )
                query = base_query + SQL(" ON CONFLICT (id) DO NOTHING")
            elif policy == DuplicatePolicy.OVERWRITE:
                base_query = SQL(INSERT_STATEMENT).format(
                    schema_name=Identifier(self.schema_name),
                    table_name=Identifier(self.table_name),
                )
                query = base_query + SQL(UPDATE_STATEMENT)
            elif policy == DuplicatePolicy.FAIL:
                query = SQL(INSERT_STATEMENT).format(
                    schema_name=Identifier(self.schema_name),
                    table_name=Identifier(self.table_name),
                )
            else:
                raise ValueError(f"Unknown duplicate policy: {policy}")

            # Insert documents
            for doc in pg_documents:
                try:
                    self.cursor.execute(query, doc)
                except IntegrityError as err:
                    if policy == DuplicatePolicy.FAIL:
                        self.connection.rollback()
                        error_msg = f"Failed to write documents due to duplicate IDs: {err}"
                        logger.error(error_msg)
                        raise DuplicateDocumentError(error_msg) from err
                    raise

            # Commit the changes
            self.connection.commit()
            return len(pg_documents)

        except Error as err:
            self.connection.rollback()
            error_msg = f"Failed to write documents: {err}"
            logger.error(error_msg)
            raise DocumentStoreError(error_msg) from err

    @staticmethod
    def _from_haystack_to_pg_documents(documents: List[Document]) -> List[Dict[str, Any]]:
        """
        Convert Haystack Document objects to PostgreSQL compatible dictionary format.

        :param documents: List of `Document`s to convert.
        :returns: List of dictionaries with PostgreSQL compatible data.
        """
        pg_documents = []

        for doc in documents:
            # Handle binary data if present
            blob_data = None
            blob_meta = None
            blob_mime_type = None
            meta_dict = doc.meta.to_dict() if doc.meta else {}

            if doc.blob:
                if isinstance(doc.blob, ByteStream):
                    blob_data = doc.blob.data
                    blob_meta = Jsonb(doc.blob.meta) if doc.blob.meta else None
                    blob_mime_type = doc.blob.mime_type
                else:
                    logger.warning(
                        f"Document {doc.id} has a blob of type {type(doc.blob)} which is not supported. "
                        "Only ByteStream is supported. The blob will be ignored."
                    )

            # Create a dictionary with PostgreSQL compatible data
            pg_document = {
                "id": doc.id,
                "embedding": doc.embedding,
                "content": doc.content,
                "blob_data": blob_data,
                "blob_meta": blob_meta,
                "blob_mime_type": blob_mime_type,
                "meta": Jsonb(meta_dict),
            }
            pg_documents.append(pg_document)

        return pg_documents

    @staticmethod
    def _from_pg_to_haystack_documents(documents: List[Dict[str, Any]]) -> List[Document]:
        """
        Convert PostgreSQL result dictionaries to Haystack Document objects.

        :param documents: List of dictionaries with PostgreSQL data.
        :returns: List of `Document`s.
        """
        haystack_documents = []

        for doc in documents:
            # Create a Document object from the PostgreSQL data
            blob = None
            if doc.get("blob_data") is not None:
                blob = ByteStream(
                    data=doc["blob_data"],
                    meta=doc.get("blob_meta"),
                    mime_type=doc.get("blob_mime_type"),
                )

            # Convert meta from JSONB to dict
            meta = doc.get("meta", {})
            if meta and not isinstance(meta, dict):
                meta = {}

            # Create the Document
            document = Document(
                id=doc["id"],
                content=doc.get("content"),
                embedding=doc.get("embedding"),
                blob=blob,
                meta=meta,
            )
            haystack_documents.append(document)

        return haystack_documents

    def delete_documents(self, document_ids: List[str]) -> None:
        """
        Delete documents from the document store.

        :param document_ids: List of IDs of the `Document`s to delete.
        """
        if not document_ids:
            return

        try:
            # Prepare the delete query
            query = SQL(DELETE_STATEMENT).format(
                schema_name=Identifier(self.schema_name),
                table_name=Identifier(self.table_name),
            )
            params = {"ids": document_ids}

            # Execute the query
            self.cursor.execute(query, params)
            self.connection.commit()

        except Error as err:
            self.connection.rollback()
            error_msg = f"Failed to delete documents: {err}"
            logger.error(error_msg)
            raise DocumentStoreError(error_msg) from err

    def _keyword_retrieval(
        self,
        query: str,
        *,
        filters: Optional[Dict[str, Any]] = None,
        top_k: int = 10,
    ) -> List[Document]:
        """
        Retrieve documents using keyword search.

        :param query: Query string.
        :param filters: Optional filters to narrow down the search space.
        :param top_k: Maximum number of documents to return.
        :returns: List of `Document`s that match the query.
        """
        # Replace whitespace with & for AND operator
        normalized_query = " & ".join(query.split())
        params = {"query": normalized_query, "top_k": top_k}
        
        # Format language as 'english'::regconfig for PostgreSQL
        language_param = SQL("{}::regconfig").format(SQLLiteral(self.language))

        # Start with the base query
        base_query = SQL(KEYWORD_SELECT_STATEMENT).format(
            schema_name=Identifier(self.schema_name),
            table_name=Identifier(self.table_name),
            language=language_param,
        )
        query_parts = [base_query]

        # If filters are provided, add a WHERE clause
        if filters:
            where_clause, where_params = _convert_filters_to_where_clause_and_params(filters)
            # Need to combine the WHERE clauses
            query_parts = [
                SQL(
                    """
                    SELECT *,
                        ts_rank_cd(to_tsvector({language}, content),
                                to_tsquery({language}, %(query)s)) AS rank
                    FROM {schema_name}.{table_name}
                    WHERE to_tsvector({language}, content) @@ to_tsquery({language}, %(query)s)
                    AND
                    """
                ).format(
                    schema_name=Identifier(self.schema_name),
                    table_name=Identifier(self.table_name),
                    language=language_param,
                ),
                where_clause,
                SQL(" ORDER BY rank DESC LIMIT %(top_k)s"),
            ]
            params.update(where_params)

        # Combine the query parts
        if len(query_parts) > 1:
            query = query_parts[0]
            for i in range(1, len(query_parts)):
                query = query + query_parts[i]
        else:
            query = query_parts[0]

        # Execute the query
        self.dict_cursor.execute(query, params)
        results = self.dict_cursor.fetchall()

        # Convert the results to Document objects
        return self._from_pg_to_haystack_documents(results)

    def _embedding_retrieval(
        self,
        query_embedding: List[float],
        *,
        filters: Optional[Dict[str, Any]] = None,
        top_k: int = 10,
        vector_function: Optional[Literal["cosine_similarity", "inner_product", "l2_distance"]] = None,
    ) -> List[Document]:
        """
        Retrieve documents using embedding similarity.

        :param query_embedding: Embedding of the query.
        :param filters: Optional filters to narrow down the search space.
        :param top_k: Maximum number of documents to return.
        :param vector_function: Vector similarity function to use.
        :returns: List of `Document`s that match the query.
        """
        # Use the provided vector_function or fall back to the default
        vector_function = vector_function or self.vector_function
        # Get the operator and order direction for the vector function
        operator = VECTOR_FUNCTION_TO_OPERATOR.get(vector_function, VECTOR_FUNCTION_TO_OPERATOR["cosine_similarity"])
        direction = VECTOR_FUNCTION_TO_ORDER_DIRECTION.get(
            vector_function, VECTOR_FUNCTION_TO_ORDER_DIRECTION["cosine_similarity"]
        )
        
        # Convert query_embedding to proper PostgreSQL vector format
        query_embedding_str = f"[{','.join(str(val) for val in query_embedding)}]"

        params = {"query_embedding": query_embedding_str, "top_k": top_k}

        # Start with the base query
        base_query = SQL("""
            SELECT *
            FROM {schema_name}.{table_name}
            ORDER BY embedding {operator} %(query_embedding)s::vector {direction}
            LIMIT %(top_k)s
        """).format(
            schema_name=Identifier(self.schema_name),
            table_name=Identifier(self.table_name),
            operator=SQL(operator),
            direction=SQL(direction),
        )
        query_parts = [base_query]

        # If filters are provided, add a WHERE clause
        if filters:
            where_clause, where_params = _convert_filters_to_where_clause_and_params(filters)
            # We need to modify the base query to include the WHERE clause
            query_parts = [
                SQL(
                    """
                    SELECT *
                    FROM {schema_name}.{table_name}
                    WHERE
                    """
                ).format(
                    schema_name=Identifier(self.schema_name),
                    table_name=Identifier(self.table_name),
                ),
                where_clause,
                SQL(" ORDER BY embedding {operator} %(query_embedding)s::vector {direction} LIMIT %(top_k)s").format(
                    operator=SQL(operator),
                    direction=SQL(direction),
                ),
            ]
            params.update(where_params)

        # Combine the query parts
        if len(query_parts) > 1:
            query = query_parts[0]
            for i in range(1, len(query_parts)):
                query = query + query_parts[i]
        else:
            query = query_parts[0]

        # Execute the query
        self.dict_cursor.execute(query, params)
        results = self.dict_cursor.fetchall()

        # Convert the results to Document objects
        return self._from_pg_to_haystack_documents(results)
