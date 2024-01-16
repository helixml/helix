import os
from pgvector.sqlalchemy import Vector
from sqlalchemy import insert, String, Integer, create_engine, text
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

def checkDocumentChunkData(data_dict):
  required_keys = ["session_id", "interaction_id", "filename", "offset", "text"]
  number_keys = ["offset"]
  for key in required_keys:
    if key not in data_dict:
      raise Exception(f"Missing required key: {key}")
    if key not in number_keys:
      if len(data_dict[key]) == 0:
        raise Exception(f"Key {key} must not be empty")

####################
#
# HANDLERS
#
####################

# example data_dict:
# {
#     "session_id": "123",
#     "interaction_id": "456",
#     "filename": "test.txt",
#     "offset": 0,
#     "text": "hello world",
#     "embedding": [1, 2, 3, 4]
# }
def insertData(engine, data_dict):
  stmt = insert(HelixDocumentChunk).values(**data_dict).returning(HelixDocumentChunk.id)
  with engine.connect() as connection:
    cursor = connection.execute(stmt)
    connection.commit()
    inserted_id = cursor.fetchone()[0]
    return inserted_id

def getRow(engine, row_id):
  with engine.connect() as connection:
    stmt = HelixDocumentChunk.__table__.select().where(HelixDocumentChunk.id == row_id)
    result = connection.execute(stmt)
    row = result.fetchone()
    return {
      "id": row.id,
      "session_id": row.session_id,
      "interaction_id": row.interaction_id,
      "filename": row.filename,
      "offset": row.offset,
      "text": row.text,
      "embedding": row.embedding.tolist()  # Convert ndarray to list
    }

