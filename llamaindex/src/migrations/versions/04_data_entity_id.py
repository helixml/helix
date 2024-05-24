from alembic import op

revision: str = '04'
down_revision = '03'
branch_labels = None
depends_on = None

def upgrade() -> None:
  query = """
alter table helix_document_chunk drop column session_id;
alter table helix_document_chunk drop column interaction_id;
alter table helix_document_chunk add column data_entity_id varchar(255);
  """
  op.execute(query)


def downgrade() -> None:
  query = """
alter table helix_document_chunk drop column data_entity_id;
alter table helix_document_chunk add column session_id varchar(255);
alter table helix_document_chunk add column interaction_id varchar(255);
  """
  op.execute(query)
