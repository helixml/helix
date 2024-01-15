import os
from pgvector.sqlalchemy import Vector
from sqlalchemy import String, Integer, create_engine, text
from sqlalchemy.orm import declarative_base, mapped_column

####################
#
# CONFIG
#
####################

TABLE_NAME = "helix_document_chunk"
VECTOR_DIMENSION = 384
postgres_host = os.getenv("POSTGRES_HOST", "pgvector")
postgres_database = os.getenv("POSTGRES_DATABASE", "postgres")
postgres_user = os.getenv("POSTGRES_USER", "postgres")
postgres_password = os.getenv("POSTGRES_PASSWORD", "postgres")
    
####################
#
# SCHEMA
#
####################

Base = declarative_base()

# the database migration class for our core database table
# the user of our api is expected to know the mapping of filename -> chunk_index
class HelixDocumentChunk(Base):
    __tablename__ = TABLE_NAME

    id = mapped_column(Integer, primary_key=True)
    session_id = mapped_column(String)
    interaction_id = mapped_column(String)
    filename = mapped_column(String)
    # the number of bytes into the root document that this chunk starts
    # this is used to re-constitute the document from its chunks
    # when it's matched to an embedding record
    offset = mapped_column(Integer)
    text = mapped_column(String)
    embedding = mapped_column(Vector(VECTOR_DIMENSION))

####################
#
# UTILS
#
####################
    
def getEngine():
    engine = create_engine(f"postgresql+psycopg2://{postgres_user}:{postgres_password}@{postgres_host}/{postgres_database}")

    with engine.connect() as conn:
        conn.execute(text("CREATE EXTENSION IF NOT EXISTS vector"))
        conn.commit()

    Base.metadata.create_all(engine)

    return engine