from alembic import op

revision: str = '02'
down_revision = '01'
branch_labels = None
depends_on = None

def upgrade() -> None:
  query = """
create table helix_document_chunk (
  id varchar(255) PRIMARY KEY,
  created timestamp default current_timestamp,
  session_id varchar(255) NOT NULL,
  interaction_id varchar(255) NOT NULL,
  document_id varchar(255) NOT NULL,
  document_group_id varchar(255) NOT NULL,
  filename varchar(255) NOT NULL,
  content_offset integer NOT NULL,
  content text,
  embedding vector(384)
);
  """
  op.execute(query)


def downgrade() -> None:
  query = """
DROP table helix_document_chunk;
  """
  op.execute(query)
