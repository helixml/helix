from alembic import op

revision: str = '05'
down_revision = '04'
branch_labels = None
depends_on = None

def upgrade() -> None:
  query = """
alter table helix_document_chunk add column source varchar(255);
  """
  op.execute(query)


def downgrade() -> None:
  query = """
alter table helix_document_chunk drop column source;
  """
  op.execute(query)
