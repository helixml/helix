import os
from alembic.command import upgrade
from alembic.config import Config
from pgvector.sqlalchemy import Vector
from sqlalchemy import insert, String, Integer, create_engine, text, select
from sqlalchemy.orm import declarative_base, mapped_column, sessionmaker
import uuid
import pprint

####################
#
# CONFIG
#
####################

TABLE_NAME = "helix_document_chunk"
EMBEDDING_COLUMN_NAME = "embedding"
VECTOR_DIMENSION = 384
postgres_host = os.getenv("POSTGRES_HOST", "pgvector")
postgres_database = os.getenv("POSTGRES_DATABASE", "postgres")
postgres_user = os.getenv("POSTGRES_USER", "postgres")
postgres_password = os.getenv("POSTGRES_PASSWORD", "postgres")

####################
#
# BOOT
#
####################

def runSQLMigrations():
  this_dir = os.path.dirname(os.path.realpath(__file__))
  migrations_dir = os.path.join(this_dir, "migrations")
  config_file = os.path.join(this_dir, "alembic.ini")
  config = Config(file_=config_file)
  config.set_main_option("script_location", migrations_dir)
  config.set_main_option("sqlalchemy.url", f"postgresql://{postgres_user}:{postgres_password}@{postgres_host}/{postgres_database}")
  upgrade(config, "head")

runSQLMigrations()
engine = create_engine(f"postgresql+psycopg2://{postgres_user}:{postgres_password}@{postgres_host}/{postgres_database}", echo=True)
Session = sessionmaker(bind=engine)

####################
#
# SCHEMA
#
####################

Base = declarative_base()

# the database migration class for our core database table
# the user of our api is expected to know the mapping of filename -> chunk_index
# TODO: work out how the hell we are supposed to do database migrations
# when we are using the Vector field - I've tried to plugin alembic
# but my lord it's complicated so I bailed (because this is an experiment)
# however, we will probably need to change this table at some point
# and then this problem will really bite us in the assd
class HelixDocumentChunk(Base):
  __tablename__ = TABLE_NAME

  id = mapped_column(String, primary_key=True)
  session_id = mapped_column(String)
  interaction_id = mapped_column(String)
  document_id = mapped_column(String)
  document_group_id = mapped_column(String)
  filename = mapped_column(String)
  # the number of bytes into the root document that this chunk starts
  # this is used to re-constitute the document from its chunks
  # when it's matched to an embedding record
  content_offset = mapped_column(Integer)
  content = mapped_column(String)
  embedding = mapped_column(Vector(VECTOR_DIMENSION))

def checkDocumentChunkData(data_dict):
  required_keys = ["session_id", "interaction_id", "document_id", "document_group_id", "filename", "content_offset", "content"]
  number_keys = ["content_offset"]
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
#     "content_offset": 0,
#     "content": "hello world",
#     "embedding": [1, 2, 3, 4]
# }
# we expect the embedding to already have been calculated before we put it into the DB
def insertData(data_dict):
  data_dict["id"] = uuid.uuid4()
  stmt = insert(HelixDocumentChunk).values(**data_dict).returning(HelixDocumentChunk.id)
  with engine.connect() as connection:
    cursor = connection.execute(stmt)
    connection.commit()
    inserted_id = cursor.fetchone()[0]
    return inserted_id

# given a database row - turn it into something we can JSON serialize
def convertRow(row):
  return {
      "id": row.id,
      "session_id": row.session_id,
      "interaction_id": row.interaction_id,
      "document_id": row.document_id,
      "document_group_id": row.document_group_id,
      "filename": row.filename,
      "content_offset": row.content_offset,
      "content": row.content,
      "embedding": row.embedding.tolist()  # Convert ndarray to list
    }

def convertRows(rows):
  return [convertRow(row) for row in rows]  

# when we query for prompts - we return not all fields plus the distance field
def convertSimpleRow(row):
  return {
      "session_id": row.session_id,
      "interaction_id": row.interaction_id,
      "document_id": row.document_id,
      "document_group_id": row.document_group_id,
      "filename": row.filename,
      "content_offset": row.content_offset,
      "content": row.content,
      "distance": row.distance,
    }

def convertSimpleRows(rows):
  return [convertSimpleRow(row) for row in rows]  

# a direct "give me a single row because I know it's ID" handler
def getRow(row_id):
  with engine.connect() as connection:
    stmt = HelixDocumentChunk.__table__.select().where(HelixDocumentChunk.id == row_id)
    result = connection.execute(stmt)
    row = result.fetchone()
    return convertRow(row)

# given a already calculated prompt embedding and a session ID - find matching rows
def queryPrompt(session_id, interaction_id, query_embedding, distance_function, distance_threshold, max_results):
  distance_functions = {
    "l2": "<->",
    "inner_product": "<#>",
    "cosine": "<=>"
  }
  distance_function_string = distance_functions[distance_function]
  if distance_function_string is None:
    raise Exception(f"Unknown distance function: {distance_function}")
  
  embedding_number_str = "[" + ",".join(str(num) for num in query_embedding) + "]"
  embedding_str = f"embedding {distance_function_string} '{embedding_number_str}'"

  raw_sql = text(f"""
select
  id, session_id, interaction_id, document_id, document_group_id, filename, content_offset, content,
  {embedding_str} as distance
from 
  {TABLE_NAME}
where
  session_id = '{session_id}'
  and
  interaction_id = '{interaction_id}'
  and
  {embedding_str} < {distance_threshold}
order by
  {embedding_str}
limit {max_results}
  """)

  session = Session()
  result = session.execute(raw_sql)
  rows = result.fetchall()
  session.close()

  return convertSimpleRows(rows)
  
  