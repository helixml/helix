# SPDX-FileCopyrightText: 2023-present deepset GmbH <info@deepset.ai> and HelixML, Inc <luke@helix.ml>
#
# SPDX-License-Identifier: Apache-2.0
import logging
from typing import Any, Dict, List, Literal, Optional
import uuid
import json

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
content_bm25vector bm25vector,
blob_data BYTEA,
blob_meta JSONB,
blob_mime_type VARCHAR(255),
meta JSONB)
"""

INSERT_STATEMENT = """
INSERT INTO {schema_name}.{table_name}
(id, embedding, content, content_bm25vector, blob_data, blob_meta, blob_mime_type, meta)
VALUES (%(id)s, %(embedding)s, %(content)s, %(content_bm25vector)s, %(blob_data)s, %(blob_meta)s, %(blob_mime_type)s, %(meta)s)
"""

UPDATE_STATEMENT = """
ON CONFLICT (id) DO UPDATE SET
embedding = EXCLUDED.embedding,
content = EXCLUDED.content,
content_bm25vector = EXCLUDED.content_bm25vector,
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

# Create a constant for VectorChord's RaBitQ index
CREATE_VCHORDRQ_INDEX_STATEMENT = """
CREATE INDEX IF NOT EXISTS {index_name}
ON {schema_name}.{table_name}
USING vchordrq (embedding {operator_type}) WITH (options = '{options}')
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
        vector_function: Literal[
            "cosine_similarity", "inner_product", "l2_distance"
        ] = "cosine_similarity",
        recreate_table: bool = False,
        search_strategy: Literal[
            "exact_nearest_neighbor", "vchordrq"
        ] = "exact_nearest_neighbor",
        vchordrq_recreate_index_if_exists: bool = False,
        vchordrq_index_name: str = "haystack_vchordrq_index",
        vchordrq_lists: int = 1000,
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
        :param recreate_table: Whether to drop and recreate the document table.
        :param search_strategy: Strategy to use for vector search, either "exact_nearest_neighbor" or "vchordrq".
        :param vchordrq_recreate_index_if_exists: Whether to recreate the VectorChord RaBitQ index if it already exists.
        :param vchordrq_index_name: Name of the VectorChord RaBitQ index.
        :param vchordrq_lists: Number of lists to use for VectorChord RaBitQ index. Recommended to be rows/1000 for up to 1M rows.
        :param keyword_index_name: Name of the keyword index for full-text search. Set to None to skip creation.
        """
        self.connection_string = connection_string
        self.create_extension = create_extension
        self.schema_name = schema_name
        self.table_name = table_name
        self.language = language
        self.embedding_dimension = embedding_dimension
        self.vector_function = vector_function
        self.recreate_table = recreate_table
        self.search_strategy = search_strategy
        self.vchordrq_recreate_index_if_exists = vchordrq_recreate_index_if_exists
        self.vchordrq_index_name = vchordrq_index_name
        self.vchordrq_lists = vchordrq_lists
        self.keyword_index_name = keyword_index_name

        # Initialize connections
        self._conn = None
        self._cursor = None
        self._dict_cursor = None

        # Initialize the database
        self._create_connection()
        self._initialize_table()

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

            # Create the VectorChord extension if needed - MUST BE DONE BEFORE register_vector
            if self.create_extension:
                try:
                    with self._conn.cursor() as cur:
                        # First ensure we have the required extensions
                        cur.execute("CREATE EXTENSION IF NOT EXISTS vector;")
                        cur.execute("CREATE EXTENSION IF NOT EXISTS vchord CASCADE;")

                        # Add the VectorChord-BM25 extension
                        cur.execute(
                            "CREATE EXTENSION IF NOT EXISTS vchord_bm25 CASCADE;"
                        )

                        # Set up search path for bm25_catalog schema (session level instead of system level)
                        # This adds bm25_catalog to the search path so PostgreSQL can find the bm25vector type
                        cur.execute('SET search_path TO "$user", public, bm25_catalog;')

                        self._conn.commit()
                except Error as err:
                    error_msg = f"Failed to create VectorChord extension: {err}"
                    logger.error(error_msg)
                    raise DocumentStoreError(error_msg) from err

            # Register vector AFTER creating the extension
            register_vector(self._conn)

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

            # Create BM25 index
            self._create_bm25_index_if_not_exists()

            # Handle HNSW index creation if needed
            if self.search_strategy == "vchordrq":
                self._handle_vchordrq()

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
            vchordrq_recreate_index_if_exists=self.vchordrq_recreate_index_if_exists,
            vchordrq_index_name=self.vchordrq_index_name,
            vchordrq_lists=self.vchordrq_lists,
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
        init_params = data["init_parameters"]

        # Handle migration from old parameter names (HNSW) to new ones (VectorChord RaBitQ)
        if (
            "hnsw_recreate_index_if_exists" in init_params
            and "vchordrq_recreate_index_if_exists" not in init_params
        ):
            init_params["vchordrq_recreate_index_if_exists"] = init_params.pop(
                "hnsw_recreate_index_if_exists"
            )

        if (
            "hnsw_index_name" in init_params
            and "vchordrq_index_name" not in init_params
        ):
            init_params["vchordrq_index_name"] = init_params.pop("hnsw_index_name")

        if "hnsw_index_creation_kwargs" in init_params:
            # Extract m and convert to lists parameter if possible
            hnsw_kwargs = init_params.pop("hnsw_index_creation_kwargs")
            if isinstance(hnsw_kwargs, dict) and "m" in hnsw_kwargs:
                # Use m * 100 as a reasonable conversion from HNSW m to vchordrq lists
                init_params["vchordrq_lists"] = hnsw_kwargs["m"] * 100
            else:
                # Default to 1000 if we can't get a reasonable value
                init_params["vchordrq_lists"] = 1000

        # Handle removal of ef_search parameter which doesn't have a direct equivalent
        if "hnsw_ef_search" in init_params:
            init_params.pop("hnsw_ef_search")

        return default_from_dict(cls, data)

    def _execute_sql(
        self,
        sql_query: Query,
        params: Optional[tuple] = None,
        error_msg: str = "",
        cursor: Optional[Cursor] = None,
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
        self._execute_sql(
            query,
            error_msg=f"Failed to create table {self.schema_name}.{self.table_name}",
        )

    def delete_table(self):
        """
        Delete the document table.
        Warning: This will delete all data in the table.
        """
        query = SQL("DROP TABLE IF EXISTS {schema_name}.{table_name}").format(
            schema_name=Identifier(self.schema_name),
            table_name=Identifier(self.table_name),
        )
        self._execute_sql(
            query,
            error_msg=f"Failed to delete table {self.schema_name}.{self.table_name}",
        )

    def _create_keyword_index_if_not_exists(self):
        """Create the keyword index if it doesn't exist."""
        if not self.keyword_index_name:
            logger.info(
                f"Skipping keyword index creation for {self.schema_name}.{self.table_name}"
            )
            return

        query = SQL(CREATE_KEYWORD_INDEX_STATEMENT).format(
            schema_name=Identifier(self.schema_name),
            table_name=Identifier(self.table_name),
            index_name=Identifier(self.keyword_index_name),
            language=SQLLiteral(self.language),
        )
        self._execute_sql(
            query,
            error_msg=f"Failed to create keyword index for {self.schema_name}.{self.table_name}",
        )
        # logger.info(f"Created keyword index {self.keyword_index_name} for {self.schema_name}.{self.table_name}")

    def _create_bm25_index_if_not_exists(self):
        """Create the BM25 index if it doesn't exist and update content_bm25vector column."""
        try:
            # First, check if the content_bm25vector column exists
            check_column_query = SQL("""
                SELECT EXISTS (
                    SELECT FROM information_schema.columns 
                    WHERE table_schema = {schema_name}
                    AND table_name = {table_name}
                    AND column_name = 'content_bm25vector'
                );
            """).format(
                schema_name=SQLLiteral(self.schema_name),
                table_name=SQLLiteral(self.table_name),
            )
            
            self.cursor.execute(check_column_query)
            column_exists = self.cursor.fetchone()[0]
            
            # If the column doesn't exist, add it
            if not column_exists:
                logger.info(f"Column content_bm25vector does not exist in {self.schema_name}.{self.table_name}. Creating it.")
                
                # Ensure the search path includes bm25_catalog for bm25vector type
                self._execute_sql(
                    SQL('SET search_path TO "$user", public, bm25_catalog;'),
                    error_msg="Failed to set search path for BM25 vector column creation"
                )
                
                # Add the content_bm25vector column
                add_column_query = SQL("""
                    ALTER TABLE {schema_name}.{table_name} 
                    ADD COLUMN content_bm25vector bm25vector;
                """).format(
                    schema_name=Identifier(self.schema_name),
                    table_name=Identifier(self.table_name),
                )
                
                self._execute_sql(
                    add_column_query,
                    error_msg=f"Failed to add content_bm25vector column to {self.schema_name}.{self.table_name}"
                )
                
                logger.info(f"Column content_bm25vector added to {self.schema_name}.{self.table_name}")
            
            # Then create the index
            index_name = (
                SQL("{table_name}_bm25_idx")
                .format(table_name=Identifier(self.table_name))
                .as_string(self.cursor)
            )

            # Remove quotes that psycopg might add
            index_name = index_name.replace('"', "")

            query = SQL("""
                CREATE INDEX IF NOT EXISTS {index_name}
                ON {schema_name}.{table_name}
                USING bm25 (content_bm25vector bm25_ops)
            """).format(
                schema_name=Identifier(self.schema_name),
                table_name=Identifier(self.table_name),
                index_name=Identifier(index_name),
            )
            
            self._execute_sql(
                query,
                error_msg=f"Failed to create BM25 index for {self.schema_name}.{self.table_name}",
            )

            logger.info(
                f"Created BM25 index {index_name} for {self.schema_name}.{self.table_name}"
            )
        except Exception as e:
            logger.error(f"Critical error creating BM25 index: {str(e)}")
            self.connection.rollback()
            # Raise the error as BM25 is critical for functionality
            raise DocumentStoreError(f"Failed to create BM25 index: {str(e)}")

    def _handle_vchordrq(self):
        """Handle the creation of the VectorChord RaBitQ index."""
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

        # Check if the VectorChord RaBitQ index already exists
        full_index_name = f"{self.table_name}_{self.vchordrq_index_name}"
        if full_index_name in existing_indexes:
            if self.vchordrq_recreate_index_if_exists:
                # If it exists and we want to recreate it, drop the index first
                drop_query = SQL(
                    "DROP INDEX IF EXISTS {schema_name}.{index_name}"
                ).format(
                    schema_name=Identifier(self.schema_name),
                    index_name=Identifier(full_index_name),
                )
                self._execute_sql(
                    drop_query,
                    error_msg=f"Failed to drop VectorChord RaBitQ index {full_index_name}",
                )
                # Create a new index
                self._create_vchordrq_index()
        else:
            # If the index doesn't exist, create it
            self._create_vchordrq_index()

    def _create_vchordrq_index(self):
        """Create the VectorChord RaBitQ index."""
        # Set the appropriate options based on the vector function
        if self.vector_function == "l2_distance":
            # For L2 distance, use residual_quantization=true and spherical_centroids=false
            options_content = f"""
            residual_quantization = true
            [build.internal]
            lists = [{self.vchordrq_lists}]
            spherical_centroids = false
            """
            operator_type = "vector_l2_ops"
        elif self.vector_function == "inner_product":
            # For inner product, use residual_quantization=false and spherical_centroids=true
            options_content = f"""
            residual_quantization = false
            [build.internal]
            lists = [{self.vchordrq_lists}]
            spherical_centroids = true
            """
            operator_type = "vector_ip_ops"
        else:  # cosine_similarity or default
            # For cosine similarity, use residual_quantization=false and spherical_centroids=true
            options_content = f"""
            residual_quantization = false
            [build.internal]
            lists = [{self.vchordrq_lists}]
            spherical_centroids = true
            """
            operator_type = "vector_cosine_ops"

        # Escape single quotes in options for SQL
        options_escaped = options_content.replace("'", "''")

        # Create the index using raw SQL to avoid parameterization issues
        # We're building the SQL directly rather than using parameters for the options
        # as PostgreSQL doesn't support parameterized values in CREATE INDEX WITH clauses
        raw_sql = f"""
        CREATE INDEX IF NOT EXISTS {self.table_name}_{self.vchordrq_index_name}
        ON {self.schema_name}.{self.table_name}
        USING vchordrq (embedding {operator_type}) WITH (options = '{options_escaped}')
        """

        # Execute the raw SQL
        self._execute_sql(
            SQL(raw_sql), error_msg="Failed to create VectorChord RaBitQ index"
        )
        logger.info(
            f"Created VectorChord RaBitQ index {self.table_name}_{self.vchordrq_index_name} for {self.schema_name}.{self.table_name}"
        )

        # Set probes and epsilon for better performance
        try:
            # Set probes to 10% of lists for better recall
            probes = max(int(self.vchordrq_lists * 0.1), 10)
            self._execute_sql(
                SQL("SET vchordrq.probes = %s"),
                params=(probes,),
                error_msg="Failed to set vchordrq.probes",
            )

            # Set epsilon to 1.5 for a balance of precision and speed
            self._execute_sql(
                SQL("SET vchordrq.epsilon = 1.5"),
                error_msg="Failed to set vchordrq.epsilon",
            )

            logger.info(
                f"Configured VectorChord RaBitQ with probes={probes} and epsilon=1.5"
            )
        except Exception as e:
            # Don't fail if we can't set the parameters
            logger.warning(f"Could not set VectorChord RaBitQ parameters: {str(e)}")

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

    def filter_documents(
        self, filters: Optional[Dict[str, Any]] = None
    ) -> List[Document]:
        """
        Get documents from the document store using filters.

        :param filters: Filters to narrow down the documents to retrieve.
        :returns: List of `Document`s that match the given filters.
        """
        # Start with the base SELECT statement without filters
        base_query = SQL(SELECT_STATEMENT).format(
            schema_name=Identifier(self.schema_name),
            table_name=Identifier(self.table_name),
        )

        # Prepare parameters and WHERE clause
        params = []

        # If filters are provided, add a WHERE clause
        if filters:
            try:
                # Import here to avoid circular imports
                from .filters import _convert_filters_to_where_clause_and_params

                # Convert the filters to a SQL WHERE clause
                where_clause, filter_params = (
                    _convert_filters_to_where_clause_and_params(filters)
                )

                # Create the full query with WHERE clause
                query = SQL("{base_query} {where_clause}").format(
                    base_query=base_query, where_clause=where_clause
                )

                # Add filter parameters to our list - ensure positional style
                if isinstance(filter_params, tuple):
                    params.extend(filter_params)
                else:
                    params.append(filter_params)
            except Exception as e:
                # Raise exception instead of logging a warning
                raise ValueError(f"Failed to apply filters: {str(e)}")
        else:
            # No filters, just use the base query
            query = base_query

        try:
            # Execute the query
            self.dict_cursor.execute(query, params)
            results = self.dict_cursor.fetchall()

            # Convert the results to Document objects
            return self._from_pg_to_haystack_documents(results)
        except Exception as e:
            raise DocumentStoreError(
                f"Error retrieving documents with filters: {str(e)}"
            ) from e

    def write_documents(
        self, documents: List[Document], policy: DuplicatePolicy = DuplicatePolicy.NONE
    ) -> int:
        """
        Write documents to the document store.

        :param documents: List of `Document`s to index.
        :param policy: How to handle duplicates.
        :returns: Number of documents indexed.
        """
        if not documents:
            return 0

        # Filter out any NUL bytes in document content before writing to PostgreSQL
        logger = logging.getLogger(__name__)
        for doc in documents:
            if doc.content and "\x00" in doc.content:
                logger.warning(
                    f"Document store: removing NUL bytes from document content before database write: {doc.id}"
                )
                doc.content = doc.content.replace("\x00", "")

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
                        error_msg = (
                            f"Failed to write documents due to duplicate IDs: {err}"
                        )
                        logger.error(error_msg)
                        raise DuplicateDocumentError(error_msg) from err
                    raise

            # Get the IDs of documents we just inserted with content
            doc_ids = [
                doc["id"]
                for doc in pg_documents
                if doc["content"] and not doc["blob_data"]
            ]

            if doc_ids:
                # Convert list of IDs to a comma-separated string of quoted IDs
                id_list = ", ".join([f"'{doc_id}'" for doc_id in doc_ids])

                # Use the 'Bert' tokenizer by default
                tokenizer = "Bert"

                # Build and execute the query to update BM25 vectors as part of the same transaction
                query = SQL("""
                    UPDATE {schema_name}.{table_name}
                    SET content_bm25vector = tokenize(content, %s)
                    WHERE id IN ({id_list}) AND content IS NOT NULL
                """).format(
                    schema_name=Identifier(self.schema_name),
                    table_name=Identifier(self.table_name),
                    id_list=SQL(id_list),
                )

                try:
                    self._execute_sql(
                        query,
                        params=(tokenizer,),
                        error_msg="Failed to update BM25 vectors for newly inserted documents",
                    )
                    logger.info(f"Updated BM25 vectors for {len(doc_ids)} documents")
                except Exception as e:
                    # Roll back the entire transaction if BM25 vector update fails
                    self.connection.rollback()
                    error_msg = (
                        f"Failed to update BM25 vectors: {str(e)}. Operation aborted."
                    )
                    logger.error(error_msg)
                    raise DocumentStoreError(error_msg) from e

            # Commit the changes
            self.connection.commit()

            return len(pg_documents)

        except Error as err:
            self.connection.rollback()
            error_msg = f"Failed to write documents: {err}"
            logger.error(error_msg)
            raise DocumentStoreError(error_msg) from err

    @staticmethod
    def _from_haystack_to_pg_documents(
        documents: List[Document],
    ) -> List[Dict[str, Any]]:
        """
        Convert Haystack Document objects to PostgreSQL compatible format.

        :param documents: List of `Document`s to convert.
        :returns: List of dictionaries with PostgreSQL compatible format.
        """
        pg_documents = []

        for document in documents:
            # Generate ID if not provided
            doc_id = document.id or f"{uuid.uuid4().hex}"

            # Extract metadata or use empty object
            metadata = document.meta or {}

            # Get embedding if available
            embedding = document.embedding

            # Get content if available
            content = document.content

            # Get blob information if available
            blob_data = None
            blob_meta = None
            blob_mime_type = None

            if hasattr(document, "blob") and document.blob:
                blob: ByteStream = document.blob
                blob_data = blob.data
                blob_meta = blob.meta
                blob_mime_type = blob.mime_type

            # Return a dict with PostgreSQL compatible keys and values
            pg_document = {
                "id": doc_id,
                "embedding": embedding,
                "content": content,
                "content_bm25vector": None,  # This will be computed separately
                "blob_data": blob_data,
                "blob_meta": json.dumps(blob_meta) if blob_meta else None,
                "blob_mime_type": blob_mime_type,
                "meta": json.dumps(metadata) if metadata else None,
            }

            pg_documents.append(pg_document)

        return pg_documents

    @staticmethod
    def _from_pg_to_haystack_documents(
        documents: List[Dict[str, Any]],
    ) -> List[Document]:
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
            try:
                meta_str = doc.get("meta")
                if meta_str and isinstance(meta_str, str):
                    meta = json.loads(meta_str)
                elif isinstance(meta_str, dict):
                    meta = meta_str
                else:
                    meta = {}
            except (json.JSONDecodeError, TypeError):
                logger.warning(
                    f"Failed to parse meta for document {doc.get('id')}, using empty dict"
                )
                meta = {}

            # Add score to meta if available
            if "score" in doc and doc["score"] is not None:
                meta["score"] = doc["score"]

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

    def _embedding_retrieval(
        self,
        query_embedding: List[float],
        *,
        filters: Optional[Dict[str, Any]] = None,
        top_k: int = 10,
        vector_function: Optional[
            Literal["cosine_similarity", "inner_product", "l2_distance"]
        ] = None,
    ) -> List[Document]:
        """
        Retrieve documents using embedding similarity.

        :param query_embedding: Embedding of the query.
        :param filters: Optional filters to narrow down the search space.
        :param top_k: Maximum number of documents to return.
        :param vector_function: Vector similarity function to use.
        :returns: List of `Document`s that match the query.
        """
        # Enhanced validation for query_embedding
        if not isinstance(query_embedding, list):
            raise ValueError(
                f"query_embedding must be a list of floats, got {type(query_embedding)}: {query_embedding}"
            )

        if not query_embedding:
            raise ValueError("query_embedding cannot be empty")

        if not all(isinstance(val, (int, float)) for val in query_embedding):
            raise ValueError(f"query_embedding must contain only numeric values")

        # Use the provided vector_function or fall back to the default
        vector_function = vector_function or self.vector_function

        # Define score based on vector function
        if vector_function == "cosine_similarity":
            score_definition = SQL("1 - (embedding <=> %s::vector) AS score")
            direction = "DESC"  # Higher score (closer to 1) means more similar
        elif vector_function == "inner_product":
            score_definition = SQL("(embedding <#> %s::vector) * -1 AS score")
            direction = "DESC"  # Higher score means more similar
        else:  # l2_distance
            score_definition = SQL("(embedding <-> %s::vector) AS score")
            direction = "ASC"  # Lower distance means more similar

        # Convert query_embedding to proper PostgreSQL vector format
        try:
            query_embedding_str = f"[{','.join(str(val) for val in query_embedding)}]"
            if not (
                query_embedding_str.startswith("[")
                and query_embedding_str.endswith("]")
            ):
                raise ValueError(
                    f"Failed to create valid vector string, got: {query_embedding_str}"
                )
        except Exception as e:
            raise ValueError(f"Error formatting query_embedding as vector: {str(e)}")

        # Prepare parameters starting with the query embedding
        params = [query_embedding_str]

        # Build the WHERE clause
        where_clause = SQL("WHERE embedding IS NOT NULL")
        if filters:
            try:
                from .filters import _convert_filters_to_where_clause_and_params

                filter_where_clause, filter_params = (
                    _convert_filters_to_where_clause_and_params(filters)
                )
                where_clause = SQL(
                    "{filter_where_clause} AND embedding IS NOT NULL"
                ).format(filter_where_clause=filter_where_clause)
                # Add filter parameters
                if isinstance(filter_params, tuple):
                    params.extend(filter_params)
                else:
                    params.append(filter_params)
            except Exception as e:
                raise ValueError(f"Failed to apply filters: {str(e)}")

        # Add top_k parameter
        params.append(top_k)

        # Construct the unified query
        query = SQL("""
            SELECT 
                *,
                {score_definition}
            FROM {schema_name}.{table_name}
            {where_clause}
            ORDER BY score {direction}
            LIMIT %s
        """).format(
            schema_name=Identifier(self.schema_name),
            table_name=Identifier(self.table_name),
            score_definition=score_definition,
            where_clause=where_clause,
            direction=SQL(direction),
        )

        try:
            # Execute the query with parameters
            self.dict_cursor.execute(query, params)
            results = self.dict_cursor.fetchall()

            # Convert the results to Document objects
            documents = self._from_pg_to_haystack_documents(results)
            return documents
        except Exception as e:
            raise DocumentStoreError(
                f"Error during embedding retrieval: {str(e)}"
            ) from e
